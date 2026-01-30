package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/progress"
	"github.com/rglonek/go-flags"
)

type FilesUploadCmd struct {
	ClusterName     TypeClusterName    `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes          `short:"l" long:"nodes" description:"Node number(s), comma-separated. Default=ALL" default:""`
	ParallelThreads int                `short:"t" long:"threads" description:"Run on this many nodes in parallel" default:"10"`
	Progress        bool               `short:"p" long:"progress" description:"Show upload progress with TUI display"`
	Files           FilesRestUploadCmd `positional-args:"true"`
	Help            HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

type FilesRestUploadCmd struct {
	Source      flags.Filename `webtype:"upload"`
	Destination string
}

func (c *FilesUploadCmd) Execute(args []string) error {
	if string(c.Files.Source) == "help" && string(c.Files.Destination) == "" {
		return PrintHelp(false, "Specify a source and destination at the end of the command. Ex: aerolab files upload -n bob ./newresolv.conf /etc/resolv.conf\n\n")
	} else if string(c.Files.Source) == "" || string(c.Files.Destination) == "" {
		return PrintHelp(false, "Specify a source and destination at the end of the command. Ex: aerolab files upload -n bob ./newresolv.conf /etc/resolv.conf\n\n")
	}
	cmd := []string{"files", "upload"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.Upload(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *FilesUploadCmd) Upload(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"files", "upload"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	instances := inventory.Instances.WithClusterName(c.ClusterName.String())
	if instances.Count() == 0 {
		return fmt.Errorf("cluster %s not found", c.ClusterName.String())
	}

	// Filter by state first to only work with running instances
	instances = instances.WithState(backends.LifeCycleStateRunning)
	if instances.Count() == 0 {
		return fmt.Errorf("no running instances found for cluster %s", c.ClusterName.String())
	}

	if c.Nodes.String() != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return err
		}
		// Check if nodes exist in the running instances
		new := instances.WithNodeNo(nodes...).Describe()
		if new.Count() != len(nodes) {
			// Find which nodes are missing
			foundNodes := []int{}
			for _, inst := range new {
				foundNodes = append(foundNodes, inst.NodeNo)
			}
			return fmt.Errorf("some nodes not found or not running: %s (requested: %v, found: %v)", c.Nodes.String(), nodes, foundNodes)
		}
		instances = new
	} else {
		instances = instances.Describe()
	}

	system.Logger.Info("Uploading files to %d instances", instances.Count())

	confs, err := instances.GetSftpConfig("root")
	if err != nil {
		return err
	}

	uploads := make([]uploadItem, len(confs))
	for i, conf := range confs {
		uploads[i] = uploadItem{
			conf:     conf,
			instance: instances.Describe()[i],
		}
	}

	if _, err := os.Stat(string(c.Files.Source)); err != nil {
		return fmt.Errorf("source %s does not exist", c.Files.Source)
	}

	// Use progress TUI if requested
	if c.Progress {
		return c.uploadWithProgress(system, uploads)
	}

	var hasErr error
	parallelize.ForEachLimit(uploads, c.ParallelThreads, func(upload uploadItem) {
		conf := upload.conf
		instance := upload.instance
		sftp, err := sshexec.NewSftp(conf)
		if err != nil {
			system.Logger.Error("Failed to create sftp client for %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
			hasErr = errors.New("some nodes failed to upload")
			return
		}
		defer sftp.Close()
		err = sftp.Upload(string(c.Files.Source), c.Files.Destination)
		if err != nil {
			system.Logger.Error("Failed to upload %s to %s:%d: %s", c.Files.Source, instance.ClusterName, instance.NodeNo, err)
			hasErr = errors.New("some nodes failed to upload")
			return
		}
	})
	return hasErr
}

type uploadItem struct {
	conf     *sshexec.ClientConf
	instance *backends.Instance
}

func (c *FilesUploadCmd) uploadWithProgress(system *System, uploads []uploadItem) error {
	// Calculate total size
	sourcePath := string(c.Files.Source)
	totalSize, err := calculateLocalSize(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to calculate source size: %v", err)
	}

	// Multiply by number of nodes for aggregate
	totalSize *= int64(len(uploads))

	// Create progress tracker
	tracker := progress.NewTracker()
	tracker.SetTotalBytes(totalSize)

	title := fmt.Sprintf("Uploading to cluster %s (%d nodes)", c.ClusterName.String(), len(uploads))

	// Run with progress TUI
	return progress.RunWithProgress(tracker, title, true, func() {
		var wg sync.WaitGroup
		sem := make(chan struct{}, c.ParallelThreads)

		for _, u := range uploads {
			wg.Add(1)
			sem <- struct{}{}
			go func(upload uploadItem) {
				defer wg.Done()
				defer func() { <-sem }()

				// Check for cancellation
				if tracker.IsCancelled() {
					return
				}

				sftp, err := sshexec.NewSftp(upload.conf)
				if err != nil {
					tracker.SetError(upload.instance.NodeNo, fmt.Errorf("node %d: failed to create sftp client: %v", upload.instance.NodeNo, err))
					return
				}
				defer sftp.Close()

				err = sftp.UploadWithProgress(sourcePath, c.Files.Destination, tracker, upload.instance.NodeNo)
				if err != nil && !tracker.IsCancelled() {
					tracker.SetError(upload.instance.NodeNo, fmt.Errorf("node %d: %v", upload.instance.NodeNo, err))
				}
			}(u)
		}

		wg.Wait()
	})
}

func calculateLocalSize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

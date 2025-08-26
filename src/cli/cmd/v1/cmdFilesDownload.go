package cmd

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/go-flags"
)

type FilesDownloadCmd struct {
	ClusterName     TypeClusterName      `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes            `short:"l" long:"nodes" description:"Node number(s), comma-separated. Default=ALL" default:""`
	ParallelThreads int                  `short:"t" long:"threads" description:"Run on this many nodes in parallel" default:"10"`
	Files           FilesRestDownloadCmd `positional-args:"true"`
	Help            HelpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

type FilesRestDownloadCmd struct {
	Source      string
	Destination flags.Filename `webtype:"download"`
}

func (c *FilesDownloadCmd) Execute(args []string) error {
	if string(c.Files.Source) == "help" && string(c.Files.Destination) == "" {
		return PrintHelp(false, "Specify a source and destination at the end of the command. Ex: aerolab files download -n bob /etc/resolv.conf ./bob\n\nIf more than one node is specified, files will be downloaded to {Destination}/{nodeNumber}/\n\n")
	} else if string(c.Files.Source) == "" || string(c.Files.Destination) == "" {
		return PrintHelp(false, "Specify a source and destination at the end of the command. Ex: aerolab files download -n bob /etc/resolv.conf ./bob\n\nIf more than one node is specified, files will be downloaded to {Destination}/{nodeNumber}/\n\n")
	}
	cmd := []string{"files", "download"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.Download(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *FilesDownloadCmd) Download(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"files", "download"}, c, args...)
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
	if c.Nodes.String() != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return err
		}
		new := instances.WithNodeNo(nodes...).Describe()
		if new.Count() != len(nodes) {
			return fmt.Errorf("some nodes not found: %s", c.Nodes.String())
		}
		instances = new
	}
	instances = instances.WithState(backends.LifeCycleStateRunning).Describe()
	if instances.Count() == 0 {
		return fmt.Errorf("no running instances found")
	}

	system.Logger.Info("Downloading files from %d instances", instances.Count())

	confs, err := instances.GetSftpConfig("root")
	if err != nil {
		return err
	}

	type download struct {
		conf     *sshexec.ClientConf
		instance *backends.Instance
	}
	downloads := make([]download, len(confs))
	for i, conf := range confs {
		downloads[i] = download{
			conf:     conf,
			instance: instances.Describe()[i],
		}
	}

	if _, err := os.Stat(string(c.Files.Destination)); err == nil {
		if IsInteractive() {
			opts, abort, err := choice.Choice("Destination already exists, do you want to remove it, continue, or abort?", choice.Items{
				choice.Item("Remove"),
				choice.Item("Continue"),
				choice.Item("Abort"),
			})
			if err != nil {
				return err
			}
			if opts == "Abort" || abort {
				return errors.New("destination already exists")
			}
			if opts == "Remove" {
				err = os.RemoveAll(string(c.Files.Destination))
				if err != nil {
					return err
				}
			}
		} else {
			return errors.New("destination already exists")
		}
	}

	// only make the Destination directory if there are multiple nodes, otherwise we just pass the path to the sftp client
	if len(downloads) > 1 {
		err = os.MkdirAll(string(c.Files.Destination), 0755)
		if err != nil {
			return err
		}
	}

	var hasErr error
	parallelize.ForEachLimit(downloads, c.ParallelThreads, func(download download) {
		conf := download.conf
		instance := download.instance
		sftp, err := sshexec.NewSftp(conf)
		if err != nil {
			system.Logger.Error("Failed to create sftp client for %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
			hasErr = errors.New("some nodes failed to download")
			return
		}
		defer sftp.Close()
		dest := string(c.Files.Destination)
		if len(downloads) > 1 {
			dest = path.Join(dest, strconv.Itoa(instance.NodeNo))
			err = os.MkdirAll(dest, 0755)
			if err != nil {
				system.Logger.Error("Failed to create local directory %s: %s", dest, err)
				hasErr = errors.New("some nodes failed to download")
				return
			}
		}
		err = sftp.Download(c.Files.Source, dest)
		if err != nil {
			system.Logger.Error("Failed to download %s from %s:%d: %s", c.Files.Source, instance.ClusterName, instance.NodeNo, err)
			hasErr = errors.New("some nodes failed to download")
			return
		}
	})
	return hasErr
}

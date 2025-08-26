package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/go-flags"
)

type FilesUploadCmd struct {
	ClusterName     TypeClusterName    `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes          `short:"l" long:"nodes" description:"Node number(s), comma-separated. Default=ALL" default:""`
	ParallelThreads int                `short:"t" long:"threads" description:"Run on this many nodes in parallel" default:"10"`
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

	system.Logger.Info("Uploading files to %d instances", instances.Count())

	confs, err := instances.GetSftpConfig("root")
	if err != nil {
		return err
	}

	type upload struct {
		conf     *sshexec.ClientConf
		instance *backends.Instance
	}
	uploads := make([]upload, len(confs))
	for i, conf := range confs {
		uploads[i] = upload{
			conf:     conf,
			instance: instances.Describe()[i],
		}
	}

	if _, err := os.Stat(string(c.Files.Source)); err != nil {
		return fmt.Errorf("source %s does not exist", c.Files.Source)
	}

	var hasErr error
	parallelize.ForEachLimit(uploads, c.ParallelThreads, func(upload upload) {
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

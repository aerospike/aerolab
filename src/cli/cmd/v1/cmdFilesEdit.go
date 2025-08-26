package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/go-flags"
)

type FilesEditCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Client group/Cluster name" default:"mydc"`
	Node        TypeNode        `short:"l" long:"node" description:"Node number" default:"1"`
	Editor      string          `short:"e" long:"editor" description:"Editor command; must be present on the node" default:"vi"`
	Path        FilesSingleCmd  `positional-args:"true"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

type FilesSingleCmd struct {
	Path flags.Filename
}

func (c *FilesEditCmd) Execute(args []string) error {
	if string(c.Path.Path) == "help" {
		return PrintHelp(false, "Specify a file at the end of the command. Ex: aerolab files edit -n bob -l 3 /etc/resolv.conf\n\n")
	}
	if c.Path.Path == "" {
		return PrintHelp(false, "Specify a file at the end of the command. Ex: aerolab files edit -n bob -l 3 /etc/resolv.conf\n\n")
	}
	cmd := []string{"files", "edit"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.Edit(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *FilesEditCmd) Edit(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"files", "edit"}, c, args...)
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
	instances = instances.WithNodeNo(int(c.Node))
	if instances.Count() == 0 {
		return fmt.Errorf("node %d not found", c.Node)
	}
	instances = instances.WithState(backends.LifeCycleStateRunning).Describe()
	if instances.Count() == 0 {
		return fmt.Errorf("instance is not running")
	}
	if instances.Count() > 1 {
		return fmt.Errorf("multiple instances found, specify cluster name and node number")
	}
	out := instances.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{c.Editor, string(c.Path.Path)},
			Stdin:          os.Stdin,
			Stdout:         os.Stdout,
			Stderr:         os.Stderr,
			SessionTimeout: 0,
			Terminal:       true,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})
	return out[0].Output.Err
}

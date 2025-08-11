package cmd

import (
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InstancesUpdateHostsFileCmd struct {
	On                 []string `short:"o" long:"on" description:"Update hosts file on these clusters only; default: all clusters"`
	With               []string `short:"w" long:"with" description:"Include only these cluster instances; default: all instances"`
	ParallelSSHThreads int      `short:"p" long:"parallel-ssh-threads" description:"Number of parallel SSH threads" default:"10"`
	Help               HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstancesUpdateHostsFileCmd) Execute(args []string) error {
	cmd := []string{"instances", "update-hosts-file"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.UpdateHostsFile(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InstancesUpdateHostsFileCmd) UpdateHostsFile(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "start"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	on := inventory.Instances.Describe()
	with := inventory.Instances.Describe()
	if len(c.On) > 0 {
		on = inventory.Instances.WithClusterName(c.On...).Describe()
	}
	if len(c.With) > 0 {
		with = inventory.Instances.WithClusterName(c.With...).Describe()
	}
	err := on.UpdateHostsFile(with, c.ParallelSSHThreads)
	if err != nil {
		return err
	}
	return nil
}

package cmd

import (
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InstancesUpdateHostsFileCmd struct {
	On                 []string `short:"o" long:"on" description:"Update hosts file on these clusters only; default: all clusters"`
	With               []string `short:"w" long:"with" description:"Include only instances in these clusters; default: all clusters"`
	IgnoreNotRunning   bool     `short:"i" long:"ignore-not-running" description:"Ignore instances that are not running"`
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
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "update-hosts-file"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	on := inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).Describe()
	with := inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).Describe()
	if len(c.On) > 0 {
		on = inventory.Instances.WithClusterName(c.On...).Describe()
	}
	if len(c.With) > 0 {
		with = inventory.Instances.WithClusterName(c.With...).Describe()
	}
	if on.WithState(backends.LifeCycleStateRunning).Count() != on.Count() && !c.IgnoreNotRunning {
		return fmt.Errorf("some instances are not running, use --ignore-not-running to update the hosts file anyway")
	}
	if with.WithState(backends.LifeCycleStateRunning).Count() != with.Count() && !c.IgnoreNotRunning {
		return fmt.Errorf("some instances are not running, use --ignore-not-running to update the hosts file anyway")
	}
	on = on.WithState(backends.LifeCycleStateRunning).Describe()
	with = with.WithState(backends.LifeCycleStateRunning).Describe()
	err := on.UpdateHostsFile(with, c.ParallelSSHThreads)
	if err != nil {
		return err
	}
	return nil
}

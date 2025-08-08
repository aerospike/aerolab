package cmd

import (
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InstancesStopCmd struct {
	DryRun  bool                `long:"dry-run" description:"Dry run, print what would be done but don't do it"`
	Force   bool                `short:"f" long:"force" description:"Force stop the instances"`
	NoWait  bool                `long:"no-wait" description:"Do not wait for the instances to stop"`
	Filters InstancesListFilter `group:"Filters" namespace:"filter"`
	Help    HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstancesStopCmd) Execute(args []string) error {
	cmd := []string{"instances", "stop"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	_, err = c.StopInstances(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InstancesStopCmd) StopInstances(system *System, inventory *backends.Inventory, args []string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "stop"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	instances, err := c.Filters.filter(inventory.Instances.WithState(backends.LifeCycleStateRunning).Describe(), true)
	if err != nil {
		return nil, err
	}

	if !c.DryRun {
		system.Logger.Info("Stopping %d instances", len(instances))
		for _, instance := range instances {
			system.Logger.Debug("Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
		}
		waitDur := 10 * time.Minute
		if c.NoWait {
			waitDur = 0
		}
		err := instances.Stop(c.Force, waitDur)
		if err != nil {
			return nil, err
		}
		return instances, nil
	}

	system.Logger.Info("DRY-RUN: Would stop %d instances", len(instances))
	for _, instance := range instances {
		system.Logger.Info("Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
	}
	return instances, nil
}

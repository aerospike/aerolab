package cmd

import (
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InstancesStartCmd struct {
	DryRun  bool                `long:"dry-run" description:"Dry run, print what would be done but don't do it"`
	NoWait  bool                `long:"no-wait" description:"Do not wait for the instances to start"`
	Filters InstancesListFilter `group:"Filters" namespace:"filter"`
	Help    HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstancesStartCmd) Execute(args []string) error {
	cmd := []string{"instances", "start"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	_, err = c.StartInstances(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InstancesStartCmd) StartInstances(system *System, inventory *backends.Inventory, args []string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "start"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	instances, err := c.Filters.filter(inventory.Instances.WithState(backends.LifeCycleStateStopped).Describe(), true)
	if err != nil {
		return nil, err
	}

	if !c.DryRun {
		system.Logger.Info("Starting %d instances", len(instances))
		for _, instance := range instances {
			system.Logger.Debug("Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
		}
		waitDur := 10 * time.Minute
		if c.NoWait {
			waitDur = 0
		}
		err := instances.Start(waitDur)
		if err != nil {
			return nil, err
		}
		return instances, nil
	}

	system.Logger.Info("DRY-RUN: Would start %d instances", len(instances))
	for _, instance := range instances {
		system.Logger.Info("Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
	}
	return instances, nil
}

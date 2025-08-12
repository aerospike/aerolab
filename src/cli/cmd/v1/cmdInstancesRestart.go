package cmd

import (
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InstancesRestartCmd struct {
	DryRun  bool                `long:"dry-run" description:"Dry run, print what would be done but don't do it"`
	Filters InstancesListFilter `group:"Filters" namespace:"filter"`
	Help    HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstancesRestartCmd) Execute(args []string) error {
	cmd := []string{"instances", "restart"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	_, _, err = c.RestartInstances(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InstancesRestartCmd) RestartInstances(system *System, inventory *backends.Inventory, args []string) (toStop backends.InstanceList, toStart backends.InstanceList, err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "restart"}, c, args...)
		if err != nil {
			return nil, nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	instances, err := c.Filters.filter(inventory.Instances.Describe(), true)
	if err != nil {
		return nil, nil, err
	}

	toStop = instances.WithState(backends.LifeCycleStateRunning).Describe()                                  // instances in running state will be stopped
	toStart = instances.WithState(backends.LifeCycleStateStopped, backends.LifeCycleStateRunning).Describe() // start all listed stopped and current running instances

	if len(toStop) == 0 && len(toStart) == 0 {
		system.Logger.Info("No instances to stop or start")
		return nil, nil, nil
	}

	if !c.DryRun {
		system.Logger.Info("Stopping %d instances", len(toStop))
		for _, instance := range toStop {
			system.Logger.Debug("STOP: Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
		}
		err := toStop.Stop(false, 10*time.Minute)
		if err != nil {
			return toStop, toStart, err
		}
		system.Logger.Info("Starting %d instances", len(toStart))
		for _, instance := range toStart {
			system.Logger.Debug("START: Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
		}
		err = toStart.Start(10 * time.Minute)
		if err != nil {
			return toStop, toStart, err
		}
		return toStop, toStart, nil
	}

	system.Logger.Info("DRY-RUN: Would stop %d instances", len(toStop))
	for _, instance := range toStop {
		system.Logger.Info("STOP: Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
	}

	system.Logger.Info("DRY-RUN: Would start %d instances", len(toStart))
	for _, instance := range toStart {
		system.Logger.Info("START: Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
	}
	return toStop, toStart, nil
}

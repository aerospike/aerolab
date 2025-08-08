package cmd

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
)

type InstancesDestroyCmd struct {
	DryRun  bool                `long:"dry-run" description:"Print what would be done, but do not actually destroy"`
	Force   bool                `long:"force" description:"Do not ask for confirmation"`
	NoWait  bool                `long:"no-wait" description:"Do not wait for the instances to terminate"`
	Filters InstancesListFilter `group:"Filters" namespace:"filter"`
	Help    HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstancesDestroyCmd) Execute(args []string) error {
	cmd := []string{"instances", "destroy"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	_, err = c.DestroyInstances(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InstancesDestroyCmd) DestroyInstances(system *System, inventory *backends.Inventory, args []string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "destroy"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	instances, err := c.Filters.filter(inventory.Instances.WithNotState(backends.LifeCycleStateTerminating, backends.LifeCycleStateTerminated).Describe(), true)
	if err != nil {
		return nil, err
	}

	if !c.DryRun {
		if !c.Force && IsInteractive() {
			choice, quitting, err := choice.Choice("Are you sure you want to destroy "+strconv.Itoa(len(instances))+" instances?", choice.Items{
				choice.Item("Yes"),
				choice.Item("No"),
			})
			if err != nil {
				return nil, err
			}
			if quitting {
				return nil, errors.New("aborted")
			}
			switch choice {
			case "No":
				return nil, errors.New("aborted")
			}
		}
		system.Logger.Info("Destroying %d instances", len(instances))
		for _, instance := range instances {
			system.Logger.Debug("Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
		}
		waitDur := 10 * time.Minute
		if c.NoWait {
			waitDur = 0
		}
		err := instances.Terminate(waitDur)
		if err != nil {
			return nil, err
		}
		return instances, nil
	}

	system.Logger.Info("DRY-RUN: Would destroy %d instances", len(instances))
	for _, instance := range instances {
		system.Logger.Info("Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
	}
	return instances, nil
}

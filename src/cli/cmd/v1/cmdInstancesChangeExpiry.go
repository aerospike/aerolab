package cmd

import (
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InstancesChangeExpiryCmd struct {
	DryRun   bool                `long:"dry-run" description:"Dry run, print what would be done but don't do it"`
	ExpireIn time.Duration       `long:"expire-in" description:"Expire in this duration from now" default:"30h"`
	Filters  InstancesListFilter `group:"Filters" namespace:"filter"`
	Help     HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstancesChangeExpiryCmd) Execute(args []string) error {
	cmd := []string{"instances", "change-expiry"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	_, err = c.ChangeExpiryInstances(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InstancesChangeExpiryCmd) ChangeExpiryInstances(system *System, inventory *backends.Inventory, args []string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "change-expiry"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	instances, err := c.Filters.filter(inventory.Instances.WithNotState(backends.LifeCycleStateTerminated, backends.LifeCycleStateTerminating).Describe(), true)
	if err != nil {
		return nil, err
	}

	newExpiry := time.Now().Add(c.ExpireIn)
	if !c.DryRun {
		system.Logger.Info("Changing expiry for %d instances to %s", len(instances), newExpiry.Format(time.RFC3339))
		for _, instance := range instances {
			system.Logger.Debug("Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
		}
		err := instances.ChangeExpiry(newExpiry)
		if err != nil {
			return nil, err
		}
		return instances, nil
	}

	system.Logger.Info("DRY-RUN: Would change expiry for %d instances to %s", len(instances), newExpiry.Format(time.RFC3339))
	for _, instance := range instances {
		system.Logger.Info("Name: %s, cluster: %s, node: %d", instance.Name, instance.ClusterName, instance.NodeNo)
	}
	return instances, nil
}

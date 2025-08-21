package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type VolumesDetachCmd struct {
	Filter   VolumesListFilter    `group:"Filters" namespace:"filter"`
	Instance VolumeInstanceFilter `group:"Instance" namespace:"instance"`
	Help     HelpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *VolumesDetachCmd) Execute(args []string) error {
	cmd := []string{"volumes", "detach"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.DetachVolumes(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *VolumesDetachCmd) DetachVolumes(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"volumes", "detach"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	instances := inventory.Instances.WithNotState(backends.LifeCycleStateTerminated)
	if c.Instance.InstanceName != "" {
		instances = instances.WithName(c.Instance.InstanceName)
	}
	if c.Instance.ClusterName != "" {
		instances = instances.WithClusterName(c.Instance.ClusterName)
	}
	if c.Instance.NodeNo != 0 {
		instances = instances.WithNodeNo(c.Instance.NodeNo)
	}
	if instances.Count() == 0 {
		return fmt.Errorf("instance not found")
	}
	if instances.Count() > 1 {
		return fmt.Errorf("multiple instances found, specify --instance-name, or --cluster-name and --node-no")
	}
	instance := instances.Describe()[0]
	if instance.InstanceState != backends.LifeCycleStateRunning {
		return fmt.Errorf("instance %s is in invalid state %s, must be running", instance.Name, instance.InstanceState.String())
	}

	volumes, err := c.Filter.filter(inventory.Volumes.Describe(), true)
	if err != nil {
		return err
	}
	if volumes.Count() == 0 {
		return fmt.Errorf("no volumes found")
	}
	return volumes.Detach(instance, 10*time.Minute)
}

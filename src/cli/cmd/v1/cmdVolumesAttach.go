package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type VolumesAttachCmd struct {
	Filter                           VolumesListFilter    `group:"Filters" namespace:"filter"`
	Instance                         VolumeInstanceFilter `group:"Instance" namespace:"instance"`
	SharedVolumeFIPS                 bool                 `long:"shared-fips" description:"Enable FIPS mode for the shared volume"`
	SharedVolumeMountTargetDirectory string               `long:"shared-target" description:"Mount target directory for the shared volume"`
	Help                             HelpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

type VolumeInstanceFilter struct {
	InstanceName string `long:"instance-name" description:"Instance name"`
	ClusterName  string `long:"cluster-name" description:"Cluster name"`
	NodeNo       int    `long:"node-no" description:"Node number"`
}

func (c *VolumesAttachCmd) Execute(args []string) error {
	cmd := []string{"volumes", "attach"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.AttachVolumes(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *VolumesAttachCmd) AttachVolumes(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"volumes", "attach"}, c, args...)
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
	for _, volume := range volumes {
		if volume.VolumeType == backends.VolumeTypeSharedDisk {
			if c.SharedVolumeMountTargetDirectory == "" {
				return fmt.Errorf("shared-target is required for shared volumes")
			}
			system.Logger.Info("Attaching a shared volume, this may take a while...")
			break
		}
	}
	return volumes.Attach(instance, &backends.VolumeAttachShared{
		MountTargetDirectory: c.SharedVolumeMountTargetDirectory,
		FIPS:                 c.SharedVolumeFIPS,
	}, 10*time.Minute)
}

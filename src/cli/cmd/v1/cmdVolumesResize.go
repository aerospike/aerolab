package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type VolumesResizeCmd struct {
	Filter    VolumesListFilter `group:"Filters" namespace:"filter"`
	NewSizeGB int               `long:"new-size-gb" description:"New size of the volume in GiB"`
	Help      HelpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *VolumesResizeCmd) Execute(args []string) error {
	cmd := []string{"volumes", "resize"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.ResizeVolumes(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *VolumesResizeCmd) ResizeVolumes(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"volumes", "resize"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	volumes, err := c.Filter.filter(inventory.Volumes.Describe(), true)
	if err != nil {
		return err
	}
	if volumes.Count() == 0 {
		return fmt.Errorf("no volumes found")
	}
	return volumes.Resize(backends.StorageSize(c.NewSizeGB), 10*time.Minute)
}

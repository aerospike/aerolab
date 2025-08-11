package cmd

import (
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InventoryRefreshDiskCacheCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryRefreshDiskCacheCmd) Execute(args []string) error {
	cmd := []string{"inventory", "refresh-disk-cache"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.RefreshDiskCache(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InventoryRefreshDiskCacheCmd) RefreshDiskCache(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"inventory", "refresh-disk-cache"}, c)
		if err != nil {
			return err
		}
	}
	if !system.Opts.Config.Backend.InventoryCache {
		system.Logger.Info("Disk cache is disabled, skipping refresh")
		return nil
	}
	system.Logger.Info("Refreshing disk cache")
	err := system.Backend.ForceRefreshInventory()
	if err != nil {
		return err
	}
	return nil
}

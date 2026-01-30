package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
)

type VolumesDeleteCmd struct {
	Filter VolumesListFilter `group:"Filters" namespace:"filter"`
	Force  bool              `long:"force" description:"Force delete, even if the volume is in use"`
	DryRun bool              `long:"dry-run" description:"Dry run, print what would be done but don't do it"`
	Help   HelpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *VolumesDeleteCmd) Execute(args []string) error {
	cmd := []string{"volumes", "delete"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))
	system.Logger.Info("Backend: %s, Project: %s", system.Opts.Config.Backend.Type, os.Getenv("AEROLAB_PROJECT"))

	defer UpdateDiskCache(system)()
	err = c.DeleteVolumes(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *VolumesDeleteCmd) DeleteVolumes(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"volumes", "delete"}, c, args...)
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
	if c.DryRun {
		system.Logger.Info("DRY-RUN: Would delete %d volumes", volumes.Count())
		for _, volume := range volumes {
			system.Logger.Info("Name: %s, size: %d GiB", volume.Name, volume.Size/backends.StorageGiB)
		}
		return nil
	}
	if !c.Force && IsInteractive() {
		opts, quitting, err := choice.Choice(fmt.Sprintf("Delete %d volumes?", volumes.Count()), choice.Items{
			choice.Item("Yes"),
			choice.Item("No"),
		})
		if err != nil {
			return err
		}
		if quitting {
			return errors.New("aborted")
		}
		if opts == "No" {
			return errors.New("aborted")
		}
	}
	system.Logger.Info("Deleting %d volumes", volumes.Count())
	return volumes.DeleteVolumes(inventory.Firewalls.Describe(), 10*time.Minute)
}

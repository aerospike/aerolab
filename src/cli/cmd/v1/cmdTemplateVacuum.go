package cmd

import (
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type TemplateVacuumCmd struct {
	DryRun bool    `short:"n" long:"dry-run" description:"Do not actually create the template, just run the basic checks"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *TemplateVacuumCmd) Execute(args []string) error {
	cmd := []string{"template", "vacuum"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.VacuumTemplate(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *TemplateVacuumCmd) VacuumTemplate(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"template", "vacuum"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// run images vacuum
	vac := &ImagesVacuumCmd{DryRun: c.DryRun}
	err := vac.VacuumImages(system, inventory, nil)
	if err != nil {
		return err
	}
	return nil
}

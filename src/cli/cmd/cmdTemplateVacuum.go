package cmd

import (
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type TemplateVacuumCmd struct {
	DryRun bool    `short:"d" long:"dry-run" description:"Do not actually delete the templates, just list them"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *TemplateVacuumCmd) Execute(args []string) error {
	cmd := []string{"template", "vacuum"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.VacuumTemplates(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *TemplateVacuumCmd) VacuumTemplates(system *System, inventory *backends.Inventory, args []string) error {
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
	dangling := inventory.Instances.WithTags(map[string]string{"aerolab.type": "template.create"}).WithNotState(backends.LifeCycleStateTerminated)
	if dangling.Count() == 0 {
		system.Logger.Info("No templates to vacuum found")
		return nil
	}
	system.Logger.Info("Found %d vacuumable templates, deleting...", dangling.Count())
	if c.DryRun {
		system.Logger.Info("Dry run, not deleting")
		for _, instance := range dangling.Describe() {
			system.Logger.Info("Name: %s, Zone: %s, State: %s, Tags: %v", instance.Name, instance.ZoneName, instance.InstanceState, instance.Tags)
		}
		return nil
	}
	err := dangling.Terminate(time.Minute * 10)
	if err != nil {
		return err
	}
	system.Logger.Info("Done")
	return nil
}

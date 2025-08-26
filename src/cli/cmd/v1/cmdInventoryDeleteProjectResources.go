package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/mattn/go-isatty"
)

type InventoryDeleteProjectResourcesCmd struct {
	Expiry bool    `long:"expiry" description:"Also remove the expiry system; WARN: expiry system is NOT project-bound but global"`
	Force  bool    `short:"f" long:"force" description:"Force deletion without confirmation"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryDeleteProjectResourcesCmd) Execute(args []string) error {
	cmd := []string{"inventory", "delete-project-resources"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))
	system.Logger.Info("Backend: %s, Project: %s", system.Opts.Config.Backend.Type, os.Getenv("AEROLAB_PROJECT"))
	defer UpdateDiskCache(system)
	err = c.DeleteProjectResources(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InventoryDeleteProjectResourcesCmd) DeleteProjectResources(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"inventory", "delete-project-resources"}, c, args...)
		if err != nil {
			return err
		}
	}
	system.Logger.Warn("WARNING: You are about to delete ALL resources associated with your project in the enabled regions")
	system.Logger.Warn("This action cannot be undone")
	if !c.Force && (isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) {
		projectName := os.Getenv("AEROLAB_PROJECT")
		if projectName == "" {
			projectName = "default"
		}

		var input string
		fmt.Printf("Enter project name '%s' to confirm: ", projectName)
		_, err := fmt.Scanln(&input)
		if err != nil {
			return err
		}

		if strings.TrimSuffix(input, "\n") != projectName {
			return errors.New("project name does not match")
		}
	}
	system.Logger.Info("Deleting resources...")
	err := system.Backend.DeleteProjectResources(backends.BackendType(system.Opts.Config.Backend.Type))
	if err != nil {
		return err
	}
	if c.Expiry {
		system.Logger.Info("Removing expiry system...")
		zones, err := system.Backend.ListEnabledRegions(backends.BackendType(system.Opts.Config.Backend.Type))
		if err != nil {
			return err
		}
		err = system.Backend.ExpiryRemove(backends.BackendType(system.Opts.Config.Backend.Type), zones...)
		if err != nil {
			return err
		}
	}
	return nil
}

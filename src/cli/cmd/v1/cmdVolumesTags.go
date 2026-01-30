package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type VolumesAddTagsCmd struct {
	Filter VolumesListFilter `group:"Filters" namespace:"filter"`
	Tags   []string          `short:"t" long:"tag" description:"Tags to add to the volumes, format: k=v"`
	Help   HelpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *VolumesAddTagsCmd) Execute(args []string) error {
	cmd := []string{"volumes", "add-tags"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.AddTags(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *VolumesAddTagsCmd) AddTags(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"volumes", "add-tags"}, c, args...)
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

	tags := make(map[string]string)
	for _, tag := range c.Tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid tag: %s, must be in the format k=v", tag)
		}
		tags[parts[0]] = parts[1]
	}

	system.Logger.Info("Adding tags %v to %d volumes", tags, volumes.Count())

	err = volumes.AddTags(tags, 10*time.Minute)
	if err != nil {
		return err
	}

	return nil
}

type VolumesRemoveTagsCmd struct {
	Filter VolumesListFilter `group:"Filters" namespace:"filter"`
	Tags   []string          `short:"t" long:"tag" description:"Tag name to remove from the volumes"`
	Help   HelpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *VolumesRemoveTagsCmd) Execute(args []string) error {
	cmd := []string{"volumes", "remove-tags"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.RemoveTags(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *VolumesRemoveTagsCmd) RemoveTags(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"volumes", "remove-tags"}, c, args...)
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

	tags := c.Tags

	system.Logger.Info("Removing tags %v from %d volumes", tags, volumes.Count())

	err = volumes.RemoveTags(tags, 10*time.Minute)
	if err != nil {
		return err
	}

	return nil
}

package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InstancesAddTagsCmd struct {
	Tags   []string            `short:"t" long:"tag" description:"Tags to add to the instances, format: k=v"`
	Filter InstancesListFilter `group:"Filters" namespace:"filter"`
	Help   HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstancesAddTagsCmd) Execute(args []string) error {
	cmd := []string{"instances", "add-tags"}
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

func (c *InstancesAddTagsCmd) AddTags(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "add-tags"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	instances, err := c.Filter.filter(inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).Describe(), true)
	if err != nil {
		return err
	}
	if instances.Count() == 0 {
		return errors.New("no instances found")
	}

	tags := make(map[string]string)
	for _, tag := range c.Tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid tag: %s, must be in the format k=v", tag)
		}
		tags[parts[0]] = parts[1]
	}

	system.Logger.Info("Adding tags %v to %d instances", tags, instances.Count())

	err = instances.AddTags(tags)
	if err != nil {
		return err
	}

	return nil
}

type InstancesRemoveTagsCmd struct {
	Tags   []string            `short:"t" long:"tag" description:"Tag names to remove from the instances"`
	Filter InstancesListFilter `group:"Filters" namespace:"filter"`
	Help   HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstancesRemoveTagsCmd) Execute(args []string) error {
	cmd := []string{"instances", "remove-tags"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
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

func (c *InstancesRemoveTagsCmd) RemoveTags(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "remove-tags"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	instances, err := c.Filter.filter(inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).Describe(), true)
	if err != nil {
		return err
	}
	if instances.Count() == 0 {
		return errors.New("no instances found")
	}

	system.Logger.Info("Removing tags %v from %d instances", c.Tags, instances.Count())

	err = instances.RemoveTags(c.Tags)
	if err != nil {
		return err
	}

	return nil
}

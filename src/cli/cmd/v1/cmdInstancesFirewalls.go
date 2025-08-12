package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InstancesAssignFirewallsCmd struct {
	Firewalls []string            `short:"f" long:"firewall" description:"Firewall names to assign to the instances"`
	Filter    InstancesListFilter `group:"Filters" namespace:"filter"`
}

func (c *InstancesAssignFirewallsCmd) Execute(args []string) error {
	cmd := []string{"instances", "assign-firewalls"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.AssignFirewalls(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

type InstancesRemoveFirewallsCmd struct {
	Firewalls []string            `short:"f" long:"firewall" description:"Firewall names to remove from the instances"`
	Filter    InstancesListFilter `group:"Filters" namespace:"filter"`
}

func (c *InstancesRemoveFirewallsCmd) Execute(args []string) error {
	cmd := []string{"instances", "remove-firewalls"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.RemoveFirewalls(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InstancesAssignFirewallsCmd) AssignFirewalls(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "assign-firewalls"}, c, args...)
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

	fw := inventory.Firewalls.WithName(c.Firewalls...)
	if fw.Count() != len(c.Firewalls) {
		return fmt.Errorf("firewall %v not found", c.Firewalls)
	}
	system.Logger.Info("Assigning firewalls %v to %d instances", c.Firewalls, instances.Count())

	err = instances.AssignFirewalls(fw.Describe())
	if err != nil {
		return err
	}

	return nil
}

func (c *InstancesRemoveFirewallsCmd) RemoveFirewalls(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "remove-firewalls"}, c, args...)
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

	fw := inventory.Firewalls.WithName(c.Firewalls...)
	if fw.Count() != len(c.Firewalls) {
		return fmt.Errorf("firewall %v not found", c.Firewalls)
	}
	system.Logger.Info("Removing firewalls %v from %d instances", c.Firewalls, instances.Count())

	err = instances.RemoveFirewalls(fw.Describe())
	if err != nil {
		return err
	}

	return nil
}

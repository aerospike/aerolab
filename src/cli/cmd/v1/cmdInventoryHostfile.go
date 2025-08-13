package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InventoryHostfileCmd struct {
	IPType string  `short:"i" long:"ip-type" description:"IP type to use (private, public, all)" default:"all"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryHostfileCmd) Execute(args []string) error {
	cmd := []string{"inventory", "hostfile"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.InventoryHostfile(system, cmd, args, system.Backend.GetInventory(), os.Stdout)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InventoryHostfileCmd) InventoryHostfile(system *System, cmd []string, args []string, inventory *backends.Inventory, out io.Writer) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	instances := inventory.Instances.WithState(backends.LifeCycleStateRunning).Describe()
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].ClusterName != instances[j].ClusterName {
			return instances[i].ClusterName < instances[j].ClusterName
		}
		return instances[i].NodeNo < instances[j].NodeNo
	})

	var hostfilePrivate []string
	var hostfilePublic []string
	for _, instance := range instances {
		if instance.IP.Private != "" {
			hostfilePrivate = append(hostfilePrivate, fmt.Sprintf("%s\t%s\t%s-%d", instance.IP.Private, instance.InstanceID, instance.ClusterName, instance.NodeNo))
		}
		if instance.IP.Public != "" {
			hostfilePublic = append(hostfilePublic, fmt.Sprintf("%s\tpub-%s\tpub-%s-%d", instance.IP.Public, instance.InstanceID, instance.ClusterName, instance.NodeNo))
		}
	}
	if c.IPType == "private" || c.IPType == "all" {
		for _, line := range hostfilePrivate {
			fmt.Fprintln(out, line)
		}
	}
	if c.IPType == "public" || c.IPType == "all" {
		for _, line := range hostfilePublic {
			fmt.Fprintln(out, line)
		}
	}
	return nil
}

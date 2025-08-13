package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type InventoryGendersCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryGendersCmd) Execute(args []string) error {
	cmd := []string{"inventory", "genders"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.InventoryGenders(system, cmd, args, system.Backend.GetInventory(), os.Stdout)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InventoryGendersCmd) InventoryGenders(system *System, cmd []string, args []string, inventory *backends.Inventory, out io.Writer) error {
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

	var genders []string
	project := os.Getenv("AEROLAB_PROJECT")
	if project == "" {
		project = "default"
	}
	for _, instance := range instances {
		group := instance.Tags["aerolab.type"]
		if group == "" {
			group = "none"
		}
		genders = append(genders, fmt.Sprintf("%s-%d\t%s,group=%s,project=%s,all,pdsh_rcmd_type=ssh", instance.ClusterName, instance.NodeNo, instance.ClusterName, group, project))
	}
	for _, line := range genders {
		fmt.Fprintln(out, line)
	}
	return nil
}

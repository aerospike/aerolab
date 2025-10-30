package cmd

import (
	"io"
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/pager"
)

type ClusterListCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Owner      string   `short:"O" long:"owner" description:"Filter by owner of the cluster"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterListCmd) Execute(args []string) error {
	cmd := []string{"cluster", "list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ListClusters(system, system.Backend.GetInventory(), args, os.Stdout, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClusterListCmd) ListClusters(system *System, inventory *backends.Inventory, args []string, out io.Writer, page *pager.Pager) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "list"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	inventory.Instances = inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).WithTags(map[string]string{"aerolab.type": "aerospike"}).Describe()
	lst := &InstancesListCmd{
		Output:     c.Output,
		TableTheme: c.TableTheme,
		SortBy:     c.SortBy,
		Pager:      c.Pager,
		Filters: InstancesListFilter{
			Owner: c.Owner,
		},
	}
	return lst.ListInstances(system, inventory, args, out, page)
}

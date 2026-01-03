package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/pager"
)

type InventoryListCmd struct {
	Output             string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme         string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy             []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Owner              string   `short:"u" long:"owner" description:"Filter by owner"`
	WithExpiries       bool     `short:"e" long:"with-expiries" description:"Include expiries"`
	WithAerospikeCloud bool     `short:"c" long:"with-aerospike-cloud" description:"Include Aerospike Cloud clusters (slower operation)"`
	Pager              bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help               HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryListCmd) Execute(args []string) error {
	cmd := []string{"inventory", "list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.InventoryList(system, cmd, args, system.Backend.GetInventory(), os.Stdout)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InventoryListCmd) InventoryList(system *System, cmd []string, args []string, inventory *backends.Inventory, out io.Writer) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}

	var page *pager.Pager
	if c.Pager {
		var err error
		page, err = pager.New(out)
		if err != nil {
			return err
		}
		err = page.Start()
		if err != nil {
			return err
		}
		defer page.Close()
		out = page
	}

	switch c.Output {
	case "jq":
		inv := c.getInventory(system, c.WithExpiries, c.WithAerospikeCloud)
		params := []string{}
		if page != nil && page.HasColors() {
			params = append(params, "-C")
		}
		cmd := exec.Command("jq", params...)
		cmd.Stdout = out
		cmd.Stderr = out
		w, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		defer w.Close()
		enc := json.NewEncoder(w)
		go func() {
			enc.Encode(inv)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		inv := c.getInventory(system, c.WithExpiries, c.WithAerospikeCloud)
		json.NewEncoder(out).Encode(inv)
	case "json-indent":
		inv := c.getInventory(system, c.WithExpiries, c.WithAerospikeCloud)
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(inv)
	default:
		fmt.Fprintln(out, "")
		if c.WithExpiries {
			expiry := &ExpiryListCmd{
				Output:     c.Output,
				SortBy:     c.SortBy,
				TableTheme: c.TableTheme,
			}
			err := expiry.ExpiryList(system, cmd, args, system.Backend.GetInventory(), out, page)
			if err != nil {
				return err
			}
		}
		err := ListSubnets(system, c.Output, c.TableTheme, c.SortBy, system.Opts.Config.Backend.Type, cmd, c, args, system.Backend.GetInventory(), out, false, page)
		if err != nil {
			return err
		}
		err = ListSecurityGroups(system, c.Output, c.TableTheme, c.SortBy, system.Opts.Config.Backend.Type, cmd, c, args, system.Backend.GetInventory(), c.Owner, out, false, page)
		if err != nil {
			return err
		}
		images := &ImagesListCmd{
			Output:     c.Output,
			TableTheme: c.TableTheme,
			SortBy:     c.SortBy,
			Pager:      false,
			Filters: ImagesListFilter{
				Owner: c.Owner,
				Type:  "custom",
			},
		}
		err = images.ListImages(system, system.Backend.GetInventory(), args, out, page)
		if err != nil {
			return err
		}
		volumes := &VolumesListCmd{
			Output:     c.Output,
			SortBy:     c.SortBy,
			TableTheme: c.TableTheme,
			Pager:      false,
			Filters: VolumesListFilter{
				Owner: c.Owner,
			},
		}
		err = volumes.ListVolumes(system, system.Backend.GetInventory(), args, out, page)
		if err != nil {
			return err
		}
		instances := &InstancesListCmd{
			Output:     c.Output,
			SortBy:     c.SortBy,
			TableTheme: c.TableTheme,
			Pager:      false,
			Filters: InstancesListFilter{
				Owner: c.Owner,
			},
		}
		err = instances.ListInstances(system, system.Backend.GetInventory(), args, out, page)
		if err != nil {
			return err
		}
		if c.WithAerospikeCloud {
			cloudCmd := &CloudClustersListCmd{
				Output:     c.Output,
				TableTheme: c.TableTheme,
				SortBy:     c.SortBy,
				StatusNe:   "decommissioned",
			}
			err = cloudCmd.ListClusters(out, page)
			if err != nil {
				system.Logger.Warn("Error listing Aerospike Cloud clusters: %s", err)
			}
		}
	}
	return nil
}

func (c *InventoryListCmd) getInventory(system *System, withExpiries bool, withAerospikeCloud bool) map[string]interface{} {
	inventory := system.Backend.GetInventory()

	// Filter instances - exclude terminated instances (same as table output)
	instances := inventory.Instances.WithNotState(backends.LifeCycleStateTerminated)
	if c.Owner != "" {
		instances = instances.WithOwner(c.Owner)
	}

	// Filter images - only custom images (same as table output)
	images := inventory.Images.WithInAccount(true)
	if c.Owner != "" {
		images = images.WithOwner(c.Owner)
	}

	// Filter volumes by owner if specified (same as table output)
	volumes := inventory.Volumes
	if c.Owner != "" {
		volumes = volumes.WithOwner(c.Owner)
	}

	inv := map[string]interface{}{
		"networks":  inventory.Networks.Describe(),
		"firewalls": inventory.Firewalls.Describe(),
		"volumes":   volumes.Describe(),
		"instances": instances.Describe(),
		"images":    images.Describe(),
	}
	if withExpiries {
		expiries, err := system.Backend.ExpiryList()
		if err != nil {
			system.Logger.Error("Error getting expiry systems: %s", err)
		}
		inv["expiries"] = expiries
	}
	if withAerospikeCloud {
		cloudCmd := &CloudClustersListCmd{
			StatusNe: "decommissioned",
		}
		clusters, _, err := cloudCmd.GetClusters()
		if err != nil {
			system.Logger.Error("Error getting Aerospike Cloud clusters: %s", err)
		} else {
			inv["aerospikeCloud"] = map[string]interface{}{
				"clusters": clusters,
			}
		}
	}
	return inv
}

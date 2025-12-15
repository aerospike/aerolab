package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

type InstancesListCmd struct {
	Output     string              `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string              `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string            `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool                `short:"p" long:"pager" description:"Use a pager to display the output"`
	Filters    InstancesListFilter `group:"Filters" namespace:"filter"`
	Help       HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

type InstancesListFilter struct {
	Backend      string   `short:"b" long:"backend" description:"Filter by backend type"`
	ClusterName  string   `short:"n" long:"cluster-name" description:"Filter by cluster name"`
	NodeNo       string   `short:"l" long:"node-no" description:"Filter by node number(s), ex: 1,2,3,10-15"`
	Name         string   `short:"N" long:"name" description:"Filter by name of the instance"`
	Owner        string   `short:"O" long:"owner" description:"Filter by owner of the instance"`
	Type         string   `short:"T" long:"type" description:"Filter by type of instance (aerolab.type tag)"`
	Version      string   `short:"v" long:"version" description:"Filter by software version on the instance (aerolab.soft.version tag)"`
	Architecture string   `short:"a" long:"architecture" description:"Filter by architecture of the instance"`
	OSName       string   `short:"d" long:"os-name" description:"Filter by OS name of the instance (OS name)"`
	OSVersion    string   `short:"i" long:"os-version" description:"Filter by OS version of the instance (OS version)"`
	Zone         string   `short:"z" long:"zone" description:"Filter by zone of the instance (zone name)"`
	Tags         []string `long:"tag" description:"Filter by tag of the instance, as k=v"`
}

func (c *InstancesListCmd) Execute(args []string) error {
	cmd := []string{"instances", "list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ListInstances(system, system.Backend.GetInventory(), args, os.Stdout, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (f *InstancesListFilter) filter(instances backends.InstanceList, errorOnNotFound bool) (backends.InstanceList, error) {
	if f.Backend != "" {
		instances = instances.WithBackendType(backends.BackendType(f.Backend)).Describe()
		if errorOnNotFound && instances.Count() == 0 {
			return nil, fmt.Errorf("no instances found for backend type: %s", f.Backend)
		}
	}
	if f.Owner != "" {
		instances = instances.WithOwner(f.Owner).Describe()
		if errorOnNotFound && instances.Count() == 0 {
			return nil, fmt.Errorf("no instances found for owner: %s", f.Owner)
		}
	}
	tags := make(map[string]string)
	for _, tag := range f.Tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag: %s, must be in the format k=v", tag)
		}
		tags[parts[0]] = parts[1]
	}
	if f.Type != "" {
		tags["aerolab.type"] = f.Type
	}
	if f.Version != "" {
		tags["aerolab.soft.version"] = f.Version
	}
	if len(tags) > 0 {
		instances = instances.WithTags(tags).Describe()
		if errorOnNotFound && instances.Count() == 0 {
			return nil, fmt.Errorf("no instances found for tags: %s", tags)
		}
	}
	if f.Architecture != "" {
		var arch backends.Architecture
		err := arch.FromString(f.Architecture)
		if err != nil {
			return nil, err
		}
		instances = instances.WithArchitecture(arch).Describe()
		if errorOnNotFound && instances.Count() == 0 {
			return nil, fmt.Errorf("no instances found for architecture: %s", f.Architecture)
		}
	}
	if f.OSName != "" {
		instances = instances.WithOSName(f.OSName).Describe()
		if errorOnNotFound && instances.Count() == 0 {
			return nil, fmt.Errorf("no instances found for OS name: %s", f.OSName)
		}
	}
	if f.OSVersion != "" {
		instances = instances.WithOSVersion(f.OSVersion).Describe()
		if errorOnNotFound && instances.Count() == 0 {
			return nil, fmt.Errorf("no instances found for OS version: %s", f.OSVersion)
		}
	}
	if f.Zone != "" {
		instances = instances.WithZoneName(f.Zone).Describe()
		if errorOnNotFound && instances.Count() == 0 {
			return nil, fmt.Errorf("no instances found for zone: %s", f.Zone)
		}
	}
	if f.ClusterName != "" {
		instances = instances.WithClusterName(f.ClusterName).Describe()
		if errorOnNotFound && instances.Count() == 0 {
			return nil, fmt.Errorf("no instances found for cluster name: %s", f.ClusterName)
		}
	}
	if len(f.NodeNo) > 0 {
		nn, err := expandNodeNumbers(f.NodeNo)
		if err != nil {
			return nil, err
		}
		instances = instances.WithNodeNo(nn...).Describe()
		if errorOnNotFound && instances.Count() == 0 {
			return nil, fmt.Errorf("no instances found for node number: %s", f.NodeNo)
		}
		// evaluate if all given node numbers exist in all selected clusters
		for _, n := range nn {
			if instances.WithNodeNo(n).Count() == 0 {
				return nil, fmt.Errorf("no instances found for node number: %d", n)
			}
		}
	}
	if f.Name != "" {
		instances = instances.WithName(f.Name).Describe()
		if errorOnNotFound && instances.Count() == 0 {
			return nil, fmt.Errorf("no instances found for name: %s", f.Name)
		}
	}
	return instances, nil
}

func (c *InstancesListCmd) ListInstances(system *System, inventory *backends.Inventory, args []string, out io.Writer, page *pager.Pager) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "list"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	instances, err := c.Filters.filter(inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).Describe(), false)
	if err != nil {
		return err
	}

	if c.Pager && page == nil {
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
			enc.Encode(instances)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(instances)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(instances)
	case "text":
		system.Logger.Info("Instances:")
		for _, instance := range instances {
			expiresIn := "never"
			if !instance.Expires.IsZero() && instance.Expires.After(time.Now()) {
				expiresIn = time.Until(instance.Expires).String()
			}
			if !instance.Expires.IsZero() && instance.Expires.Before(time.Now()) {
				expiresIn = "expired"
			}
			fmt.Fprintf(out, "Backend: %s, Zone: %s, Owner: %s, Cluster: %s, Node: %d, Name: %s, PublicIP: %s, PrivateIP: %s, AccessURL: %s, State: %s, Cost: %0.2f, Expires: %s, Firewalls: %s, Instance: %s, Type: %s, Version: %s, OS: %s, Arch: %s, ClusterUUID: %s, CreationTime: %s, Spot: %t, NetworkID: %s, SubnetID: %s, Tags: %s, Description: %s\n",
				instance.BackendType,
				instance.ZoneName,
				instance.Owner,
				instance.ClusterName,
				instance.NodeNo,
				instance.Name,
				instance.IP.Public,
				instance.IP.Private,
				instance.AccessURL,
				instance.InstanceState.String(),
				instance.EstimatedCostUSD.AccruedCost(),
				expiresIn,
				instance.Firewalls,
				instance.InstanceType,
				instance.Tags["aerolab.type"],
				instance.Tags["aerolab.soft.version"],
				instance.OperatingSystem.Name+":"+instance.OperatingSystem.Version,
				instance.Architecture.String(),
				instance.ClusterUUID,
				instance.CreationTime.Format(time.RFC3339),
				instance.SpotInstance,
				instance.NetworkID,
				instance.SubnetID,
				instance.Tags,
				instance.Description,
			)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Backend:asc", "Zone:asc", "Owner:asc", "Cluster:asc", "Node:ascnum", "Name:asc"}
		}
		header := table.Row{"Backend", "Zone", "Owner", "Cluster", "Node", "Name", "PublicIP", "PrivateIP", "AccessURL", "State", "Cost", "Expires", "Firewalls", "Instance", "Type", "Version", "OS", "Arch", "ClusterUUID", "CreationTime", "Spot", "NetworkID", "SubnetID", "Tags", "Description"}
		if system.Opts.Config.Backend.Type == "docker" {
			header = table.Row{"Backend", "Zone", "Owner", "Cluster", "Node", "Name", "PrivateIP", "AccessURL", "State", "Firewalls", "Type", "Version", "OS", "Arch", "ClusterUUID", "CreationTime", "NetworkID", "SubnetID", "Tags", "Description"}
		}
		rows := []table.Row{}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		for _, instance := range instances {
			expiresIn := t.ColorErr.Sprint("NEVER")
			if !instance.Expires.IsZero() && instance.Expires.After(time.Now()) {
				if instance.Expires.Before(time.Now().Add(time.Hour * 6)) {
					expiresIn = t.ColorWarn.Sprint(time.Until(instance.Expires).String())
				} else {
					expiresIn = time.Until(instance.Expires).String()
				}
			} else if !instance.Expires.IsZero() && instance.Expires.Before(time.Now()) {
				expiresIn = t.ColorErr.Sprint(time.Until(instance.Expires).String())
			}
			if system.Opts.Config.Backend.Type == "docker" {
				rows = append(rows, table.Row{
					instance.BackendType,
					instance.ZoneName,
					instance.Owner,
					instance.ClusterName,
					instance.NodeNo,
					instance.Name,
					instance.IP.Private,
					instance.AccessURL,
					instance.InstanceState.String(),
					strings.Join(instance.Firewalls, "\n"),
					instance.Tags["aerolab.type"],
					instance.Tags["aerolab.soft.version"],
					instance.OperatingSystem.Name + ":" + instance.OperatingSystem.Version,
					instance.Architecture.String(),
					instance.ClusterUUID,
					instance.CreationTime.Format(time.RFC3339),
					instance.NetworkID,
					instance.SubnetID,
					instance.Tags,
					instance.Description,
				})
			} else {
				rows = append(rows, table.Row{
					instance.BackendType,
					instance.ZoneName,
					instance.Owner,
					instance.ClusterName,
					instance.NodeNo,
					instance.Name,
					instance.IP.Public,
					instance.IP.Private,
					instance.AccessURL,
					instance.InstanceState.String(),
					fmt.Sprintf("%0.2f", instance.EstimatedCostUSD.AccruedCost()),
					expiresIn,
					strings.Join(instance.Firewalls, "\n"),
					instance.InstanceType,
					instance.Tags["aerolab.type"],
					instance.Tags["aerolab.soft.version"],
					instance.OperatingSystem.Name + ":" + instance.OperatingSystem.Version,
					instance.Architecture.String(),
					instance.ClusterUUID,
					instance.CreationTime.Format(time.RFC3339),
					instance.SpotInstance,
					instance.NetworkID,
					instance.SubnetID,
					instance.Tags,
					instance.Description,
				})
			}
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("INSTANCES"), header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

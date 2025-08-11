package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

type InventoryInstanceTypesCmd struct {
	Output       string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme   string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy       []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum\n; Fields: Region, Name, Arch, CPUs, MemoryGiB, NVMEs, NvmeTotalSizeGiB, Price, SpotPrice"`
	Nodes        int      `short:"N" long:"nodes" description:"Number of nodes (essentially a price multiplier for the result)" default:"1"`
	Zone         []string `short:"z" long:"zone" description:"Filter by region/zone (can be specified multiple times)"`
	Arch         string   `short:"a" long:"arch" description:"Filter by architecture (amd64, arm64)"`
	FilterName   string   `short:"n" long:"name" description:"Filter by full or partial name"`
	FilterMinCPU int      `short:"c" long:"min-cpus" description:"Search for at least X CPUs"`
	FilterMaxCPU int      `short:"C" long:"max-cpus" description:"Search for max X CPUs"`
	FilterMinRAM float64  `short:"r" long:"min-ram" description:"Search for at least X RAM GB"`
	FilterMaxRAM float64  `short:"R" long:"max-ram" description:"Search for max X RAM GB"`
	EphemeralMin int      `short:"e" long:"min-ephemeral" description:"Search only for instances with at least X ephemeral devices"`
	EphemeralMax int      `short:"E" long:"max-ephemeral" description:"Search only for instances with max X ephemeral devices"`
	Pager        bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help         HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryInstanceTypesCmd) Execute(args []string) error {
	cmd := []string{"inventory", "instance-types"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: &backends.Inventory{
		Instances: backends.InstanceList{},
		Networks:  backends.NetworkList{},
		Firewalls: backends.FirewallList{},
		Volumes:   backends.VolumeList{},
		Images:    backends.ImageList{},
	}}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.InventoryInstanceTypes(system, cmd, args, system.Backend.GetInventory(), os.Stdout, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InventoryInstanceTypesCmd) InventoryInstanceTypes(system *System, cmd []string, args []string, inventory *backends.Inventory, out io.Writer, page *pager.Pager) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}

	xtypes, err := system.Backend.GetInstanceTypes(backends.BackendType(system.Opts.Config.Backend.Type))
	if err != nil {
		return err
	}

	var types backends.InstanceTypeList
	for _, t := range xtypes {
		if c.FilterName != "" && !strings.Contains(t.Name, c.FilterName) {
			continue
		}
		if c.FilterMinCPU > 0 && t.CPUs < c.FilterMinCPU {
			continue
		}
		if c.FilterMaxCPU > 0 && t.CPUs > c.FilterMaxCPU {
			continue
		}
		if c.FilterMinRAM > 0 && t.MemoryGiB < c.FilterMinRAM {
			continue
		}
		if c.FilterMaxRAM > 0 && t.MemoryGiB > c.FilterMaxRAM {
			continue
		}
		if c.EphemeralMin > 0 && t.NvmeCount < c.EphemeralMin {
			continue
		}
		if c.EphemeralMax > 0 && t.NvmeCount > c.EphemeralMax {
			continue
		}
		if len(c.Zone) > 0 && !slices.Contains(c.Zone, t.Region) {
			continue
		}
		if c.Arch != "" && !slices.Contains(t.Arch.String(), c.Arch) {
			continue
		}
		t.PricePerHour.OnDemand = t.PricePerHour.OnDemand * float64(c.Nodes)
		t.PricePerHour.Spot = t.PricePerHour.Spot * float64(c.Nodes)
		types = append(types, t)
	}

	if c.Pager && page == nil {
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
			enc.Encode(types)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(types)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(types)
	case "text":
		system.Logger.Info("Instance Types:")
		for _, t := range types {
			onDemandDay := t.PricePerHour.OnDemand * 24
			spotDay := t.PricePerHour.Spot * 24
			onDemandMonth := onDemandDay * 31
			spotMonth := spotDay * 31
			fmt.Fprintf(out, "Region: %s, Name: %s, Arch: %s, CPUs: %d, MemoryGiB: %0.0f, NVMEs: %d, NvmeTotalSizeGiB: %d, GPUs: %d, OnDemand $/h: %0.4f, OnDemand $/day: %0.2f, OnDemand $/month: %0.2f, Spot $/h: %0.4f, Spot $/day: %0.2f, Spot $/month: %0.2f\n",
				t.Region, t.Name, strings.Join(t.Arch.String(), ","), t.CPUs, t.MemoryGiB, t.NvmeCount, t.NvmeTotalSizeGiB, t.GPUs, t.PricePerHour.OnDemand, onDemandDay, onDemandMonth, t.PricePerHour.Spot, spotDay, spotMonth)
		}
		fmt.Fprintln(out, "")
	default:
		header := table.Row{"Region", "Name", "Arch", "vCPUs", "MemoryGiB", "NVMEs", "NvmeTotalSizeGiB", "GPUs", "OnDemand $/h", "OnDemand $/day", "OnDemand $/month", "Spot $/h", "Spot $/day", "Spot $/month"}
		rows := []table.Row{}
		for _, t := range types {
			onDemandDay := t.PricePerHour.OnDemand * 24
			spotDay := t.PricePerHour.Spot * 24
			onDemandMonth := onDemandDay * 31
			spotMonth := spotDay * 31
			rows = append(rows, table.Row{t.Region, t.Name, strings.Join(t.Arch.String(), ","), t.CPUs, int(t.MemoryGiB), t.NvmeCount, t.NvmeTotalSizeGiB, t.GPUs, fmt.Sprintf("%0.4f", t.PricePerHour.OnDemand), fmt.Sprintf("%0.2f", onDemandDay), fmt.Sprintf("%0.2f", onDemandMonth), fmt.Sprintf("%0.4f", t.PricePerHour.Spot), fmt.Sprintf("%0.2f", spotDay), fmt.Sprintf("%0.2f", spotMonth)})
		}
		if c.SortBy == nil {
			c.SortBy = []string{"Region:asc", "Name:asc"}
		}
		for i := range c.SortBy {
			if strings.ToLower(c.SortBy[i]) == "price:ascnum" || strings.ToLower(c.SortBy[i]) == "price:asc" {
				c.SortBy[i] = "OnDemand $/h:ascnum"
			}
			if strings.ToLower(c.SortBy[i]) == "spotprice:ascnum" || strings.ToLower(c.SortBy[i]) == "spotprice:asc" {
				c.SortBy[i] = "Spot $/h:ascnum"
			}
			if strings.ToLower(c.SortBy[i]) == "price:dscnum" || strings.ToLower(c.SortBy[i]) == "price:dsc" {
				c.SortBy[i] = "OnDemand $/h:dscnum"
			}
			if strings.ToLower(c.SortBy[i]) == "spotprice:dscnum" || strings.ToLower(c.SortBy[i]) == "spotprice:dsc" {
				c.SortBy[i] = "Spot $/h:dscnum"
			}
		}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("INSTANCE TYPES"), header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

type CloudListInstanceTypesCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Region     string   `long:"region" description:"Filter by region (only for non-json, non-jq output)"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

// CloudProvidersResponse represents the API response structure
type CloudProvidersResponse struct {
	Count   int             `json:"count"`
	Results []CloudProvider `json:"results"`
}

// CloudProvider represents a cloud provider
type CloudProvider struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Regions []Region `json:"regions"`
}

// Region represents a region with instance types
type Region struct {
	Name          string         `json:"name"`
	InstanceTypes []InstanceType `json:"instanceTypes"`
}

// InstanceType represents a single instance type
type InstanceType struct {
	InstanceType           string       `json:"instanceType"`
	LocalStorage           LocalStorage `json:"localStorage"`
	MemoryGib              int          `json:"memoryGib"`
	SupportedArchitectures []string     `json:"supportedArchitectures"`
	Vcpus                  int          `json:"vcpus"`
	RegionName             string       `json:"-"` // Added during flattening
}

// LocalStorage represents local storage information
type LocalStorage struct {
	DiskSizeGib   int `json:"diskSizeGib"`
	NumberOfDisks int `json:"numberOfDisks"`
	TotalSizeGib  int `json:"totalSizeGib"`
}

// FlattenedInstanceType represents a flattened instance type entry for display
type FlattenedInstanceType struct {
	Region       string
	Type         string
	Arch         string
	Vcpus        int
	RAMGib       int
	Disks        int
	DiskSizeGib  int
	TotalSizeGib int
}

func (c *CloudListInstanceTypesCmd) Execute(args []string) error {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	var result CloudProvidersResponse
	path := "/database/cloud-providers"

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	// Flatten the nested structure
	flattened := c.flattenInstanceTypes(result.Results)

	// Filter by region if specified (only for non-json, non-jq output)
	if c.Region != "" && c.Output != "json" && c.Output != "json-indent" && c.Output != "jq" {
		filtered := []FlattenedInstanceType{}
		for _, inst := range flattened {
			if strings.EqualFold(inst.Region, c.Region) {
				filtered = append(filtered, inst)
			}
		}
		flattened = filtered
	}

	return c.formatOutput(flattened, os.Stdout)
}

func (c *CloudListInstanceTypesCmd) flattenInstanceTypes(providers []CloudProvider) []FlattenedInstanceType {
	var flattened []FlattenedInstanceType

	for _, provider := range providers {
		for _, region := range provider.Regions {
			for _, instType := range region.InstanceTypes {
				// Get architectures as comma-separated string
				arch := strings.Join(instType.SupportedArchitectures, ",")
				if arch == "" {
					arch = "N/A"
				}

				flattened = append(flattened, FlattenedInstanceType{
					Region:       region.Name,
					Type:         instType.InstanceType,
					Arch:         arch,
					Vcpus:        instType.Vcpus,
					RAMGib:       instType.MemoryGib,
					Disks:        instType.LocalStorage.NumberOfDisks,
					DiskSizeGib:  instType.LocalStorage.DiskSizeGib,
					TotalSizeGib: instType.LocalStorage.TotalSizeGib,
				})
			}
		}
	}

	return flattened
}

func (c *CloudListInstanceTypesCmd) formatOutput(instanceTypes []FlattenedInstanceType, out io.Writer) error {
	var err error
	var page *pager.Pager

	if c.Pager {
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
			enc.Encode(instanceTypes)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(instanceTypes)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(instanceTypes)
	case "text":
		fmt.Fprintln(out, "Instance Types:")
		for _, inst := range instanceTypes {
			fmt.Fprintf(out, "Region: %s, Type: %s, Arch: %s, vCPUs: %d, RAMGib: %d, Disks: %d, DiskSizeGib: %d, TotalSizeGib: %d\n",
				inst.Region, inst.Type, inst.Arch, inst.Vcpus, inst.RAMGib, inst.Disks, inst.DiskSizeGib, inst.TotalSizeGib)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Region:asc", "Type:asc"}
		}
		header := table.Row{"Region", "Type", "Arch", "vCPUs", "RAMGib", "Disks", "DiskSizeGib", "TotalSizeGib"}
		rows := []table.Row{}
		for _, inst := range instanceTypes {
			rows = append(rows, table.Row{
				inst.Region,
				inst.Type,
				inst.Arch,
				inst.Vcpus,
				inst.RAMGib,
				inst.Disks,
				inst.DiskSizeGib,
				inst.TotalSizeGib,
			})
		}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				fmt.Fprintf(os.Stderr, "Warning: Couldn't get terminal width, using default width\n")
			} else {
				return err
			}
		}
		title := printer.String("INSTANCE TYPES")
		fmt.Fprintln(out, t.RenderTable(title, header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
)

type inventoryInstanceTypesCmd struct {
	Json         bool                         `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty   bool                         `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	Arm          bool                         `short:"a" long:"arm" description:"Set to look for ARM instances instead of amd64"`
	FilterMinCPU int                          `short:"c" long:"min-cpus" description:"Search for at least X CPUs"`
	FilterMaxCPU int                          `short:"C" long:"max-cpus" description:"Search for max X CPUs"`
	FilterMinRAM float64                      `short:"r" long:"min-ram" description:"Search for at least X RAM GB"`
	FilterMaxRAM float64                      `short:"R" long:"max-ram" description:"Search for max X RAM GB"`
	EphemeralMin int                          `short:"e" long:"min-ephemeral" description:"Search only for instances with at least X ephemeral devices"`
	EphemeralMax int                          `short:"E" long:"max-ephemeral" description:"Search only for instances with max X ephemeral devices"`
	Gcp          inventoryInstanceTypesCmdGcp `no-flag:"true"`
	Help         helpCmd                      `command:"help" subcommands-optional:"true" description:"Print help"`
}

type inventoryInstanceTypesCmdGcp struct {
	Zone string `short:"z" long:"zone" description:"zone name to query"`
}

func init() {
	addBackendSwitch("inventory.instance-types", "gcp", &a.opts.Inventory.InstanceTypes.Gcp)
}

func (c *inventoryInstanceTypesCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type == "docker" {
		return errors.New("feature not available on docker")
	}
	t, err := b.GetInstanceTypes(c.FilterMinCPU, c.FilterMaxCPU, c.FilterMinRAM, c.FilterMaxRAM, c.EphemeralMin, c.EphemeralMax, c.Arm, c.Gcp.Zone)
	if err != nil {
		return err
	}
	if c.Json {
		enc := json.NewEncoder(os.Stdout)
		if c.JsonPretty {
			enc.SetIndent("", "    ")
		}
		err = enc.Encode(t)
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Instance Name", "CPUs", "Ram GB", "Local Disks", "Local Disk Total Size GB"})
	table.SetAutoFormatHeaders(false)
	for _, v := range t {
		edisks := strconv.Itoa(v.EphemeralDisks)
		edisksize := strings.TrimSuffix(strconv.FormatFloat(v.EphemeralDiskTotalSizeGB, 'f', 2, 64), ".00")
		if v.EphemeralDisks == -1 {
			edisks = "unknown"
		}
		if v.EphemeralDiskTotalSizeGB == -1 {
			edisksize = "unknown"
		}
		vv := []string{
			v.InstanceName,
			strconv.Itoa(v.CPUs),
			strings.TrimSuffix(strconv.FormatFloat(v.RamGB, 'f', 2, 64), ".00"),
			edisks,
			edisksize,
		}
		table.Append(vv)
	}
	table.Render()
	if a.opts.Config.Backend.Type == "gcp" {
		fmt.Println("* local ephemeral disks are not automatically allocated to the machines; these need to be requested in the quantity required; each disk is always 375 GB")
	}
	return nil
}

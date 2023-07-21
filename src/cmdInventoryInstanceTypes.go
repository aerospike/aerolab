package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
	"github.com/olekukonko/tablewriter"
)

type inventoryInstanceTypesCmd struct {
	Json         bool                         `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty   bool                         `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	Arm          bool                         `short:"a" long:"arm" description:"Set to look for ARM instances instead of amd64"`
	Nodes        int                          `short:"N" long:"nodes" description:"Number of nodes (essentially a price multiplier for the result)" default:"1"`
	FilterName   string                       `short:"n" long:"name" description:"Filter by full or partial name"`
	FilterMinCPU int                          `short:"c" long:"min-cpus" description:"Search for at least X CPUs"`
	FilterMaxCPU int                          `short:"C" long:"max-cpus" description:"Search for max X CPUs"`
	FilterMinRAM float64                      `short:"r" long:"min-ram" description:"Search for at least X RAM GB"`
	FilterMaxRAM float64                      `short:"R" long:"max-ram" description:"Search for max X RAM GB"`
	EphemeralMin int                          `short:"e" long:"min-ephemeral" description:"Search only for instances with at least X ephemeral devices"`
	EphemeralMax int                          `short:"E" long:"max-ephemeral" description:"Search only for instances with max X ephemeral devices"`
	SortOrder    []string                     `short:"s" long:"sort" description:"Sort order; can be specified multiple times; values: name, cpu, ram, disks, size, price" default:"name"`
	Gcp          inventoryInstanceTypesCmdGcp `no-flag:"true"`
	Help         helpCmd                      `command:"help" subcommands-optional:"true" description:"Print help"`
}

type inventoryInstanceTypesCmdGcp struct {
	Zone string `short:"z" long:"zone" description:"zone name to query"`
}

func init() {
	addBackendSwitch("inventory.instance-types", "gcp", &a.opts.Inventory.InstanceTypes.Gcp)
}

type inventorySorter struct {
	SortOrders []string
	currentSo  int
	cmpItem    *[]instanceType
}

func (c *inventorySorter) instanceTypesSort(i, j int) bool {
	if c.currentSo == len(c.SortOrders) {
		c.currentSo = 0
		return false
	}
	switch c.SortOrders[c.currentSo] {
	case "cpu":
		cmpl := (*c.cmpItem)[i].CPUs
		cmpr := (*c.cmpItem)[j].CPUs
		if cmpl < cmpr {
			c.currentSo = 0
			return true
		} else if cmpl > cmpr {
			c.currentSo = 0
			return false
		} else {
			c.currentSo++
			return c.instanceTypesSort(i, j)
		}
	case "ram":
		cmpl := (*c.cmpItem)[i].RamGB
		cmpr := (*c.cmpItem)[j].RamGB
		if cmpl < cmpr {
			c.currentSo = 0
			return true
		} else if cmpl > cmpr {
			c.currentSo = 0
			return false
		} else {
			c.currentSo++
			return c.instanceTypesSort(i, j)
		}
	case "disks":
		cmpl := (*c.cmpItem)[i].EphemeralDisks
		cmpr := (*c.cmpItem)[j].EphemeralDisks
		if cmpl < cmpr {
			c.currentSo = 0
			return true
		} else if cmpl > cmpr {
			c.currentSo = 0
			return false
		} else {
			c.currentSo++
			return c.instanceTypesSort(i, j)
		}
	case "size":
		cmpl := (*c.cmpItem)[i].EphemeralDiskTotalSizeGB
		cmpr := (*c.cmpItem)[j].EphemeralDiskTotalSizeGB
		if cmpl < cmpr {
			c.currentSo = 0
			return true
		} else if cmpl > cmpr {
			c.currentSo = 0
			return false
		} else {
			c.currentSo++
			return c.instanceTypesSort(i, j)
		}
	case "price":
		cmpl := (*c.cmpItem)[i].PriceUSD
		cmpr := (*c.cmpItem)[j].PriceUSD
		if cmpl < cmpr {
			c.currentSo = 0
			return true
		} else if cmpl > cmpr {
			c.currentSo = 0
			return false
		} else {
			c.currentSo++
			return c.instanceTypesSort(i, j)
		}
	default:
		cmpl := strings.Split(strings.Split((*c.cmpItem)[i].InstanceName, ".")[0], "-")[0]
		cmpr := strings.Split(strings.Split((*c.cmpItem)[j].InstanceName, ".")[0], "-")[0]
		if a.opts.Config.Backend.Type == "gcp" {
			cmpl = strings.Join(strings.Split(strings.Split((*c.cmpItem)[i].InstanceName, ".")[0], "-")[0:2], "-")
			cmpr = strings.Join(strings.Split(strings.Split((*c.cmpItem)[j].InstanceName, ".")[0], "-")[0:2], "-")
		}
		if cmpl < cmpr {
			c.currentSo = 0
			return true
		} else if cmpl > cmpr {
			c.currentSo = 0
			return false
		} else {
			c.currentSo++
			return c.instanceTypesSort(i, j)
		}
	}
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

	if !inslice.HasString(c.SortOrder, "name") {
		c.SortOrder = append(c.SortOrder, "name")
	}
	if !inslice.HasString(c.SortOrder, "price") {
		c.SortOrder = append(c.SortOrder, "price")
	}
	sorter := inventorySorter{
		SortOrders: c.SortOrder,
		currentSo:  0,
		cmpItem:    &t,
	}
	sort.Slice(t, sorter.instanceTypesSort)

	if c.Json {
		enc := json.NewEncoder(os.Stdout)
		if c.JsonPretty {
			enc.SetIndent("", "    ")
		}
		err = enc.Encode(t)
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Instance Name", "CPUs", "Ram GB", "Local Disks", "Local Disk Total Size GB", "On-Demand $/hour", "On-Demand $/day", "On-Demand $/month"})
	table.SetAutoFormatHeaders(false)
	for _, v := range t {
		if c.FilterName != "" && !strings.HasPrefix(v.InstanceName, c.FilterName) {
			continue
		}
		edisks := strconv.Itoa(v.EphemeralDisks)
		edisksize := strings.TrimSuffix(strconv.FormatFloat(v.EphemeralDiskTotalSizeGB, 'f', 2, 64), ".00")
		if v.EphemeralDisks == -1 {
			edisks = "unknown"
		}
		if v.EphemeralDiskTotalSizeGB == -1 {
			edisksize = "unknown"
		}
		price := strconv.FormatFloat(v.PriceUSD*float64(c.Nodes), 'f', 4, 64)
		if v.PriceUSD <= 0 {
			price = "unknown"
		}
		pricepd := strconv.FormatFloat(v.PriceUSD*24*float64(c.Nodes), 'f', 2, 64)
		if v.PriceUSD <= 0 {
			pricepd = "unknown"
		}
		pricepm := strconv.FormatFloat(v.PriceUSD*24*30.5*float64(c.Nodes), 'f', 2, 64)
		if v.PriceUSD <= 0 {
			pricepm = "unknown"
		}
		vv := []string{
			v.InstanceName,
			strconv.Itoa(v.CPUs),
			strings.TrimSuffix(strconv.FormatFloat(v.RamGB, 'f', 2, 64), ".00"),
			edisks,
			edisksize,
			price,
			pricepd,
			pricepm,
		}
		table.Append(vv)
	}
	table.Render()
	if a.opts.Config.Backend.Type == "gcp" {
		fmt.Println("* local ephemeral disks are not automatically allocated to the machines; these need to be requested in the quantity required; each local disk is always 375 GB")
		fmt.Println("* pricing does not include any disks; disk pricing at https://cloud.google.com/compute/disks-image-pricing#disk")
	} else if a.opts.Config.Backend.Type == "aws" {
		fmt.Println("* pricing does not include attached persistent disks (EBS); disk pricing at https://aws.amazon.com/ebs/pricing/")
	}
	fmt.Println("* above prices do not include taxes, add tax as required for total price; prices are estimates, see the calculator for exact pricing: https://cloudpricingcalculator.appspot.com/")
	return nil
}

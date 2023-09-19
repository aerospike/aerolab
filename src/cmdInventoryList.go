package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	isatty "github.com/mattn/go-isatty"

	"github.com/jedib0t/go-pretty/v6/table"
)

type inventoryListCmd struct {
	Owner      string  `long:"owner" description:"Only show resources tagged with this owner"`
	Json       bool    `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty bool    `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *inventoryListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.run(true, true, true, true, true, inventoryShowExpirySystem)
}

const inventoryShowExpirySystem = 1

func (c *inventoryListCmd) run(showClusters bool, showClients bool, showTemplates bool, showFirewalls bool, showSubnets bool, showOthers ...int) error {
	inventoryItems := []int{}
	if showClusters {
		inventoryItems = append(inventoryItems, InventoryItemClusters)
	}
	if showClients {
		inventoryItems = append(inventoryItems, InventoryItemClients)
	}
	if showTemplates {
		inventoryItems = append(inventoryItems, InventoryItemTemplates)
	}
	if showFirewalls || showSubnets {
		inventoryItems = append(inventoryItems, InventoryItemFirewalls)
	}
	for _, showOther := range showOthers {
		if showOther&inventoryShowExpirySystem > 0 {
			inventoryItems = append(inventoryItems, InventoryItemExpirySystem)
		}
	}

	inv, err := b.Inventory(c.Owner, inventoryItems)
	if err != nil {
		return err
	}

	for vi, v := range inv.Clients {
		nip := v.PublicIp
		if nip == "" {
			nip = v.PrivateIp
		}
		port := ""
		if a.opts.Config.Backend.Type == "docker" && inv.Clients[vi].DockerExposePorts != "" {
			nip = "127.0.0.1"
			port = ":" + inv.Clients[vi].DockerExposePorts
		}
		switch strings.ToLower(v.ClientType) {
		case "ams":
			if port == "" {
				port = ":3000"
			}
			inv.Clients[vi].AccessUrl = "http://" + nip + port
			inv.Clients[vi].AccessPort = "3000"
		case "elasticsearch":
			if port == "" {
				port = ":9200"
			}
			inv.Clients[vi].AccessUrl = "http://" + nip + port + "/NAMESPACE/_search"
			inv.Clients[vi].AccessPort = "9200"
		case "rest-gateway":
			if port == "" {
				port = ":8081"
			}
			inv.Clients[vi].AccessUrl = "http://" + nip + port
			inv.Clients[vi].AccessPort = "8081"
		case "vscode":
			if port == "" {
				port = ":8080"
			}
			inv.Clients[vi].AccessUrl = "http://" + nip + port
			inv.Clients[vi].AccessPort = "8080"
		}
	}

	if c.Json {
		enc := json.NewEncoder(os.Stdout)
		if c.JsonPretty {
			enc.SetIndent("", "    ")
		}
		if showClusters && showClients && showTemplates && showFirewalls && showSubnets {
			err = enc.Encode(inv)
			return err
		}
		if showClusters {
			err = enc.Encode(inv.Clusters)
			if err != nil {
				return err
			}
		}
		if showClients {
			err = enc.Encode(inv.Clients)
			if err != nil {
				return err
			}
		}
		if showTemplates {
			err = enc.Encode(inv.Templates)
			if err != nil {
				return err
			}
		}
		if showFirewalls {
			err = enc.Encode(inv.FirewallRules)
			if err != nil {
				return err
			}
		}
		if showSubnets {
			err = enc.Encode(inv.Subnets)
			if err != nil {
				return err
			}
		}
		for _, showOther := range showOthers {
			if showOther&inventoryShowExpirySystem > 0 {
				err = enc.Encode(inv.ExpirySystem)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}

	sort.Slice(inv.Templates, func(i, j int) bool {
		if inv.Templates[i].AerospikeVersion > inv.Templates[j].AerospikeVersion {
			return true
		} else if inv.Templates[i].AerospikeVersion < inv.Templates[j].AerospikeVersion {
			return false
		} else {
			if inv.Templates[i].Arch < inv.Templates[j].Arch {
				return true
			} else if inv.Templates[i].Arch > inv.Templates[j].Arch {
				return false
			} else {
				if inv.Templates[i].Distribution < inv.Templates[j].Distribution {
					return true
				} else if inv.Templates[i].Distribution > inv.Templates[j].Distribution {
					return false
				} else {
					return inv.Templates[i].OSVersion < inv.Templates[j].OSVersion
				}
			}
		}
	})

	sort.Slice(inv.Clusters, func(i, j int) bool {
		if inv.Clusters[i].ClusterName < inv.Clusters[j].ClusterName {
			return true
		} else if inv.Clusters[i].ClusterName > inv.Clusters[j].ClusterName {
			return false
		} else {
			return inv.Clusters[i].NodeNo < inv.Clusters[j].NodeNo
		}
	})

	sort.Slice(inv.Clients, func(i, j int) bool {
		if inv.Clients[i].ClientName < inv.Clients[j].ClientName {
			return true
		} else if inv.Clients[i].ClientName > inv.Clients[j].ClientName {
			return false
		} else {
			return inv.Clients[i].NodeNo < inv.Clients[j].NodeNo
		}
	})

	sort.Slice(inv.FirewallRules, func(i, j int) bool {
		switch a.opts.Config.Backend.Type {
		case "gcp":
			return inv.FirewallRules[i].GCP.FirewallName < inv.FirewallRules[j].GCP.FirewallName
		case "aws":
			if inv.FirewallRules[i].AWS.VPC < inv.FirewallRules[j].AWS.VPC {
				return true
			} else if inv.FirewallRules[i].AWS.VPC > inv.FirewallRules[j].AWS.VPC {
				return false
			} else {
				return inv.FirewallRules[i].AWS.SecurityGroupName < inv.FirewallRules[j].AWS.SecurityGroupName
			}
		default:
			return inv.FirewallRules[i].Docker.NetworkName < inv.FirewallRules[j].Docker.NetworkName
		}
	})

	t := table.NewWriter()
	// For now, don't set the allowed row lenght, wrapping is better
	// until we do something more clever...
	if isatty.IsTerminal(os.Stdout.Fd()) {
		// fmt.Println("Is Terminal")
		t.SetStyle(table.StyleColoredBlackOnBlueWhite)

		// s, err := tsize.GetSize()
		if err != nil {
			fmt.Println("Couldn't get terminal width")
		}
		// t.SetAllowedRowLength(s.Width)
	} else if isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		// fmt.Println("Is Cygwin/MSYS2 Terminal")
		t.SetStyle(table.StyleColoredBlackOnBlueWhite)

		// s, err := tsize.GetSize()
		if err != nil {
			fmt.Println("Couldn't get terminal width")
		}
		// t.SetAllowedRowLength(s.Width)
	} else {
		fmt.Fprintln(os.Stderr, "aerolab does not have a stable CLI interface. Use with caution in scripts.\nIn scripts, the JSON output should be used for stability.")
		t.SetStyle(table.StyleDefault)
	}

	if showTemplates {
		t.SetTitle("TEMPLATES")
		t.ResetHeaders()
		t.ResetRows()
		t.ResetFooters()
		t.AppendHeader(table.Row{"Aerospike Version", "Arch", "Distribution", "OS Version"})
		for _, v := range inv.Templates {
			vv := table.Row{
				v.AerospikeVersion,
				v.Arch,
				v.Distribution,
				v.OSVersion,
			}
			t.AppendRow(vv)
		}
		fmt.Println(t.Render())
		fmt.Println()
	}

	if showClusters {
		t.SetTitle("CLUSTERS")
		t.ResetHeaders()
		t.ResetRows()
		t.ResetFooters()
		if a.opts.Config.Backend.Type == "gcp" {
			t.AppendHeader(table.Row{"Cluster Name", "Node No", "Instance ID", "Zone", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Aerospike Version", "Firewalls", "Owner", "Instance Running Cost", "Expires"})
		} else if a.opts.Config.Backend.Type == "aws" {
			t.AppendHeader(table.Row{"Cluster Name", "Node No", "Instance ID", "Image ID", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Aerospike Version", "Firewalls", "Owner", "Instance Running Cost", "Expires"})
		} else {
			t.AppendHeader(table.Row{"Cluster Name", "Node No", "Instance ID", "Image ID", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Aerospike Version", "Firewalls", "Owner", "Exposed Port 1"})
		}
		for _, v := range inv.Clusters {
			vv := table.Row{
				v.ClusterName,
				v.NodeNo,
				v.InstanceId,
			}
			if a.opts.Config.Backend.Type == "gcp" {
				vv = append(vv, v.Zone)
			} else {
				vv = append(vv, v.ImageId)
			}
			vv = append(vv,
				v.Arch,
				v.PrivateIp,
				v.PublicIp,
				v.State,
				v.Distribution,
				strings.ReplaceAll(v.OSVersion, "-", "."),
				strings.ReplaceAll(v.AerospikeVersion, "-", "."),
				strings.Join(v.Firewalls, ","),
				v.Owner,
			)
			if a.opts.Config.Backend.Type != "docker" {
				vv = append(vv, strconv.FormatFloat(v.InstanceRunningCost, 'f', 4, 64))
				vv = append(vv, v.Expires)
			} else {
				vv = append(vv, v.DockerExposePorts)
			}
			t.AppendRow(vv)
		}

		fmt.Println(t.Render())
		if a.opts.Config.Backend.Type != "docker" {
			fmt.Println("* instance Running Cost displays only the cost of owning the instance in a running state for the duration it was running so far. It does not account for taxes, disk, network or transfer costs.")
			fmt.Println()
		} else {
			fmt.Println("* to connect directly to the cluster (non-docker-desktop), execute 'aerolab cluster list' and connect to the node IP on the given exposed port (or configured aerospike services port - default 3000)")
			fmt.Println("* to connect to the cluster when using Docker Desktop, execute 'aerolab cluster list` and connect to IP 127.0.0.1:EXPOSED_PORT with a connect policy of `--services-alternate`")
			fmt.Println()
		}
	}

	if showClients {
		t.SetTitle("CLIENTS")
		t.ResetHeaders()
		t.ResetRows()
		t.ResetFooters()
		if a.opts.Config.Backend.Type == "gcp" {
			t.AppendHeader(table.Row{"Cluster Name", "Node No", "Instance ID", "Zone", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Firewalls", "Owner", "Client Type", "Access URL", "Access Port", "Instance Running Cost", "Expires"})
		} else if a.opts.Config.Backend.Type == "aws" {
			t.AppendHeader(table.Row{"Cluster Name", "Node No", "Instance ID", "Image ID", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Firewalls", "Owner", "Client Type", "Access URL", "Access Port", "Instance Running Cost", "Expires"})
		} else {
			t.AppendHeader(table.Row{"Cluster Name", "Node No", "Instance ID", "Image ID", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Firewalls", "Owner", "Client Type", "Access URL", "Access Port", "Exposed Port 1"})
		}
		for _, v := range inv.Clients {
			vv := table.Row{
				v.ClientName,
				v.NodeNo,
				v.InstanceId,
			}
			if a.opts.Config.Backend.Type == "gcp" {
				vv = append(vv, v.Zone)
			} else {
				vv = append(vv, v.ImageId)
			}
			vv = append(vv,
				v.Arch,
				v.PrivateIp,
				v.PublicIp,
				v.State,
				v.Distribution,
				strings.ReplaceAll(v.OSVersion, "-", "."),
				strings.Join(v.Firewalls, ","),
				v.Owner,
				v.ClientType,
				v.AccessUrl,
				v.AccessPort,
			)
			if a.opts.Config.Backend.Type != "docker" {
				vv = append(vv, strconv.FormatFloat(v.InstanceRunningCost, 'f', 4, 64))
				vv = append(vv, v.Expires)
			} else {
				vv = append(vv, v.DockerExposePorts)
			}
			t.AppendRow(vv)
		}
		fmt.Println(t.Render())
		if a.opts.Config.Backend.Type == "docker" {
			fmt.Println("* if using Docker Desktop and forwaring ports by exposing them (-e ...), use IP 127.0.0.1 for the Access URL")
			fmt.Println()
		} else {
			fmt.Println("* instance Running Cost displays only the cost of owning the instance in a running state for the duration it was running so far. It does not account for taxes, disk, network or transfer costs.")
			fmt.Println()
		}
	}

	if showFirewalls {
		t.ResetHeaders()
		t.ResetRows()
		t.ResetFooters()
		switch a.opts.Config.Backend.Type {
		case "gcp":
			t.SetTitle("FIREWALL RULES")
			t.AppendHeader(table.Row{"Firewall Name", "Target Tags", "Source Tags", "Source Ranges", "Allow Ports", "Deny Ports"})
			for _, v := range inv.FirewallRules {
				vv := table.Row{
					v.GCP.FirewallName,
					strings.Join(v.GCP.TargetTags, " "),
					strings.Join(v.GCP.SourceTags, " "),
					strings.Join(v.GCP.SourceRanges, " "),
					strings.Join(v.GCP.AllowPorts, " "),
					strings.Join(v.GCP.DenyPorts, " "),
				}
				t.AppendRow(vv)
			}
		case "aws":
			t.SetTitle("SECURITY GROUPS")
			t.AppendHeader(table.Row{"VPC", "Security Group Name", "Security Group ID", "IPs"})
			for _, v := range inv.FirewallRules {
				vv := table.Row{
					v.AWS.VPC,
					v.AWS.SecurityGroupName,
					v.AWS.SecurityGroupID,
					strings.Join(v.AWS.IPs, ","),
				}
				t.AppendRow(vv)
			}
		case "docker":
			t.SetTitle("NETWORKS")
			t.AppendHeader(table.Row{"Network Name", "Network Driver", "Subnets", "MTU"})
			for _, v := range inv.FirewallRules {
				vv := table.Row{
					v.Docker.NetworkName,
					v.Docker.NetworkDriver,
					v.Docker.Subnets,
					v.Docker.MTU,
				}
				t.AppendRow(vv)
			}

		}

		fmt.Println(t.Render())
		fmt.Println()
	}

	if showSubnets {
		t.ResetHeaders()
		t.ResetRows()
		t.ResetFooters()
		switch a.opts.Config.Backend.Type {
		case "aws":
			fmt.Println("\nSUBNETS:")
			t.AppendHeader(table.Row{"VPC ID", "VPC Name", "VPC Cidr", "Avail. Zone", "Subnet ID", "Subnet Cidr", "AZ Default", "Subnet Name", "Auto-Assign IP"})
			for _, v := range inv.Subnets {
				autoIP := "no (enable to use with aerolab)"
				if v.AWS.AutoPublicIP {
					autoIP = "yes (ok)"
				}
				vv := table.Row{
					v.AWS.VpcId,
					v.AWS.VpcName,
					v.AWS.VpcCidr,
					v.AWS.AvailabilityZone,
					v.AWS.SubnetId,
					v.AWS.SubnetCidr,
					fmt.Sprintf("%t", v.AWS.IsAzDefault),
					v.AWS.SubnetName,
					autoIP,
				}
				t.AppendRow(vv)
			}
			fmt.Println(t.Render())
			fmt.Println()
		}
	}

	for _, showOther := range showOthers {
		if showOther&inventoryShowExpirySystem > 0 {
			t.ResetHeaders()
			t.ResetRows()
			t.ResetFooters()
			t.AppendHeader(table.Row{"#", "Subsystem", "Details"})
			switch a.opts.Config.Backend.Type {
			case "aws":
				t.SetTitle("EXPIRY_SYSTEM")
				for i, v := range inv.ExpirySystem {
					t.AppendRow(table.Row{i, "IAM Function Rule", v.IAMFunction})
					t.AppendRow(table.Row{i, "IAM Scheduler Rule", v.IAMScheduler})
					t.AppendRow(table.Row{i, "Function", v.Function})
					t.AppendRow(table.Row{i, "Scheduler", v.Scheduler})
					t.AppendRow(table.Row{i, "Schedule", v.Schedule})
				}
				fmt.Println(t.Render())
			case "gcp":
				t.SetTitle("EXPIRY_SYSTEM")
				for i, v := range inv.ExpirySystem {
					t.AppendRow(table.Row{i, "Function", v.Function})
					t.AppendRow(table.Row{i, "Source Bucket", v.SourceBucket})
					t.AppendRow(table.Row{i, "Scheduler", v.Scheduler})
					t.AppendRow(table.Row{i, "Schedule", v.Schedule})
				}
				fmt.Println(t.Render())
				fmt.Println()
			}
		}
	}

	return nil
}

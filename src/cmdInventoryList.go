package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
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
	return c.run(true, true, true, true, true)
}

func (c *inventoryListCmd) run(showClusters bool, showClients bool, showTemplates bool, showFirewalls bool, showSubnets bool) error {
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

	inv, err := b.Inventory(c.Owner, inventoryItems)
	if err != nil {
		return err
	}

	for vi, v := range inv.Clients {
		nip := v.PublicIp
		if nip == "" {
			nip = v.PrivateIp
		}
		switch strings.ToLower(v.ClientType) {
		case "ams":
			inv.Clients[vi].AccessUrl = "http://" + nip + ":3000"
			inv.Clients[vi].AccessPort = "3000"
		case "elasticsearch":
			inv.Clients[vi].AccessUrl = "http://" + nip + ":9200/NAMESPACE/_search"
			inv.Clients[vi].AccessPort = "9200"
		case "rest-gateway":
			inv.Clients[vi].AccessUrl = "http://" + nip + ":8082"
			inv.Clients[vi].AccessPort = "8082"
		case "vscode":
			inv.Clients[vi].AccessUrl = "http://" + nip + ":8080"
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

	if showTemplates {
		fmt.Println("\nTEMPLATES:")
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Aerospike Version", "Arch", "Distribution", "OS Version"})
		table.SetAutoFormatHeaders(false)
		for _, v := range inv.Templates {
			vv := []string{
				v.AerospikeVersion,
				v.Arch,
				v.Distribution,
				v.OSVersion,
			}
			table.Append(vv)
		}
		table.Render()
	}

	if showClusters {
		fmt.Println("\nCLUSTERS:")
		table := tablewriter.NewWriter(os.Stdout)
		if a.opts.Config.Backend.Type == "gcp" {
			table.SetHeader([]string{"Cluster Name", "Node No", "Instance ID", "Zone", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Aerospike Version", "Firewalls", "Owner", "Instance Running Cost"})
		} else if a.opts.Config.Backend.Type == "aws" {
			table.SetHeader([]string{"Cluster Name", "Node No", "Instance ID", "Image ID", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Aerospike Version", "Firewalls", "Owner", "Instance Running Cost"})
		} else {
			table.SetHeader([]string{"Cluster Name", "Node No", "Instance ID", "Image ID", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Aerospike Version", "Firewalls", "Owner"})
		}
		table.SetAutoFormatHeaders(false)
		for _, v := range inv.Clusters {
			vv := []string{
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
			}
			table.Append(vv)
		}
		table.Render()
		if a.opts.Config.Backend.Type != "docker" {
			fmt.Println("* instance Running Cost displays only the cost of owning the instance in a running state for the duration it was running so far. It does not account for taxes, disk, network or transfer costs.")
		}
	}

	if showClients {
		fmt.Println("\nCLIENTS:")
		table := tablewriter.NewWriter(os.Stdout)
		if a.opts.Config.Backend.Type == "gcp" {
			table.SetHeader([]string{"Cluster Name", "Node No", "Instance ID", "Zone", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Firewalls", "Owner", "Client Type", "Access URL", "Access Port", "Instance Running Cost"})
		} else if a.opts.Config.Backend.Type == "aws" {
			table.SetHeader([]string{"Cluster Name", "Node No", "Instance ID", "Image ID", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Firewalls", "Owner", "Client Type", "Access URL", "Access Port", "Instance Running Cost"})
		} else {
			table.SetHeader([]string{"Cluster Name", "Node No", "Instance ID", "Image ID", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Firewalls", "Owner", "Client Type", "Access URL", "Access Port"})
		}
		table.SetAutoFormatHeaders(false)
		for _, v := range inv.Clients {
			vv := []string{
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
			}
			table.Append(vv)
		}
		table.Render()
		if a.opts.Config.Backend.Type == "docker" {
			fmt.Println("* if using Docker Desktop and forwaring ports by exposing them (-e ...), use IP 127.0.0.1 for the Access URL")
		} else {
			fmt.Println("* instance Running Cost displays only the cost of owning the instance in a running state for the duration it was running so far. It does not account for taxes, disk, network or transfer costs.")
		}
	}

	if showFirewalls {
		table := tablewriter.NewWriter(os.Stdout)
		table.SetAutoFormatHeaders(false)
		switch a.opts.Config.Backend.Type {
		case "gcp":
			fmt.Println("\nFIREWALL RULES:")
			table.SetHeader([]string{"Firewall Name", "Target Tags", "Source Tags", "Source Ranges", "Allow Ports", "Deny Ports"})
			for _, v := range inv.FirewallRules {
				vv := []string{
					v.GCP.FirewallName,
					strings.Join(v.GCP.TargetTags, " "),
					strings.Join(v.GCP.SourceTags, " "),
					strings.Join(v.GCP.SourceRanges, " "),
					strings.Join(v.GCP.AllowPorts, " "),
					strings.Join(v.GCP.DenyPorts, " "),
				}
				table.Append(vv)
			}
			table.Render()
		case "aws":
			fmt.Println("\nSECURITY GROUPS:")
			table.SetHeader([]string{"VPC", "Security Group Name", "Security Group ID", "IPs"})
			for _, v := range inv.FirewallRules {
				vv := []string{
					v.AWS.VPC,
					v.AWS.SecurityGroupName,
					v.AWS.SecurityGroupID,
					strings.Join(v.AWS.IPs, ","),
				}
				table.Append(vv)
			}
			table.Render()
		case "docker":
			fmt.Println("\nNETWORKS:")
			table.SetHeader([]string{"Network Name", "Network Driver", "Subnets", "MTU"})
			for _, v := range inv.FirewallRules {
				vv := []string{
					v.Docker.NetworkName,
					v.Docker.NetworkDriver,
					v.Docker.Subnets,
					v.Docker.MTU,
				}
				table.Append(vv)
			}
			table.Render()
		}
	}

	if showSubnets {
		table := tablewriter.NewWriter(os.Stdout)
		table.SetAutoFormatHeaders(false)
		switch a.opts.Config.Backend.Type {
		case "aws":
			fmt.Println("\nSUBNETS:")
			table.SetHeader([]string{"VPC ID", "VPC Name", "VPC Cidr", "Avail. Zone", "Subnet ID", "Subnet Cidr", "AZ Default", "Subnet Name", "Auto-Assign IP"})
			for _, v := range inv.Subnets {
				autoIP := "no (enable to use with aerolab)"
				if v.AWS.AutoPublicIP {
					autoIP = "yes (ok)"
				}
				vv := []string{
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
				table.Append(vv)
			}
			table.Render()
		}
	}
	return nil
}

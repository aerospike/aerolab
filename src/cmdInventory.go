package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/olekukonko/tablewriter"
)

type inventoryCmd struct {
	List inventoryListCmd `command:"list" subcommands-optional:"true" description:"List clusters, clients and templates"`
	Help helpCmd          `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *inventoryCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type inventoryListCmd struct {
	Json       bool    `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty bool    `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *inventoryListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	inv, err := b.Inventory()
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
		err = enc.Encode(inv)
		return err
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

	fmt.Println("\nCLUSTERS:")
	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Cluster Name", "Node No", "Instance ID", "Image ID", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Aerospike Version"})
	table.SetAutoFormatHeaders(false)
	for _, v := range inv.Clusters {
		vv := []string{
			v.ClusterName,
			v.NodeNo,
			v.InstanceId,
			v.ImageId,
			v.Arch,
			v.PrivateIp,
			v.PublicIp,
			v.State,
			v.Distribution,
			strings.ReplaceAll(v.OSVersion, "-", "."),
			strings.ReplaceAll(v.AerospikeVersion, "-", "."),
		}
		table.Append(vv)
	}
	table.Render()

	fmt.Println("\nCLIENTS:")
	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Cluster Name", "Node No", "Instance ID", "Image ID", "Arch", "Private IP", "Public IP", "State", "Distribution", "OS Version", "Client Type", "Access URL", "Access Port"})
	table.SetAutoFormatHeaders(false)
	for _, v := range inv.Clients {
		vv := []string{
			v.ClientName,
			v.NodeNo,
			v.InstanceId,
			v.ImageId,
			v.Arch,
			v.PrivateIp,
			v.PublicIp,
			v.State,
			v.Distribution,
			strings.ReplaceAll(v.OSVersion, "-", "."),
			v.ClientType,
			v.AccessUrl,
			v.AccessPort,
		}
		table.Append(vv)
	}
	table.Render()
	fmt.Println("* if using Docker Desktop and forwaring ports by exposing them (-e ...), use IP 127.0.0.1 for the Access URL")

	table = tablewriter.NewWriter(os.Stdout)
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
		table.SetHeader([]string{"VPC", "Security Group Name", "Security Group ID"})
		for _, v := range inv.FirewallRules {
			vv := []string{
				v.AWS.VPC,
				v.AWS.SecurityGroupName,
				v.AWS.SecurityGroupID,
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
	return nil
}

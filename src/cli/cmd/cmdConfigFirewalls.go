package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

func ListSubnets(system *System, output string, tableTheme string, sortBy []string, backendType string, cmd []string, c interface{}, args []string, inventory *backends.Inventory) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != backendType {
		return errors.New("this command is only available for AWS/GCP backend types; selected backend does not match command constraints")
	}
	inventory = system.Backend.GetInventory()
	net := inventory.Networks.Describe()

	switch output {
	case "json":
		json.NewEncoder(os.Stdout).Encode(net)
	case "json-indent":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(net)
	case "text":
		system.Logger.Info("Networks:")
		for _, net := range net {
			for _, subnet := range net.Subnets {
				ntype := "manual"
				if subnet.IsAerolabManaged {
					ntype = "aerolab"
				} else if subnet.IsDefault {
					ntype = "default"
				}
				fmt.Printf("Backend: %s, Network: %s, NetID: %s, Subnet: %s, SubnetID: %s, CIDR: %s, Owner: %s, PublicIP: %t, Type: %s\n",
					net.BackendType, net.Name, net.NetworkId, subnet.Name, subnet.SubnetId, subnet.Cidr, subnet.Owner, subnet.PublicIP, ntype)
			}
		}
	default:
		header := table.Row{"Backend", "Network", "NetID", "Subnet", "SubnetID", "CIDR", "Owner", "PublicIP", "Type"}
		rows := []table.Row{}
		for _, net := range net {
			for _, subnet := range net.Subnets {
				ntype := "manual"
				if subnet.IsAerolabManaged {
					ntype = "aerolab"
				} else if subnet.IsDefault {
					ntype = "default"
				}
				rows = append(rows, table.Row{net.BackendType, net.Name, net.NetworkId, subnet.Name, subnet.SubnetId, subnet.Cidr, subnet.Owner, subnet.PublicIP, ntype})
			}
		}
		t, err := printer.GetTableWriter(output, tableTheme, sortBy)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Println(t.RenderTable(printer.String("NETWORKS"), header, rows))
	}
	return nil
}

func ListSecurityGroups(system *System, output string, tableTheme string, sortBy []string, backendType string, cmd []string, c interface{}, args []string, inventory *backends.Inventory) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != backendType {
		return errors.New("this command is only available for AWS/GCP backend types; selected backend does not match command constraints")
	}
	inventory = system.Backend.GetInventory()
	fw := inventory.Firewalls.Describe()

	switch output {
	case "json":
		json.NewEncoder(os.Stdout).Encode(fw)
	case "json-indent":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(fw)
	case "text":
		system.Logger.Info("Firewalls:")
		for _, fw := range fw {
			ports := []string{}
			targets := []string{}
			for _, port := range fw.Ports {
				source := port.Port.SourceCidr
				if port.Port.SourceId != "" {
					source = port.Port.SourceId
				}
				switch v := port.BackendSpecific.(type) {
				case *bgcp.PortDetail:
					targets = append(targets, v.TargetTags...)
					targets = append(targets, v.DestinationRanges...)
				}
				ports = append(ports, fmt.Sprintf("%s->%d:%d", source, port.Port.FromPort, port.Port.ToPort))
			}
			fmt.Printf("Backend: %s, Name: %s, ID: %s, Ports: %v, Targets: %v, Owner: %s, Zone: %s, Network: %s, NetworkID: %s\n",
				fw.BackendType, fw.Name, fw.FirewallID, ports, targets, fw.Owner, fw.ZoneName, fw.Network.Name, fw.Network.NetworkId)
		}
	default:
		header := table.Row{"Backend", "Name", "Ports", "Targets", "Owner", "Zone", "FwID", "Network", "NetworkID"}
		rows := []table.Row{}
		for _, fw := range fw {
			ports := []string{}
			targets := []string{}
			for _, port := range fw.Ports {
				source := port.Port.SourceCidr
				if port.Port.SourceId != "" {
					source = port.Port.SourceId
				}
				switch v := port.BackendSpecific.(type) {
				case *bgcp.PortDetail:
					targets = append(targets, v.TargetTags...)
					targets = append(targets, v.DestinationRanges...)
				}
				ports = append(ports, fmt.Sprintf("%s->%d:%d", source, port.Port.FromPort, port.Port.ToPort))
			}
			rows = append(rows, table.Row{fw.BackendType, fw.Name, strings.Join(ports, "\n"), strings.Join(targets, "\n"), fw.Owner, fw.ZoneName, fw.FirewallID, fw.Network.Name, fw.Network.NetworkId})
		}
		t, err := printer.GetTableWriter(output, tableTheme, sortBy)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Println(t.RenderTable(printer.String("FIREWALLS"), header, rows))
	}
	return nil
}

func CreateSecurityGroups(system *System, namePrefix string, ip string, portList []string, vpc string, backendType string, cmd []string, c interface{}, args []string, inventory *backends.Inventory) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != backendType {
		return errors.New("this command is only available for AWS/GCP backend types; selected backend does not match command constraints")
	}
	inv := system.Backend.GetInventory()
	var net backends.Networks
	if vpc != "" {
		net = inv.Networks.WithName(vpc)
	} else {
		net = inv.Networks.WithDefault(true)
	}
	if net == nil || net.Count() == 0 {
		return errors.New("no network found")
	}

	ports := []*backends.Port{}
	for _, port := range portList {
		protocol, from, to, err := parsePortRange(port)
		if err != nil {
			return err
		}
		ports = append(ports, &backends.Port{
			FromPort:   from,
			ToPort:     to,
			SourceCidr: ip,
			SourceId:   "",
			Protocol:   protocol,
		})
	}
	_, err := system.Backend.CreateFirewall(&backends.CreateFirewallInput{
		BackendType: backends.BackendType(system.Opts.Config.Backend.Type),
		Name:        namePrefix,
		Description: "AeroLab-managed security group",
		Owner:       GetCurrentOwnerUser(),
		Ports:       ports,
		Network:     net.Describe()[0],
	}, time.Minute)
	return err
}

func DeleteSecurityGroups(system *System, namePrefix string, all bool, backendType string, cmd []string, c interface{}, args []string, inventory *backends.Inventory) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != backendType {
		return errors.New("this command is only available for AWS/GCP backend types; selected backend does not match command constraints")
	}
	inv := system.Backend.GetInventory()
	var fw backends.Firewalls
	if all {
		fw = inv.Firewalls
	} else {
		fw = inv.Firewalls.WithName(namePrefix)
	}
	if fw == nil || fw.Count() == 0 {
		return errors.New("no security groups found")
	}
	return fw.Delete(time.Minute)
}

func LockSecurityGroups(system *System, namePrefix string, ip string, portList []string, backendType string, cmd []string, c interface{}, args []string, inventory *backends.Inventory) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != backendType {
		return errors.New("this command is only available for AWS/GCP backend types; selected backend does not match command constraints")
	}
	inv := system.Backend.GetInventory()
	fw := inv.Firewalls.WithName(namePrefix)
	if fw == nil || fw.Count() == 0 {
		return errors.New("no security groups found")
	}
	ports := backends.PortsIn{}
	for _, port := range portList {
		protocol, from, to, err := parsePortRange(port)
		if err != nil {
			return err
		}
		ports = append(ports, &backends.PortIn{
			Port: backends.Port{
				FromPort:   from,
				ToPort:     to,
				SourceCidr: ip,
				SourceId:   "",
				Protocol:   protocol,
			},
		})
	}
	return fw.Update(ports, time.Minute)
}

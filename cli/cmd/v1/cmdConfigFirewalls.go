package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/citrusleaf/aerolab/pkg/backend/backends"
	"github.com/citrusleaf/aerolab/pkg/backend/clouds/baws"
	"github.com/citrusleaf/aerolab/pkg/backend/clouds/bgcp"
	"github.com/citrusleaf/aerolab/pkg/utils/pager"
	"github.com/citrusleaf/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

// defaultFirewallNamePrefix is the value the security group commands default
// their --name to. Left at the default, the lock command works on the caller's
// own per-user groups instead.
const defaultFirewallNamePrefix = "AeroLab"

// firewallRoleTagKey returns the tag key each backend records a firewall's
// role under.
func firewallRoleTagKey(backendType string) string {
	if backendType == "gcp" {
		return bgcp.TAG_FIREWALL_ROLE
	}
	return baws.TAG_FIREWALL_ROLE
}

func ListSubnets(system *System, output string, tableTheme string, sortBy []string, backendType string, cmd []string, c any, args []string, inventory *backends.Inventory, out io.Writer, usePager bool, page *pager.Pager) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	if backendType == "docker" {
		docker := &ListNetworksCmd{
			Output:     output,
			SortBy:     sortBy,
			TableTheme: tableTheme,
			Pager:      usePager,
		}
		err := docker.ListNetworks(system, system.Backend.GetInventory(), args, out, nil)
		if err != nil {
			return err
		}
		return nil
	}
	if system.Opts.Config.Backend.Type != backendType {
		return errors.New("selected backend does not match command constraints")
	}
	inventory = system.Backend.GetInventory()
	net := inventory.Networks.Describe()

	if usePager && page == nil {
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
	switch output {
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
			enc.Encode(net) //nolint:errcheck
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(net) //nolint:errcheck
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(net) //nolint:errcheck
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
				fmt.Fprintf(out, "Backend: %s, Network: %s, NetID: %s, Subnet: %s, SubnetID: %s, CIDR: %s, Owner: %s, PublicIP: %t, Type: %s\n", //nolint:errcheck
					net.BackendType, net.Name, net.NetworkId, subnet.Name, subnet.SubnetId, subnet.Cidr, subnet.Owner, subnet.PublicIP, ntype)
			}
		}
		fmt.Fprintln(out, "") //nolint:errcheck
	default:
		if len(sortBy) == 0 {
			sortBy = []string{"Backend:asc", "Network:asc", "NetID:asc", "Subnet:asc", "SubnetID:asc"}
		}
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
		t, err := printer.GetTableWriter(output, tableTheme, sortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Fprintln(out, t.RenderTable(new("NETWORKS"), header, rows)) //nolint:errcheck
		fmt.Fprintln(out, "")                                           //nolint:errcheck
	}
	return nil
}

func ListSecurityGroups(system *System, output string, tableTheme string, sortBy []string, backendType string, cmd []string, c any, args []string, inventory *backends.Inventory, owner string, out io.Writer, usePager bool, page *pager.Pager) error {
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
	if backendType == "docker" {
		return nil
	}
	inventory = system.Backend.GetInventory()
	fw := inventory.Firewalls.Describe()

	if owner != "" {
		fw = fw.WithOwner(owner).Describe()
	}

	if usePager && page == nil {
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
	switch output {
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
			enc.Encode(fw) //nolint:errcheck
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(fw) //nolint:errcheck
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(fw) //nolint:errcheck
	case "text":
		system.Logger.Info("Firewalls:")
		for _, fw := range fw {
			ports := []string{}
			targets := []string{}
			for _, port := range fw.Ports {
				source := port.SourceCidr
				if port.SourceId != "" {
					source = port.SourceId
				}
				if tags, dests := firewallGcpPortDetails(port.BackendSpecific); tags != nil || dests != nil {
					targets = append(targets, tags...)
					targets = append(targets, dests...)
				}
				ports = append(ports, fmt.Sprintf("%s->%d:%d", source, port.FromPort, port.ToPort))
			}
			fmt.Fprintf(out, "Backend: %s, Name: %s, ID: %s, Ports: %v, Targets: %v, Owner: %s, Zone: %s, Network: %s, NetworkID: %s\n", //nolint:errcheck
				fw.BackendType, fw.Name, fw.FirewallID, ports, targets, fw.Owner, fw.ZoneName, fw.Network.Name, fw.Network.NetworkId)
		}
		fmt.Fprintln(out, "") //nolint:errcheck
	default:
		if len(sortBy) == 0 {
			sortBy = []string{"Backend:asc", "Name:asc"}
		}
		header := table.Row{"Backend", "Name", "Ports", "Targets", "Owner", "Zone", "FwID", "Network", "NetworkID"}
		rows := []table.Row{}
		for _, fw := range fw {
			ports := []string{}
			targets := []string{}
			for _, port := range fw.Ports {
				source := port.SourceCidr
				if port.SourceId != "" {
					source = port.SourceId
				}
				if tags, dests := firewallGcpPortDetails(port.BackendSpecific); tags != nil || dests != nil {
					targets = append(targets, tags...)
					targets = append(targets, dests...)
				}
				ports = append(ports, fmt.Sprintf("%s->%d:%d", source, port.FromPort, port.ToPort))
			}
			rows = append(rows, table.Row{fw.BackendType, fw.Name, strings.Join(ports, "\n"), strings.Join(targets, "\n"), fw.Owner, fw.ZoneName, fw.FirewallID, fw.Network.Name, fw.Network.NetworkId})
		}
		t, err := printer.GetTableWriter(output, tableTheme, sortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Fprintln(out, t.RenderTable(new("FIREWALLS"), header, rows)) //nolint:errcheck
		fmt.Fprintln(out, "")                                            //nolint:errcheck
	}
	return nil
}

func CreateSecurityGroups(system *System, namePrefix string, ips []string, portList []string, vpc string, backendType string, cmd []string, c any, args []string, inventory *backends.Inventory) error {
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
		for _, ip := range ips {
			ports = append(ports, &backends.Port{
				FromPort:   from,
				ToPort:     to,
				SourceCidr: ip,
				SourceId:   "",
				Protocol:   protocol,
			})
		}
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

func DeleteSecurityGroups(system *System, namePrefix string, all bool, backendType string, cmd []string, c any, args []string, inventory *backends.Inventory) error {
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

// LockSecurityGroups restricts the given ports of the named security groups to
// the given source CIDRs, revoking whatever else those ports allowed. With no
// name it locks the caller's own per-user groups, and with no ports it locks
// all protocols, matching the per-user autolock default.
func LockSecurityGroups(system *System, namePrefix string, ips []string, portList []string, backendType string, cmd []string, c any, args []string, inventory *backends.Inventory) error {
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
	if len(ips) == 0 {
		return errors.New("no source address to lock the security groups to")
	}
	inv := system.Backend.GetInventory()
	var fw backends.FirewallList
	if namePrefix != "" && namePrefix != defaultFirewallNamePrefix {
		fw = inv.Firewalls.WithName(namePrefix).Describe()
	} else {
		fw = callerDefaultFirewalls(inv, backendType)
		if len(fw) == 0 {
			// Nothing of ours to lock; fall back to the literal name so an
			// explicit '-n AeroLab' still works.
			fw = inv.Firewalls.WithName(namePrefix).Describe()
		}
	}
	if len(fw) == 0 {
		return errors.New("no security groups found")
	}
	if len(portList) == 0 {
		portList = []string{"all"}
	}

	ports := backends.PortsIn{}
	for _, port := range portList {
		protocol, from, to, err := parsePortRange(port)
		if err != nil {
			return err
		}
		// Drop whatever these ports allow today, other than the addresses we
		// are about to (re)authorise. Protocol-all is compared in canonical
		// form so AWS's omitted (zero) ports still match FromPort -1.
		want := backends.CanonicalCallerRule(backends.CallerRule{Protocol: protocol, FromPort: from, ToPort: to})
		for _, group := range fw {
			for _, existing := range group.Ports {
				if existing.SourceCidr == "" {
					continue
				}
				have := backends.CanonicalCallerRule(backends.CallerRule{
					Protocol: existing.Protocol,
					FromPort: existing.FromPort,
					ToPort:   existing.ToPort,
				})
				if have.Protocol != want.Protocol || have.FromPort != want.FromPort || have.ToPort != want.ToPort {
					continue
				}
				if slices.Contains(ips, existing.SourceCidr) || hasPortIn(ports, existing.Protocol, existing.FromPort, existing.ToPort, existing.SourceCidr) {
					continue
				}
				ports = append(ports, &backends.PortIn{
					Port: backends.Port{
						FromPort:   existing.FromPort,
						ToPort:     existing.ToPort,
						SourceCidr: existing.SourceCidr,
						Protocol:   existing.Protocol,
					},
					Action: backends.PortActionDelete,
				})
			}
		}
		for _, ip := range ips {
			ports = append(ports, &backends.PortIn{
				Port: backends.Port{
					FromPort:   from,
					ToPort:     to,
					SourceCidr: ip,
					SourceId:   "",
					Protocol:   protocol,
				},
				Action: backends.PortActionAdd,
			})
		}
	}
	for _, group := range fw {
		system.Logger.Info("Locking %s ports %s to %s", group.Name, strings.Join(portList, ","), strings.Join(ips, ","))
	}
	return fw.Update(ports, time.Minute)
}

// hasPortIn reports whether a rule for this protocol, port range and source is
// already in the update.
func hasPortIn(ports backends.PortsIn, protocol string, from int, to int, cidr string) bool {
	for _, port := range ports {
		if port.Protocol == protocol && port.FromPort == from && port.ToPort == to && port.SourceCidr == cidr {
			return true
		}
	}
	return false
}

// callerDefaultFirewalls returns the per-user groups AeroLab manages for the
// current user.
func callerDefaultFirewalls(inv *backends.Inventory, backendType string) backends.FirewallList {
	roleTag := firewallRoleTagKey(backendType)
	sanitize := sanitizeOwnerFor(backendType)
	owner := sanitize(GetCurrentOwnerUser())
	out := backends.FirewallList{}
	for _, fw := range inv.Firewalls.Describe() {
		if fw.Tags[roleTag] != backends.FirewallRoleDefault {
			continue
		}
		if sanitize(fw.Owner) != owner {
			continue
		}
		out = append(out, fw)
	}
	return out
}

// sanitizeOwnerFor returns the way the given backend folds a username, so a
// group tagged 'firstlast' is still recognised as belonging to 'First.Last'.
func sanitizeOwnerFor(backendType string) func(string) string {
	if backendType == "gcp" {
		return bgcp.SanitizeOwner
	}
	return baws.SanitizeOwner
}

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bdocker"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

type ConfigDockerCmd struct {
	CreateNetwork CreateNetworkCmd `command:"create-network" subcommands-optional:"true" description:"create a new docker network" webicon:"fas fa-circle-plus" invwebforce:"true"`
	DeleteNetwork DeleteNetworkCmd `command:"delete-network" subcommands-optional:"true" description:"delete a docker network" webicon:"fas fa-trash" invwebforce:"true"`
	ListNetworks  ListNetworksCmd  `command:"list-networks" subcommands-optional:"true" description:"list docker networks" webicon:"fas fa-list"`
	PruneNetworks PruneNetworksCmd `command:"prune-networks" subcommands-optional:"true" description:"remove unused docker networks" webicon:"fas fa-dumpster" invwebforce:"true"`
	Help          HelpCmd          `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigDockerCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

type PruneNetworksCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ListNetworksCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
	CSV        bool     `short:"c" long:"csv" hidden:"true" webhidden:"true" description:"this no longer applies"` // NOTE: obsolete, but kept for backwards compatibility
}

type DeleteNetworkCmd struct {
	Name string  `short:"n" long:"name" description:"network name to delete" default:""`
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type CreateNetworkCmd struct {
	Name   string  `short:"n" long:"name" description:"network name to create" default:""`
	Driver string  `short:"d" long:"driver" description:"network driver" default:"bridge"`
	Subnet string  `short:"s" long:"subnet" description:"network subnet to create, ex. 172.18.0.0/24 or 172.18.1.0/24" default:"default"`
	MTU    string  `short:"m" long:"mtu" description:"MTU in the network" default:"default"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *PruneNetworksCmd) Execute(args []string) error {
	cmd := []string{"config", "docker", "prune-networks"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.PruneNetworks(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *PruneNetworksCmd) PruneNetworks(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"config", "docker", "prune-networks"}, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != "docker" {
		return errors.New("this function is only available for docker backend")
	}
	return system.Backend.DockerPruneNetworks("")
}

func (c *DeleteNetworkCmd) Execute(args []string) error {
	cmd := []string{"config", "docker", "delete-network"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.DeleteNetwork(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *DeleteNetworkCmd) DeleteNetwork(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"config", "docker", "delete-network"}, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != "docker" {
		return errors.New("this function is only available for docker backend")
	}
	return system.Backend.DockerDeleteNetwork("", c.Name)
}

func (c *CreateNetworkCmd) Execute(args []string) error {
	cmd := []string{"config", "docker", "create-network"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.CreateNetwork(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *CreateNetworkCmd) CreateNetwork(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"config", "docker", "create-network"}, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != "docker" {
		return errors.New("this function is only available for docker backend")
	}
	if c.Name == "" {
		return errors.New("name must be specified")
	}
	if c.Subnet == "default" {
		c.Subnet = ""
	}
	if c.MTU == "default" {
		c.MTU = ""
	}
	return system.Backend.DockerCreateNetwork("", c.Name, c.Driver, c.Subnet, c.MTU)
}

func (c *ListNetworksCmd) Execute(args []string) error {
	cmd := []string{"config", "docker", "list-networks"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ListNetworks(system, system.Backend.GetInventory(), args, os.Stdout, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ListNetworksCmd) ListNetworks(system *System, inventory *backends.Inventory, args []string, out io.Writer, page *pager.Pager) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"config", "docker", "list-networks"}, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != "docker" {
		return errors.New("this function is only available for docker backend")
	}
	if c.CSV {
		c.Output = "csv"
	}
	inventory = system.Backend.GetInventory()
	net := inventory.Networks.Describe()

	var err error
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
			enc.Encode(net)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(net)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(net)
	case "text":
		system.Logger.Info("Networks:")
		for _, net := range net {
			detail := net.BackendSpecific.(*bdocker.NetworkDetails)
			fmt.Fprintf(out, "Backend: %s, Name: %s, CIDR: %s, Driver: %s, MTU: %s, NetID: %s\n",
				net.BackendType, net.Name, net.Cidr, detail.Driver, detail.Options["com.docker.network.driver.mtu"], net.NetworkId)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Backend:asc", "Driver:asc", "Name:asc", "CIDR:asc"}
		}
		header := table.Row{"Backend", "Name", "CIDR", "Driver", "MTU", "NetID"}
		rows := []table.Row{}
		for _, net := range net {
			detail := net.BackendSpecific.(*bdocker.NetworkDetails)
			rows = append(rows, table.Row{net.BackendType, net.Name, net.Cidr, detail.Driver, detail.Options["com.docker.network.driver.mtu"], net.NetworkId})
		}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), c.Pager)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("NETWORKS"), header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

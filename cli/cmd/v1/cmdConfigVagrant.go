package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bvagrant"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

// vagrantBoxBackend is the subset of the vagrant Cloud implementation's methods needed by
// this file's commands. Declared locally (rather than added to backends.Cloud) because
// box listing/removal and preflight reporting are CLI-only surface backed by types
// specific to the bvagrant package.
type vagrantBoxBackend interface {
	ListBoxes() ([]bvagrant.BoxInfo, error)
	RemoveBox(name string) error
	PreflightCheck(force bool) (*bvagrant.PreflightInfo, error)
}

func getVagrantBackend(system *System) (vagrantBoxBackend, error) {
	if system.Opts.Config.Backend.Type != "vagrant" {
		return nil, errors.New("this function is only available for the vagrant backend")
	}
	cloud, ok := backends.LookupBackend(backends.BackendTypeVagrant)
	if !ok {
		return nil, errors.New("vagrant backend is not available in this build")
	}
	vb, ok := cloud.(vagrantBoxBackend)
	if !ok {
		return nil, errors.New("vagrant backend does not support this operation")
	}
	return vb, nil
}

type ConfigVagrantCmd struct {
	ListBoxes VagrantListBoxesCmd `command:"list-boxes" subcommands-optional:"true" description:"list vagrant boxes registered with the local vagrant install" webicon:"fas fa-list"`
	DeleteBox VagrantDeleteBoxCmd `command:"delete-box" subcommands-optional:"true" description:"remove a vagrant box from the local vagrant install" webicon:"fas fa-trash" invwebforce:"true"`
	Check     VagrantCheckCmd     `command:"check" subcommands-optional:"true" description:"check the local vagrant environment (binary, version, plugins, providers)" webicon:"fas fa-stethoscope"`
	Help      HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigVagrantCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type VagrantListBoxesCmd struct {
	Output string  `short:"o" long:"output" description:"Output format (text, table, json, json-indent)" default:"table"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *VagrantListBoxesCmd) Execute(args []string) error {
	cmd := []string{"config", "vagrant", "list-boxes"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	vb, err := getVagrantBackend(system)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	boxes, err := vb.ListBoxes()
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	switch c.Output {
	case "json":
		//nolint:errcheck
		json.NewEncoder(os.Stdout).Encode(boxes)
	case "json-indent":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		//nolint:errcheck
		enc.Encode(boxes)
	case "text":
		for _, b := range boxes {
			fmt.Printf("Name: %s, Provider: %s, Version: %s\n", b.Name, b.Provider, b.Version)
		}
	default:
		header := table.Row{"Name", "Provider", "Version"}
		rows := []table.Row{}
		for _, b := range boxes {
			rows = append(rows, table.Row{b.Name, b.Provider, b.Version})
		}
		t, err := printer.GetTableWriter("table", "default", nil, false, false)
		if err != nil && err != printer.ErrTerminalWidthUnknown {
			return Error(err, system, cmd, c, args)
		}
		fmt.Println(t.RenderTable(new("VAGRANT BOXES"), header, rows))
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

type VagrantDeleteBoxCmd struct {
	Name string  `short:"n" long:"name" description:"box name to remove" default:""`
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *VagrantDeleteBoxCmd) Execute(args []string) error {
	cmd := []string{"config", "vagrant", "delete-box"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	if c.Name == "" {
		return Error(errors.New("name must be specified"), system, cmd, c, args)
	}
	vb, err := getVagrantBackend(system)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	if err := vb.RemoveBox(c.Name); err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

type VagrantCheckCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *VagrantCheckCmd) Execute(args []string) error {
	cmd := []string{"config", "vagrant", "check"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	vb, err := getVagrantBackend(system)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	info, err := vb.PreflightCheck(true)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	fmt.Printf("Vagrant version: %s\n", info.VagrantVersion)
	fmt.Printf("Plugins: %s\n", strings.Join(info.Plugins, ", "))
	fmt.Printf("Providers: %s\n", strings.Join(info.Providers, ", "))
	fmt.Printf("OK: %v\n", info.OK)
	if len(info.Issues) > 0 {
		fmt.Println("Issues:")
		for _, issue := range info.Issues {
			fmt.Printf("  - %s\n", issue)
		}
	}

	if !info.OK {
		return Error(errors.New("vagrant environment check failed"), system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

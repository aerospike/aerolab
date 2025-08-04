package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

type ExpiryInstallCmd struct {
	Frequency  int  `short:"f" long:"frequency" description:"Scheduler frequency in minutes" default:"15"`
	CleanupDNS bool `short:"c" long:"cleanup-dns" description:"Cleanup DNS records"`
	ExpireEks  bool `short:"e" long:"expire-eks" description:"Expire EKS clusters"`
	Force      bool `long:"force" description:"Force the installation even if latest expiry is already installed"`
	//Gcp        ExpiryInstallCmdGcp `group:"GCP Backend" description:"backend-gcp"`
	//Aws        ExpiryInstallCmdAws `group:"AWS Backend" description:"backend-aws"`
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ExpiryInstallCmd) Execute(args []string) error {
	cmd := []string{"config", "expiry-install"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ExpiryInstall(system, cmd, args, system.Backend.GetInventory())
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ExpiryInstallCmd) ExpiryInstall(system *System, cmd []string, args []string, inventory *backends.Inventory) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	regions, err := system.Backend.ListEnabledRegions(backends.BackendType(system.Opts.Config.Backend.Type))
	if err != nil {
		return err
	}
	return system.Backend.ExpiryInstall(backends.BackendType(system.Opts.Config.Backend.Type), c.Frequency, 4, c.ExpireEks, c.CleanupDNS, c.Force, false, regions...)
}

type ExpiryRemoveCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ExpiryRemoveCmd) Execute(args []string) error {
	cmd := []string{"config", "expiry-remove"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ExpiryRemove(system, cmd, args, system.Backend.GetInventory())
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ExpiryRemoveCmd) ExpiryRemove(system *System, cmd []string, args []string, inventory *backends.Inventory) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	regions, err := system.Backend.ListEnabledRegions(backends.BackendType(system.Opts.Config.Backend.Type))
	if err != nil {
		return err
	}
	return system.Backend.ExpiryRemove(backends.BackendType(system.Opts.Config.Backend.Type), regions...)
}

type ExpiryCheckFreqCmd struct {
	Frequency int     `short:"f" long:"frequency" description:"Scheduler frequency in minutes" default:"15"`
	Help      HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ExpiryCheckFreqCmd) Execute(args []string) error {
	cmd := []string{"config", "expiry-check-freq"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ExpiryCheckFreq(system, cmd, args, system.Backend.GetInventory())
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ExpiryCheckFreqCmd) ExpiryCheckFreq(system *System, cmd []string, args []string, inventory *backends.Inventory) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	regions, err := system.Backend.ListEnabledRegions(backends.BackendType(system.Opts.Config.Backend.Type))
	if err != nil {
		return err
	}
	return system.Backend.ExpiryChangeFrequency(backends.BackendType(system.Opts.Config.Backend.Type), c.Frequency, regions...)
}

type ExpiryListCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ExpiryListCmd) Execute(args []string) error {
	cmd := []string{"config", "expiry-list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ExpiryList(system, cmd, args, system.Backend.GetInventory())
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ExpiryListCmd) ExpiryList(system *System, cmd []string, args []string, inventory *backends.Inventory) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	expiries, err := system.Backend.ExpiryList()
	if err != nil {
		return err
	}
	switch c.Output {
	case "json":
		json.NewEncoder(os.Stdout).Encode(expiries)
	case "json-indent":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(expiries)
	case "text":
		system.Logger.Info("Expiries:")
		for _, expiry := range expiries.ExpirySystems {
			fmt.Printf("Backend: %s, Version: %s, Zone: %s, InstallationSuccess: %t, FrequencyMinutes: %d\n", expiry.BackendType, expiry.Version, expiry.Zone, expiry.InstallationSuccess, expiry.FrequencyMinutes)
		}
	default:
		header := table.Row{"Backend", "Version", "Zone", "InstallationSuccess", "FrequencyMinutes"}
		rows := []table.Row{}
		for _, expiry := range expiries.ExpirySystems {
			rows = append(rows, table.Row{expiry.BackendType, expiry.Version, expiry.Zone, expiry.InstallationSuccess, expiry.FrequencyMinutes})
		}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Println(t.RenderTable(printer.String("EXPIRIES"), header, rows))
	}
	return nil
}

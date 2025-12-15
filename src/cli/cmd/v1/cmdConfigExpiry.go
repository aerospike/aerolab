package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rglonek/logger"
)

// v7ExpiryCheckCache caches v7 expiry check results to avoid repeated API calls
var v7ExpiryCheckCache = struct {
	sync.RWMutex
	checked map[backends.BackendType]bool
}{
	checked: make(map[backends.BackendType]bool),
}

// warnIfV7ExpiryInstalled checks if v7 expiry system is still installed and warns the user.
// This is called during cluster/instance/client/volume creation and expiry-related commands.
// The warning is only shown once per backend type per session.
func warnIfV7ExpiryInstalled(backend backends.Backend, backendType backends.BackendType, log *logger.Logger) {
	// Skip for Docker - v7 never had Docker expiry
	if backendType == backends.BackendTypeDocker {
		return
	}

	// Check cache to avoid repeated warnings
	v7ExpiryCheckCache.RLock()
	if v7ExpiryCheckCache.checked[backendType] {
		v7ExpiryCheckCache.RUnlock()
		return
	}
	v7ExpiryCheckCache.RUnlock()

	// Mark as checked before performing the check
	v7ExpiryCheckCache.Lock()
	// Double-check after acquiring write lock
	if v7ExpiryCheckCache.checked[backendType] {
		v7ExpiryCheckCache.Unlock()
		return
	}
	v7ExpiryCheckCache.checked[backendType] = true
	v7ExpiryCheckCache.Unlock()

	// Perform the check
	found, regions, err := backend.ExpiryV7Check(backendType)
	if err != nil {
		log.Debug("Error checking for v7 expiry system: %s", err)
		return
	}

	if found {
		log.Warn("=======================================================================")
		log.Warn("DETECTED: AeroLab v7 expiry system is still installed!")
		log.Warn("Regions with v7 expiry: %s", strings.Join(regions, ", "))
		log.Warn("")
		log.Warn("The v7 expiry system runs alongside v8.")
		log.Warn("If you are not using aerolab v7 anymore, you can remove the v7 expiry system.")
		log.Warn("")
		log.Warn("See https://github.com/aerospike/aerolab/blob/v8.0.0/docs/migration-expiry-system.md")
		log.Warn("=======================================================================")
	}
}

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

	defer UpdateDiskCache(system)
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
	// Check for v7 expiry system and warn user
	warnIfV7ExpiryInstalled(system.Backend, backends.BackendType(system.Opts.Config.Backend.Type), system.Logger)

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

	defer UpdateDiskCache(system)
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

	defer UpdateDiskCache(system)
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
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ExpiryListCmd) Execute(args []string) error {
	cmd := []string{"config", "expiry-list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ExpiryList(system, cmd, args, system.Backend.GetInventory(), os.Stdout, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ExpiryListCmd) ExpiryList(system *System, cmd []string, args []string, inventory *backends.Inventory, out io.Writer, page *pager.Pager) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: false, ExistingInventory: inventory}, cmd, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type == "docker" {
		return nil
	}
	// Check for v7 expiry system and warn user
	warnIfV7ExpiryInstalled(system.Backend, backends.BackendType(system.Opts.Config.Backend.Type), system.Logger)

	expiries, err := system.Backend.ExpiryList()
	if err != nil {
		return err
	}
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
			enc.Encode(expiries)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(expiries)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(expiries)
	case "text":
		system.Logger.Info("Expiries:")
		for _, expiry := range expiries.ExpirySystems {
			fmt.Fprintf(out, "Backend: %s, Version: %s, Zone: %s, InstallationSuccess: %t, FrequencyMinutes: %d\n", expiry.BackendType, expiry.Version, expiry.Zone, expiry.InstallationSuccess, expiry.FrequencyMinutes)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Backend:asc", "Zone:asc"}
		}
		header := table.Row{"Backend", "Version", "Zone", "InstallationSuccess", "FrequencyMinutes"}
		rows := []table.Row{}
		for _, expiry := range expiries.ExpirySystems {
			rows = append(rows, table.Row{expiry.BackendType, expiry.Version, expiry.Zone, expiry.InstallationSuccess, expiry.FrequencyMinutes})
		}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("EXPIRIES"), header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

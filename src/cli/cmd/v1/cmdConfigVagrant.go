package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

type ConfigVagrantCmd struct {
	ListBoxes     ListVagrantBoxesCmd     `command:"list-boxes" subcommands-optional:"true" description:"list locally available Vagrant boxes" webicon:"fas fa-list"`
	CheckProvider CheckVagrantProviderCmd `command:"check-provider" subcommands-optional:"true" description:"check if the configured provider is available" webicon:"fas fa-check-circle"`
	ListProviders ListVagrantProvidersCmd `command:"list-providers" subcommands-optional:"true" description:"list available Vagrant providers on this system" webicon:"fas fa-list-check"`
	Help          HelpCmd                 `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigVagrantCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

type ListVagrantBoxesCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type CheckVagrantProviderCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ListVagrantProvidersCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ListVagrantBoxesCmd) Execute(args []string) error {
	cmd := []string{"config", "vagrant", "list-boxes"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ListBoxes(system, args, os.Stdout, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ListVagrantBoxesCmd) ListBoxes(system *System, args []string, out io.Writer, page *pager.Pager) error {
	type BoxInfo struct {
		Name     string `json:"name"`
		Provider string `json:"provider"`
		Version  string `json:"version"`
	}

	// Run vagrant box list
	vagrantCmd := exec.Command("vagrant", "box", "list")
	output, err := vagrantCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list vagrant boxes: %w (output: %s)", err, string(output))
	}

	// Parse output
	boxes := []BoxInfo{}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: boxname (provider, version)
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		providerAndVersion := strings.Join(parts[1:], " ")
		providerAndVersion = strings.Trim(providerAndVersion, "()")
		pvParts := strings.Split(providerAndVersion, ",")
		provider := strings.TrimSpace(pvParts[0])
		version := ""
		if len(pvParts) > 1 {
			version = strings.TrimSpace(pvParts[1])
		}
		boxes = append(boxes, BoxInfo{
			Name:     name,
			Provider: provider,
			Version:  version,
		})
	}

	var outputErr error
	if c.Pager && page == nil {
		page, outputErr = pager.New(out)
		if outputErr != nil {
			return outputErr
		}
		outputErr = page.Start()
		if outputErr != nil {
			return outputErr
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
		jqCmd := exec.Command("jq", params...)
		jqCmd.Stdout = out
		jqCmd.Stderr = out
		w, err := jqCmd.StdinPipe()
		if err != nil {
			return err
		}
		defer w.Close()
		enc := json.NewEncoder(w)
		go func() {
			enc.Encode(boxes)
			w.Close()
		}()
		err = jqCmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(boxes)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(boxes)
	case "text":
		fmt.Fprintf(out, "Vagrant Boxes:\n")
		for _, box := range boxes {
			fmt.Fprintf(out, "  Name: %s, Provider: %s, Version: %s\n", box.Name, box.Provider, box.Version)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Name:asc", "Provider:asc", "Version:asc"}
		}
		header := table.Row{"Name", "Provider", "Version"}
		rows := []table.Row{}
		for _, box := range boxes {
			rows = append(rows, table.Row{box.Name, box.Provider, box.Version})
		}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("VAGRANT BOXES"), header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

func (c *CheckVagrantProviderCmd) Execute(args []string) error {
	cmd := []string{"config", "vagrant", "check-provider"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.CheckProvider(system, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *CheckVagrantProviderCmd) CheckProvider(system *System, args []string) error {
	if system.Opts.Config.Backend.Type != "vagrant" {
		return errors.New("this function is only available for vagrant backend")
	}

	provider := system.Opts.Config.Backend.VagrantProvider
	if provider == "" {
		provider = "virtualbox"
	}

	system.Logger.Info("Checking provider: %s", provider)

	// Check if vagrant is available
	vagrantCmd := exec.Command("vagrant", "--version")
	output, err := vagrantCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("vagrant command not found: %w (output: %s)", err, string(output))
	}
	fmt.Printf("Vagrant version: %s\n", strings.TrimSpace(string(output)))

	// Check provider-specific commands
	var checkCmd *exec.Cmd
	switch provider {
	case "virtualbox":
		checkCmd = exec.Command("VBoxManage", "--version")
	case "vmware_desktop", "vmware_fusion", "vmware_workstation":
		checkCmd = exec.Command("vmrun", "-v")
	case "libvirt":
		checkCmd = exec.Command("virsh", "--version")
	case "hyperv":
		// Hyper-V check is Windows-specific and complex
		system.Logger.Info("Hyper-V provider configured, skipping provider check (Windows only)")
		return nil
	case "parallels":
		checkCmd = exec.Command("prlctl", "--version")
	default:
		return fmt.Errorf("unknown provider: %s", provider)
	}

	if checkCmd != nil {
		output, err = checkCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("provider '%s' not found or not working: %w (output: %s)", provider, err, string(output))
		}
		fmt.Printf("Provider '%s' version: %s\n", provider, strings.TrimSpace(string(output)))
	}

	fmt.Printf("Provider '%s' is available and working\n", provider)
	return nil
}

func (c *ListVagrantProvidersCmd) Execute(args []string) error {
	cmd := []string{"config", "vagrant", "list-providers"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ListProviders(system, args, os.Stdout, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ListVagrantProvidersCmd) ListProviders(system *System, args []string, out io.Writer, page *pager.Pager) error {
	type ProviderInfo struct {
		Name      string `json:"name"`
		Available bool   `json:"available"`
		Version   string `json:"version"`
	}

	providers := []ProviderInfo{
		{Name: "virtualbox", Available: false, Version: ""},
		{Name: "vmware_desktop", Available: false, Version: ""},
		{Name: "libvirt", Available: false, Version: ""},
		{Name: "hyperv", Available: false, Version: ""},
		{Name: "parallels", Available: false, Version: ""},
	}

	// Check each provider
	for i, p := range providers {
		var checkCmd *exec.Cmd
		switch p.Name {
		case "virtualbox":
			checkCmd = exec.Command("VBoxManage", "--version")
		case "vmware_desktop":
			checkCmd = exec.Command("vmrun", "-v")
		case "libvirt":
			checkCmd = exec.Command("virsh", "--version")
		case "hyperv":
			// Skip complex Windows check
			continue
		case "parallels":
			checkCmd = exec.Command("prlctl", "--version")
		}

		if checkCmd != nil {
			output, err := checkCmd.CombinedOutput()
			if err == nil {
				providers[i].Available = true
				providers[i].Version = strings.TrimSpace(string(output))
			}
		}
	}

	var outputErr error
	if c.Pager && page == nil {
		page, outputErr = pager.New(out)
		if outputErr != nil {
			return outputErr
		}
		outputErr = page.Start()
		if outputErr != nil {
			return outputErr
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
		jqCmd := exec.Command("jq", params...)
		jqCmd.Stdout = out
		jqCmd.Stderr = out
		w, err := jqCmd.StdinPipe()
		if err != nil {
			return err
		}
		defer w.Close()
		enc := json.NewEncoder(w)
		go func() {
			enc.Encode(providers)
			w.Close()
		}()
		err = jqCmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(providers)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(providers)
	case "text":
		fmt.Fprintf(out, "Vagrant Providers:\n")
		for _, p := range providers {
			available := "No"
			if p.Available {
				available = "Yes"
			}
			fmt.Fprintf(out, "  Name: %s, Available: %s, Version: %s\n", p.Name, available, p.Version)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Name:asc"}
		}
		header := table.Row{"Name", "Available", "Version"}
		rows := []table.Row{}
		for _, p := range providers {
			available := "No"
			if p.Available {
				available = "Yes"
			}
			rows = append(rows, table.Row{p.Name, available, p.Version})
		}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("VAGRANT PROVIDERS"), header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}


package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

type ConfigEnvVarsCmd struct {
	Output     string  `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string  `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	Pager      bool    `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigEnvVarsCmd) Execute(args []string) error {
	cmd := []string{"config", "env-vars"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))
	c.PrintEnvVars(system, os.Stdout, nil)
	system.Logger.Info("Done")
	return nil
}

func (c *ConfigEnvVarsCmd) PrintEnvVars(system *System, out io.Writer, page *pager.Pager) error {
	var err error
	type envVar struct {
		Key         string
		Value       string
		Description string
	}
	envVars := []envVar{
		{Key: "AEROLAB_HOME", Value: os.Getenv("AEROLAB_HOME"), Description: "If set, will override the default ~/.config/aerolab home directory"},
		{Key: "AEROLAB_LOG_LEVEL", Value: os.Getenv("AEROLAB_LOG_LEVEL"), Description: "0=NONE,1=CRITICAL,2=ERROR,3=WARN,4=INFO,5=DEBUG,6=DETAIL"},
		{Key: "AEROLAB_PROJECT", Value: os.Getenv("AEROLAB_PROJECT"), Description: "Aerolab v8 has a notion of projects; setting this will make it work on resources other than in the 'default' aerolab project"},
		{Key: "AEROLAB_DISABLE_UPGRADE_CHECK", Value: os.Getenv("AEROLAB_DISABLE_UPGRADE_CHECK"), Description: "If set to a non-empty value, aerolab will not check if upgrades are available"},
		{Key: "AEROLAB_TELEMETRY_DISABLE", Value: os.Getenv("AEROLAB_TELEMETRY_DISABLE"), Description: "If set to a non-empty value, telemetry will be disabled"},
		{Key: "AEROLAB_CONFIG_FILE", Value: os.Getenv("AEROLAB_CONFIG_FILE"), Description: "If set, aerolab will read the given defaults config file instead of $AEROLAB_HOME/conf"},
		{Key: "AEROLAB_NONINTERACTIVE", Value: os.Getenv("AEROLAB_NONINTERACTIVE"), Description: "If set to a non-empty value, aerolab will not ask for confirmation or choices at any point"},
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
			enc.Encode(envVars)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(envVars)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(envVars)
	case "text":
		for _, envVar := range envVars {
			fmt.Fprintf(out, "%s=%s\n", envVar.Key, envVar.Value)
		}
		fmt.Fprintln(out, "")
	default:
		header := table.Row{"Key", "Value", "Description"}
		rows := []table.Row{}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, []string{}, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		for _, envVar := range envVars {
			rows = append(rows, table.Row{envVar.Key, envVar.Value, envVar.Description})
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("ENV VARS"), header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

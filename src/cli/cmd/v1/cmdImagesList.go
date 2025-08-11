package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

type ImagesListCmd struct {
	Output     string           `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string           `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string         `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool             `short:"p" long:"pager" description:"Use a pager to display the output"`
	Filters    ImagesListFilter `group:"Filters" namespace:"filter"`
	Help       HelpCmd          `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ImagesListFilter struct {
	Type  string `short:"T" long:"type" description:"Filter by type of image. Values: custom, public, all" default:"custom"`
	Owner string `short:"O" long:"owner" description:"Filter by owner of the image"`
}

func (c *ImagesListCmd) Execute(args []string) error {
	cmd := []string{"images", "list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ListImages(system, system.Backend.GetInventory(), args, os.Stdout, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ImagesListCmd) ListImages(system *System, inventory *backends.Inventory, args []string, out io.Writer, page *pager.Pager) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"images", "list"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	images := inventory.Images.Describe()

	if c.Filters.Owner != "" {
		images = images.WithOwner(c.Filters.Owner).Describe()
	}

	switch c.Filters.Type {
	case "custom":
		images = images.WithInAccount(true).Describe()
	case "public":
		images = images.WithInAccount(false).Describe()
	case "all":
		// do nothing
	default:
		return fmt.Errorf("invalid type: %s", c.Filters.Type)
	}

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
			enc.Encode(images)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(images)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(images)
	case "text":
		system.Logger.Info("Images:")
		for _, image := range images {
			fmt.Fprintf(out, "Backend: %s, Name: %s, ID: %s, Zone: %s, Public: %t, SizeGiB: %d, Owner: %s, Architecture: %s, OSName: %s, OSVersion: %s, Type: %s, Version: %s, Description: %s\n",
				image.BackendType, image.Name, image.ImageId, image.ZoneName, !image.InAccount, image.Size/1024/1024/1024, image.Owner, image.Architecture.String(), image.OSName, image.OSVersion, image.Tags["aerolab.image.type"], image.Tags["aerolab.soft.version"], image.Description)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Backend:asc", "Type:asc", "OSName:asc", "OSVersion:ascnum", "Architecture:asc", "Version:asc"}
		}
		header := table.Row{"Backend", "Name", "Zone", "Public", "SizeGiB", "Owner", "Architecture", "OSName", "OSVersion", "Type", "Version", "Description", "ID"}
		rows := []table.Row{}
		for _, image := range images {
			name := image.Name
			if name == "" {
				name = image.ImageId
			}
			rows = append(rows, table.Row{image.BackendType, name, image.ZoneName, !image.InAccount, image.Size / 1024 / 1024 / 1024, image.Owner, image.Architecture.String(), image.OSName, image.OSVersion, image.Tags["aerolab.image.type"], image.Tags["aerolab.soft.version"], image.Description, image.ImageId})
		}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("IMAGES"), header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

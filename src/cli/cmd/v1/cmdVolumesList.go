package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

type VolumesListCmd struct {
	Output     string            `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string            `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string          `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool              `short:"p" long:"pager" description:"Use a pager to display the output"`
	Filters    VolumesListFilter `group:"Filters" namespace:"filter"`
	Help       HelpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

type VolumesListFilter struct {
	Backend string   `short:"B" long:"backend" description:"Filter by backend type"`
	Name    string   `short:"N" long:"name" description:"Filter by name of the volume"`
	Owner   string   `short:"O" long:"owner" description:"Filter by owner of the volume"`
	Type    string   `short:"T" long:"type" description:"Filter by type of volume (shared/attached)"`
	Zone    string   `short:"Z" long:"zone" description:"Filter by zone of the volume (zone name)"`
	State   string   `short:"S" long:"state" description:"Filter by state of the volume (attached/detached); default: all"`
	Tags    []string `long:"tag" description:"Filter by tag of the volume, as k=v"`
}

func (c *VolumesListCmd) Execute(args []string) error {
	cmd := []string{"volumes", "list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ListVolumes(system, system.Backend.GetInventory(), args, os.Stdout, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *VolumesListCmd) ListVolumes(system *System, inventory *backends.Inventory, args []string, out io.Writer, page *pager.Pager) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "list"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	volumes, err := c.Filters.filter(inventory.Volumes.Describe(), false)
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
			enc.Encode(volumes)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(volumes)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(volumes)
	case "text":
		system.Logger.Info("Volumes:")
		for _, volume := range volumes {
			expiresIn := "NEVER"
			if !volume.Expires.IsZero() && volume.Expires.After(time.Now()) {
				expiresIn = time.Until(volume.Expires).String()
			}
			if !volume.Expires.IsZero() && volume.Expires.Before(time.Now()) {
				expiresIn = "expired"
			}
			fmt.Fprintf(out, "Backend: %s, Zone: %s, Name: %s, Type: %s, Size: %d, Expires: %s, State: %s, DeleteOnTermination: %t, Owner: %s, AttachedTo: %v, DiskType: %s, FileSystemId: %s, EstimatedCost: %0.2f, Iops: %d, Throughput: %d, Encrypted: %t, Tags: %v, Description: %s, CreationTime: %s\n",
				volume.BackendType,
				volume.ZoneName,
				volume.Name,
				volume.VolumeType.String(),
				volume.Size/backends.StorageGiB,
				expiresIn,
				volume.State.String(),
				volume.DeleteOnTermination,
				volume.Owner,
				volume.AttachedTo,
				volume.DiskType,
				volume.FileSystemId,
				volume.EstimatedCostUSD.AccruedCost(),
				volume.Iops,
				volume.Throughput,
				volume.Encrypted,
				volume.Tags,
				volume.Description,
				volume.CreationTime.Format(time.RFC3339),
			)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Backend:asc", "Zone:asc", "Name:asc"}
		}
		header := table.Row{"Backend", "Zone", "Name", "Type", "SizeGiB", "Expires", "State", "DeleteOnTermination", "Owner", "AttachedTo", "DiskType", "FileSystemId", "EstimatedCost", "Iops", "Throughput", "Encrypted", "Tags", "Description", "CreationTime"}
		rows := []table.Row{}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				system.Logger.Warn("Couldn't get terminal width, using default width")
			} else {
				return err
			}
		}
		for _, volume := range volumes {
			expiresIn := t.ColorErr.Sprint("NEVER")
			if !volume.Expires.IsZero() && volume.Expires.After(time.Now()) {
				if volume.Expires.Before(time.Now().Add(time.Hour * 6)) {
					expiresIn = t.ColorWarn.Sprint(time.Until(volume.Expires).String())
				} else {
					expiresIn = time.Until(volume.Expires).String()
				}
			} else if !volume.Expires.IsZero() && volume.Expires.Before(time.Now()) {
				expiresIn = t.ColorErr.Sprint(time.Until(volume.Expires).String())
			}
			rows = append(rows, table.Row{
				volume.BackendType,
				volume.ZoneName,
				volume.Name,
				volume.VolumeType.String(),
				volume.Size / backends.StorageGiB,
				expiresIn,
				volume.State.String(),
				volume.DeleteOnTermination,
				volume.Owner,
				strings.Join(volume.AttachedTo, "\n"),
				volume.DiskType,
				volume.FileSystemId,
				fmt.Sprintf("%0.2f", volume.EstimatedCostUSD.AccruedCost()),
				volume.Iops,
				volume.Throughput,
				volume.Encrypted,
				volume.Tags,
				volume.Description,
				volume.CreationTime.Format(time.RFC3339),
			})
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("VOLUMES"), header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

func (f *VolumesListFilter) filter(volumes backends.VolumeList, errorOnNotFound bool) (backends.VolumeList, error) {
	if f.Backend != "" {
		volumes = volumes.WithBackendType(backends.BackendType(f.Backend)).Describe()
		if errorOnNotFound && volumes.Count() == 0 {
			return nil, fmt.Errorf("no volumes found for backend type: %s", f.Backend)
		}
	}
	if f.Owner != "" {
		volumes = volumes.WithOwner(f.Owner).Describe()
		if errorOnNotFound && volumes.Count() == 0 {
			return nil, fmt.Errorf("no volumes found for owner: %s", f.Owner)
		}
	}
	tags := make(map[string]string)
	for _, tag := range f.Tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag: %s, must be in the format k=v", tag)
		}
		tags[parts[0]] = parts[1]
	}
	if len(tags) > 0 {
		volumes = volumes.WithTags(tags).Describe()
		if errorOnNotFound && volumes.Count() == 0 {
			return nil, fmt.Errorf("no volumes found for tags: %s", tags)
		}
	}
	if f.Zone != "" {
		volumes = volumes.WithZoneName(f.Zone).Describe()
		if errorOnNotFound && volumes.Count() == 0 {
			return nil, fmt.Errorf("no volumes found for zone: %s", f.Zone)
		}
	}
	if f.Name != "" {
		volumes = volumes.WithName(f.Name).Describe()
		if errorOnNotFound && volumes.Count() == 0 {
			return nil, fmt.Errorf("no volumes found for name: %s", f.Name)
		}
	}
	if f.State != "" {
		var attached bool
		switch f.State {
		case "attached":
			attached = true
		case "detached":
			attached = false
		}
		volumes = volumes.WithAttached(attached).Describe()
		if errorOnNotFound && volumes.Count() == 0 {
			return nil, fmt.Errorf("no volumes found for state: %s", f.State)
		}
	}
	if f.Type != "" {
		var vtype backends.VolumeType
		switch f.Type {
		case "attached":
			vtype = backends.VolumeTypeAttachedDisk
		case "shared":
			vtype = backends.VolumeTypeSharedDisk
		}
		volumes = volumes.WithType(vtype).Describe()
		if errorOnNotFound && volumes.Count() == 0 {
			return nil, fmt.Errorf("no volumes found for type: %s", f.Type)
		}
	}
	return volumes, nil
}

package cmd

import (
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/rglonek/logger"
)

// ClientTemplateCmd is the parent command group for client template management.
// Templates are pre-built images that let `client create <type>` skip heavy
// package installation. Currently only AMS clients have a template implementation.
type ClientTemplateCmd struct {
	AMS  ClientTemplateAMSCmd `command:"ams" subcommands-optional:"true" description:"AMS (Prometheus/Grafana/Loki) template management" webicon:"fas fa-layer-group"`
	Help HelpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute prints the help when `client template` is invoked without a subcommand.
func (c *ClientTemplateCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

// ClientTemplateAMSCmd is the AMS template management command group.
// AMS templates are pre-built images containing Prometheus, Grafana (with plugins
// and generic datasources) and Loki, used for fast AMS client creation.
type ClientTemplateAMSCmd struct {
	Create  ClientTemplateAMSCreateCmd  `command:"create" subcommands-optional:"true" description:"Create an AMS template (fails if one already exists)" webicon:"fas fa-plus" invwebforce:"true"`
	Refresh ClientTemplateAMSRefreshCmd `command:"refresh" subcommands-optional:"true" description:"Create a new AMS template generation and destroy the previous one(s)" webicon:"fas fa-arrows-rotate" invwebforce:"true"`
	List    ClientTemplateAMSListCmd    `command:"list" subcommands-optional:"true" description:"List AMS templates" webicon:"fas fa-list"`
	Destroy ClientTemplateAMSDestroyCmd `command:"destroy" subcommands-optional:"true" description:"Destroy AMS template(s)" webicon:"fas fa-trash" invwebforce:"true"`
	Cleanup ClientTemplateAMSCleanupCmd `command:"cleanup" subcommands-optional:"true" description:"Clean up dangling AMS template creation instances and superseded templates" webicon:"fas fa-broom"`
	Help    HelpCmd                     `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute prints the help when `client template ams` is invoked without a subcommand.
func (c *ClientTemplateAMSCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

// ClientTemplateAMSListCmd lists available AMS templates from inventory.
// Emits a warning when more than one template exists for a given architecture
// (since `client create ams` will only pick the highest generation and the
// others are unused).
//
// Usage:
//
//	aerolab client template ams list
//	aerolab client template ams list -o json
type ClientTemplateAMSListCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute runs `client template ams list`.
func (c *ClientTemplateAMSListCmd) Execute(args []string) error {
	cmd := []string{"client", "template", "ams", "list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ListAMSTemplates(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ListAMSTemplates lists all AMS templates in the inventory. When more than
// one template exists for the same architecture it emits a warning
// recommending `client template ams cleanup` since only the highest generation
// is used by `client create ams`.
func (c *ClientTemplateAMSListCmd) ListAMSTemplates(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "template", "ams", "list"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	ls := &ImagesListCmd{Output: c.Output, TableTheme: c.TableTheme, SortBy: c.SortBy, Pager: c.Pager, Filters: ImagesListFilter{Type: "custom", SoftwareType: "ams"}}
	if err := ls.ListImages(system, inventory, nil, os.Stdout, nil); err != nil {
		return err
	}

	// Warn if more than one template exists per architecture.
	images := inventory.Images.WithTags(map[string]string{"aerolab.image.type": "ams"}).Describe()
	byArch := map[string][]*backends.Image{}
	for _, img := range images {
		byArch[img.Architecture.String()] = append(byArch[img.Architecture.String()], img)
	}
	// Emit warnings in a deterministic order to keep output stable.
	archKeys := make([]string, 0, len(byArch))
	for k := range byArch {
		archKeys = append(archKeys, k)
	}
	sort.Strings(archKeys)
	for _, arch := range archKeys {
		imgs := byArch[arch]
		if len(imgs) <= 1 {
			continue
		}
		names := []string{}
		for _, img := range imgs {
			gen := img.Tags["aerolab.ams.generation"]
			if gen == "" {
				gen = "?"
			}
			names = append(names, img.Name+" (generation="+gen+")")
		}
		sort.Strings(names)
		system.Logger.Warn("Found %d AMS templates for architecture %s: %s. Only the highest generation is used by `client create ams`; run `aerolab client template ams cleanup` to remove the superseded ones.", len(imgs), arch, strings.Join(names, ", "))
	}
	return nil
}

// ClientTemplateAMSDestroyCmd destroys AMS template images.
//
// Usage:
//
//	aerolab client template ams destroy            # destroy all (prompt)
//	aerolab client template ams destroy -a amd64   # destroy only amd64 templates
//	aerolab client template ams destroy --generation 3
//	aerolab client template ams destroy --force
type ClientTemplateAMSDestroyCmd struct {
	Arch              string  `short:"a" long:"arch" description:"Architecture to destroy the template for (amd64, arm64)"`
	Generation        int     `short:"i" long:"generation" description:"AMS template generation number to destroy"`
	PrometheusVersion string  `short:"P" long:"prometheus-version" description:"Prometheus version to destroy the template for"`
	GrafanaVersion    string  `short:"g" long:"grafana-version" description:"Grafana version to destroy the template for"`
	Force             bool    `short:"f" long:"force" description:"Force the destruction of the template - do not ask for confirmation"`
	DryRun            bool    `short:"n" long:"dry-run" description:"Do not actually destroy the template, just run the basic checks"`
	Help              HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute runs `client template ams destroy`.
func (c *ClientTemplateAMSDestroyCmd) Execute(args []string) error {
	cmd := []string{"client", "template", "ams", "destroy"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))
	system.Logger.Info("Backend: %s, Project: %s", system.Opts.Config.Backend.Type, os.Getenv("AEROLAB_PROJECT"))
	defer UpdateDiskCache(system)()
	err = c.DestroyAMSTemplates(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// DestroyAMSTemplates destroys AMS templates matching the specified criteria.
func (c *ClientTemplateAMSDestroyCmd) DestroyAMSTemplates(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "template", "ams", "destroy"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	images := inventory.Images.WithTags(map[string]string{"aerolab.image.type": "ams"})
	if c.Generation > 0 {
		images = images.WithTags(map[string]string{"aerolab.ams.generation": strconv.Itoa(c.Generation)})
	}
	if c.PrometheusVersion != "" {
		images = images.WithTags(map[string]string{"aerolab.ams.prometheus": c.PrometheusVersion})
	}
	if c.GrafanaVersion != "" {
		images = images.WithTags(map[string]string{"aerolab.ams.grafana": c.GrafanaVersion})
	}
	if c.Arch != "" {
		var arch backends.Architecture
		if err := arch.FromString(c.Arch); err != nil {
			return err
		}
		images = images.WithArchitecture(arch)
	}

	if c.DryRun {
		logger.Info("Dry run, would destroy the following AMS templates:")
		for _, image := range images.Describe() {
			logger.Info("  name=%s, arch=%s, generation=%s, prometheus=%s, grafana=%s",
				image.Name,
				image.Architecture,
				image.Tags["aerolab.ams.generation"],
				image.Tags["aerolab.ams.prometheus"],
				image.Tags["aerolab.ams.grafana"])
		}
		return nil
	}

	if images.Count() == 0 {
		logger.Info("No AMS templates found matching the criteria")
		return nil
	}

	if IsInteractive() && !c.Force {
		ch, quitting, err := choice.Choice("Are you sure you want to destroy "+strconv.Itoa(images.Count())+" AMS template(s)?", choice.Items{
			choice.Item("Yes"),
			choice.Item("No"),
		})
		if err != nil {
			return err
		}
		if quitting {
			return errors.New("aborted")
		}
		if ch == "No" {
			return errors.New("aborted")
		}
	}

	logger.Info("Destroying %d AMS template(s)", images.Count())
	return images.DeleteImages(time.Minute * 10)
}

// ClientTemplateAMSCleanupCmd cleans dangling AMS template creation instances
// and superseded AMS template images.
//
// This removes:
//   - Dangling temporary instances used during AMS template creation
//     (these are tagged aerolab.type=images.create + aerolab.tmpl.version=ams-*
//     and may survive a crashed build or Ctrl-C during creation).
//   - Superseded AMS templates: for each architecture, anything whose
//     aerolab.ams.generation tag is lower than the highest generation found,
//     or missing/unparsable.
//
// Usage:
//
//	aerolab client template ams cleanup
//	aerolab client template ams cleanup --dry-run
type ClientTemplateAMSCleanupCmd struct {
	DryRun bool    `short:"n" long:"dry-run" description:"Do not actually destroy templates/instances, just show what would be removed"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute runs `client template ams cleanup`.
func (c *ClientTemplateAMSCleanupCmd) Execute(args []string) error {
	cmd := []string{"client", "template", "ams", "cleanup"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.CleanupAMSTemplates(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// CleanupAMSTemplates removes dangling AMS template creation instances and
// any AMS template images whose generation tag is lower than the current
// highest generation (or is missing/unparsable) for their architecture.
func (c *ClientTemplateAMSCleanupCmd) CleanupAMSTemplates(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "template", "ams", "cleanup"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Step 1: dangling temporary AMS template creation instances.
	danglingInstances := inventory.Instances.WithTags(map[string]string{
		"aerolab.type": "images.create",
	}).WithNotState(backends.LifeCycleStateTerminated)

	var amsDangling backends.InstanceList
	for _, inst := range danglingInstances.Describe() {
		if strings.HasPrefix(inst.Tags["aerolab.tmpl.version"], "ams-") {
			amsDangling = append(amsDangling, inst)
		}
	}

	if len(amsDangling) > 0 {
		if c.DryRun {
			logger.Info("Dry run, would remove the following dangling AMS template creation instances:")
			for _, inst := range amsDangling {
				logger.Info("  name=%s, state=%s, tmpl-version=%s",
					inst.Name,
					inst.InstanceState,
					inst.Tags["aerolab.tmpl.version"])
			}
		} else {
			logger.Info("Removing %d dangling AMS template creation instance(s)", len(amsDangling))
			if err := amsDangling.Terminate(time.Minute * 10); err != nil {
				return err
			}
		}
	} else {
		logger.Info("No dangling AMS template creation instances found")
	}

	// Step 2: superseded AMS template images. For each architecture, find the
	// highest generation and delete anything below it (plus anything with
	// a missing or unparsable generation tag).
	allImages := inventory.Images.WithTags(map[string]string{"aerolab.image.type": "ams"}).Describe()

	maxPerArch := map[string]int{}
	for _, img := range allImages {
		gen, err := strconv.Atoi(img.Tags["aerolab.ams.generation"])
		if err != nil {
			continue
		}
		arch := img.Architecture.String()
		if gen > maxPerArch[arch] {
			maxPerArch[arch] = gen
		}
	}

	var superseded backends.ImageList
	for _, img := range allImages {
		arch := img.Architecture.String()
		maxGen := maxPerArch[arch]
		gen, err := strconv.Atoi(img.Tags["aerolab.ams.generation"])
		if err != nil {
			// Missing or unparsable generation: treat as superseded only if a
			// valid current template exists for the same arch (don't delete a
			// lone untagged template — the user may want to inspect it).
			if maxGen > 0 {
				superseded = append(superseded, img)
			}
			continue
		}
		if gen < maxGen {
			superseded = append(superseded, img)
		}
	}

	if len(superseded) > 0 {
		if c.DryRun {
			logger.Info("Dry run, would remove the following superseded AMS templates:")
			for _, image := range superseded {
				logger.Info("  name=%s, arch=%s, generation=%s (current: %d)",
					image.Name,
					image.Architecture,
					image.Tags["aerolab.ams.generation"],
					maxPerArch[image.Architecture.String()])
			}
		} else {
			logger.Info("Removing %d superseded AMS template(s)", len(superseded))
			if err := superseded.DeleteImages(time.Minute * 10); err != nil {
				return err
			}
		}
	} else {
		logger.Info("No superseded AMS templates found")
	}

	return nil
}

package cmd

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/agi"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/rglonek/logger"
)

// AgiTemplateCmd is the AGI template management command structure.
// AGI templates are pre-built images containing all required software
// (aerospike, grafana, plugin, ingest, ttyd, filebrowser) for fast AGI instance creation.
type AgiTemplateCmd struct {
	Create  AgiTemplateCreateCmd  `command:"create" subcommands-optional:"true" description:"Create AGI template" webicon:"fas fa-plus"`
	List    AgiTemplateListCmd    `command:"list" subcommands-optional:"true" description:"List AGI templates" webicon:"fas fa-list"`
	Destroy AgiTemplateDestroyCmd `command:"destroy" subcommands-optional:"true" description:"Destroy AGI template" webicon:"fas fa-trash"`
	Vacuum  AgiTemplateVacuumCmd  `command:"vacuum" subcommands-optional:"true" description:"Clean dangling templates" webicon:"fas fa-broom"`
	Help    HelpCmd               `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiTemplateCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

// AgiTemplateCreateCmd is defined in cmdAgiTemplateCreate.go

// AgiTemplateListCmd lists available AGI templates from inventory.
//
// Usage:
//
//	aerolab agi template list
//	aerolab agi template list -o json
type AgiTemplateListCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi template list.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiTemplateListCmd) Execute(args []string) error {
	cmd := []string{"agi", "template", "list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ListAgiTemplates(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ListAgiTemplates lists all AGI templates in the inventory.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The backend inventory to search
//   - args: Additional command arguments
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiTemplateListCmd) ListAgiTemplates(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "template", "list"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	ls := &ImagesListCmd{Output: c.Output, TableTheme: c.TableTheme, SortBy: c.SortBy, Pager: c.Pager, Filters: ImagesListFilter{Type: "custom", SoftwareType: "agi"}}
	err := ls.ListImages(system, inventory, nil, os.Stdout, nil)
	if err != nil {
		return err
	}
	return nil
}

// AgiTemplateDestroyCmd destroys a specific AGI template.
//
// Usage:
//
//	aerolab agi template destroy
//	aerolab agi template destroy -a amd64
//	aerolab agi template destroy --agi-version 7
type AgiTemplateDestroyCmd struct {
	Arch             string  `short:"a" long:"arch" description:"Architecture to destroy the template for (amd64, arm64)"`
	AgiVersion       int     `short:"i" long:"agi-version" description:"AGI version number to destroy"`
	AerospikeVersion string  `short:"v" long:"aerospike-version" description:"Aerospike version to destroy the template for"`
	GrafanaVersion   string  `short:"g" long:"grafana-version" description:"Grafana version to destroy the template for"`
	Force            bool    `short:"f" long:"force" description:"Force the destruction of the template - do not ask for confirmation"`
	DryRun           bool    `short:"n" long:"dry-run" description:"Do not actually destroy the template, just run the basic checks"`
	Help             HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi template destroy.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiTemplateDestroyCmd) Execute(args []string) error {
	cmd := []string{"agi", "template", "destroy"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))
	system.Logger.Info("Backend: %s, Project: %s", system.Opts.Config.Backend.Type, os.Getenv("AEROLAB_PROJECT"))
	defer UpdateDiskCache(system)()
	err = c.DestroyAgiTemplate(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// DestroyAgiTemplate destroys AGI templates matching the specified criteria.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The backend inventory to search
//   - logger: Logger for output messages
//   - args: Additional command arguments
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiTemplateDestroyCmd) DestroyAgiTemplate(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "template", "destroy"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Find all AGI templates
	images := inventory.Images.WithTags(map[string]string{"aerolab.image.type": "agi"})

	// Filter by AGI version
	if c.AgiVersion > 0 {
		images = images.WithTags(map[string]string{"aerolab.agi.version": strconv.Itoa(c.AgiVersion)})
	}

	// Filter by Aerospike version
	if c.AerospikeVersion != "" {
		images = images.WithTags(map[string]string{"aerolab.agi.aerospike": c.AerospikeVersion})
	}

	// Filter by Grafana version
	if c.GrafanaVersion != "" {
		images = images.WithTags(map[string]string{"aerolab.agi.grafana": c.GrafanaVersion})
	}

	// Filter by architecture
	if c.Arch != "" {
		var arch backends.Architecture
		err := arch.FromString(c.Arch)
		if err != nil {
			return err
		}
		images = images.WithArchitecture(arch)
	}

	if c.DryRun {
		logger.Info("Dry run, would destroy the following AGI templates:")
		for _, image := range images.Describe() {
			logger.Info("  name=%s, arch=%s, agi-version=%s, aerospike=%s, grafana=%s",
				image.Name,
				image.Architecture,
				image.Tags["aerolab.agi.version"],
				image.Tags["aerolab.agi.aerospike"],
				image.Tags["aerolab.agi.grafana"])
		}
		return nil
	}

	if images.Count() == 0 {
		logger.Info("No AGI templates found matching the criteria")
		return nil
	}

	if IsInteractive() && !c.Force {
		ch, quitting, err := choice.Choice("Are you sure you want to destroy "+strconv.Itoa(images.Count())+" AGI template(s)?", choice.Items{
			choice.Item("Yes"),
			choice.Item("No"),
		})
		if err != nil {
			return err
		}
		if quitting {
			return errors.New("aborted")
		}
		switch ch {
		case "No":
			return errors.New("aborted")
		}
	}

	logger.Info("Destroying %d AGI template(s)", images.Count())
	err := images.DeleteImages(time.Minute * 10)
	if err != nil {
		return err
	}
	return nil
}

// AgiTemplateVacuumCmd cleans dangling/outdated AGI templates and instances.
// This removes:
// - Dangling temporary instances used during AGI template creation (crashed builds)
// - AGI templates that are older than the current AGI version
//
// Usage:
//
//	aerolab agi template vacuum
//	aerolab agi template vacuum --dry-run
type AgiTemplateVacuumCmd struct {
	DryRun bool    `short:"n" long:"dry-run" description:"Do not actually destroy templates/instances, just show what would be removed"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi template vacuum.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiTemplateVacuumCmd) Execute(args []string) error {
	cmd := []string{"agi", "template", "vacuum"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.VacuumAgiTemplates(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// VacuumAgiTemplates removes dangling AGI template creation instances and outdated AGI templates.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The backend inventory to search
//   - logger: Logger for output messages
//   - args: Additional command arguments
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiTemplateVacuumCmd) VacuumAgiTemplates(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "template", "vacuum"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	currentVersion := agi.AGIVersion
	logger.Info("Current AGI version: %d", currentVersion)

	// Step 1: Find and clean up dangling AGI template creation instances
	// These are temporary instances used during template creation that may have crashed
	danglingInstances := inventory.Instances.WithTags(map[string]string{
		"aerolab.type": "images.create",
	}).WithNotState(backends.LifeCycleStateTerminated)

	// Filter to only AGI-related instances (aerolab.tmpl.version starts with "agi-")
	var agiDanglingInstances backends.InstanceList
	for _, inst := range danglingInstances.Describe() {
		tmplVersion := inst.Tags["aerolab.tmpl.version"]
		if strings.HasPrefix(tmplVersion, "agi-") {
			agiDanglingInstances = append(agiDanglingInstances, inst)
		}
	}

	if len(agiDanglingInstances) > 0 {
		if c.DryRun {
			logger.Info("Dry run, would remove the following dangling AGI template creation instances:")
			for _, inst := range agiDanglingInstances {
				logger.Info("  name=%s, state=%s, tmpl-version=%s",
					inst.Name,
					inst.InstanceState,
					inst.Tags["aerolab.tmpl.version"])
			}
		} else {
			logger.Info("Removing %d dangling AGI template creation instance(s)", len(agiDanglingInstances))
			err := backends.InstanceList(agiDanglingInstances).Terminate(time.Minute * 10)
			if err != nil {
				return err
			}
		}
	} else {
		logger.Info("No dangling AGI template creation instances found")
	}

	// Step 2: Find and clean up outdated AGI templates
	allImages := inventory.Images.WithTags(map[string]string{"aerolab.image.type": "agi"})

	// Find templates with outdated AGI versions
	var outdatedImages backends.ImageList
	for _, img := range allImages.Describe() {
		versionStr := img.Tags["aerolab.agi.version"]
		if versionStr == "" {
			// No version tag - consider it outdated
			outdatedImages = append(outdatedImages, img)
			continue
		}
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			// Invalid version - consider it outdated
			outdatedImages = append(outdatedImages, img)
			continue
		}
		if version < currentVersion {
			outdatedImages = append(outdatedImages, img)
		}
	}

	if len(outdatedImages) > 0 {
		if c.DryRun {
			logger.Info("Dry run, would remove the following outdated AGI templates:")
			for _, image := range outdatedImages {
				logger.Info("  name=%s, arch=%s, agi-version=%s (current: %d)",
					image.Name,
					image.Architecture,
					image.Tags["aerolab.agi.version"],
					currentVersion)
			}
		} else {
			logger.Info("Removing %d outdated AGI template(s)", len(outdatedImages))
			err := backends.ImageList(outdatedImages).DeleteImages(time.Minute * 10)
			if err != nil {
				return err
			}
		}
	} else {
		logger.Info("No outdated AGI templates found")
	}

	return nil
}

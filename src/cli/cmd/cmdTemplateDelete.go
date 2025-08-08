package cmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
)

type TemplateDeleteCmd struct {
	Name         string   `short:"n" long:"name" description:"Name of the template to delete"`
	Region       string   `short:"r" long:"region" description:"Region of the template to delete"`
	Owner        string   `short:"o" long:"owner" description:"Owner of the template to delete"`
	Architecture string   `short:"a" long:"architecture" description:"Architecture of the template to delete"`
	OSName       string   `short:"d" long:"os-name" description:"OS name of the template to delete"`
	OSVersion    string   `short:"i" long:"os-version" description:"OS version of the template to delete"`
	Type         string   `short:"t" long:"type" description:"Type of the template to delete"`
	Version      string   `short:"v" long:"version" description:"Software version of the template to delete"`
	Tags         []string `short:"T" long:"tag" description:"Tag of the template to delete, as k=v"`
	DryRun       bool     `long:"dry-run" description:"Print what would be done, but do not actually delete"`
	Force        bool     `long:"force" description:"Do not ask for confirmation"`
	Help         HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *TemplateDeleteCmd) Execute(args []string) error {
	cmd := []string{"template", "delete"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.DeleteTemplate(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *TemplateDeleteCmd) DeleteTemplate(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"template", "delete"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	images := inventory.Images.WithInAccount(true)
	if c.Name != "" {
		images = images.WithName(c.Name)
	}
	if c.Region != "" {
		images = images.WithZoneName(c.Region)
	}
	if c.Owner != "" {
		images = images.WithOwner(c.Owner)
	}
	if c.Architecture != "" {
		var arch backends.Architecture
		err := arch.FromString(c.Architecture)
		if err != nil {
			return err
		}
		images = images.WithArchitecture(arch)
	}
	if c.OSName != "" {
		images = images.WithOSName(c.OSName)
	}
	if c.OSVersion != "" {
		images = images.WithOSVersion(c.OSVersion)
	}
	if c.Type != "" {
		images = images.WithTags(map[string]string{"aerolab.image.type": c.Type})
	}
	if c.Version != "" {
		images = images.WithTags(map[string]string{"aerolab.soft.version": c.Version})
	}
	if len(c.Tags) > 0 {
		tags := make(map[string]string)
		for _, tag := range c.Tags {
			parts := strings.SplitN(tag, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid tag: %s, must be in the format k=v", tag)
			}
			tags[parts[0]] = parts[1]
		}
		images = images.WithTags(tags)
	}

	if images.Count() == 0 {
		system.Logger.Info("No templates found matching the criteria")
		return nil
	}
	system.Logger.Info("Found %d templates to delete", images.Count())
	if !c.DryRun {
		if !c.Force && IsInteractive() {
			choice, quitting, err := choice.Choice("Are you sure you want to destroy "+strconv.Itoa(images.Count())+" templates?", choice.Items{
				choice.Item("Yes"),
				choice.Item("No"),
			})
			if err != nil {
				return err
			}
			if quitting {
				return errors.New("aborted")
			}
			switch choice {
			case "No":
				return errors.New("aborted")
			}
		}
		err := images.DeleteImages(time.Minute * 10)
		if err != nil {
			return err
		}
		return nil
	}
	system.Logger.Info("DRY-RUN: Would delete %d templates", images.Count())
	system.Opts.Template.List.Filters.Owner = ""
	system.Opts.Template.List.Filters.Type = "all"
	system.Opts.Template.List.ListTemplates(system, &backends.Inventory{Images: images}, []string{}, os.Stdout, nil)
	return nil
}

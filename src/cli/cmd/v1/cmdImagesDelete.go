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

type ImagesDeleteCmd struct {
	Name         string   `short:"n" long:"name" description:"Name of the image to delete"`
	Region       string   `short:"r" long:"region" description:"Region of the image to delete"`
	Owner        string   `short:"o" long:"owner" description:"Owner of the image to delete"`
	Architecture string   `short:"a" long:"architecture" description:"Architecture of the image to delete"`
	OSName       string   `short:"d" long:"os-name" description:"OS name of the image to delete"`
	OSVersion    string   `short:"i" long:"os-version" description:"OS version of the image to delete"`
	Type         string   `short:"t" long:"type" description:"Type of the image to delete"`
	Version      string   `short:"v" long:"version" description:"Software version of the image to delete"`
	Tags         []string `short:"T" long:"tag" description:"Tag of the image to delete, as k=v"`
	DryRun       bool     `long:"dry-run" description:"Print what would be done, but do not actually delete"`
	Force        bool     `long:"force" description:"Do not ask for confirmation"`
	Help         HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ImagesDeleteCmd) Execute(args []string) error {
	cmd := []string{"images", "delete"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))
	system.Logger.Info("Backend: %s, Project: %s", system.Opts.Config.Backend.Type, os.Getenv("AEROLAB_PROJECT"))
	defer UpdateDiskCache(system)
	err = c.DeleteImage(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ImagesDeleteCmd) DeleteImage(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"images", "delete"}, c, args...)
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
		system.Logger.Info("No images found matching the criteria")
		return nil
	}
	system.Logger.Info("Found %d images to delete", images.Count())
	if !c.DryRun {
		if !c.Force && IsInteractive() {
			choice, quitting, err := choice.Choice("Are you sure you want to destroy "+strconv.Itoa(images.Count())+" images?", choice.Items{
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
	system.Logger.Info("DRY-RUN: Would delete %d images", images.Count())
	system.Opts.Images.List.Filters.Owner = ""
	system.Opts.Images.List.Filters.Type = "all"
	system.Opts.Images.List.ListImages(system, &backends.Inventory{Images: images}, []string{}, os.Stdout, nil)
	return nil
}

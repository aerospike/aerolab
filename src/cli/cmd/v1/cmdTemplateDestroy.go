package cmd

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/rglonek/logger"
)

type TemplateDestroyCmd struct {
	Distro           string  `short:"d" long:"distro" description:"Distro to destroy the template for"`
	DistroVersion    string  `short:"i" long:"distro-version" description:"Version of the distro to destroy the template for"`
	Arch             string  `short:"a" long:"arch" description:"Architecture to destroy the template for"`
	AerospikeVersion string  `short:"v" long:"aerospike-version" description:"Aerospike version to destroy the template for"`
	Force            bool    `short:"f" long:"force" description:"Force the destruction of the template - do not ask for confirmation"`
	DryRun           bool    `short:"n" long:"dry-run" description:"Do not actually destroy the template, just run the basic checks"`
	Help             HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *TemplateDestroyCmd) Execute(args []string) error {
	cmd := []string{"template", "destroy"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))
	system.Logger.Info("Backend: %s, Project: %s", system.Opts.Config.Backend.Type, os.Getenv("AEROLAB_PROJECT"))
	defer UpdateDiskCache(system)()
	err = c.DestroyTemplate(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *TemplateDestroyCmd) DestroyTemplate(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"template", "destroy"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// find all templates that match the distro, version, arch and aerospike version criteria
	images := inventory.Images.WithTags(map[string]string{"aerolab.image.type": "aerospike"})

	if c.Distro != "" {
		images = images.WithOSName(c.Distro)
	}
	if c.DistroVersion != "" {
		images = images.WithOSVersion(c.DistroVersion)
	}
	if c.AerospikeVersion != "" {
		if strings.HasSuffix(c.AerospikeVersion, "c") {
			c.AerospikeVersion = strings.TrimRight(c.AerospikeVersion, "c") + "-community"
		} else if strings.HasSuffix(c.AerospikeVersion, "f") {
			c.AerospikeVersion = strings.TrimRight(c.AerospikeVersion, "f") + "-federal"
		} else if !strings.HasSuffix(c.AerospikeVersion, "-enterprise") {
			c.AerospikeVersion = strings.TrimRight(c.AerospikeVersion, "e") + "-enterprise"
		}
		logger.Info("Aerospike Version: %s", c.AerospikeVersion)
		images = images.WithTags(map[string]string{"aerolab.soft.version": c.AerospikeVersion})
	}
	if c.Arch != "" {
		var arch backends.Architecture
		err := arch.FromString(c.Arch)
		if err != nil {
			return err
		}
		images = images.WithArchitecture(arch)
	}

	if c.DryRun {
		logger.Info("Dry run, would destroy the following images:")
		for _, image := range images.Describe() {
			logger.Info("  name=%s, distro=%s, version=%s, arch=%s, aerospike-version=%s", image.Name, image.OSName, image.OSVersion, image.Architecture, image.Tags["aerolab.soft.version"])
		}
		return nil
	}

	if images.Count() == 0 {
		logger.Info("No images found matching the criteria")
		return nil
	}

	if IsInteractive() && !c.Force {
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

	logger.Info("Destroying %d images", images.Count())
	err := images.DeleteImages(time.Minute * 10)
	if err != nil {
		return err
	}
	return nil
}

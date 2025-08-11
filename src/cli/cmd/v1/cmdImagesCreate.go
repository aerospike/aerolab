package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type ImagesCreateCmd struct {
	Name         string   `short:"n" long:"name" description:"Set image name"`
	Description  string   `short:"d" long:"description" description:"Set image description"`
	InstanceName string   `short:"N" long:"instance-name" description:"Name of the instance to create the image from"`
	SizeGiB      int      `short:"s" long:"size" description:"Set image size in GiB; default is the size of the root volume of the instance"`
	Owner        string   `short:"o" long:"owner" description:"Set image owner"`
	Type         string   `short:"t" long:"type" description:"Set image type; sets aerolab.image.type tag"`
	Version      string   `short:"v" long:"version" description:"Set image version; sets aerolab.soft.version tag"`
	Tags         []string `short:"T" long:"tag" description:"Set extra image tags, as k=v"`
	DryRun       bool     `long:"dry-run" description:"Do not actually create the image, just run the basic checks"`
	Help         HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ImagesCreateCmd) Execute(args []string) error {
	cmd := []string{"images", "create"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	_, err = c.CreateImage(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	err = UpdateDiskCache(system)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ImagesCreateCmd) CreateImage(system *System, inventory *backends.Inventory, args []string) (*backends.Image, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"images", "create"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// find the instance and check it's state
	system.Logger.Info("Validating parameters")
	if c.InstanceName == "" {
		return nil, fmt.Errorf("instance-name is required")
	}
	instances := inventory.Instances.WithName(c.InstanceName)
	if instances.Count() == 0 {
		return nil, fmt.Errorf("instance %s not found", c.InstanceName)
	}
	if instances.Count() > 1 {
		return nil, fmt.Errorf("multiple instances found with name %s", c.InstanceName)
	}
	instance := instances.Describe()[0]
	if instance.InstanceState != backends.LifeCycleStateRunning && instance.InstanceState != backends.LifeCycleStateStopped {
		return nil, fmt.Errorf("instance %s is in invalid state %s, must be running or stopped", c.InstanceName, instance.InstanceState.String())
	}

	// check if the size is set
	if c.SizeGiB == 0 {
		c.SizeGiB = int(instance.AttachedVolumes.Describe()[0].Size / 1024 / 1024 / 1024)
	} else if instance.AttachedVolumes.Count() > 0 && c.SizeGiB < int(instance.AttachedVolumes.Describe()[0].Size/1024/1024/1024) {
		return nil, fmt.Errorf("image size %d is smaller than the size of the root volume %d", c.SizeGiB, int(instance.AttachedVolumes.Describe()[0].Size/1024/1024/1024))
	}

	// check name validity
	if c.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`).MatchString(c.Name) {
		return nil, fmt.Errorf("name must match regex ^[a-zA-Z0-9][a-zA-Z0-9-]*$")
	}

	// check variables are set and valid
	if c.Type == "" {
		return nil, fmt.Errorf("type is required")
	}
	if c.Version == "" {
		return nil, fmt.Errorf("version is required")
	}
	if c.Owner == "" {
		return nil, fmt.Errorf("owner is required")
	}

	// check if the image already exists
	images := inventory.Images.WithName(c.Name)
	if images.Count() > 0 {
		return nil, fmt.Errorf("image %s already exists", c.Name)
	}

	// check if the tags can be parsed
	tags := make(map[string]string)
	for _, tag := range c.Tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag: %s, must be in the format k=v", tag)
		}
		tags[parts[0]] = parts[1]
	}
	tags["aerolab.image.type"] = c.Type
	tags["aerolab.soft.version"] = c.Version

	system.Logger.Info("Name: %s, Type: %s, Version: %s, Owner: %s, Tags: %v, FromInstance: %s", c.Name, c.Type, c.Version, c.Owner, tags, c.InstanceName)

	// dry run
	if c.DryRun {
		system.Logger.Info("Dry run, not creating image")
		return nil, nil
	}

	// create the image
	system.Logger.Info("Creating image")
	image, err := system.Backend.CreateImage(&backends.CreateImageInput{
		BackendType: backends.BackendType(system.Opts.Config.Backend.Type),
		Instance:    instance,
		Name:        c.Name,
		Description: c.Description,
		SizeGiB:     backends.StorageSize(c.SizeGiB),
		Owner:       c.Owner,
		Tags:        tags,
		Encrypted:   true,
		OSName:      instance.OperatingSystem.Name,
		OSVersion:   instance.OperatingSystem.Version,
	}, time.Minute*10)
	if err != nil {
		return nil, err
	}
	system.Logger.Info("Image created with ImageID: %s", image.Image.ImageId)
	return image.Image, nil
}

package cmd

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bdocker"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"
	"gopkg.in/yaml.v3"
)

type VolumesCreateCmd struct {
	Name            string                 `long:"name" description:"Name of the volume"`
	Description     string                 `long:"description" description:"Description of the volume"`
	Owner           string                 `long:"owner" description:"Owner of the volume"`
	Tags            []string               `long:"tag" description:"Tags to add to the volume, format: k=v"`
	VolumeType      string                 `long:"volume-type" description:"Type of volume to create: attached or shared"`
	NoInstallExpiry bool                   `long:"no-install-expiry" description:"Do not install the expiry system, even if volume expiry is set"`
	AWS             VolumesCreateCmdAws    `group:"AWS" description:"backend-aws" namespace:"aws"`
	GCP             VolumesCreateCmdGcp    `group:"GCP" description:"backend-gcp" namespace:"gcp"`
	Docker          VolumesCreateCmdDocker `group:"Docker" description:"backend-docker" namespace:"docker"`
	DryRun          bool                   `long:"dry-run" description:"Dry run, print what would be done but don't do it"`
	Help            HelpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

type VolumesCreateCmdAws struct {
	SizeGiB           int           `long:"size" description:"Size of the volume in GB"`
	Placement         string        `long:"placement" description:"Placement of the volume"`
	DiskType          string        `long:"disk-type" description:"Type of disk to use"`
	Iops              int           `long:"iops" description:"Iops of the volume"`
	Throughput        int           `long:"throughput" description:"Throughput of the volume"`
	Encrypted         bool          `long:"encrypted" description:"Whether the volume is encrypted"`
	SharedDiskOneZone bool          `long:"shared-disk-one-zone" description:"Whether the volume is shared in one zone"`
	Expire            time.Duration `long:"expire" description:"Expire the volume in a given time, format: 1h, 1d, 1w, 1m, 1y" default:"30h"`
}

type VolumesCreateCmdGcp struct {
	SizeGiB    int           `long:"size" description:"Size of the volume in GB"`
	Zone       string        `long:"zone" description:"Zone of the volume"`
	DiskType   string        `long:"disk-type" description:"Type of disk to use"`
	Iops       int           `long:"iops" description:"Iops of the volume"`
	Throughput int           `long:"throughput" description:"Throughput of the volume"`
	Expire     time.Duration `long:"expire" description:"Expire the volume in a given time, format: 1h, 1d, 1w, 1m, 1y" default:"30h"`
}

type VolumesCreateCmdDocker struct {
	Driver string `long:"driver" description:"Driver to use for the volume"`
}

func (c *VolumesCreateCmd) Execute(args []string) error {
	cmd := []string{"volumes", "create"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	_, err = c.CreateVolumes(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *VolumesCreateCmd) CreateVolumes(system *System, inventory *backends.Inventory, args []string) (*backends.Volume, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"volumes", "create"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	if inventory.Volumes.WithName(c.Name).Count() > 0 {
		return nil, fmt.Errorf("volume with name %s already exists", c.Name)
	}

	var volumeType backends.VolumeType
	switch c.VolumeType {
	case "attached":
		volumeType = backends.VolumeTypeAttachedDisk
	case "shared":
		volumeType = backends.VolumeTypeSharedDisk
		c.AWS.DiskType = "shared"
	default:
		return nil, fmt.Errorf("invalid volume type: %s", c.VolumeType)
	}
	tags := map[string]string{}
	for _, tag := range c.Tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag: %s, must be in the format k=v", tag)
		}
		tags[parts[0]] = parts[1]
	}
	// Add telemetry tag if telemetry is enabled
	if enabled, telemetryUUID := IsTelemetryEnabled(system); enabled {
		tags["aerolab.telemetry"] = telemetryUUID
	}
	var expire time.Time
	switch system.Opts.Config.Backend.Type {
	case "aws":
		if c.AWS.Expire > 0 {
			expire = time.Now().Add(c.AWS.Expire)
		}
	case "gcp":
		if c.GCP.Expire > 0 {
			expire = time.Now().Add(c.GCP.Expire)
		}
	}
	backendSpecificParams := map[backends.BackendType]interface{}{
		"aws": &baws.CreateVolumeParams{
			SizeGiB:           c.AWS.SizeGiB,
			Placement:         c.AWS.Placement,
			DiskType:          c.AWS.DiskType,
			Iops:              c.AWS.Iops,
			Throughput:        c.AWS.Throughput,
			Encrypted:         c.AWS.Encrypted,
			SharedDiskOneZone: c.AWS.SharedDiskOneZone,
		},
		"gcp": &bgcp.CreateVolumeParams{
			SizeGiB:    c.GCP.SizeGiB,
			Placement:  c.GCP.Zone,
			DiskType:   c.GCP.DiskType,
			Iops:       c.GCP.Iops,
			Throughput: c.GCP.Throughput,
		},
		"docker": &bdocker.CreateVolumeParams{
			Driver: c.Docker.Driver,
		},
	}
	create := &backends.CreateVolumeInput{
		BackendType:           backends.BackendType(system.Opts.Config.Backend.Type),
		VolumeType:            volumeType,
		Name:                  c.Name,
		Description:           c.Description,
		Owner:                 c.Owner,
		Tags:                  tags,
		Expires:               expire,
		BackendSpecificParams: backendSpecificParams,
	}
	for k := range create.BackendSpecificParams {
		if string(k) != system.Opts.Config.Backend.Type {
			delete(create.BackendSpecificParams, k)
		}
	}
	awsRegion := c.AWS.Placement
	var err error
	if system.Opts.Config.Backend.Type == "aws" {
		_, _, awsRegion, err = system.Backend.ResolveNetworkPlacement(backends.BackendTypeAWS, awsRegion)
		if err != nil {
			return nil, err
		}
		// Update the placement in backend params with the resolved region/zone
		if awsParams, ok := create.BackendSpecificParams[backends.BackendTypeAWS].(*baws.CreateVolumeParams); ok {
			awsParams.Placement = awsRegion
		}
	}
	if system.Opts.Config.Backend.Type != "docker" {
		costDB, err := system.Backend.CreateVolumeGetPrice(create)
		if err != nil {
			system.Logger.Warn("Could not get volume price: %s", err)
		} else {
			switch system.Opts.Config.Backend.Type {
			case "aws":
				costDB = costDB * float64(c.AWS.SizeGiB)
			case "gcp":
				costDB = costDB * float64(c.GCP.SizeGiB)
			}
			system.Logger.Info("Volume GB cost: hour: $%.2f, day: $%.2f, month: $%.2f", math.Ceil(costDB*100)/100, math.Ceil(costDB*24*100)/100, math.Ceil(costDB*24*30*100)/100)
		}
	}
	if c.DryRun {
		system.Logger.Info("Create Volumes Configuration:")
		pf := &prefixWriter{prefix: "  ", logger: system.Logger}
		enc := yaml.NewEncoder(pf)
		enc.SetIndent(2)
		enc.Encode(create)
		pf.Flush()
		system.Logger.Info("Dry run, not creating volumes")
		return nil, nil
	}
	installExpiry := false
	switch system.Opts.Config.Backend.Type {
	case "aws":
		installExpiry = c.AWS.Expire > 0 && !c.NoInstallExpiry
	case "gcp":
		installExpiry = c.GCP.Expire > 0 && !c.NoInstallExpiry
	}
	if installExpiry {
		// Check for v7 expiry system and warn user
		warnIfV7ExpiryInstalled(system.Backend, backends.BackendType(system.Opts.Config.Backend.Type), system.Logger)

		wg := new(sync.WaitGroup)
		wg.Add(1)
		defer wg.Wait()
		go func() {
			defer wg.Done()
			volumeRegion := awsRegion
			if strings.Count(volumeRegion, "-") == 2 {
				if len(volumeRegion[strings.LastIndex(volumeRegion, "-")+1:]) == 2 {
					volumeRegion = volumeRegion[:len(volumeRegion)-1]
				}
			}
			if system.Opts.Config.Backend.Type == "gcp" {
				volumeRegion = c.GCP.Zone
				if strings.Count(volumeRegion, "-") == 2 {
					volumeRegion = volumeRegion[:strings.LastIndex(volumeRegion, "-")]
				}
			}
			err := system.Backend.ExpiryInstall(backends.BackendType(system.Opts.Config.Backend.Type), 15, 4, false, false, false, true, volumeRegion)
			if err != nil {
				system.Logger.Error("Error installing expiry system, volumes will not auto expire. Details: %s", err)
			}
		}()
	}
	system.Logger.Info("Creating volumes, this may take a while...")
	volumes, err := system.Backend.CreateVolume(create)
	if err != nil {
		return nil, err
	}
	return &volumes.Volume, nil
}

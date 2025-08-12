package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bdocker"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/strslice"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
	"gopkg.in/yaml.v3"
)

type InstancesCreateCmd struct {
	ClusterName        string                   `short:"n" long:"cluster-name" description:"Name of the cluster to create" default:"mydc"`
	Count              int                      `short:"c" long:"count" description:"Number of instances to create" default:"1"`
	Name               string                   `short:"N" long:"name" description:"Name of the instance to create (since count instances only)"`
	Owner              string                   `short:"o" long:"owner" description:"Owner of the instances"`
	Tags               []string                 `short:"t" long:"tag" description:"Tags to add to the instances, format: k=v"`
	Description        string                   `short:"d" long:"description" description:"Description of the instances"`
	TerminateOnStop    bool                     `short:"T" long:"terminate-on-stop" description:"Terminate the instances when they are stopped"`
	ParallelSSHThreads int                      `short:"p" long:"parallel-ssh-threads" description:"Number of parallel SSH threads to use for the instances" default:"10"`
	SSHKeyName         string                   `short:"k" long:"ssh-key-name" description:"Name of a custom SSH key to use for the instances"`
	AWS                InstancesCreateCmdAws    `group:"AWS" description:"backend-aws" namespace:"aws"`
	GCP                InstancesCreateCmdGcp    `group:"GCP" description:"backend-gcp" namespace:"gcp"`
	Docker             InstancesCreateCmdDocker `group:"Docker" description:"backend-docker" namespace:"docker"`
	NoInstallExpiry    bool                     `long:"no-install-expiry" description:"Do not install the expiry system, even if instance expiry is set"`
	DryRun             bool                     `long:"dry-run" description:"Dry run, print what would be done but don't do it"`
	Help               HelpCmd                  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type InstanceDNS struct {
	DomainID   string `long:"domain-id" description:"The ID of the domain, as defined for DomainID"`
	DomainName string `long:"domain-name" description:"The name of the domain, as defined for DomainID"`
	Name       string `long:"name" description:"The name to assign the instance, if not set, the instance ID will be used"`
	Region     string `long:"region" description:"The region to use for the assignment"`
}

func (d *InstanceDNS) makeInstanceDNS() *backends.InstanceDNS {
	if d.DomainID == "" && d.DomainName == "" {
		return nil
	}
	return &backends.InstanceDNS{
		DomainID:   d.DomainID,
		DomainName: d.DomainName,
		Name:       d.Name,
		Region:     d.Region,
	}
}

type InstancesCreateCmdAws struct {
	ImageName          string        `long:"image" description:"Name of the image to use for the instances"`
	Expire             time.Duration `long:"expire" description:"Expire the instances in a given time, format: 1h, 1d, 1w, 1m, 1y" default:"30h"`
	NetworkPlacement   string        `long:"placement" description:"Network placement of the instances, specify either region name, VPC-ID or subnet-ID; empty=default at first region"`
	InstanceType       string        `long:"instance" description:"Instance type to use for the instances"`
	Disks              []string      `long:"disk" description:"Format: type={gp2|gp3|io2|io1},size={GB}[,iops={cnt}][,throughput={mb/s}][,count=5][,encrypted=true|false]\n; example: type=gp2,size=20 type=gp3,size=100,iops=5000,throughput=200,count=2; first specified volume is the root volume, all subsequent volumes are additional attached volumes" default:"type=gp2,size=20,encrypted=true"`
	Firewalls          []string      `long:"firewall" description:"Extra security group names to assign to the instances"`
	SpotInstance       bool          `long:"spot" description:"Create spot instances"`
	DisablePublicIP    bool          `long:"no-public-ip" description:"Disable public IP assignment to the instances"`
	IAMInstanceProfile string        `long:"instance-profile" description:"IAM instance profile to use for the instances"`
	CustomDNS          InstanceDNS   `group:"Automated Custom Route53 DNS" namespace:"dns" description:"backend-aws"`
	CustomImage        bool          `long:"custom-image" description:"Use a custom image, even if it is not in the inventory; will cause an image lookup during creation; use with --image <image-id>"`
}

type InstancesCreateCmdGcp struct {
	ImageName          string        `long:"image" description:"Name of the image to use for the instances"`
	Expire             time.Duration `long:"expire" description:"Expire the instances in a given time, format: 1h, 1d, 1w, 1m, 1y" default:"30h"`
	Zone               string        `long:"zone" description:"Network placement of the instances, specify a zone name; empty=default at first region"`
	InstanceType       string        `long:"instance" description:"Instance type to use for the instances"`
	Disks              []string      `long:"disk" description:"Format: type={pd-*,hyperdisk-*,local-ssd}[,size={GB}][,iops={cnt}][,throughput={mb/s}][,count=5]\n; example: type=pd-ssd,size=20 type=hyperdisk-balanced,size=20,iops=3060,throughput=155,count=2\n; first specified volume is the root volume, all subsequent volumes are additional attached volumes" default:"type=pd-ssd,size=20"`
	Firewalls          []string      `long:"firewall" description:"Extra firewall names to assign to the instances"`
	SpotInstance       bool          `long:"spot" description:"Create spot instances"`
	IAMInstanceProfile string        `long:"instance-profile" description:"IAM instance profile to use for the instances"`
	MinCPUPlatform     string        `long:"min-cpu-platform" description:"Minimum CPU platform to use for the instances"`
	CustomDNS          InstanceDNS   `group:"Automated Custom GCP DNS" namespace:"dns" description:"backend-gcp"`
	CustomImage        bool          `long:"custom-image" description:"Use a custom image, even if it is not in the inventory; will cause an image lookup during creation; use with --image projects/<project>/global/images/<image>"`
}

type InstancesCreateCmdDocker struct {
	ImageName          string         `long:"image" description:"Name of the image to use for the instances"`
	NetworkName        string         `long:"network" description:"Name of the network to use for the instances; default: default"` // convert to ",VALUE" for docker
	Disks              []string       `long:"disk" description:"Format: {volumeName}:{mountTargetDirectory}; example: volume1:/mnt/data; used for mounting volumes to containers at startup"`
	ExposePorts        []string       `long:"expose" description:"Format: [+]{hostPort}:{containerPort} or host={hostIP:hostPORT},container={containerPORT},incr or [+]{hostIP:hostPORT},{containerPORT}\n; example: 8080:80 or +8080:80 or host=0.0.0.0:8080,container=80,incr\n; + or incr maps to next available port"`
	StopTimeout        *int           `long:"stop-timeout" description:"Container default stop timeout in seconds before force-stop"`
	Privileged         bool           `long:"privileged" description:"Give extended privileges to container"`
	RestartPolicy      string         `long:"restart" description:"Container restart policy: Always, None, OnFailure, UnlessStopped"`
	MaxRestartRetries  int            `long:"max-restart-retries" description:"Maximum number of restart attempts"`
	ShmSize            int64          `long:"shm-size" description:"Size of /dev/shm in bytes"`
	AdvancedConfigPath flags.Filename `long:"advanced-config" description:"Path to JSON file containing advanced Docker container configuration"`
	CustomImage        bool           `long:"custom-image" description:"Use a custom image, even if it is not in the inventory; won't install systemd/ssh and will use docker's exec for attaching"`
}

type InstancesGrowCmd struct {
	InstancesCreateCmd
}

func (c *InstancesGrowCmd) Execute(args []string) error {
	cmd := []string{"instances", "grow"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	_, err = c.CreateInstances(system, system.Backend.GetInventory(), args, "grow")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InstancesCreateCmd) Execute(args []string) error {
	cmd := []string{"instances", "create"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	_, err = c.CreateInstances(system, system.Backend.GetInventory(), args, "create")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InstancesCreateCmd) CreateInstances(system *System, inventory *backends.Inventory, args []string, action string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", action}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	if c.Count < 1 {
		return nil, errors.New("count must be at least 1")
	}

	if c.Name != "" && c.Count > 1 {
		return nil, errors.New("name cannot be specified when count is greater than 1")
	}

	if c.Name != "" {
		if inventory.Instances.WithName(c.Name).Count() > 0 {
			if IsInteractive() {
				choice, quitting, err := choice.Choice("Instance with name "+c.Name+" already exists. What do you want to do?", choice.Items{
					choice.Item("Destroy"),
					choice.Item("Pick a new name"),
					choice.Item("Exit"),
				})
				if err != nil {
					return nil, err
				}
				if quitting {
					return nil, errors.New("aborted")
				}
				switch choice {
				case "Destroy":
					system.Logger.Info("Destroying instance %s", c.Name)
					act := &InstancesDestroyCmd{
						DryRun: false,
						Force:  true,
						Filters: InstancesListFilter{
							ClusterName: c.ClusterName,
						},
					}
					_, err = act.DestroyInstances(system, inventory, nil)
					if err != nil {
						return nil, err
					}
				case "Pick a new name":
					fmt.Printf("Enter a new name for the instance: ")
					reader := bufio.NewReader(os.Stdin)
					c.Name, err = reader.ReadString('\n')
					if err != nil {
						return nil, err
					}
					c.Name = strings.TrimSpace(c.Name)
				case "Exit":
					return nil, errors.New("aborted")
				}
			} else {
				return nil, errors.New("instance with name " + c.Name + " already exists")
			}
		}
	}

	switch action {
	case "create":
		if inventory.Instances.WithClusterName(c.ClusterName).Count() > 0 {
			if IsInteractive() {
				choice, quitting, err := choice.Choice("Cluster "+c.ClusterName+" already exists. What do you want to do?", choice.Items{
					choice.Item("Destroy"),
					choice.Item("Grow"),
					choice.Item("Exit"),
				})
				if err != nil {
					return nil, err
				}
				if quitting {
					return nil, errors.New("aborted")
				}
				switch choice {
				case "Destroy":
					system.Logger.Info("Destroying cluster %s and creating new one", c.ClusterName)
					act := &InstancesDestroyCmd{
						DryRun: false,
						Force:  true,
						Filters: InstancesListFilter{
							ClusterName: c.ClusterName,
						},
					}
					_, err = act.DestroyInstances(system, inventory, nil)
					if err != nil {
						return nil, err
					}
				case "Grow":
					system.Logger.Info("Growing cluster %s with new instances", c.ClusterName)
					action = "grow"
				case "Exit":
					return nil, errors.New("aborted")
				}
			} else {
				return nil, errors.New("cluster " + c.ClusterName + " already exists")
			}
		}
	case "grow":
		if inventory.Instances.WithClusterName(c.ClusterName).Count() == 0 {
			if IsInteractive() {
				choice, quitting, err := choice.Choice("Cluster "+c.ClusterName+" does not exist. What do you want to do?", choice.Items{
					choice.Item("Create"),
					choice.Item("Exit"),
				})
				if err != nil {
					return nil, err
				}
				if quitting {
					return nil, errors.New("aborted")
				}
				switch choice {
				case "Create":
					system.Logger.Info("Creating cluster %s", c.ClusterName)
					action = "create"
				case "Exit":
					return nil, errors.New("aborted")
				}
			} else {
				return nil, errors.New("cluster " + c.ClusterName + " does not exist")
			}
		}
	}

	// sanity-check cluster name and name, must match regex ^[a-zA-Z0-9][a-zA-Z0-9-]*$
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`).MatchString(c.ClusterName) {
		return nil, errors.New("cluster name must match regex ^[a-zA-Z0-9][a-zA-Z0-9-]*$ (only letters, numbers and dashes)")
	}
	if c.Name != "" && !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`).MatchString(c.Name) {
		return nil, errors.New("name must match regex ^[a-zA-Z0-9][a-zA-Z0-9-]*$ (only letters, numbers and dashes)")
	}

	if c.ParallelSSHThreads < 1 {
		return nil, errors.New("parallel-ssh-threads must be at least 1")
	}
	if c.GCP.Expire < 0 {
		return nil, errors.New("GCP expire must be at least 0")
	}
	if c.AWS.Expire < 0 {
		return nil, errors.New("AWS expire must be at least 0")
	}
	if system.Opts.Config.Backend.Type == "aws" && c.AWS.Firewalls != nil && inventory.Firewalls.WithName(c.AWS.Firewalls...).Count() != len(c.AWS.Firewalls) {
		return nil, errors.New("firewall " + strings.Join(c.AWS.Firewalls, ", ") + " does not exist")
	}
	if system.Opts.Config.Backend.Type == "gcp" && c.GCP.Firewalls != nil && inventory.Firewalls.WithName(c.GCP.Firewalls...).Count() != len(c.GCP.Firewalls) {
		return nil, errors.New("firewall " + strings.Join(c.GCP.Firewalls, ", ") + " does not exist")
	}
	if system.Opts.Config.Backend.Type == "docker" {
		if c.Docker.RestartPolicy != "" {
			if !slices.Contains([]string{"Always", "None", "OnFailure", "UnlessStopped"}, c.Docker.RestartPolicy) {
				return nil, errors.New("restart-policy must be one of: Always, None, OnFailure, UnlessStopped")
			}
		}
		if c.Docker.StopTimeout != nil && *c.Docker.StopTimeout < 0 {
			return nil, errors.New("stop-timeout must be at least 0")
		}
		if c.Docker.ShmSize < 0 {
			return nil, errors.New("shm-size must be at least 0")
		}
		if c.Docker.AdvancedConfigPath != "" {
			if _, err := os.Stat(string(c.Docker.AdvancedConfigPath)); os.IsNotExist(err) {
				return nil, errors.New("advanced-config file does not exist")
			}
		}
		for _, disk := range c.Docker.Disks {
			if !strings.Contains(disk, ":") {
				return nil, errors.New("disk must be in the format {volumeName}:{mountTargetDirectory}; example: volume1:/mnt/data")
			}
		}
		for _, expose := range c.Docker.ExposePorts {
			if !strings.Contains(expose, ":") {
				return nil, errors.New("expose must be in the format [+]{hostPort}:{containerPort} or host={hostIP:hostPORT},container={containerPORT},incr or [+]{hostIP:hostPORT},{containerPORT}")
			}
		}
		if c.Docker.NetworkName != "" {
			if inventory.Networks.WithName(c.Docker.NetworkName).Count() == 0 {
				return nil, errors.New("network " + c.Docker.NetworkName + " does not exist")
			}
		}
	}

	itype := ""
	installExpiry := false
	switch system.Opts.Config.Backend.Type {
	case "aws":
		itype = c.AWS.InstanceType
		if c.AWS.ImageName == "" {
			return nil, errors.New("aws: image-name is required")
		}
		if strings.HasPrefix(c.AWS.ImageName, "ami-") {
			if inventory.Images.WithImageID(c.AWS.ImageName).Count() == 0 {
				return nil, errors.New("aws: image ID " + c.AWS.ImageName + " does not exist")
			}
		} else if inventory.Images.WithName(c.AWS.ImageName).Count() == 0 {
			return nil, errors.New("aws: image Name " + c.AWS.ImageName + " does not exist")
		}
		if c.AWS.Expire > 0 && !c.NoInstallExpiry {
			installExpiry = true
		}
	case "gcp":
		itype = c.GCP.InstanceType
		if c.GCP.Zone == "" {
			return nil, errors.New("zone is required")
		}
		regions, err := system.Backend.ListEnabledRegions(backends.BackendType(system.Opts.Config.Backend.Type))
		if err != nil {
			return nil, err
		}
		zoneTest := c.GCP.Zone
		if strings.Count(zoneTest, "-") == 2 {
			zoneTest = zoneTest[:strings.LastIndex(zoneTest, "-")]
		}
		if !slices.Contains(regions, zoneTest) {
			return nil, errors.New("zone " + zoneTest + " is not enabled")
		}
		if c.GCP.ImageName == "" {
			return nil, errors.New("gcp: image-name is required")
		}
		if inventory.Images.WithName(c.GCP.ImageName).Count() == 0 {
			return nil, errors.New("gcp: image " + c.GCP.ImageName + " does not exist")
		}
		if c.GCP.Expire > 0 && !c.NoInstallExpiry {
			installExpiry = true
		}
	case "docker":
		if c.Docker.ImageName == "" {
			return nil, errors.New("docker: image-name is required")
		}
		if !c.Docker.CustomImage && inventory.Images.WithName(c.Docker.ImageName).Count() == 0 {
			return nil, errors.New("docker: image " + c.Docker.ImageName + " does not exist")
		}
	}
	if system.Opts.Config.Backend.Type != "docker" {
		if itype == "" {
			return nil, errors.New("instance type is required")
		}
		instanceTypes, err := system.Backend.GetInstanceTypes(backends.BackendType(system.Opts.Config.Backend.Type))
		if err != nil {
			return nil, err
		}
		found := false
		for _, it := range instanceTypes {
			if it.Name == itype {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("instance type " + itype + " does not exist")
		}
	}

	// Fill CreateInstancesInput struct
	tags := map[string]string{}
	for _, tag := range c.Tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 {
			return nil, errors.New("tag must be in the format k=v")
		}
		tags[parts[0]] = parts[1]
	}
	if system.Opts.Config.Backend.Type == "docker" && c.Docker.CustomImage {
		tags["aerolab.custom.image"] = "true"
	}
	dockerParams := &bdocker.CreateInstanceParams{
		Image:             nil,
		NetworkPlacement:  c.Docker.NetworkName,
		Disks:             c.Docker.Disks,
		Firewalls:         c.Docker.ExposePorts,
		Cmd:               strslice.StrSlice{},
		StopTimeout:       c.Docker.StopTimeout,
		CapAdd:            strslice.StrSlice{},
		CapDrop:           strslice.StrSlice{},
		DNS:               strslice.StrSlice{},
		DNSOptions:        strslice.StrSlice{},
		DNSSearch:         strslice.StrSlice{},
		Privileged:        c.Docker.Privileged,
		SecurityOpt:       strslice.StrSlice{},
		Tmpfs:             map[string]string{},
		RestartPolicy:     c.Docker.RestartPolicy,
		MaxRestartRetries: c.Docker.MaxRestartRetries,
		ShmSize:           c.Docker.ShmSize,
		Sysctls:           map[string]string{},
		Resources:         container.Resources{},
		MaskedPaths:       strslice.StrSlice{},
		ReadonlyPaths:     strslice.StrSlice{},
		SkipSshReadyCheck: c.Docker.CustomImage,
	}
	if c.Docker.AdvancedConfigPath != "" {
		f, err := os.Open(string(c.Docker.AdvancedConfigPath))
		if err != nil {
			return nil, err
		}
		err = json.NewDecoder(f).Decode(dockerParams)
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	imageName := ""
	switch system.Opts.Config.Backend.Type {
	case "aws":
		imageName = c.AWS.ImageName
	case "gcp":
		imageName = c.GCP.ImageName
	case "docker":
		imageName = c.Docker.ImageName
	}
	var expire time.Time
	if system.Opts.Config.Backend.Type == "aws" {
		expire = time.Now().Add(c.AWS.Expire)
	}
	if system.Opts.Config.Backend.Type == "gcp" {
		expire = time.Now().Add(c.GCP.Expire)
	}
	awsCustomImageID := ""
	if c.AWS.CustomImage {
		awsCustomImageID = c.AWS.ImageName
	}
	gcpCustomImageID := ""
	if c.GCP.CustomImage {
		gcpCustomImageID = c.GCP.ImageName
	}
	createInstancesInput := &backends.CreateInstanceInput{
		ClusterName:        c.ClusterName,
		Nodes:              c.Count,
		BackendType:        backends.BackendType(system.Opts.Config.Backend.Type),
		ImageName:          imageName,
		SSHKeyName:         c.SSHKeyName,
		Name:               c.Name,
		Owner:              c.Owner,
		Tags:               tags,
		Expires:            expire,
		Description:        c.Description,
		TerminateOnStop:    c.TerminateOnStop,
		ParallelSSHThreads: c.ParallelSSHThreads,
		BackendSpecificParams: map[backends.BackendType]interface{}{
			"aws": &baws.CreateInstanceParams{
				Image:              nil,
				NetworkPlacement:   c.AWS.NetworkPlacement,
				InstanceType:       itype,
				Disks:              c.AWS.Disks,
				Firewalls:          c.AWS.Firewalls,
				SpotInstance:       c.AWS.SpotInstance,
				DisablePublicIP:    c.AWS.DisablePublicIP,
				IAMInstanceProfile: c.AWS.IAMInstanceProfile,
				CustomDNS:          c.AWS.CustomDNS.makeInstanceDNS(),
				CustomImageID:      awsCustomImageID,
			},
			"gcp": &bgcp.CreateInstanceParams{
				Image:              nil,
				NetworkPlacement:   c.GCP.Zone,
				InstanceType:       itype,
				Disks:              c.GCP.Disks,
				Firewalls:          c.GCP.Firewalls,
				SpotInstance:       c.GCP.SpotInstance,
				IAMInstanceProfile: c.GCP.IAMInstanceProfile,
				CustomDNS:          c.GCP.CustomDNS.makeInstanceDNS(),
				MinCpuPlatform:     c.GCP.MinCPUPlatform,
				CustomImageID:      gcpCustomImageID,
			},
			"docker": dockerParams,
		},
	}
	for k := range createInstancesInput.BackendSpecificParams {
		if string(k) != system.Opts.Config.Backend.Type {
			delete(createInstancesInput.BackendSpecificParams, k)
		}
	}
	if c.DryRun {
		system.Logger.Info("Create Instances Configuration:")
		pf := &prefixWriter{prefix: "  ", logger: system.Logger}
		enc := yaml.NewEncoder(pf)
		enc.SetIndent(2)
		enc.Encode(createInstancesInput)
		pf.Flush()
	}
	if _, ok := createInstancesInput.BackendSpecificParams["aws"]; ok {
		if strings.HasPrefix(c.AWS.ImageName, "ami-") {
			createInstancesInput.BackendSpecificParams["aws"].(*baws.CreateInstanceParams).Image = inventory.Images.WithImageID(c.AWS.ImageName).Describe()[0]
		} else {
			createInstancesInput.BackendSpecificParams["aws"].(*baws.CreateInstanceParams).Image = inventory.Images.WithName(c.AWS.ImageName).Describe()[0]
		}
	}
	if _, ok := createInstancesInput.BackendSpecificParams["gcp"]; ok {
		createInstancesInput.BackendSpecificParams["gcp"].(*bgcp.CreateInstanceParams).Image = inventory.Images.WithName(c.GCP.ImageName).Describe()[0]
	}
	if _, ok := createInstancesInput.BackendSpecificParams["docker"]; ok {
		if !c.Docker.CustomImage {
			createInstancesInput.BackendSpecificParams["docker"].(*bdocker.CreateInstanceParams).Image = inventory.Images.WithName(c.Docker.ImageName).Describe()[0]
		} else {
			createInstancesInput.BackendSpecificParams["docker"].(*bdocker.CreateInstanceParams).Image = &backends.Image{
				Name:      c.Docker.ImageName,
				ZoneName:  "default",
				Public:    false,
				InAccount: true,
				BackendSpecific: &bdocker.ImageDetail{
					Docker: &image.Summary{},
				},
			}
		}
	}
	awsRegion := c.AWS.NetworkPlacement
	var err error
	if system.Opts.Config.Backend.Type == "aws" {
		_, _, awsRegion, err = system.Backend.ResolveNetworkPlacement(backends.BackendTypeAWS, awsRegion)
		if err != nil {
			return nil, err
		}
	}
	if system.Opts.Config.Backend.Type != "docker" {
		system.Logger.Info("Getting price...")
		costPPH, _, err := system.Backend.CreateInstancesGetPrice(createInstancesInput)
		if err != nil {
			return nil, err
		}
		pl, err := system.Backend.GetVolumePrices(backends.BackendType(system.Opts.Config.Backend.Type))
		if err != nil {
			return nil, err
		}
		disks := c.AWS.Disks
		if system.Opts.Config.Backend.Type == "gcp" {
			disks = c.GCP.Disks
		}
		costGB := 0.0
		for _, disk := range disks {
			size := 0
			count := 1
			t := ""
			parts := strings.Split(disk, ",")
			for _, part := range parts {
				parts2 := strings.SplitN(part, "=", 2)
				if len(parts2) != 2 {
					continue
				}
				if parts2[0] == "size" {
					size, _ = strconv.Atoi(parts2[1])
				}
				if parts2[0] == "count" {
					count, _ = strconv.Atoi(parts2[1])
				}
				if parts2[0] == "type" {
					t = parts2[1]
				}
			}
			if size == 0 || t == "" {
				continue
			}
			for _, p := range pl {
				if system.Opts.Config.Backend.Type == "gcp" && !strings.HasPrefix(c.GCP.Zone, p.Region) {
					continue
				}
				if system.Opts.Config.Backend.Type == "aws" && !strings.HasPrefix(awsRegion, p.Region) {
					continue
				}
				if p.Type != t {
					continue
				}
				costGB += float64(size) * float64(count) * p.PricePerGBHour
				break
			}
		}
		costGB = costGB * float64(c.Count)
		system.Logger.Info("  Instance cost: hour: $%.2f, day: $%.2f, month: $%.2f", math.Ceil(costPPH*100)/100, math.Ceil(costPPH*24*100)/100, math.Ceil(costPPH*24*30*100)/100)
		system.Logger.Info("  Storage cost: hour: $%.2f, day: $%.2f, month: $%.2f", math.Ceil(costGB*100)/100, math.Ceil(costGB*24*100)/100, math.Ceil(costGB*24*30*100)/100)
	}
	if c.DryRun {
		system.Logger.Info("Dry run, not creating instances")
		return nil, nil
	}
	if installExpiry {
		wg := new(sync.WaitGroup)
		wg.Add(1)
		defer wg.Wait()
		go func() {
			defer wg.Done()
			instanceRegion := awsRegion
			if strings.Count(instanceRegion, "-") == 2 {
				if len(instanceRegion[strings.LastIndex(instanceRegion, "-")+1:]) == 2 {
					instanceRegion = instanceRegion[:len(instanceRegion)-1]
				}
			}
			if system.Opts.Config.Backend.Type == "gcp" {
				instanceRegion = c.GCP.Zone
				if strings.Count(instanceRegion, "-") == 2 {
					instanceRegion = instanceRegion[:strings.LastIndex(instanceRegion, "-")]
				}
			}
			err = system.Backend.ExpiryInstall(backends.BackendType(system.Opts.Config.Backend.Type), 15, 4, false, false, false, true, instanceRegion)
			if err != nil {
				system.Logger.Error("Error installing expiry system, instances will not auto expire. Details: %s", err)
			}
		}()
	}
	system.Logger.Info("Creating instances, this may take a while...")
	instances, err := system.Backend.CreateInstances(createInstancesInput, time.Duration(10+(c.Count/2))*time.Minute)
	if err != nil {
		return nil, err
	}
	return instances.Instances, nil
}

type prefixWriter struct {
	prefix string
	logger *logger.Logger
	buf    []byte
}

func (w *prefixWriter) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *prefixWriter) Flush() {
	lines := strings.Split(string(w.buf), "\n")
	for _, line := range lines {
		w.logger.Info("%s%s", w.prefix, line)
	}
	w.buf = []byte{}
}

package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bdocker"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClusterCreateCmd struct {
	ClusterName             TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	NodeCount               int             `short:"c" long:"count" description:"Number of nodes" default:"1"`
	CustomConfigFilePath    flags.Filename  `short:"o" long:"customconf" description:"Custom aerospike config file path to install"`
	CustomToolsFilePath     flags.Filename  `short:"z" long:"toolsconf" description:"Custom astools config file path to install"`
	FeaturesFilePath        flags.Filename  `short:"f" long:"featurefile" description:"Features file to install, or directory containing feature files"`
	FeaturesFilePrintDetail bool            `long:"featurefile-printdetail" description:"Print details of discovered features files" hidden:"true"`
	HeartbeatMode           TypeHBMode      `short:"m" long:"mode" description:"Heartbeat mode, one of: mcast|mesh|default" default:"mesh" webchoice:"mesh,mcast,default" simplemode:"false"`
	MulticastAddress        string          `short:"a" long:"mcast-address" description:"Multicast address to change to in config file" simplemode:"false"`
	MulticastPort           string          `short:"p" long:"mcast-port" description:"Multicast port to change to in config file" simplemode:"false"`
	aerospikeVersionSelectorCmd
	AutoStartAerospike    TypeYesNo              `short:"s" long:"start" description:"Auto-start aerospike after creation of cluster (y/n)" default:"y" webchoice:"y,n"`
	NoOverrideClusterName bool                   `short:"O" long:"no-override-cluster-name" description:"Aerolab sets cluster-name by default, use this parameter to not set cluster-name" simplemode:"false"`
	NoSetDNS              bool                   `long:"no-set-dns" description:"set to prevent aerolab from updating resolved to use 1.1.1.1/8.8.8.8 DNS"`
	ScriptEarly           flags.Filename         `short:"X" long:"early-script" description:"optionally specify a script to be installed which will run before every aerospike start" simplemode:"false"`
	ScriptLate            flags.Filename         `short:"Z" long:"late-script" description:"optionally specify a script to be installed which will run after every aerospike stop" simplemode:"false"`
	ParallelThreads       int                    `short:"P" long:"parallel-threads" description:"number of threads to use for parallel operations" default:"10" simplemode:"false"`
	NoVacuumOnFail        bool                   `long:"no-vacuum" description:"if set, will not remove the template instance/container should it fail installation" simplemode:"false"`
	Owner                 string                 `long:"owner" description:"AWS/GCP only: create owner tag with this value" simplemode:"false"`
	PriceOnly             bool                   `long:"price" description:"Only display price of ownership; do not actually create the cluster" simplemode:"false"`
	Aws                   ClusterCreateCmdAws    `group:"AWS" description:"backend-aws"`
	Gcp                   ClusterCreateCmdGcp    `group:"GCP" description:"backend-gcp"`
	Docker                ClusterCreateCmdDocker `group:"Docker" description:"backend-docker"`
	gcpMeta               map[string]string      // TODO: extra tags to apply to the instance being created in gcp
	useAgiFirewall        bool                   // TODO: use special firewall dedicated to AGI (allow ports 443 and 80 from anywhere)
	volExtraTags          map[string]string      // TODO: extra tags to apply to the volume being created
	spotFallback          bool                   // TODO: if doing a spot instance and it fails, try again with on-demand
	efsDelOnError         bool                   // if creating an extra volume, and if instance creation fails, delete the new volume
	Help                  HelpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ClusterCreateCmdAws struct {
	AMI                 string          `short:"A" long:"ami" description:"custom AMI to use (default debian, ubuntu, centos, rocky and amazon are supported in eu-west-1,us-west-1,us-east-1,ap-south-1)" simplemode:"false"`
	InstanceType        guiInstanceType `short:"I" long:"instance-type" description:"instance type to use" default:"" webrequired:"true" webchoice:"method::List"`
	Disk                []string        `long:"aws-disk" description:"EBS disks, format: type={gp2|gp3|io2|io1},size={GB}[,iops={cnt}][,throughput={mb/s}][,count=5] ex: --disk type=gp2,size=20 --disk type=gp3,size=100,iops=5000,throughput=200,count=2 ; first one is root volume ; this parameter can be specified multiple times" default:"type=gp2,size=20"`
	SubnetID            string          `short:"U" long:"subnet-id" description:"subnet-id, availability-zone name, or empty; default: empty: first found in default VPC" simplemode:"false"`
	PublicIP            bool            `short:"L" long:"public-ip" description:"if set, will install systemd script which will set access-address to internal IP and alternate-access-address to allow public IP connections"`
	NoBestPractices     bool            `long:"no-best-practices" description:"set to stop best practices from being executed in setup" simplemode:"false"`
	Tags                []string        `long:"tags" description:"apply custom tags to instances; format: key=value; this parameter can be specified multiple times"`
	SecGroupName        []string        `long:"secgroup-name" description:"Name to use for extra security groups, can be specified multiple times" simplemode:"false"`
	EFSMount            string          `long:"aws-efs-mount" description:"mount EFS volume; format: NAME:MountPath to mount the EFS root" simplemode:"false"`
	EFSCreate           bool            `long:"aws-efs-create" description:"set to create the EFS volume if it doesn't exist" simplemode:"false"`
	EFSOneZone          bool            `long:"aws-efs-onezone" description:"set to force the volume to be in one AZ only; half the price for reduced flexibility with multi-AZ deployments" simplemode:"false"`
	TerminateOnPoweroff bool            `long:"aws-terminate-on-poweroff" description:"if set, when shutdown or poweroff is executed from the instance itself, it will be stopped AND terminated" simplemode:"false"`
	SpotInstance        bool            `long:"aws-spot-instance" description:"set to request a spot instance in place of on-demand"`
	Expires             time.Duration   `long:"aws-expire" description:"length of life of nodes prior to expiry; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry; grow default: match existing cluster" default:"30h"`
	EFSExpires          time.Duration   `long:"aws-efs-expire" description:"if EFS is not remounted using aerolab for this amount of time, it will be expired" simplemode:"false"`
	IAMInstanceProfile  string          `long:"aws-instance-profile" description:"IAM instance profile to use for the instances"`
	InstanceDNS         InstanceDNS     `group:"Automated AWS Route53 DNS" namespace:"aws" description:"backend-aws"`
}

type ClusterCreateCmdGcp struct {
	Image               string          `long:"image" description:"custom source image to use; format: full https selfLink from GCP; see: gcloud compute images list --uri"`
	InstanceType        guiInstanceType `long:"instance" description:"instance type to use" default:"" webrequired:"true" webchoice:"method::List"`
	Disk                []string        `long:"gcp-disk" description:"disks, format: type={pd-*,hyperdisk-*,local-ssd}[,size={GB}][,iops={cnt}][,throughput={mb/s}][,count=5] ex: --disk type=pd-ssd,size=20 --disk type=hyperdisk-balanced,size=20,iops=3060,throughput=155,count=2 ; first in list is root volume, cannot be local-ssd ; this parameter can be specified multiple times" default:"type=pd-ssd,size=20"`
	PublicIP            bool            `long:"external-ip" description:"if set, will install systemd script which will set access-address to internal IP and alternate-access-address to allow public IP connections"`
	Zone                guiZone         `long:"zone" description:"zone name to deploy to" webrequired:"true" webchoice:"method::List"`
	NoBestPractices     bool            `long:"ignore-best-practices" description:"set to stop best practices from being executed in setup" simplemode:"false"`
	Labels              []string        `long:"label" description:"apply custom labels to instances; format: key=value; this parameter can be specified multiple times"`
	FirewallName        []string        `long:"firewall" description:"Name to use for an extra firewall, can be specified multiple times" simplemode:"false"`
	SpotInstance        bool            `long:"gcp-spot-instance" description:"set to request a spot instance in place of on-demand"`
	Expires             time.Duration   `long:"gcp-expire" description:"length of life of nodes prior to expiry; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry; grow default: match existing cluster" default:"30h"`
	VolMount            string          `long:"gcp-vol-mount" description:"mount an extra volume; format: NAME:MountPath" simplemode:"false"`
	VolCreate           bool            `long:"gcp-vol-create" description:"set to create the volume if it doesn't exist" simplemode:"false"`
	VolExpires          time.Duration   `long:"gcp-vol-expire" description:"if the volume is not remounted using aerolab for this amount of time, it will be expired" simplemode:"false"`
	VolDescription      string          `long:"gcp-vol-desc" description:"set volume description field value" simplemode:"false"`
	VolLabels           []string        `long:"gcp-vol-label" description:"apply custom labels to volume; format: key=value; this parameter can be specified multiple times" simplemode:"false"`
	VolSize             int             `long:"gcp-vol-size" description:"set volume size in GB" simplemode:"false"`
	TerminateOnPoweroff bool            `long:"gcp-terminate-on-poweroff" description:"if set, when shutdown or poweroff is executed from the instance itself, it will be stopped AND terminated" simplemode:"false"`
	OnHostMaintenance   string          `long:"on-host-maintenance-policy" description:"optionally specify a custom policy onHostMaintenance"` // TODO: implement this one
	MinCPUPlatform      string          `long:"gcp-min-cpu-platform" description:"set the minimum CPU platform; see https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform"`
	IAMInstanceProfile  string          `long:"gcp-instance-profile" description:"IAM instance profile to use for the instances"`
	InstanceDNS         InstanceDNS     `group:"Automated GCP DNS" namespace:"gcp" description:"backend-gcp"`
}

type ClusterCreateCmdDocker struct {
	ExposePortsToHost       string   `short:"e" long:"expose-ports" description:"If a single machine is being deployed, port forward. Format: HOST_PORT:NODE_PORT,HOST_PORT:NODE_PORT" default:""`
	NoAutoExpose            bool     `long:"no-autoexpose" description:"The easiest way to create multi-node clusters on docker desktop is to expose custom ports; this switch disables the functionality and leaves the listen/advertised IP:PORT in aerospike.conf untouched"`
	CpuLimit                string   `short:"l" long:"cpu-limit" description:"Impose CPU speed limit. Values acceptable could be '1' or '2' or '0.5' etc." default:"" simplemode:"false"`
	RamLimit                string   `short:"t" long:"ram-limit" description:"Limit RAM available to each node, e.g. 500m, or 1g." default:"" simplemode:"false"`
	SwapLimit               string   `short:"w" long:"swap-limit" description:"Limit the amount of total memory (ram+swap) each node can use, e.g. 600m. If ram-limit==swap-limit, no swap is available." default:"" simplemode:"false"`
	NoFILELimit             int      `long:"nofile-limit" description:"for clusters, default will attempt to set to proto-fd-max+5000; you can set this manually or set to -1 to disable the parameter" default:"0" simplemode:"false"`
	NoPatchV7Config         bool     `long:"nopatch-v7-config" description:"for clusters, if a custom aerospike.conf is not provided, by default the config file will be patched to remove bar namespace and set test to file backing; set to disable this" simplemode:"false"`
	Privileged              bool     `short:"B" long:"privileged" description:"Docker only: run container in privileged mode"`
	NetworkName             string   `long:"network" description:"specify a network name to use for non-default docker network; for more info see: aerolab config docker help" default:"" simplemode:"false"`
	ClientType              string   `hidden:"true" description:"specify client type on a cluster, valid for AGI" default:""`
	Labels                  []string `long:"docker-label" description:"apply custom labels to instances; format: key=value; this parameter can be specified multiple times"`
	clientCustomDockerImage string   // TODO: what is this?
}

type ClusterGrowCmd struct {
	ClusterCreateCmd
}

type guiZone string

func (g guiZone) String() string {
	return string(g)
}

// TODO: list, default, error
/*
func (g guiZone) List(system *System) ([][]string, string, error) {
	zones, err := system.Backend.ListZones(system.Opts.Config.Backend.Type, system.Opts.Config.Backend.Region)
	if err != nil {
		return nil, "", err
	}
	z := [][]string{}
	for _, zone := range zones {
		z = append(z, []string{zone, zone})
	}
	return z, "us-central1-a", nil
}
*/

type guiInstanceType string

func (g guiInstanceType) String() string {
	return string(g)
}

// returns a list of instance types, and the default instance type; if chosen instance type exists, returns that instead of default
func (g guiInstanceType) List(system *System) ([][]string, string, error) {
	instanceTypes, err := system.Backend.GetInstanceTypes(backends.BackendType(system.Opts.Config.Backend.Type))
	if err != nil {
		return nil, "", err
	}
	var itypes backends.InstanceTypeList
	def := ""
	for _, it := range instanceTypes {
		if it.Region != system.Opts.Config.Backend.Region {
			continue
		}
		if len(it.Arch) == 0 {
			continue
		}
		if it.Name == string(g) {
			def = it.Name
		}
		itypes = append(itypes, it)
	}
	sep := "."
	if system.Opts.Config.Backend.Type == "gcp" {
		sep = "-"
	}
	sort.Slice(itypes, func(i, j int) bool {
		ni := strings.Split(itypes[i].Name, sep)[0]
		nj := strings.Split(itypes[j].Name, sep)[0]
		if ni < nj {
			return true
		}
		if ni > nj {
			return false
		}
		if itypes[i].CPUs != itypes[j].CPUs {
			return itypes[i].CPUs < itypes[j].CPUs
		}
		return itypes[i].MemoryGiB < itypes[j].MemoryGiB
	})
	types := [][]string{}
	for _, nType := range itypes {
		arch := "amd64"
		if slices.Contains(nType.Arch, backends.ArchitectureARM64) {
			arch = "arm64"
		}
		types = append(types, []string{nType.Name, fmt.Sprintf("%s (ARCH:%s CPUs:%d RamGB:%0.2f NVMe:%d/%dG on-demand:$%0.2f/hr spot:$%0.2f/hr)", nType.Name, arch, nType.CPUs, nType.MemoryGiB, nType.NvmeCount, nType.NvmeTotalSizeGiB, nType.PricePerHour.OnDemand, nType.PricePerHour.Spot)})
	}
	if def == "" {
		def = "e2-standard-4"
		if system.Opts.Config.Backend.Type == "aws" {
			def = "t3a.xlarge"
		}
	}
	return types, def, nil
}

func (c *ClusterCreateCmd) Execute(args []string) error {
	cmd := []string{"cluster", "create"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	instances, err := c.CreateCluster(system, system.Backend.GetInventory(), system.Logger, args, "create")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Created %d instances", instances.Count())
	for _, i := range instances.Describe() {
		fmt.Printf("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s\n", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClusterGrowCmd) Execute(args []string) error {
	cmd := []string{"cluster", "grow"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	instances, err := c.CreateCluster(system, system.Backend.GetInventory(), system.Logger, args, "grow")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Created %d instances", instances.Count())
	for _, i := range instances.Describe() {
		fmt.Printf("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s\n", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClusterCreateCmd) CreateCluster(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", action}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	if err := c.SanityChecks(system, inventory, logger, action); err != nil {
		return nil, err
	}

	if c.Owner == "" {
		c.Owner = currentOwnerUser
	}

	// check if template exists, or create it
	var templateName string
	var arch backends.Architecture
	switch system.Opts.Config.Backend.Type {
	case "docker":
		ar := system.Opts.Config.Backend.Arch
		if ar == "" {
			ar = runtime.GOARCH
		}
		arch.FromString(ar)
	case "aws":
		itypes, err := system.Backend.GetInstanceTypes(backends.BackendTypeAWS)
		if err != nil {
			return nil, err
		}
		for _, i := range itypes {
			if i.Name == c.Aws.InstanceType.String() && len(i.Arch) > 0 {
				arch.FromString(i.Arch[0].String())
				break
			}
		}
	case "gcp":
		itypes, err := system.Backend.GetInstanceTypes(backends.BackendTypeGCP)
		if err != nil {
			return nil, err
		}
		for _, i := range itypes {
			if i.Name == c.Gcp.InstanceType.String() && len(i.Arch) > 0 {
				arch.FromString(i.Arch[0].String())
				break
			}
		}
	}
	if arch.String() == "" {
		return nil, errors.New("architecture not found for instance type " + c.Aws.InstanceType.String() + " or " + c.Gcp.InstanceType.String())
	}

	version, flavor, err := resolveAerospikeServerVersion(c.AerospikeVersion.String())
	if err != nil {
		return nil, err
	}
	av := version.Name + "-" + flavor
	switch flavor {
	case "enterprise":
		c.AerospikeVersion = TypeAerospikeVersion(version.Name)
	case "community":
		c.AerospikeVersion = TypeAerospikeVersion(version.Name + "c")
	case "federal":
		c.AerospikeVersion = TypeAerospikeVersion(version.Name + "f")
	}
	featuresFilePath, err := c.FeaturesFilePathResolve(system, inventory, logger)
	if err != nil {
		return nil, err
	}

	var versionList []string
	switch c.DistroName.String() {
	case "ubuntu":
		versionList = []string{"24.04", "22.04", "20.04", "18.04"}
	case "centos":
		versionList = []string{"9", "8", "7"}
	case "rocky":
		versionList = []string{"9", "8"}
	case "debian":
		versionList = []string{"13", "12", "11", "10", "9", "8"}
	case "amazon":
		versionList = []string{"2023", "2"}
	}
	if c.DistroVersion.String() == "latest" {
		c.DistroVersion = TypeDistroVersion(versionList[0])
	}

	templates := inventory.Images.WithOSName(c.DistroName.String()).WithOSVersion(c.DistroVersion.String()).WithArchitecture(arch).WithTags(
		map[string]string{
			"aerolab.soft.version": av,
			"aerolab.image.type":   "aerospike",
		},
	)
	if templates.Count() == 0 {
		tpl := &TemplateCreateCmd{
			Distro:           c.DistroName.String(),
			DistroVersion:    c.DistroVersion.String(),
			Arch:             arch.String(),
			AerospikeVersion: c.AerospikeVersion.String(),
			Owner:            c.Owner,
			DisablePublicIP:  system.Opts.Config.Backend.Type == "aws" && system.Opts.Config.Backend.AWSNoPublicIps,
			Timeout:          10,
			NoVacuum:         c.NoVacuumOnFail,
			DryRun:           c.PriceOnly,
		}
		_, err := tpl.CreateTemplate(system, inventory, logger.WithPrefix("[template.create] "), nil)
		if err != nil {
			return nil, err
		}
		logger.Info("Refreshing inventory")
		err = system.Backend.RefreshChangedInventory()
		if err != nil {
			return nil, err
		}
		inventory = system.Backend.GetInventory()
		templates := inventory.Images.WithOSName(c.DistroName.String()).WithOSVersion(c.DistroVersion.String()).WithArchitecture(arch).WithTags(
			map[string]string{
				"aerolab.soft.version": av,
				"aerolab.image.type":   "aerospike",
			},
		)
		templateName = templates.Describe()[0].Name
	} else {
		templateName = templates.Describe()[0].Name
		logger.Info("Aerospike Version: %s, Flavor: %s, Distro: %s, OS Version: %s, Arch: %s", c.AerospikeVersion, flavor, c.DistroName.String(), c.DistroVersion.String(), arch.String())
	}

	// run instances create
	var tags []string
	var terminateOnStop bool
	stopTimeout := 60
	if system.Opts.Config.Backend.Type == "aws" {
		tags = c.Aws.Tags
		terminateOnStop = c.Aws.TerminateOnPoweroff
		if !c.Aws.PublicIP {
			logger.Warn("Public IP access address is not enabled for this cluster, you will be unable to connect to the instances from outside AWS.")
			logger.Warn("To enable public IP access address, run: aerolab cluster add public-ip -n %s", c.ClusterName.String())
		}
	} else if system.Opts.Config.Backend.Type == "gcp" {
		tags = c.Gcp.Labels
		terminateOnStop = c.Gcp.TerminateOnPoweroff
		if !c.Gcp.PublicIP {
			logger.Warn("Public IP access address is not enabled for this cluster, you will be unable to connect to the instances from outside GCP.")
			logger.Warn("To enable public IP access address, run: aerolab cluster add public-ip -n %s", c.ClusterName.String())
		}
	}
	dockerExposePorts := []string{"+3100:3000"}
	if c.Docker.ExposePortsToHost != "" {
		dockerExposePorts = append([]string{"+3100:3000"}, strings.Split(c.Docker.ExposePortsToHost, ",")...)
	}

	inst := &InstancesCreateCmd{
		ClusterName:        c.ClusterName.String(),
		Count:              c.NodeCount,
		Name:               "",
		Owner:              c.Owner,
		Type:               "aerospike",
		Tags:               tags,
		Description:        "AeroLab managed aerospike server cluster",
		TerminateOnStop:    terminateOnStop,
		ParallelSSHThreads: c.ParallelThreads,
		SSHKeyName:         "",
		OS:                 c.DistroName.String(),
		Version:            c.DistroVersion.String(),
		Arch:               arch.String(),
		ImageType:          "aerospike",
		ImageVersion:       av,
		AWS: InstancesCreateCmdAws{
			Expire:             c.Aws.Expires,
			NetworkPlacement:   c.Aws.SubnetID,
			InstanceType:       c.Aws.InstanceType.String(),
			Disks:              c.Aws.Disk,
			Firewalls:          c.Aws.SecGroupName,
			SpotInstance:       c.Aws.SpotInstance,
			DisablePublicIP:    system.Opts.Config.Backend.AWSNoPublicIps,
			IAMInstanceProfile: c.Aws.IAMInstanceProfile,
			CustomDNS:          c.Aws.InstanceDNS,
		},
		GCP: InstancesCreateCmdGcp{
			Expire:             c.Gcp.Expires,
			Zone:               c.Gcp.Zone.String(),
			InstanceType:       c.Gcp.InstanceType.String(),
			Disks:              c.Gcp.Disk,
			Firewalls:          c.Gcp.FirewallName,
			SpotInstance:       c.Gcp.SpotInstance,
			IAMInstanceProfile: c.Gcp.IAMInstanceProfile,
			MinCPUPlatform:     c.Gcp.MinCPUPlatform,
			CustomDNS:          c.Gcp.InstanceDNS,
		},
		Docker: InstancesCreateCmdDocker{
			ImageName:          templateName,
			NetworkName:        c.Docker.NetworkName,
			Disks:              nil,
			ExposePorts:        dockerExposePorts,
			StopTimeout:        &stopTimeout,
			Privileged:         c.Docker.Privileged,
			RestartPolicy:      "",
			MaxRestartRetries:  0,
			ShmSize:            0,
			AdvancedConfigPath: "",
		},
		NoInstallExpiry: false,
		DryRun:          false,
	}
	oldInst := backends.InstanceList{}
	if action == "grow" {
		oldInst = inventory.Instances.WithClusterName(c.ClusterName.String()).WithState(backends.LifeCycleStateRunning).Describe()
	}
	if c.PriceOnly {
		inst.DryRun = true
	}

	// create extra volume if requested
	deleteVolumeOnFail := false
	var volName string
	var volMountPath string
	if !c.PriceOnly {
		switch system.Opts.Config.Backend.Type {
		case "aws":
			if c.Aws.EFSMount != "" {
				efsDetail := strings.Split(c.Aws.EFSMount, ":")
				if len(efsDetail) != 2 {
					return nil, fmt.Errorf("invalid efs mount: %s", c.Aws.EFSMount)
				}
				volName = efsDetail[0]
				volMountPath = efsDetail[1]
				if inventory.Volumes.WithType(backends.VolumeTypeSharedDisk).WithName(volName).Count() == 0 {
					if !c.Aws.EFSCreate {
						return nil, fmt.Errorf("efs volume %s does not exist", volName)
					}
					if c.efsDelOnError {
						deleteVolumeOnFail = true
					}
					// create efs volume
					volume := &VolumesCreateCmd{
						Name:            volName,
						Description:     fmt.Sprintf("EFS volume for %s", c.ClusterName.String()),
						Owner:           c.Owner,
						Tags:            tags,
						VolumeType:      "shared",
						NoInstallExpiry: false,
						AWS: VolumesCreateCmdAws{
							SizeGiB:           0,
							Placement:         c.Aws.SubnetID,
							DiskType:          "shared",
							Iops:              0,
							Throughput:        0,
							Encrypted:         true,
							SharedDiskOneZone: c.Aws.EFSOneZone,
							Expire:            c.Aws.EFSExpires,
						},
						DryRun: false,
					}
					_, err := volume.CreateVolumes(system, inventory, nil)
					if err != nil {
						return nil, err
					}
					inventory, err = system.Backend.GetRefreshedInventory()
					if err != nil {
						return nil, err
					}
				}
			}
		case "gcp":
			if c.Gcp.VolMount != "" {
				volDetail := strings.Split(c.Gcp.VolMount, ":")
				if len(volDetail) != 2 {
					return nil, fmt.Errorf("invalid gcp volume mount: %s", c.Gcp.VolMount)
				}
				volName = volDetail[0]
				volMountPath = volDetail[1]
				if inventory.Volumes.WithType(backends.VolumeTypeAttachedDisk).WithName(volName).Count() == 0 {
					if !c.Gcp.VolCreate {
						return nil, fmt.Errorf("gcp volume %s does not exist", volName)
					}
					if c.efsDelOnError {
						deleteVolumeOnFail = true
					}
					volume := &VolumesCreateCmd{
						Name:            volName,
						Description:     c.Gcp.VolDescription,
						Owner:           c.Owner,
						Tags:            c.Gcp.VolLabels,
						VolumeType:      "attached",
						NoInstallExpiry: false,
						GCP: VolumesCreateCmdGcp{
							SizeGiB:    c.Gcp.VolSize,
							Zone:       c.Gcp.Zone.String(),
							DiskType:   "pd-ssd",
							Iops:       0,
							Throughput: 0,
							Expire:     c.Gcp.VolExpires,
						},
						DryRun: false,
					}
					_, err := volume.CreateVolumes(system, inventory, nil)
					if err != nil {
						return nil, err
					}
					inventory, err = system.Backend.GetRefreshedInventory()
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}

	defer func() {
		if deleteVolumeOnFail {
			system.Logger.Info("Deleting volume %s on error", volName)
			err := inventory.Volumes.WithName(volName).DeleteVolumes(inventory.Firewalls.Describe(), 10*time.Minute)
			if err != nil {
				system.Logger.Error("Error deleting volume %s on error: %s", volName, err)
			}
		}
	}()

	// create instances
	newInst, err := inst.CreateInstances(system, inventory, args, action)
	if err != nil {
		return nil, err
	}
	if c.PriceOnly {
		return nil, nil
	}
	type instanceList struct {
		inst  *backends.Instance
		sftp  *sshexec.ClientConf
		isNew bool
		IP    backends.IP
	}
	instances := []instanceList{}
	for _, i := range oldInst {
		sftp, err := i.GetSftpConfig("root")
		if err != nil {
			return nil, err
		}
		instances = append(instances, instanceList{
			inst:  i,
			sftp:  sftp,
			isNew: false,
			IP:    i.IP,
		})
	}
	for _, i := range newInst.Describe() {
		sftp, err := i.GetSftpConfig("root")
		if err != nil {
			return nil, err
		}
		instances = append(instances, instanceList{
			inst:  i,
			sftp:  sftp,
			isNew: true,
			IP:    i.IP,
		})
	}
	if volName != "" {
		for _, i := range newInst.Describe() {
			err := inventory.Volumes.WithName(volName).Attach(i, &backends.VolumeAttachShared{
				MountTargetDirectory: volMountPath,
				FIPS:                 false, // TODO: add FIPS support option
			}, 10*time.Minute)
			if err != nil {
				return nil, err
			}
		}
	}
	intIps := []string{}
	for _, i := range instances {
		intIps = append(intIps, i.IP.Private)
	}
	var errs []error
	parallelize.ForEachLimit(instances, c.ParallelThreads, func(i instanceList) {
		// connect
		client, err := sshexec.NewSftp(i.sftp)
		if err != nil {
			errs = append(errs, err)
			return
		}
		defer client.Close()
		// read existing config
		var conf []byte
		if c.CustomConfigFilePath != "" && i.isNew {
			conf, err = os.ReadFile(string(c.CustomConfigFilePath))
			if err != nil {
				errs = append(errs, err)
				return
			}
		} else {
			var buf bytes.Buffer
			err = client.ReadFile(&sshexec.FileReader{
				SourcePath:  "/etc/aerospike/aerospike.conf",
				Destination: &buf,
			})
			if err != nil {
				errs = append(errs, err)
				return
			}
			conf = buf.Bytes()
		}
		// fix mesh
		newConfig, err := fixHeartbeats(conf, c.HeartbeatMode.String(), c.MulticastAddress, c.MulticastPort, intIps)
		if err != nil {
			errs = append(errs, err)
			return
		}
		// set cluster name
		if !c.NoOverrideClusterName && i.isNew {
			newConfig, err = setClusterName(newConfig, c.ClusterName.String())
			if err != nil {
				errs = append(errs, err)
				return
			}
		}
		if i.isNew && system.Opts.Config.Backend.Type == "docker" {
			port := "3000"
			if len(i.inst.BackendSpecific.(*bdocker.InstanceDetail).Docker.Ports) > 0 {
				for _, p := range i.inst.BackendSpecific.(*bdocker.InstanceDetail).Docker.Ports {
					if p.PublicPort >= 3100 && p.PublicPort <= 3199 {
						port = strconv.Itoa(int(p.PublicPort))
					}
				}
			}
			newConfig, err = patchAccessAddressForDocker(newConfig, port, i.IP.Private)
			if err != nil {
				errs = append(errs, err)
				return
			}
			if !c.Docker.NoPatchV7Config && i.isNew {
				newConfig, err = patchDockerNamespacesV7(newConfig)
				if err != nil {
					errs = append(errs, err)
					return
				}
			}
		}
		// fix access-address
		if i.isNew {
			newConfig, err = fixAccessAddress(newConfig, i.inst.IP.Private)
			if err != nil {
				errs = append(errs, err)
				return
			}
		}
		// write new config
		err = client.WriteFile(false, &sshexec.FileWriter{
			DestPath:    "/etc/aerospike/aerospike.conf",
			Source:      bytes.NewReader([]byte(newConfig)),
			Permissions: 0644,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}
		// upload existing tools file path if specified
		if c.CustomToolsFilePath != "" && i.isNew {
			tools, err := os.ReadFile(string(c.CustomToolsFilePath))
			if err != nil {
				errs = append(errs, err)
				return
			}
			err = client.WriteFile(false, &sshexec.FileWriter{
				DestPath:    "/etc/aerospike/astools.conf",
				Source:      bytes.NewReader(tools),
				Permissions: 0644,
			})
			if err != nil {
				errs = append(errs, err)
				return
			}
		}
		// upload existing features file path if specified
		if featuresFilePath != "" && i.isNew {
			features, err := os.ReadFile(string(featuresFilePath))
			if err != nil {
				errs = append(errs, err)
				return
			}
			err = client.WriteFile(false, &sshexec.FileWriter{
				DestPath:    "/etc/aerospike/features.conf",
				Source:      bytes.NewReader(features),
				Permissions: 0644,
			})
			if err != nil {
				errs = append(errs, err)
				return
			}
		}
		deployScript := "#!/bin/bash\n"
		if i.isNew && system.Opts.Config.Backend.Type == "gcp" && c.Gcp.TerminateOnPoweroff {
			scr, err := scripts.ReadFile("scripts/cluster-create/gcp-terminate-on-poweroff.sh.tpl")
			if err != nil {
				errs = append(errs, err)
				return
			}
			deployScript = deployScript + "\n" + string(scr)
		}
		if i.isNew && system.Opts.Config.Backend.Type != "docker" {
			if system.Opts.Config.Backend.Type == "aws" && c.Aws.PublicIP {
				scr, err := scripts.ReadFile("scripts/cluster-create/aws-public-ip.sh.tpl")
				if err != nil {
					errs = append(errs, err)
					return
				}
				deployScript = deployScript + "\n" + string(scr)
			}
			if system.Opts.Config.Backend.Type == "gcp" && c.Gcp.PublicIP {
				scr, err := scripts.ReadFile("scripts/cluster-create/gcp-public-ip.sh.tpl")
				if err != nil {
					errs = append(errs, err)
					return
				}
				deployScript = deployScript + "\n" + string(scr)
			}
			if (system.Opts.Config.Backend.Type == "aws" && !c.Aws.NoBestPractices) || (system.Opts.Config.Backend.Type == "gcp" && !c.Gcp.NoBestPractices) {
				scr, err := scripts.ReadFile("scripts/cluster-create/thp-disable.sh.tpl")
				if err != nil {
					errs = append(errs, err)
					return
				}
				deployScript = deployScript + "\n" + string(scr)
			}
		}
		scr, err := scripts.ReadFile("scripts/cluster-create/early-late-scripts.sh.tpl")
		if err != nil {
			errs = append(errs, err)
			return
		}
		deployScript = deployScript + "\n" + string(scr)
		if i.isNew && !c.NoSetDNS {
			scr, err := scripts.ReadFile("scripts/cluster-create/resolvconf-patch.sh.tpl")
			if err != nil {
				errs = append(errs, err)
				return
			}
			deployScript = deployScript + "\n" + string(scr)
		}
		if c.ScriptEarly != "" {
			scr, err := os.ReadFile(string(c.ScriptEarly))
			if err != nil {
				errs = append(errs, err)
				return
			}
			err = client.WriteFile(false, &sshexec.FileWriter{
				DestPath:    "/usr/local/bin/early.sh",
				Source:      bytes.NewReader(scr),
				Permissions: 0755,
			})
			if err != nil {
				errs = append(errs, err)
				return
			}
		}
		if c.ScriptLate != "" {
			scr, err := os.ReadFile(string(c.ScriptLate))
			if err != nil {
				errs = append(errs, err)
				return
			}
			err = client.WriteFile(false, &sshexec.FileWriter{
				DestPath:    "/usr/local/bin/late.sh",
				Source:      bytes.NewReader(scr),
				Permissions: 0755,
			})
			if err != nil {
				errs = append(errs, err)
				return
			}
		}
		// handle aerospike auto-start
		if i.isNew && (c.AutoStartAerospike == "true" || c.AutoStartAerospike == "yes" || c.AutoStartAerospike == "y") {
			deployScript = deployScript + "\n" + "systemctl start aerospike\n"
		}
		// upload and run the deploy script
		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/opt/aerolab/scripts/cluster-create.sh",
			Source:      bytes.NewReader([]byte(deployScript)),
			Permissions: 0755,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}
		outputs := i.inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command: []string{"bash", "/opt/aerolab/scripts/cluster-create.sh"},
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if outputs == nil {
			errs = append(errs, fmt.Errorf("no output from deploy script"))
			return
		}
		if outputs.Output.Err != nil {
			errs = append(errs, fmt.Errorf("%s\n%s\n%s", outputs.Output.Err.Error(), string(outputs.Output.Stdout), string(outputs.Output.Stderr)))
			return
		}
	})
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	deleteVolumeOnFail = false
	return newInst, nil
}

// port is the docker-exposed port
func patchAccessAddressForDocker(in []byte, port string, privateIp string) (out []byte, err error) {
	s, err := aeroconf.Parse(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	if s.Type("network") == aeroconf.ValueNil {
		s.NewStanza("network")
	}
	if s.Stanza("network").Type("service") == aeroconf.ValueNil {
		s.Stanza("network").NewStanza("service")
	}
	err = s.Stanza("network").Stanza("service").SetValue("alternate-access-port", port)
	if err != nil {
		return nil, err
	}
	err = s.Stanza("network").Stanza("service").SetValue("access-address", privateIp)
	if err != nil {
		return nil, err
	}
	err = s.Stanza("network").Stanza("service").SetValue("alternate-access-address", "127.0.0.1")
	if err != nil {
		return nil, err
	}
	err = s.Stanza("network").Stanza("service").SetValue("tls-alternate-access-port", port)
	if err != nil {
		return nil, err
	}
	err = s.Stanza("network").Stanza("service").SetValue("tls-access-address", privateIp)
	if err != nil {
		return nil, err
	}
	err = s.Stanza("network").Stanza("service").SetValue("tls-alternate-access-address", "127.0.0.1")
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	err = s.Write(buf, "", "    ", true)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func patchDockerNamespacesV7(conf []byte) (newconf []byte, err error) {
	ac, err := aeroconf.Parse(bytes.NewReader(conf))
	if err != nil {
		return conf, err
	}

	// remove all namespaces
	for _, key := range ac.ListKeys() {
		if ac.Type(key) != aeroconf.ValueStanza {
			continue
		}
		if !strings.HasPrefix(key, "namespace ") {
			continue
		}
		err = ac.Delete(key)
		if err != nil {
			return conf, err
		}
	}

	// add test namespace, file backing
	err = ac.NewStanza("namespace test")
	if err != nil {
		return conf, err
	}
	st := ac.Stanza("namespace test")
	err = st.SetValue("default-ttl", "0")
	if err != nil {
		return conf, err
	}
	err = st.SetValue("replication-factor", "2")
	if err != nil {
		return conf, err
	}
	err = st.SetValue("index-stage-size", "128M")
	if err != nil {
		return conf, err
	}
	err = st.SetValue("sindex-stage-size", "128M")
	if err != nil {
		return conf, err
	}
	err = st.NewStanza("storage-engine device")
	if err != nil {
		return conf, err
	}
	st = st.Stanza("storage-engine device")
	err = st.SetValue("file", "/opt/aerospike/data/test.dat")
	if err != nil {
		return conf, err
	}
	err = st.SetValue("filesize", "4G")
	if err != nil {
		return conf, err
	}

	// export new config
	var buf bytes.Buffer
	err = ac.Write(&buf, "", "    ", true)
	if err != nil {
		return conf, err
	}
	conf = buf.Bytes()
	return conf, nil
}

func (c *ClusterCreateCmd) SanityChecks(system *System, inventory *backends.Inventory, logger *logger.Logger, action string) error {
	if c.CustomConfigFilePath != "" {
		if _, err := os.Stat(string(c.CustomConfigFilePath)); os.IsNotExist(err) {
			return errors.New("custom-config-file path " + string(c.CustomConfigFilePath) + " does not exist")
		}
	}

	if c.CustomToolsFilePath != "" {
		if _, err := os.Stat(string(c.CustomToolsFilePath)); os.IsNotExist(err) {
			return errors.New("custom-tools-file path " + string(c.CustomToolsFilePath) + " does not exist")
		}
	}
	if c.FeaturesFilePath != "" {
		if _, err := os.Stat(string(c.FeaturesFilePath)); os.IsNotExist(err) {
			return errors.New("features-file path " + string(c.FeaturesFilePath) + " does not exist")
		}
	}
	return nil
}

func (c *ClusterCreateCmd) FeaturesFilePathResolve(system *System, inventory *backends.Inventory, logger *logger.Logger) (string, error) {
	vers := strings.Split(c.AerospikeVersion.String(), ".")
	aver_major, _ := strconv.Atoi(vers[0])
	aver_minor, _ := strconv.Atoi(vers[1])

	type featureFile struct {
		name       string    // fileName
		version    string    // feature-key-version              1
		validUntil time.Time // valid-until-date                 2024-01-15
		serial     int       // serial-number                    680515527
		maxNodes   int       // asdb-cluster-nodes-limit 0
	}
	if strings.HasSuffix(c.AerospikeVersion.String(), "c") {
		return "", nil
	}
	ff, err := os.Stat(string(c.FeaturesFilePath))
	if err != nil {
		logger.Warn("No feature file provided, using embedded single-node feature file; multi-node clusters will not form")
		return "", nil
	}
	fffileList := []string{}
	ffFiles := []featureFile{}
	if ff.IsDir() {
		ffDir, err := os.ReadDir(string(c.FeaturesFilePath))
		if err != nil {
			return "", errors.New("Features file path director read failed: " + err.Error())
		}
		for _, ffFile := range ffDir {
			if ffFile.IsDir() {
				continue
			}
			fffileList = append(fffileList, path.Join(string(c.FeaturesFilePath), ffFile.Name()))
		}
	} else {
		fffileList = []string{string(c.FeaturesFilePath)}
	}
	for _, ffFile := range fffileList {
		ffc, err := os.ReadFile(ffFile)
		if err != nil {
			return "", errors.New("Features file read failed for " + ffFile + ": " + err.Error())
		}
		// populate ffFiles from ffc contents for unexpired features files, WARN on finding expired ones
		ffFiles1 := featureFile{
			name: ffFile,
		}
		scanner := bufio.NewScanner(bytes.NewReader(ffc))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "feature-key-version") {
				ffVer := strings.TrimLeft(strings.TrimPrefix(line, "feature-key-version"), " \t")
				ffVer = strings.TrimRight(ffVer, " \t\n")
				ffFiles1.version = ffVer
			} else if strings.HasPrefix(line, "valid-until-date") {
				ffDate := strings.TrimLeft(strings.TrimPrefix(line, "valid-until-date"), " \t")
				ffDateSplit := strings.Split(strings.TrimRight(ffDate, " \t\n"), "-")
				ffy := 3000
				ffm := 1
				ffd := 1
				if len(ffDateSplit) == 3 {
					ffy, err = strconv.Atoi(ffDateSplit[0])
					if err != nil {
						ffy = 3000
					}
					ffm, err = strconv.Atoi(ffDateSplit[1])
					if err != nil {
						ffm = 1
					}
					ffd, err = strconv.Atoi(ffDateSplit[2])
					if err != nil {
						ffd = 1
					}
				}
				// 2024-01-15
				ffFiles1.validUntil = time.Date(ffy, time.Month(ffm), ffd, 0, 0, 0, 0, time.UTC)
			} else if strings.HasPrefix(line, "serial-number") {
				ffser := strings.TrimLeft(strings.TrimPrefix(line, "serial-number"), " \t")
				ffser = strings.TrimRight(ffser, " \t\n")
				ffFiles1.serial, _ = strconv.Atoi(ffser)
			} else if strings.HasPrefix(line, "asdb-cluster-nodes-limit") {
				ffser := strings.TrimLeft(strings.TrimPrefix(line, "asdb-cluster-nodes-limit"), " \t")
				ffser = strings.TrimRight(ffser, " \t\n")
				ffFiles1.maxNodes, _ = strconv.Atoi(ffser)
			}
		}
		if ffFiles1.version != "" {
			if ffFiles1.validUntil.IsZero() {
				ffFiles1.validUntil = time.Now().AddDate(0, 0, 1)
			}
			if ffFiles1.validUntil.Before(time.Now()) {
				logger.Warn("WARNING: Expired feature file found in FeaturesFilePath, ignoring: " + ffFile)
				continue
			}
			ffFiles = append(ffFiles, ffFiles1)
		}
	}
	foundFile := featureFile{}
	var featuresFilePath string
	if (aver_major == 6 && aver_minor >= 3) || aver_major > 6 {
		for _, ffFile := range ffFiles {
			if ffFile.version != "2" {
				continue
			}
			if ffFile.serial > foundFile.serial && ffFile.validUntil.After(time.Now()) {
				foundFile = ffFile
			} else if ffFile.serial == foundFile.serial && ffFile.validUntil.After(foundFile.validUntil) && ffFile.validUntil.After(time.Now()) {
				foundFile = ffFile
			}
		}
		if foundFile.name == "" {
			logger.Warn("A valid features file v2 not found in the configured FeaturesFilePath")
		}
		featuresFilePath = foundFile.name
	} else if (aver_major == 5 && aver_minor <= 4) || (aver_major == 4 && aver_minor > 5) {
		for _, ffFile := range ffFiles {
			if ffFile.version != "1" {
				continue
			}
			if ffFile.serial > foundFile.serial && ffFile.validUntil.After(time.Now()) {
				foundFile = ffFile
			} else if ffFile.serial == foundFile.serial && ffFile.validUntil.After(foundFile.validUntil) && ffFile.validUntil.After(time.Now()) {
				foundFile = ffFile
			}
		}
		if foundFile.name == "" {
			logger.Warn("A valid features file v1 not found in the configured FeaturesFilePath")
		}
		featuresFilePath = foundFile.name
	} else if (aver_major == 6 && aver_minor < 3) || (aver_major == 5 && aver_minor > 4) {
		for _, ffFile := range ffFiles {
			if ffFile.version == "2" && (foundFile.version == "1" || foundFile.version == "") {
				foundFile = ffFile
				continue
			}
			if ffFile.serial > foundFile.serial && ffFile.validUntil.After(time.Now()) {
				foundFile = ffFile
			} else if ffFile.serial == foundFile.serial && ffFile.validUntil.After(foundFile.validUntil) && ffFile.validUntil.After(time.Now()) {
				foundFile = ffFile
			}
		}
		if foundFile.name == "" {
			logger.Warn("A valid features file not found in the configured FeaturesFilePath")
		}
		featuresFilePath = foundFile.name
	}
	if c.FeaturesFilePrintDetail {
		for _, ffFile := range ffFiles {
			logger.Info("feature-file=%s version=%s valid-until=%s serial=%d maxNodes=%d", ffFile.name, ffFile.version, ffFile.validUntil.String(), ffFile.serial, ffFile.maxNodes)
		}
	}
	if string(featuresFilePath) == "" && (aver_major == 5 || (aver_major == 4 && aver_minor > 5) || (aver_major == 6 && aver_minor == 0)) {
		logger.Warn("you are attempting to install version 4.6-6.0 and a valid features file could not be found. This will not work. You can either provide a feature file by using the '-f' switch, or configure it as default by using:\n\n$ aerolab config defaults -k '*.FeaturesFilePath' -v /path/to/features.conf\n\nPress ENTER if you still wish to proceed")
		var ignoreMe string
		fmt.Scanln(&ignoreMe)
	} else if string(featuresFilePath) == "" && aver_major == 6 && aver_minor > 0 {
		if c.NodeCount == 1 {
			logger.Warn("FeaturesFilePath does not contain a valid feature file. Using embedded features files.")
		} else {
			logger.Warn("you are attempting to install more than 1 node and a valid features file could not be found. This will not work. You can either provide a feature file by using the '-f' switch, or configure it as default by using:\n\n$ aerolab config defaults -k '*.FeaturesFilePath' -v /path/to/features.conf\n\nPress ENTER if you still wish to proceed")
			var ignoreMe string
			fmt.Scanln(&ignoreMe)
		}
	} else if string(featuresFilePath) != "" && foundFile.maxNodes > 0 && foundFile.maxNodes < c.NodeCount {
		logger.Warn("selected cluster size %d is larger than the feature file allows (%d). This will cause the cluster to not form.\n\nPress ENTER if you still wish to proceed", c.NodeCount, foundFile.maxNodes)
		var ignoreMe string
		fmt.Scanln(&ignoreMe)
	} else if (aver_major == 4 && aver_minor > 5) || aver_major > 4 {
		logger.Info("Features file: %s", featuresFilePath)
	} else {
		featuresFilePath = ""
	}
	return featuresFilePath, nil
}

func setClusterName(conf []byte, name string) (data []byte, err error) {
	cfg, err := aeroconf.Parse(bytes.NewReader(conf))
	if err != nil {
		return nil, err
	}
	if cfg.Type("service") == aeroconf.ValueNil {
		err = cfg.NewStanza("service")
		if err != nil {
			return nil, err
		}
	}
	err = cfg.Stanza("service").SetValue("cluster-name", name)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(nil)
	err = cfg.Write(buf, "", "  ", true)
	if err != nil {
		return nil, err
	}
	data = buf.Bytes()
	return data, nil
}

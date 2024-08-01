package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/gcplabels"
	"github.com/aerospike/aerolab/parallelize"
	"github.com/bestmethod/inslice"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type guiZone string

func (g guiZone) String() string {
	return string(g)
}

// list, default, error
func (g guiZone) List(zone string) ([][]string, string, error) {
	zones, err := listZones()
	if err != nil {
		return nil, "", err
	}
	z := [][]string{}
	for _, zone := range zones {
		z = append(z, []string{zone, zone})
	}
	return z, "us-central1-a", nil
}

type guiInstanceType string

func (g guiInstanceType) String() string {
	return string(g)
}

func (g guiInstanceType) List(zone string) ([][]string, string, error) {
	out, err := exec.Command(os.Args[0], "inventory", "instance-types", "-j", "--zone", zone).CombinedOutput()
	if err != nil {
		return nil, "", err
	}
	var itypesAmd []instanceType
	var itypesArm []instanceType
	err = json.Unmarshal(out, &itypesAmd)
	if err != nil {
		return nil, "", err
	}
	out, err = exec.Command(os.Args[0], "inventory", "instance-types", "-j", "--arm", "--zone", zone).CombinedOutput()
	if err != nil {
		return nil, "", err
	}
	err = json.Unmarshal(out, &itypesArm)
	if err != nil {
		return nil, "", err
	}
	itypes := []instanceType{}
	for _, itype := range append(itypesAmd, itypesArm...) {
		found := false
		for _, i := range itypes {
			if i.InstanceName == itype.InstanceName {
				found = true
				break
			}
		}
		if !found {
			itypes = append(itypes, itype)
		}
	}
	sep := "."
	if a.opts.Config.Backend.Type == "gcp" {
		sep = "-"
	}
	sort.Slice(itypes, func(i, j int) bool {
		ni := strings.Split(itypes[i].InstanceName, sep)[0]
		nj := strings.Split(itypes[j].InstanceName, sep)[0]
		if ni < nj {
			return true
		}
		if ni > nj {
			return false
		}
		if itypes[i].CPUs != itypes[j].CPUs {
			return itypes[i].CPUs < itypes[j].CPUs
		}
		return itypes[i].RamGB < itypes[j].RamGB
	})
	types := [][]string{}
	for _, nType := range itypes {
		arch := "amd64"
		if nType.IsArm {
			arch = "arm64"
		}
		types = append(types, []string{nType.InstanceName, fmt.Sprintf("%s (ARCH:%s CPUs:%d RamGB:%0.2f NVMe:%d/%0.2fG on-demand:$%0.2f/hr spot:$%0.2f/hr)", nType.InstanceName, arch, nType.CPUs, nType.RamGB, nType.EphemeralDisks, nType.EphemeralDiskTotalSizeGB, nType.PriceUSD, nType.SpotPriceUSD)})
	}
	def := "e2-standard-4"
	if a.opts.Config.Backend.Type == "aws" {
		def = "t3a.xlarge"
	}
	return types, def, nil
}

type clusterCreateCmd struct {
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
	AutoStartAerospike    TypeYesNo      `short:"s" long:"start" description:"Auto-start aerospike after creation of cluster (y/n)" default:"y" webchoice:"y,n"`
	NoOverrideClusterName bool           `short:"O" long:"no-override-cluster-name" description:"Aerolab sets cluster-name by default, use this parameter to not set cluster-name" simplemode:"false"`
	NoSetHostname         bool           `short:"H" long:"no-set-hostname" description:"by default, hostname of each machine will be set, use this to prevent hostname change" simplemode:"false"`
	NoSetDNS              bool           `long:"no-set-dns" description:"set to prevent aerolab from updating resolved to use 1.1.1.1/8.8.8.8 DNS"`
	ScriptEarly           flags.Filename `short:"X" long:"early-script" description:"optionally specify a script to be installed which will run before every aerospike start" simplemode:"false"`
	ScriptLate            flags.Filename `short:"Z" long:"late-script" description:"optionally specify a script to be installed which will run after every aerospike stop" simplemode:"false"`
	parallelThreadsCmd
	NoVacuumOnFail bool                   `long:"no-vacuum" description:"if set, will not remove the template instance/container should it fail installation" simplemode:"false"`
	Owner          string                 `long:"owner" description:"AWS/GCP only: create owner tag with this value" simplemode:"false"`
	PriceOnly      bool                   `long:"price" description:"Only display price of ownership; do not actually create the cluster" simplemode:"false"`
	Aws            clusterCreateCmdAws    `no-flag:"true"`
	Gcp            clusterCreateCmdGcp    `no-flag:"true"`
	Docker         clusterCreateCmdDocker `no-flag:"true"`
	gcpMeta        map[string]string
	useAgiFirewall bool
	volExtraTags   map[string]string
	spotFallback   bool
	efsDelOnError  bool
}

type osSelectorCmd struct {
	DistroName    TypeDistro        `short:"d" long:"distro" description:"Linux distro, one of: debian|ubuntu|centos|rocky|amazon" default:"ubuntu" webchoice:"debian,ubuntu,rocky,centos,amazon"`
	DistroVersion TypeDistroVersion `short:"i" long:"distro-version" description:"ubuntu:24.04|22.04|20.04|18.04 rocky:9,8 centos:9,7 amazon:2|2023 debian:12|11|10|9|8" default:"latest" webchoice:"latest,24.04,22.04,20.04,18.04,2023,2,12,11,10,9,8,7"`
}

type chDirCmd struct {
	ChDir flags.Filename `short:"W" long:"work-dir" description:"Specify working directory, this is where all installers will download and CA certs will initially generate to" webtype:"text" simplemode:"false"`
}

type aerospikeVersionCmd struct {
	AerospikeVersion TypeAerospikeVersion `short:"v" long:"aerospike-version" description:"Aerospike server version; add 'c' to the end for community edition, or 'f' for federal edition" default:"latest"`
	Username         string               `long:"username" description:"Required for downloading older enterprise editions" simplemode:"false"`
	Password         string               `long:"password" description:"Required for downloading older enterprise editions" webtype:"password" simplemode:"false"`
}

type aerospikeVersionSelectorCmd struct {
	osSelectorCmd
	aerospikeVersionCmd
	chDirCmd
}

type clusterCreateCmdAws struct {
	AMI                 string          `short:"A" long:"ami" description:"custom AMI to use (default debian, ubuntu, centos, rocky and amazon are supported in eu-west-1,us-west-1,us-east-1,ap-south-1)" simplemode:"false"`
	InstanceType        guiInstanceType `short:"I" long:"instance-type" description:"instance type to use" default:"" webrequired:"true" webchoice:"method::List"`
	Ebs                 string          `webhidden:"true" short:"E" long:"ebs" description:"Deprecated: EBS volume sizes in GB, comma-separated. First one is root size. Ex: 12,100,100" default:"12" simplemode:"false"`
	Disk                []string        `long:"aws-disk" description:"EBS disks, format: type={gp2|gp3|io2|io1},size={GB}[,iops={cnt}][,throughput={mb/s}][,count=5] ex: --disk type=gp2,size=20 --disk type=gp3,size=100,iops=5000,throughput=200,count=2 ; first one is root volume ; this parameter can be specified multiple times"`
	SecurityGroupID     string          `short:"S" long:"secgroup-id" description:"security group IDs to use, comma-separated; default: empty: create and auto-manage" simplemode:"false"`
	SubnetID            string          `short:"U" long:"subnet-id" description:"subnet-id, availability-zone name, or empty; default: empty: first found in default VPC" simplemode:"false"`
	PublicIP            bool            `short:"L" long:"public-ip" description:"if set, will install systemd script which will set access-address to internal IP and alternate-access-address to allow public IP connections"`
	IsArm               bool            `long:"arm" hidden:"true" description:"indicate installing on an arm instance"`
	NoBestPractices     bool            `long:"no-best-practices" description:"set to stop best practices from being executed in setup" simplemode:"false"`
	Tags                []string        `long:"tags" description:"apply custom tags to instances; format: key=value; this parameter can be specified multiple times"`
	NamePrefix          []string        `long:"secgroup-name" description:"Name prefix to use for the security groups, can be specified multiple times" default:"AeroLab" simplemode:"false"`
	EFSMount            string          `long:"aws-efs-mount" description:"mount EFS volume; format: NAME:EfsPath:MountPath OR use NAME:MountPath to mount the EFS root" simplemode:"false"`
	EFSCreate           bool            `long:"aws-efs-create" description:"set to create the EFS volume if it doesn't exist" simplemode:"false"`
	EFSOneZone          bool            `long:"aws-efs-onezone" description:"set to force the volume to be in one AZ only; half the price for reduced flexibility with multi-AZ deployments" simplemode:"false"`
	TerminateOnPoweroff bool            `long:"aws-terminate-on-poweroff" description:"if set, when shutdown or poweroff is executed from the instance itself, it will be stopped AND terminated" simplemode:"false"`
	SpotInstance        bool            `long:"aws-spot-instance" description:"set to request a spot instance in place of on-demand"`
	Expires             time.Duration   `long:"aws-expire" description:"length of life of nodes prior to expiry; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry; grow default: match existing cluster" default:"30h"`
	EFSExpires          time.Duration   `long:"aws-efs-expire" description:"if EFS is not remounted using aerolab for this amount of time, it will be expired" simplemode:"false"`
}

type clusterCreateCmdGcp struct {
	Image               string          `long:"image" description:"custom source image to use; format: full https selfLink from GCP; see: gcloud compute images list --uri"`
	InstanceType        guiInstanceType `long:"instance" description:"instance type to use" default:"" webrequired:"true" webchoice:"method::List"`
	Disks               []string        `webhidden:"true" long:"disk" description:"Deprecated: format type:sizeGB[:iops:throughputMb][@count] or local-ssd[@count]; ex: pd-ssd:20 pd-balanced:40@2 local-ssd local-ssd@5 hyperdisk-balanced:20:3060:155; first in list is root volume, cannot be local-ssd; can be specified multiple times" simplemode:"false"`
	Disk                []string        `long:"gcp-disk" description:"disks, format: type={pd-*,hyperdisk-*,local-ssd}[,size={GB}][,iops={cnt}][,throughput={mb/s}][,count=5] ex: --disk type=pd-ssd,size=20 --disk type=hyperdisk-balanced,size=20,iops=3060,throughput=155,count=2 ; first in list is root volume, cannot be local-ssd ; this parameter can be specified multiple times"`
	PublicIP            bool            `long:"external-ip" description:"if set, will install systemd script which will set access-address to internal IP and alternate-access-address to allow public IP connections"`
	Zone                guiZone         `long:"zone" description:"zone name to deploy to" webrequired:"true" webchoice:"method::List"`
	IsArm               bool            `long:"is-arm" hidden:"true" description:"indicate installing on an arm instance"`
	NoBestPractices     bool            `long:"ignore-best-practices" description:"set to stop best practices from being executed in setup" simplemode:"false"`
	Tags                []string        `long:"tag" description:"apply custom tags to instances; this parameter can be specified multiple times"`
	Labels              []string        `long:"label" description:"apply custom labels to instances; format: key=value; this parameter can be specified multiple times"`
	NamePrefix          []string        `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external" simplemode:"false"`
	SpotInstance        bool            `long:"gcp-spot-instance" description:"set to request a spot instance in place of on-demand"`
	Expires             time.Duration   `long:"gcp-expire" description:"length of life of nodes prior to expiry; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry; grow default: match existing cluster" default:"30h"`
	VolMount            string          `long:"gcp-vol-mount" description:"mount an extra volume; format: NAME:MountPath" simplemode:"false"`
	VolCreate           bool            `long:"gcp-vol-create" description:"set to create the volume if it doesn't exist" simplemode:"false"`
	VolExpires          time.Duration   `long:"gcp-vol-expire" description:"if the volume is not remounted using aerolab for this amount of time, it will be expired" simplemode:"false"`
	VolDescription      string          `long:"gcp-vol-desc" description:"set volume description field value" simplemode:"false"`
	VolLabels           []string        `long:"gcp-vol-label" description:"apply custom labels to volume; format: key=value; this parameter can be specified multiple times" simplemode:"false"`
	TerminateOnPoweroff bool            `long:"gcp-terminate-on-poweroff" description:"if set, when shutdown or poweroff is executed from the instance itself, it will be stopped AND terminated" simplemode:"false"`
	OnHostMaintenance   string          `long:"on-host-maintenance-policy" description:"optionally specify a custom policy onHostMaintenance"`
	MinCPUPlatform      string          `long:"gcp-min-cpu-platform" description:"set the minimum CPU platform; see https://cloud.google.com/compute/docs/instances/specify-min-cpu-platform"`
}

type clusterCreateCmdDocker struct {
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
	clientCustomDockerImage string
}

type featureFile struct {
	name       string    // fileName
	version    string    // feature-key-version              1
	validUntil time.Time // valid-until-date                 2024-01-15
	serial     int       // serial-number                    680515527
}

func init() {
	addBackendSwitch("cluster.create", "aws", &a.opts.Cluster.Create.Aws)
	addBackendSwitch("cluster.create", "docker", &a.opts.Cluster.Create.Docker)
	addBackendSwitch("cluster.create", "gcp", &a.opts.Cluster.Create.Gcp)
}

func (c *clusterCreateCmd) Execute(args []string) error {
	return c.realExecute(args, false)
}

func (c *clusterCreateCmd) preChDir() {
	cur, err := os.Getwd()
	if err != nil {
		return
	}

	if string(c.CustomConfigFilePath) != "" && !filepath.IsAbs(string(c.CustomConfigFilePath)) {
		if _, err := os.Stat(string(c.CustomConfigFilePath)); err == nil {
			c.CustomConfigFilePath = flags.Filename(path.Join(cur, string(c.CustomConfigFilePath)))
		}
	}

	if string(c.CustomToolsFilePath) != "" && !filepath.IsAbs(string(c.CustomToolsFilePath)) {
		if _, err := os.Stat(string(c.CustomToolsFilePath)); err == nil {
			c.CustomToolsFilePath = flags.Filename(path.Join(cur, string(c.CustomToolsFilePath)))
		}
	}

	if string(c.FeaturesFilePath) != "" && !filepath.IsAbs(string(c.FeaturesFilePath)) {
		if _, err := os.Stat(string(c.FeaturesFilePath)); err == nil {
			c.FeaturesFilePath = flags.Filename(path.Join(cur, string(c.FeaturesFilePath)))
		}
	}

	if string(c.ScriptEarly) != "" && !filepath.IsAbs(string(c.ScriptEarly)) {
		if _, err := os.Stat(string(c.ScriptEarly)); err == nil {
			c.ScriptEarly = flags.Filename(path.Join(cur, string(c.ScriptEarly)))
		}
	}

	if string(c.ScriptLate) != "" && !filepath.IsAbs(string(c.ScriptLate)) {
		if _, err := os.Stat(string(c.ScriptLate)); err == nil {
			c.ScriptLate = flags.Filename(path.Join(cur, string(c.ScriptLate)))
		}
	}
}

func printPrice(zone string, iType string, instances int, spot bool) {
	printPriceDo(zone, iType, instances, spot, 0)
}

func printPriceDo(zone string, iType string, instances int, spot bool, iter int) {
	price := float64(-1)
	iTypes, err := b.GetInstanceTypes(0, 0, 0, 0, 0, 0, iter == 0, zone)
	if err != nil {
		log.Printf("Could not get instance pricing: %s", err)
	} else {
		for _, i := range iTypes {
			if i.InstanceName == iType {
				if !spot {
					price = i.PriceUSD * float64(instances)
				} else {
					price = i.SpotPriceUSD * float64(instances)
				}
			}
		}
		priceH := "unknown"
		priceD := "unknown"
		priceM := "unknown"
		if price > 0 {
			priceH = strconv.FormatFloat(price, 'f', 4, 64)
			priceD = strconv.FormatFloat(price*24, 'f', 2, 64)
			priceM = strconv.FormatFloat(price*24*30.5, 'f', 2, 64)
		} else if iter == 0 {
			printPriceDo(zone, iType, instances, spot, 1)
			return
		}
		log.Printf("Pre-tax cost for %d %s instances (does not include disk or network costs): $ %s/hour ; $ %s/day ; $ %s/month", instances, iType, priceH, priceD, priceM)
	}
}

func (c *clusterCreateCmd) realExecute(args []string, isGrow bool) error {
	if c.DistroName == "centos" && c.DistroVersion == "7" {
		a.opts.Config.Backend.Arch = "amd64"
	}
	if earlyProcessV2(nil, true) {
		return nil
	}
	return c.realExecute2(args, isGrow)
}

func (c *clusterCreateCmd) realExecute2(args []string, isGrow bool) error {
	if inslice.HasString(args, "help") {
		if a.opts.Config.Backend.Type == "docker" {
			printHelp("The aerolab command can be optionally followed by '--' and then extra switches that will be passed directory to Docker. Ex: aerolab cluster create -c 2 -n bob -- -v local:remote --device-read-bps=...\n\n")
		} else {
			printHelp("")
		}
	}

	if c.Owner == "" {
		c.Owner = currentOwnerUser
	}
	if !isGrow {
		log.Println("Running cluster.create")
	} else {
		log.Println("Running cluster.grow")
	}

	if c.PriceOnly && a.opts.Config.Backend.Type == "docker" {
		return logFatal("Docker backend does not support pricing")
	}
	iType := c.Aws.InstanceType
	if a.opts.Config.Backend.Type == "gcp" {
		iType = c.Gcp.InstanceType
		printPrice(c.Gcp.Zone.String(), iType.String(), c.NodeCount, false)
	} else if a.opts.Config.Backend.Type == "aws" {
		printPrice(c.Gcp.Zone.String(), iType.String(), c.NodeCount, c.Aws.SpotInstance)
	}
	if c.PriceOnly {
		return nil
	}

	var foundVol *inventoryVolume
	var efsName, efsLocalPath, efsPath string
	isArm := false
	if a.opts.Config.Backend.Type == "aws" {
		if c.Aws.EFSMount != "" && len(strings.Split(c.Aws.EFSMount, ":")) < 2 {
			return logFatal("EFS Mount format incorrect")
		}
		isArm = c.Aws.IsArm
		if c.Aws.InstanceType == "" {
			return logFatal("AWS backend requires InstanceType to be specified")
		}
		// efs mounts
		if c.Aws.EFSMount != "" {
			mountDetail := strings.Split(c.Aws.EFSMount, ":")
			efsName = mountDetail[0]
			efsLocalPath = mountDetail[1]
			efsPath = "/"
			if len(mountDetail) > 2 {
				efsPath = mountDetail[1]
				efsLocalPath = mountDetail[2]
			}
			inv, err := b.Inventory("", []int{InventoryItemVolumes})
			if err != nil {
				return err
			}
			for _, vol := range inv.Volumes {
				if vol.Name != efsName {
					continue
				}
				foundVol = &vol
				break
			}
			if foundVol == nil && !c.Aws.EFSCreate {
				return logFatal("EFS Volume not found, and is not set to be created")
			} else if foundVol == nil {
				a.opts.Volume.Delete.Name = efsName
				a.opts.Volume.Create.Name = efsName
				if c.Aws.EFSOneZone {
					a.opts.Volume.Create.Aws.Zone, err = b.GetAZName(c.Aws.SubnetID)
					if err != nil {
						return err
					}
				}
				a.opts.Volume.Create.Tags = c.Aws.Tags
				a.opts.Volume.Create.Owner = c.Owner
				a.opts.Volume.Create.Expires = c.Aws.EFSExpires
				err = a.opts.Volume.Create.Execute(nil)
				if err != nil {
					return err
				}
				c.efsDelOnError = true
			} else {
				err = b.TagVolume(foundVol.FileSystemId, "expireDuration", c.Aws.EFSExpires.String(), foundVol.AvailabilityZoneName)
				if err != nil {
					return err
				}
				ii := -1
				for i, v := range c.Aws.Tags {
					if strings.HasPrefix(v, "agiLabel=") {
						ii = i
						break
					}
				}
				if ii >= 0 {
					if agiLabel, ok := foundVol.Tags["agiLabel"]; ok {
						c.Aws.Tags[ii] = "agiLabel=" + agiLabel
					}
				}
				if c.volExtraTags != nil {
					for ek, ev := range c.volExtraTags {
						err = b.TagVolume(foundVol.FileSystemId, ek, ev, foundVol.AvailabilityZoneName)
						if err != nil {
							return err
						}
					}
				}
			}
		}
		b.WorkOnServers()
	}
	if a.opts.Config.Backend.Type == "gcp" {
		isArm = c.Gcp.IsArm
		if c.Gcp.InstanceType == "" {
			return logFatal("GCP backend requires InstanceType to be specified")
		}
		if c.Gcp.VolMount != "" && len(strings.Split(c.Gcp.VolMount, ":")) < 2 {
			return logFatal("Mount format incorrect")
		}
		if c.Gcp.VolMount != "" {
			mountDetail := strings.Split(c.Gcp.VolMount, ":")
			efsName = mountDetail[0]
			efsLocalPath = mountDetail[1]
			inv, err := b.Inventory("", []int{InventoryItemVolumes})
			if err != nil {
				return err
			}
			for _, vol := range inv.Volumes {
				if vol.Name != efsName {
					continue
				}
				foundVol = &vol
				break
			}
			if foundVol == nil && !c.Gcp.VolCreate {
				return logFatal("Volume not found, and is not set to be created")
			} else if foundVol == nil {
				a.opts.Volume.Delete.Name = efsName
				a.opts.Volume.Delete.Gcp.Zone = string(c.Gcp.Zone)
				a.opts.Volume.Create.Name = efsName
				a.opts.Volume.Create.Owner = c.Owner
				a.opts.Volume.Create.Expires = c.Gcp.VolExpires
				a.opts.Volume.Create.Gcp.Zone = c.Gcp.Zone.String()
				a.opts.Volume.Create.Gcp.Description = c.Gcp.VolDescription
				a.opts.Volume.Create.Tags = c.Gcp.VolLabels
				err = a.opts.Volume.Create.Execute(nil)
				if err != nil {
					return err
				}
				c.efsDelOnError = true
			} else {
				err = b.TagVolume(foundVol.FileSystemId, "expireduration", strings.ToLower(strings.ReplaceAll(c.Gcp.VolExpires.String(), ".", "_")), foundVol.AvailabilityZoneName)
				if err != nil {
					return err
				}
				if _, ok := c.gcpMeta["agiLabel"]; ok {
					if agiLabel, err := gcplabels.Unpack(foundVol.Tags, "agilabel"); err == nil {
						c.gcpMeta["agiLabel"] = agiLabel
					}
				}
				if c.volExtraTags != nil {
					for ek, ev := range c.volExtraTags {
						err = b.TagVolume(foundVol.FileSystemId, ek, ev, foundVol.AvailabilityZoneName)
						if err != nil {
							return err
						}
					}
				}
			}
		}
		b.WorkOnServers()
	}
	defer c.delVolume()
	c.preChDir()
	if err := chDir(string(c.ChDir)); err != nil {
		return logFatal("ChDir failed: %s", err)
	}

	var earlySize os.FileInfo
	var lateSize os.FileInfo
	var err error
	if string(c.ScriptEarly) != "" {
		earlySize, err = os.Stat(string(c.ScriptEarly))
		if err != nil {
			return logFatal("Early Script does not exist: %s", err)
		}
	}
	if string(c.ScriptLate) != "" {
		lateSize, err = os.Stat(string(c.ScriptLate))
		if err != nil {
			return logFatal("Late Script does not exist: %s", err)
		}
	}

	if len(string(c.ClusterName)) == 0 || len(string(c.ClusterName)) > 24 {
		return logFatal("Cluster name must be up to 24 characters long")
	}

	if !isLegalName(c.ClusterName.String()) {
		return logFatal("Cluster name is not legal, only use a-zA-Z0-9_-")
	}

	clusterList, err := b.ClusterList()
	if err != nil {
		return logFatal("Could not get cluster list: %s", err)
	}

	if !isGrow && inslice.HasString(clusterList, string(c.ClusterName)) {
		return logFatal("Cluster by this name already exists, did you mean 'cluster grow'?")
	}
	if isGrow && !inslice.HasString(clusterList, string(c.ClusterName)) {
		return logFatal("Cluster by this name does not exists, did you mean 'cluster create'?")
	}

	totalNodes := c.NodeCount
	var nlic []int
	if isGrow {
		nlic, err = b.NodeListInCluster(string(c.ClusterName))
		if err != nil {
			return logFatal(err)
		}
		totalNodes += len(nlic)
	}

	if totalNodes > 255 || totalNodes < 1 {
		return logFatal("Max node count is 255")
	}

	if totalNodes > 1 && c.Docker.ExposePortsToHost != "" {
		return logFatal("Cannot use docker export-ports feature with more than 1 node")
	}

	if err := checkDistroVersion(c.DistroName.String(), c.DistroVersion.String()); err != nil {
		return logFatal(err)
	}

	for _, p := range []string{string(c.CustomConfigFilePath), string(c.FeaturesFilePath), string(c.CustomToolsFilePath)} {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return logFatal("File %s does not exist", p)
		}
	}

	if c.HeartbeatMode == "mcast" || c.HeartbeatMode == "multicast" {
		if c.MulticastAddress == "" || c.MulticastPort == "" {
			return logFatal("When using multicase mode, multicast address and port must be specified")
		}
	} else if c.HeartbeatMode != "mesh" && c.HeartbeatMode != "default" {
		return logFatal("Heartbeat mode %s not supported", c.HeartbeatMode)
	}

	if !inslice.HasString([]string{"YES", "NO", "Y", "N"}, strings.ToUpper(c.AutoStartAerospike.String())) {
		return logFatal("Invalid value for AutoStartAerospike: %s", c.AutoStartAerospike)
	}

	log.Println("Checking if template exists")
	templates, err := b.ListTemplates()
	if err != nil {
		return logFatal("Could not list templates: %s", err)
	}

	var edition string
	isCommunity := false
	if strings.HasSuffix(c.AerospikeVersion.String(), "c") {
		edition = "aerospike-server-community"
		isCommunity = true
	} else if strings.HasSuffix(c.AerospikeVersion.String(), "f") {
		edition = "aerospike-server-federal"
	} else {
		edition = "aerospike-server-enterprise"
	}

	// arm fill
	if a.opts.Config.Backend.Type == "aws" {
		c.Aws.IsArm, err = b.IsSystemArm(c.Aws.InstanceType.String())
		if err != nil {
			return fmt.Errorf("IsSystemArm check: %s", err)
		}
		isArm = c.Aws.IsArm
	}
	if a.opts.Config.Backend.Type == "gcp" {
		c.Gcp.IsArm, err = b.IsSystemArm(c.Gcp.InstanceType.String())
		if err != nil {
			return fmt.Errorf("IsSystemArm check: %s", err)
		}
		isArm = c.Gcp.IsArm
	}

	// if we need to lookup version, do it
	var url string
	if b.Arch() == TypeArchAmd {
		isArm = false
	}
	if b.Arch() == TypeArchArm {
		isArm = true
	}
	bv := &backendVersion{c.DistroName.String(), c.DistroVersion.String(), c.AerospikeVersion.String(), isArm}
	if strings.HasPrefix(c.AerospikeVersion.String(), "latest") || strings.HasSuffix(c.AerospikeVersion.String(), "*") || strings.HasPrefix(c.DistroVersion.String(), "latest") {
		url, err = aerospikeGetUrl(bv, c.Username, c.Password)
		if err != nil {
			return fmt.Errorf("aerospike Version not found: %s", err)
		}
		c.AerospikeVersion = TypeAerospikeVersion(bv.aerospikeVersion)
		c.DistroName = TypeDistro(bv.distroName)
		c.DistroVersion = TypeDistroVersion(bv.distroVersion)
	}

	log.Printf("Distro = %s:%s ; AerospikeVersion = %s", c.DistroName, c.DistroVersion, c.AerospikeVersion)
	verNoSuffix := strings.TrimSuffix(c.AerospikeVersion.String(), "c")
	verNoSuffix = strings.TrimSuffix(verNoSuffix, "f")

	// build extra
	var ep []string
	if c.Docker.ExposePortsToHost != "" {
		ep = strings.Split(c.Docker.ExposePortsToHost, ",")
	}
	cloudDisks, err := disk2backend(c.Aws.Disk)
	if err != nil {
		return err
	}
	extra := &backendExtra{
		cpuLimit:        c.Docker.CpuLimit,
		ramLimit:        c.Docker.RamLimit,
		swapLimit:       c.Docker.SwapLimit,
		privileged:      c.Docker.Privileged,
		network:         c.Docker.NetworkName,
		labels:          c.Docker.Labels,
		exposePorts:     ep,
		switches:        args,
		dockerHostname:  !c.NoSetHostname,
		ami:             c.Aws.AMI,
		instanceType:    c.Aws.InstanceType.String(),
		ebs:             c.Aws.Ebs,
		securityGroupID: c.Aws.SecurityGroupID,
		subnetID:        c.Aws.SubnetID,
		publicIP:        c.Aws.PublicIP,
		tags:            c.Aws.Tags,
		cloudDisks:      cloudDisks,
	}
	if a.opts.Config.Backend.Type == "gcp" {
		cloudDisks, err := disk2backend(c.Gcp.Disk)
		if err != nil {
			return err
		}
		extra = &backendExtra{
			instanceType: c.Gcp.InstanceType.String(),
			ami:          c.Gcp.Image,
			publicIP:     c.Gcp.PublicIP,
			tags:         c.Gcp.Tags,
			disks:        c.Gcp.Disks,
			zone:         c.Gcp.Zone.String(),
			labels:       c.Gcp.Labels,
			cloudDisks:   cloudDisks,
		}
	}
	// check if template exists
	inSlice, err := inslice.Reflect(templates, backendVersion{c.DistroName.String(), c.DistroVersion.String(), c.AerospikeVersion.String(), isArm}, 1)
	if err != nil {
		return err
	}
	if len(inSlice) == 0 {
		// template doesn't exist, create one
		if url == "" {
			url, err = aerospikeGetUrl(bv, c.Username, c.Password)
			if err != nil {
				return fmt.Errorf("aerospike Version URL not found: %s", err)
			}
			c.AerospikeVersion = TypeAerospikeVersion(bv.aerospikeVersion)
			c.DistroName = TypeDistro(bv.distroName)
			c.DistroVersion = TypeDistroVersion(bv.distroVersion)
		}

		archString := ".x86_64"
		if bv.isArm {
			archString = ".arm64"
		}
		fn := edition + "-" + verNoSuffix + "-" + c.DistroName.String() + c.DistroVersion.String() + archString + ".tgz"
		// download file if not exists
		if _, err := os.Stat(fn); os.IsNotExist(err) {
			log.Println("Downloading installer")
			err = downloadFile(url, fn, c.Username, c.Password)
			if err != nil {
				return err
			}
		}

		// make template here
		log.Println("Creating template image")
		stat, err := os.Stat(fn)
		pfilelen := 0
		if err != nil {
			return err
		}
		pfilelen = int(stat.Size())
		packagefile, err := os.Open(fn)
		if err != nil {
			return err
		}
		defer packagefile.Close()
		nFiles := []fileListReader{}
		nFiles = append(nFiles, fileListReader{"/root/installer.tgz", packagefile, pfilelen})
		nscript := aerospikeInstallScript[a.opts.Config.Backend.Type+":"+c.DistroName.String()+":"+c.DistroVersion.String()]
		if a.opts.Config.Backend.Type == "gcp" {
			extra.firewallNamePrefix = c.Gcp.NamePrefix
		} else {
			extra.firewallNamePrefix = c.Aws.NamePrefix
		}
		err = b.DeployTemplate(*bv, nscript, nFiles, extra)
		if err != nil {
			if !c.NoVacuumOnFail {
				log.Print("Removing temporary template machine")
				errA := b.VacuumTemplate(*bv)
				if errA != nil {
					log.Printf("Failed to vacuum failed template: %s", errA)
				}
			}
			return err
		}
	}

	// version 4.6+ warning check
	aver := strings.Split(c.AerospikeVersion.String(), ".")
	aver_major, averr := strconv.Atoi(aver[0])
	if averr != nil {
		return errors.New("aerospike Version is not an int.int.*")
	}
	aver_minor, averr := strconv.Atoi(aver[1])
	if averr != nil {
		return errors.New("aerospike Version is not an int.int.*")
	}

	featuresFilePath := c.FeaturesFilePath
	if !isCommunity {
		if string(featuresFilePath) == "" && (aver_major == 5 || (aver_major == 4 && aver_minor > 5) || (aver_major == 6 && aver_minor == 0)) {
			log.Print("WARNING: you are attempting to install version 4.6-6.0 and did not provide feature.conf file. This will not work. You can either provide a feature file by using the '-f' switch, or configure it as default by using:\n\n$ aerolab config defaults -k '*.FeaturesFilePath' -v /path/to/features.conf\n\nPress ENTER if you still wish to proceed")
			var ignoreMe string
			fmt.Scanln(&ignoreMe)
		}
		if string(featuresFilePath) == "" && ((aver_major == 6 && aver_minor > 0) || aver_major > 6) {
			if c.NodeCount == 1 {
				log.Print("WARNING: FeaturesFilePath not configured. Using embedded features files.")
			} else {
				log.Print("WARNING: you are attempting to install more than 1 node and did not provide feature.conf file. This will not work. You can either provide a feature file by using the '-f' switch, or configure it as default by using:\n\n$ aerolab config defaults -k '*.FeaturesFilePath' -v /path/to/features.conf\n\nPress ENTER if you still wish to proceed")
				var ignoreMe string
				fmt.Scanln(&ignoreMe)
			}
		}
		if featuresFilePath != "" {
			ff, err := os.Stat(string(featuresFilePath))
			if err != nil {
				return logFatal("Features file path specified does not exist: %s", err)
			}
			fffileList := []string{}
			ffFiles := []featureFile{}
			if ff.IsDir() {
				ffDir, err := os.ReadDir(string(featuresFilePath))
				if err != nil {
					return logFatal("Features file path director read failed: %s", err)
				}
				for _, ffFile := range ffDir {
					if ffFile.IsDir() {
						continue
					}
					fffileList = append(fffileList, path.Join(string(featuresFilePath), ffFile.Name()))
				}
			} else {
				fffileList = []string{string(featuresFilePath)}
			}
			for _, ffFile := range fffileList {
				ffc, err := os.ReadFile(ffFile)
				if err != nil {
					return logFatal("Features file read failed for %s: %s", ffFile, err)
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
					}
				}
				if ffFiles1.version != "" {
					if ffFiles1.validUntil.IsZero() {
						ffFiles1.validUntil = time.Now().AddDate(0, 0, 1)
					}
					ffFiles = append(ffFiles, ffFiles1)
				}
			}
			foundFile := featureFile{}
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
					log.Print("WARNING: A valid features file v2 not found in the configured FeaturesFilePath")
				}
				featuresFilePath = flags.Filename(foundFile.name)
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
					log.Print("WARNING: A valid features file v1 not found in the configured FeaturesFilePath")
				}
				featuresFilePath = flags.Filename(foundFile.name)
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
					log.Print("WARNING: A valid features file not found in the configured FeaturesFilePath")
				}
				featuresFilePath = flags.Filename(foundFile.name)
			}
			if c.FeaturesFilePrintDetail {
				for _, ffFile := range ffFiles {
					log.Printf("feature-file=%s version=%s valid-until=%s serial=%d", ffFile.name, ffFile.version, ffFile.validUntil.String(), ffFile.serial)
				}
			}
			if string(featuresFilePath) == "" && (aver_major == 5 || (aver_major == 4 && aver_minor > 5) || (aver_major == 6 && aver_minor == 0)) {
				log.Print("WARNING: you are attempting to install version 4.6-6.0 and a valid features file could not be found. This will not work. You can either provide a feature file by using the '-f' switch, or configure it as default by using:\n\n$ aerolab config defaults -k '*.FeaturesFilePath' -v /path/to/features.conf\n\nPress ENTER if you still wish to proceed")
				var ignoreMe string
				fmt.Scanln(&ignoreMe)
			} else if string(featuresFilePath) == "" && aver_major == 6 && aver_minor > 0 {
				if c.NodeCount == 1 {
					log.Print("WARNING: FeaturesFilePath does not contain a valid feature file. Using embedded features files.")
				} else {
					log.Print("WARNING: you are attempting to install more than 1 node and a valid features file could not be found. This will not work. You can either provide a feature file by using the '-f' switch, or configure it as default by using:\n\n$ aerolab config defaults -k '*.FeaturesFilePath' -v /path/to/features.conf\n\nPress ENTER if you still wish to proceed")
					var ignoreMe string
					fmt.Scanln(&ignoreMe)
				}
			} else if (aver_major == 4 && aver_minor > 5) || aver_major > 4 {
				log.Printf("Features file: %s", featuresFilePath)
			} else {
				featuresFilePath = ""
			}
		}
	}
	log.Print("Starting deployment")
	extra.isAgiFirewall = c.useAgiFirewall
	if a.opts.Config.Backend.Type != "aws" {
		extra.firewallNamePrefix = c.Gcp.NamePrefix
		extra.labels = append(extra.labels, "owner="+c.Owner)
	} else {
		extra.firewallNamePrefix = c.Aws.NamePrefix
		extra.tags = append(extra.tags, "owner="+c.Owner)
	}
	extra.autoExpose = !c.Docker.NoAutoExpose
	if a.opts.Config.Backend.Type == "aws" {
		if c.Aws.Expires == 0 {
			extra.expiresTime = time.Time{}
		} else {
			extra.expiresTime = time.Now().Add(c.Aws.Expires)
		}
	} else if a.opts.Config.Backend.Type == "gcp" {
		if c.Gcp.Expires == 0 {
			extra.expiresTime = time.Time{}
		} else {
			extra.expiresTime = time.Now().Add(c.Gcp.Expires)
		}
	}
	if c.Docker.ClientType != "" && a.opts.Config.Backend.Type == "docker" {
		extra.labels = append(extra.labels, "aerolab.client.type="+c.Docker.ClientType)
	}
	expirySet := false
	for _, aaa := range os.Args {
		if strings.HasPrefix(aaa, "--aws-expire") || strings.HasPrefix(aaa, "--gcp-expire") {
			expirySet = true
		}
	}
	if isGrow && !expirySet {
		extra.expiresTime = time.Time{}
		ij, err := b.Inventory("", []int{InventoryItemClusters})
		b.WorkOnServers()
		if err != nil {
			return err
		}
		for _, item := range ij.Clusters {
			if item.ClusterName != string(c.ClusterName) {
				continue
			}
			if item.Expires == "" || item.Expires == "0001-01-01T00:00:00Z" {
				extra.expiresTime = time.Time{}
				break
			}
			expiry, err := time.Parse(time.RFC3339, item.Expires)
			if err != nil {
				return err
			}
			if extra.expiresTime.IsZero() || expiry.After(extra.expiresTime) {
				extra.expiresTime = expiry
			}
		}
	} else if isGrow && expirySet {
		log.Println("WARNING: you are setting a different expiry to these nodes than the existing ones. To change expiry for all nodes, use: aerolab cluster add expiry")
	}
	extra.gcpMeta = c.gcpMeta
	extra.terminateOnPoweroff = c.Aws.TerminateOnPoweroff
	extra.spotInstance = c.Aws.SpotInstance
	if a.opts.Config.Backend.Type == "gcp" {
		extra.spotInstance = c.Gcp.SpotInstance
		extra.terminateOnPoweroff = c.Gcp.TerminateOnPoweroff
	}
	extra.spotFallback = c.spotFallback

	// limitnofile check
	if a.opts.Config.Backend.Type == "docker" {
		if c.Docker.NoFILELimit == 0 {
			if string(c.CustomConfigFilePath) != "" {
				cf, err := aeroconf.ParseFile(string(c.CustomConfigFilePath))
				if err != nil {
					log.Printf("WARNING: Could not parse aerospike.conf: %s", err)
				} else {
					if cf.Type("service") == aeroconf.ValueStanza {
						vals, err := cf.Stanza("service").GetValues("proto-fd-max")
						if err == nil && len(vals) > 0 {
							fdmax, err := strconv.Atoi(*vals[0])
							if err == nil && fdmax > 0 {
								extra.limitNoFile = fdmax + 5000
							}
						}
					}
				}
			}
			if extra.limitNoFile == 0 {
				extra.limitNoFile = 20000
			}
		} else if c.Docker.NoFILELimit > 0 {
			extra.limitNoFile = c.Docker.NoFILELimit
		}
	}
	extra.onHostMaintenance = c.Gcp.OnHostMaintenance
	if c.Gcp.MinCPUPlatform != "" {
		extra.gcpMinCpuPlatform = &c.Gcp.MinCPUPlatform
	}
	err = b.DeployCluster(*bv, string(c.ClusterName), c.NodeCount, extra)
	if err != nil {
		return err
	}

	files := []fileList{}

	err = b.ClusterStart(string(c.ClusterName), nil)
	if err != nil {
		return err
	}

	// get cluster IPs and node list
	clusterIps, err := b.GetClusterNodeIps(string(c.ClusterName))
	if err != nil {
		return err
	}
	nodeList, err := b.NodeListInCluster(string(c.ClusterName))
	if err != nil {
		return err
	}

	newconf := ""
	// fix config if needed, read custom config file path if needed
	if string(c.CustomConfigFilePath) != "" {
		conf, err := os.ReadFile(string(c.CustomConfigFilePath))
		if err != nil {
			return err
		}
		newconf, err = fixAerospikeConfig(string(conf), c.MulticastAddress, c.HeartbeatMode.String(), clusterIps)
		if err != nil {
			return err
		}
	} else {
		var r [][]string
		r = append(r, []string{"cat", "/etc/aerospike/aerospike.conf"})
		var nr [][]byte
		nr, err = b.RunCommands(string(c.ClusterName), r, []int{nodeList[0]})
		if err != nil {
			return err
		}
		newconf = string(nr[0])
		if c.HeartbeatMode == "mesh" || c.HeartbeatMode == "mcast" {
			// nr has contents of aerospike.conf
			newconf, err = fixAerospikeConfig(string(nr[0]), c.MulticastAddress, c.HeartbeatMode.String(), clusterIps)
			if err != nil {
				return err
			}
		}
		if aver_major >= 7 && !c.Docker.NoPatchV7Config {
			newconf, err = patchDockerNamespacesV7(newconf)
			if err != nil {
				log.Printf("Failed to patch default namespaces, memory requirements will be huge and Aerospike could OOM: %s", err)
			}
		}
	}

	// add cluster name
	newconf2 := newconf
	if !c.NoOverrideClusterName {
		newconf2, err = fixClusterNameConfig(string(newconf), string(c.ClusterName))
		if err != nil {
			return err
		}
	}

	if c.HeartbeatMode == "mesh" || c.HeartbeatMode == "mcast" || !c.NoOverrideClusterName || string(c.CustomConfigFilePath) != "" {
		files = append(files, fileList{"/etc/aerospike/aerospike.conf", newconf2, len(newconf2)})
	}
	if string(c.CustomToolsFilePath) != "" {
		toolsconf, err := os.ReadFile(string(c.CustomToolsFilePath))
		if err != nil {
			return err
		}
		files = append(files, fileList{"/etc/aerospike/astools.conf", string(toolsconf), len(toolsconf)})
	}

	// load features file path if needed
	if string(featuresFilePath) != "" {
		ffp, err := os.ReadFile(string(featuresFilePath))
		if err != nil {
			return err
		}
		files = append(files, fileList{"/etc/aerospike/features.conf", string(ffp), len(ffp)})
	}

	nodeListNew := []int{}
	for _, i := range nodeList {
		if !inslice.HasInt(nlic, i) {
			nodeListNew = append(nodeListNew, i)
		}
	}

	// set hostnames for aws and gcp
	if a.opts.Config.Backend.Type != "docker" && !c.NoSetHostname {
		nip, err := b.GetNodeIpMap(string(c.ClusterName), false)
		if err != nil {
			return err
		}
		log.Printf("Node IP map: %v", nip)
		returns := parallelize.MapLimit(nodeListNew, c.ParallelThreads, func(nnode int) error {
			newHostname := fmt.Sprintf("%s-%d", string(c.ClusterName), nnode)
			newHostname = strings.ReplaceAll(newHostname, "_", "-")
			hComm := [][]string{
				{"hostname", newHostname},
			}
			nr, err := b.RunCommands(string(c.ClusterName), hComm, []int{nnode})
			if err != nil {
				return fmt.Errorf("could not set hostname: %s:%s", err, nr)
			}
			nr, err = b.RunCommands(string(c.ClusterName), [][]string{{"sed", "s/" + nip[nnode] + ".*//g", "/etc/hosts"}}, []int{nnode})
			if err != nil {
				return fmt.Errorf("could not set hostname: %s:%s", err, nr)
			}
			nr[0] = append(nr[0], []byte(fmt.Sprintf("\n%s %s-%d\n", nip[nnode], string(c.ClusterName), nnode))...)
			hst := fmt.Sprintf("%s-%d\n", string(c.ClusterName), nnode)
			err = b.CopyFilesToClusterReader(string(c.ClusterName), []fileListReader{{"/etc/hostname", strings.NewReader(hst), len(hst)}}, []int{nnode})
			if err != nil {
				return err
			}
			err = b.CopyFilesToClusterReader(string(c.ClusterName), []fileListReader{{"/etc/hosts", bytes.NewReader(nr[0]), len(nr[0])}}, []int{nnode})
			if err != nil {
				return err
			}
			return nil
		})
		isError := false
		for i, ret := range returns {
			if ret != nil {
				log.Printf("Node %d returned %s", nodeListNew[i], ret)
				isError = true
			}
		}
		if isError {
			return errors.New("some nodes returned errors")
		}
	}

	// store deployed aerospike version
	files = append(files, fileList{"/opt/aerolab.aerospike.version", c.AerospikeVersion.String(), len(c.AerospikeVersion)})
	if a.opts.Config.Backend.Type == "gcp" && c.Gcp.TerminateOnPoweroff {
		termonpoweroffContents := "export NAME=$(curl -X GET http://metadata.google.internal/computeMetadata/v1/instance/name -H 'Metadata-Flavor: Google'); export ZONE=$(curl -X GET http://metadata.google.internal/computeMetadata/v1/instance/zone -H 'Metadata-Flavor: Google'); gcloud --quiet compute instances delete $NAME --zone=$ZONE; systemctl poweroff"
		files = append(files, fileList{"/usr/local/bin/poweroff", termonpoweroffContents, len(termonpoweroffContents)})
	}

	// actually save files to nodes in cluster if needed
	if len(files) > 0 {
		returns := parallelize.MapLimit(nodeListNew, c.ParallelThreads, func(nnode int) error {
			err := b.CopyFilesToCluster(string(c.ClusterName), files, []int{nnode})
			if err != nil {
				return err
			}
			if a.opts.Config.Backend.Type == "gcp" && c.Gcp.TerminateOnPoweroff {
				out, err := b.RunCommands(string(c.ClusterName), [][]string{{"/bin/bash", "-c", "rm -f /sbin/poweroff; ln -s /usr/local/bin/poweroff /sbin/poweroff"}}, []int{nnode})
				if err != nil {
					log.Printf("ERROR: failed to install TerminateOnPoweroff script, instance will not terminate on poweroff: %s: %s", err, string(out[0]))
				}
			}
			return nil
		})
		isError := false
		for i, ret := range returns {
			if ret != nil {
				log.Printf("Node %d returned %s", nodeListNew[i], ret)
				isError = true
			}
		}
		if isError {
			return errors.New("some nodes returned errors")
		}
	}

	// if docker fix logging location
	// if docker also fix autoexpose
	// if docker auto-adjust astools.conf on each node
	// if aws, adopt best-practices
	var inv inventoryJson
	if a.opts.Config.Backend.Type == "docker" && !c.Docker.NoAutoExpose {
		inv, err = b.Inventory("", []int{InventoryItemClusters})
		if err != nil {
			return err
		}
	}
	b.WorkOnServers()
	returns := parallelize.MapLimit(nodeListNew, c.ParallelThreads, func(nnode int) error {
		if a.opts.Config.Backend.Type != "docker" && !c.NoSetDNS {
			dnsScript := `mkdir -p /etc/systemd/resolved.conf.d
cat <<'EOF' > /etc/systemd/resolved.conf.d/aerolab.conf
[Resolve]
DNS=1.1.1.1
FallbackDNS=8.8.8.8
EOF
systemctl restart systemd-resolved
`
			if err = b.CopyFilesToClusterReader(string(c.ClusterName), []fileListReader{{filePath: "/tmp/fix-dns.sh", fileContents: strings.NewReader(dnsScript), fileSize: len(dnsScript)}}, []int{nnode}); err == nil {
				if _, err = b.RunCommands(string(c.ClusterName), [][]string{{"/bin/bash", "-c", "chmod 755 /tmp/fix-dns.sh; bash /tmp/fix-dns.sh"}}, []int{nnode}); err != nil {
					log.Print("Failed to set DNS resolvers by running /tmp/fix-dns.sh")
				}
			} else {
				log.Printf("Failed to upload DNS resolver script to /tmp/fix-dns.sh: %s", err)
			}
		}
		out, err := b.RunCommands(string(c.ClusterName), [][]string{{"cat", "/etc/aerospike/aerospike.conf"}}, []int{nnode})
		if err != nil {
			return err
		}
		if a.opts.Config.Backend.Type == "docker" {
			in := strings.Replace(string(out[0]), "console {", "file /var/log/aerospike.log {", 1)
			if !c.Docker.NoAutoExpose {
				port := ""
				privateIp := ""
				for _, item := range inv.Clusters {
					if item.ClusterName == c.ClusterName.String() && item.NodeNo == strconv.Itoa(nnode) {
						port = item.DockerExposePorts
						privateIp = item.PrivateIp
					}
				}
				if port == "" || privateIp == "" {
					return errors.New("WARN: could not find privateIp/exposed port; not fixing")
				}
				if strings.Contains(port, "-") {
					port = strings.Split(port, "-")[0]
				}
				confReader := strings.NewReader(in)
				s, err := aeroconf.Parse(confReader)
				if err != nil {
					return err
				}
				if s.Type("network") == aeroconf.ValueNil {
					s.NewStanza("network")
				}
				if s.Stanza("network").Type("service") == aeroconf.ValueNil {
					s.Stanza("network").NewStanza("service")
				}
				if inslice.HasString(s.Stanza("network").Stanza("service").ListKeys(), "port") {
					err = s.Stanza("network").Stanza("service").SetValue("port", port)
					if err != nil {
						return err
					}
					err = s.Stanza("network").Stanza("service").SetValue("access-address", privateIp)
					if err != nil {
						return err
					}
					err = s.Stanza("network").Stanza("service").SetValue("alternate-access-address", "127.0.0.1")
					if err != nil {
						return err
					}
				}
				if inslice.HasString(s.Stanza("network").Stanza("service").ListKeys(), "tls-port") {
					err = s.Stanza("network").Stanza("service").SetValue("tls-port", port)
					if err != nil {
						return err
					}
					err = s.Stanza("network").Stanza("service").SetValue("tls-access-address", privateIp)
					if err != nil {
						return err
					}
					err = s.Stanza("network").Stanza("service").SetValue("tls-alternate-access-address", "127.0.0.1")
					if err != nil {
						return err
					}
				}
				buf := &bytes.Buffer{}
				err = s.Write(buf, "", "    ", true)
				if err != nil {
					return err
				}
				in = buf.String()
				// astools.conf
				for _, item := range inv.Clusters {
					if item.ClusterName == c.ClusterName.String() && item.NodeNo == strconv.Itoa(nnode) && item.DockerExposePorts != "" {
						tools, err := b.RunCommands(string(c.ClusterName), [][]string{{"cat", "/etc/aerospike/astools.conf"}}, []int{nnode})
						if err != nil {
							return err
						}
						toolsConf := tools[0]
						toolsConfNew := ""
						// adjust astools
						scanner := bufio.NewScanner(bytes.NewReader(toolsConf))
						inCluster := false
						found := false
						for scanner.Scan() {
							line := scanner.Text()
							if strings.HasPrefix(line, "[cluster]") {
								inCluster = true
								toolsConfNew = toolsConfNew + line + "\n"
							} else if strings.HasPrefix(line, "[") {
								inCluster = false
							} else if inCluster && strings.HasPrefix(line, "host") {
								found = true
								nline := strings.Split(strings.Trim(line, "\r\t\n "), ":")
								if len(nline) == 3 {
									nline[2] = item.DockerExposePorts + "\""
								} else if len(nline) == 2 {
									nline[1] = item.DockerExposePorts + "\""
								}
								line = strings.Join(nline, ":")
								toolsConfNew = toolsConfNew + line + "\n"
							}
						}
						if !found {
							toolsConfNew = strings.ReplaceAll(string(toolsConf), "[cluster]", "[cluster]\nhost = \"localhost:"+item.DockerExposePorts+"\"")
						}
						// adjust end
						err = b.CopyFilesToCluster(string(c.ClusterName), []fileList{{"/etc/aerospike/astools.conf", toolsConfNew, len(toolsConfNew)}}, []int{nnode})
						if err != nil {
							return err
						}
					}
				}
			}
			err = b.CopyFilesToCluster(string(c.ClusterName), []fileList{{"/etc/aerospike/aerospike.conf", in, len(in)}}, []int{nnode})
			if err != nil {
				return err
			}
		}
		if (a.opts.Config.Backend.Type == "aws" && !c.Aws.NoBestPractices) || (a.opts.Config.Backend.Type == "gcp" && !c.Gcp.NoBestPractices) {
			thpString := c.thpString()
			err := b.CopyFilesToClusterReader(string(c.ClusterName), []fileListReader{{
				filePath:     "/etc/systemd/system/aerospike.service.d/aerolab-thp.conf",
				fileSize:     len(thpString),
				fileContents: strings.NewReader(thpString),
			}}, []int{nnode})
			if err != nil {
				log.Printf("WARNING! THP Disable script could not be installed: %s", err)
			}
		}
		// also create locations if not exist
		logx := string(out[0])
		scanner := bufio.NewScanner(strings.NewReader(logx))
		for scanner.Scan() {
			t := scanner.Text()
			if (strings.Contains(t, "/var") || strings.Contains(t, "/opt") || strings.Contains(t, "/etc") || strings.Contains(t, "/tmp")) && !strings.HasPrefix(strings.TrimLeft(t, " "), "#") {
				tStart := strings.Index(t, " /") + 1
				var nLoc string
				if strings.Contains(t[tStart:], " ") {
					tEnd := strings.Index(t[tStart:], " ")
					nLoc = t[tStart:(tEnd + tStart)]
				} else {
					nLoc = t[tStart:]
				}
				var nDir string
				_, nFile := path.Split(nLoc)
				if strings.Contains(t, "file /") || strings.Contains(t, "xdr-digestlog-path /") || strings.Contains(t, "file:/") || strings.Contains(nFile, ".") {
					nDir = path.Dir(nLoc)
				} else {
					nDir = nLoc
				}
				// create dir
				nout, err := b.RunCommands(string(c.ClusterName), [][]string{{"mkdir", "-p", nDir}}, []int{nnode})
				if err != nil {
					return fmt.Errorf("could not create directory on node: %s\n%s\n%s", nDir, err, string(nout[0]))
				}
			}
		}
		// aws-public-ip
		if c.Aws.PublicIP && a.opts.Config.Backend.Type == "aws" {
			systemdScriptContents := `[Unit]
Description=Fix Aerospike access-address and alternate-access-address
RequiredBy=aerospike.service
Before=aerospike.service
		
[Service]
Type=oneshot
ExecStart=/bin/bash /usr/local/bin/aerospike-access-address.sh
		
[Install]
WantedBy=multi-user.target`
			var systemdScript fileListReader
			var accessAddressScript fileListReader
			systemdScript.filePath = "/etc/systemd/system/aerospike-access-address.service"
			systemdScript.fileContents = strings.NewReader(systemdScriptContents)
			systemdScript.fileSize = len(systemdScriptContents)
			accessAddressScriptContents := `grep 'alternate-access-address' /etc/aerospike/aerospike.conf
if [ $? -ne 0 ]
then
	sed -i 's/address any/address any\naccess-address\nalternate-access-address\n/g' /etc/aerospike/aerospike.conf
fi
sed -e "s/access-address.*/access-address $(curl http://169.254.169.254/latest/meta-data/local-ipv4)/g" -e "s/alternate-access-address.*/alternate-access-address $(curl http://169.254.169.254/latest/meta-data/public-ipv4)/g"  /etc/aerospike/aerospike.conf > ~/aerospike.conf.new && cp /etc/aerospike/aerospike.conf /etc/aerospike/aerospike.conf.bck && cp ~/aerospike.conf.new /etc/aerospike/aerospike.conf
`
			accessAddressScript.filePath = "/usr/local/bin/aerospike-access-address.sh"
			accessAddressScript.fileContents = strings.NewReader(accessAddressScriptContents)
			accessAddressScript.fileSize = len(accessAddressScriptContents)
			err = b.CopyFilesToClusterReader(string(c.ClusterName), []fileListReader{systemdScript, accessAddressScript}, []int{nnode})
			if err != nil {
				return fmt.Errorf("could not make access-address script in aws: %s", err)
			}
			bouta, err := b.RunCommands(string(c.ClusterName), [][]string{{"chmod", "755", "/usr/local/bin/aerospike-access-address.sh"}, {"chmod", "755", "/etc/systemd/system/aerospike-access-address.service"}, {"systemctl", "daemon-reload"}, {"systemctl", "enable", "aerospike-access-address.service"}, {"service", "aerospike-access-address", "start"}}, []int{nnode})
			if err != nil {
				nstr := ""
				for _, bout := range bouta {
					nstr = fmt.Sprintf("%s\n%s", nstr, string(bout))
				}
				return fmt.Errorf("could not register access-address script in aws: %s\n%s", err, nstr)
			}
		} else if c.Gcp.PublicIP && a.opts.Config.Backend.Type == "gcp" {
			// curl -H "Metadata-Flavor: Google" http://169.254.169.254/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip
			systemdScriptContents := `[Unit]
Description=Fix Aerospike access-address and alternate-access-address
RequiredBy=aerospike.service
Before=aerospike.service
		
[Service]
Type=oneshot
ExecStart=/bin/bash /usr/local/bin/aerospike-access-address.sh
		
[Install]
WantedBy=multi-user.target`
			var systemdScript fileListReader
			var accessAddressScript fileListReader
			systemdScript.filePath = "/etc/systemd/system/aerospike-access-address.service"
			systemdScript.fileContents = strings.NewReader(systemdScriptContents)
			systemdScript.fileSize = len(systemdScriptContents)
			accessAddressScriptContents := `INTIP=""; EXTIP=""
attempts=0
max=120
while [ "${INTIP}" = "" ]
do
	INTIP=$(curl -H "Metadata-Flavor: Google" http://169.254.169.254/computeMetadata/v1/instance/network-interfaces/0/ip)
	[ "${INTIP}" = "" ] && sleep 1 || break
	attempts=$(( $attempts + 1 ))
	[ $attempts -eq $max ] && exit 1
done
while [ "${EXTIP}" = "" ]
do
	EXTIP=$(curl -H "Metadata-Flavor: Google" http://169.254.169.254/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip)
	[ "${EXTIP}" = "" ] && sleep 1 || break
	attempts=$(( $attempts + 1 ))
	[ $attempts -eq $max ] && exit 1
done
grep 'alternate-access-address' /etc/aerospike/aerospike.conf
if [ $? -ne 0 ]
then
	sed -i 's/address any/address any\naccess-address\nalternate-access-address\n/g' /etc/aerospike/aerospike.conf
fi
sed -e "s/access-address.*/access-address ${INTIP}/g" -e "s/alternate-access-address.*/alternate-access-address ${EXTIP}/g"  /etc/aerospike/aerospike.conf > ~/aerospike.conf.new && cp /etc/aerospike/aerospike.conf /etc/aerospike/aerospike.conf.bck && cp ~/aerospike.conf.new /etc/aerospike/aerospike.conf
`
			accessAddressScript.filePath = "/usr/local/bin/aerospike-access-address.sh"
			accessAddressScript.fileContents = strings.NewReader(accessAddressScriptContents)
			accessAddressScript.fileSize = len(accessAddressScriptContents)
			err = b.CopyFilesToClusterReader(string(c.ClusterName), []fileListReader{systemdScript, accessAddressScript}, []int{nnode})
			if err != nil {
				return fmt.Errorf("could not make access-address script in aws: %s", err)
			}
			bouta, err := b.RunCommands(string(c.ClusterName), [][]string{{"chmod", "755", "/usr/local/bin/aerospike-access-address.sh"}, {"chmod", "755", "/etc/systemd/system/aerospike-access-address.service"}, {"systemctl", "daemon-reload"}, {"systemctl", "enable", "aerospike-access-address.service"}, {"service", "aerospike-access-address", "start"}}, []int{nnode})
			if err != nil {
				nstr := ""
				for _, bout := range bouta {
					nstr = fmt.Sprintf("%s\n%s", nstr, string(bout))
				}
				return fmt.Errorf("could not register access-address script in aws: %s\n%s", err, nstr)
			}
		}
		// install early/late scripts
		if string(c.ScriptEarly) != "" {
			earlyFile, err := os.Open(string(c.ScriptEarly))
			if err != nil {
				log.Printf("ERROR: could not install early script: %s", err)
			} else {
				err = b.CopyFilesToClusterReader(string(c.ClusterName), []fileListReader{{"/usr/local/bin/early.sh", earlyFile, int(earlySize.Size())}}, []int{nnode})
				if err != nil {
					log.Printf("ERROR: could not install early script: %s", err)
				}
				earlyFile.Close()
			}
		}
		if string(c.ScriptLate) != "" {
			lateFile, err := os.Open(string(c.ScriptLate))
			if err != nil {
				log.Printf("ERROR: could not install late script: %s", err)
			} else {
				err = b.CopyFilesToClusterReader(string(c.ClusterName), []fileListReader{{"/usr/local/bin/late.sh", lateFile, int(lateSize.Size())}}, []int{nnode})
				if err != nil {
					log.Printf("ERROR: could not install late script: %s", err)
				}
				lateFile.Close()
			}
		}
		return nil
	})
	isError := false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %d returned %s", nodeListNew[i], ret)
			isError = true
		}
	}
	if isError {
		return errors.New("some nodes returned errors")
	}

	// efs mounts
	if a.opts.Config.Backend.Type == "aws" && c.Aws.EFSMount != "" {
		a.opts.Volume.Mount.ClusterName = c.ClusterName.String()
		a.opts.Volume.Mount.Aws.EfsPath = efsPath
		a.opts.Volume.Mount.IsClient = false
		a.opts.Volume.Mount.LocalPath = efsLocalPath
		a.opts.Volume.Mount.Name = efsName
		a.opts.Volume.Mount.ParallelThreads = c.ParallelThreads
		err = a.opts.Volume.Mount.Execute(nil)
		if err != nil {
			return err
		}
	} else if a.opts.Config.Backend.Type == "gcp" && c.Gcp.VolMount != "" {
		a.opts.Volume.Mount.ClusterName = string(c.ClusterName)
		a.opts.Volume.Mount.Aws.EfsPath = efsPath
		a.opts.Volume.Mount.IsClient = false
		a.opts.Volume.Mount.LocalPath = efsLocalPath
		a.opts.Volume.Mount.Name = efsName
		a.opts.Volume.Mount.ParallelThreads = c.ParallelThreads
		err = a.opts.Volume.Mount.Execute(nil)
		if err != nil {
			return err
		}
	}
	b.WorkOnServers()

	// start cluster
	if c.AutoStartAerospike == "y" {
		returns := parallelize.MapLimit(nodeListNew, c.ParallelThreads, func(node int) error {
			var comm [][]string
			comm = append(comm, []string{"service", "aerospike", "start"})
			_, err = b.RunCommands(string(c.ClusterName), comm, []int{node})
			if err != nil {
				return err
			}
			return nil
		})
		isError := false
		for i, ret := range returns {
			if ret != nil {
				log.Printf("Node %d returned %s", nodeListNew[i], ret)
				isError = true
			}
		}
		if isError {
			return errors.New("some nodes returned errors")
		}
	}

	// done
	log.Println("INFO: Cluster monitoring can be setup using `aerolab cluster add exporter` and `aerolab client create ams` commands.")
	log.Println("See documentation for more information about the monitoring stack: https://github.com/aerospike/aerolab/blob/master/docs/usage/monitoring/ams.md")
	if a.opts.Config.Backend.Type == "docker" && !c.Docker.NoAutoExpose {
		log.Println("To connect directly to the cluster (non-docker-desktop), execute 'aerolab cluster list' and connect to the node IP on the given exposed port")
		log.Println("To connect to the cluster when using Docker Desktop, execute 'aerolab cluster list` and connect to IP 127.0.0.1:EXPOSED_PORT with a connect policy of `--services-alternate`")
	} else if a.opts.Config.Backend.Type == "docker" {
		log.Println("To connect directly to the cluster (non-docker-desktop), execute 'aerolab cluster list' and connect to the node IP:SERVICE_PORT (default:3000)")
	}
	if a.opts.Config.Backend.Type != "docker" && !extra.expiresTime.IsZero() {
		log.Printf("CLUSTER EXPIRES: %s (in: %s); to extend, use: aerolab cluster add expiry", extra.expiresTime.Format(time.RFC850), time.Until(extra.expiresTime).Truncate(time.Second).String())
	}
	log.Println("Done")
	c.efsDelOnError = false
	return nil
}

func (c *clusterCreateCmd) thpString() string {
	return `[Service]
	ExecStartPre=/bin/bash -c "echo 'never' > /sys/kernel/mm/transparent_hugepage/enabled || echo"
	ExecStartPre=/bin/bash -c "echo 'never' > /sys/kernel/mm/transparent_hugepage/defrag || echo"
	ExecStartPre=/bin/bash -c "echo 'never' > /sys/kernel/mm/redhat_transparent_hugepage/enabled || echo"
	ExecStartPre=/bin/bash -c "echo 'never' > /sys/kernel/mm/redhat_transparent_hugepage/defrag || echo"
	ExecStartPre=/bin/bash -c "echo 0 > /sys/kernel/mm/transparent_hugepage/khugepaged/defrag || echo"
	ExecStartPre=/bin/bash -c "echo 0 > /sys/kernel/mm/redhat_transparent_hugepage/khugepaged/defrag || echo"
	ExecStartPre=/bin/bash -c "sysctl -w vm.min_free_kbytes=1310720 || echo"
	ExecStartPre=/bin/bash -c "sysctl -w vm.swappiness=0 || echo"
	`
}

func isLegalName(name string) bool {
	for _, char := range name {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '-') {
			return false
		}
	}
	return true
}

func patchDockerNamespacesV7(conf string) (newconf string, err error) {
	ac, err := aeroconf.Parse(strings.NewReader(conf))
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
	conf = buf.String()
	return conf, nil
}

func disk2backend(ds []string) (disks []*cloudDisk, err error) {
	for _, d := range ds {
		defs := strings.Split(d, ",")
		disk := cloudDisk{}
		cnt := 1
		for _, def := range defs {
			kv := strings.Split(def, "=")
			if len(kv) != 2 {
				return nil, fmt.Errorf("definition %s is incorrect, must be key=value", def)
			}
			switch kv[0] {
			case "type":
				disk.Type = kv[1]
			case "size":
				size, err := strconv.Atoi(kv[1])
				if err != nil {
					return nil, fmt.Errorf("size value of %s not an int", def)
				}
				disk.Size = int64(size)
			case "iops":
				iops, err := strconv.Atoi(kv[1])
				if err != nil {
					return nil, fmt.Errorf("iops value of %s not an int", def)
				}
				disk.ProvisionedIOPS = int64(iops)
			case "throughput":
				tp, err := strconv.Atoi(kv[1])
				if err != nil {
					return nil, fmt.Errorf("throughput value of %s not an int", def)
				}
				disk.ProvisionedThroughput = int64(tp)
			case "count":
				cnt, err = strconv.Atoi(kv[1])
				if err != nil {
					return nil, fmt.Errorf("count value of %s not an int", def)
				}
			default:
				return nil, fmt.Errorf("key name '%s' not supported in %s", kv[0], def)
			}
		}
		for c := 1; c <= cnt; c++ {
			disks = append(disks, &disk)
		}
	}
	return
}

func (c *clusterCreateCmd) delVolume() {
	if c.efsDelOnError {
		log.Println("ErrorCleanupHandler: Removing stale volume")
		a.opts.Volume.Delete.Execute(nil)
	}
}

package cmd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	"github.com/lithammer/shortuuid"
	"gopkg.in/yaml.v3"
)

type TemplateCreateCmd struct {
	Distro           string  `short:"d" long:"distro" description:"Distro to create the template for" default:"ubuntu"`
	DistroVersion    string  `short:"v" long:"distro-version" description:"Version of the distro to create the template for" default:"latest"`
	Arch             string  `short:"a" long:"arch" description:"Architecture to create the template for" default:"amd64"`
	AerospikeVersion string  `short:"A" long:"aerospike-version" description:"Aerospike version to create the template for" default:"latest"`
	Owner            string  `short:"o" long:"owner" description:"Owner of the template" default:"none"`
	DisablePublicIP  bool    `short:"p" long:"disable-public-ip" description:"Disable public IP assignment to the instances in AWS"`
	DryRun           bool    `short:"n" long:"dry-run" description:"Do not actually create the template, just run the basic checks"`
	NoVacuum         bool    `short:"V" long:"no-vacuum" description:"Do not vacuum an existing template creation instance on failure"`
	Help             HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *TemplateCreateCmd) Execute(args []string) error {
	cmd := []string{"template", "create"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	_, err = c.CreateTemplate(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *TemplateCreateCmd) CreateTemplate(system *System, inventory *backends.Inventory, args []string) (name string, err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"template", "create"}, c, args...)
		if err != nil {
			return "", err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// find and resolve aerospike version
	products, err := aerospike.GetProducts(time.Second * 10)
	if err != nil {
		return "", fmt.Errorf("could not get products: %s", err)
	}
	products = products.WithNamePrefix("aerospike-server-")
	var flavor string
	if strings.HasSuffix(c.AerospikeVersion, "c") {
		products = products.WithNameSuffix("-community")
		flavor = "community"
	} else if strings.HasSuffix(c.AerospikeVersion, "f") {
		products = products.WithNameSuffix("-federal")
		flavor = "federal"
	} else {
		products = products.WithNameSuffix("-enterprise")
		flavor = "enterprise"
	}
	if len(products) == 0 {
		return "", fmt.Errorf("aerospike version %s not found", c.AerospikeVersion)
	}
	versions, err := aerospike.GetVersions(time.Second*10, products[0])
	if err != nil {
		return "", fmt.Errorf("could not get versions: %s", err)
	}
	var version *aerospike.Version
	if strings.HasPrefix(c.AerospikeVersion, "latest") {
		version = versions.Latest()
		if version == nil {
			return "", fmt.Errorf("version not found")
		}
		c.AerospikeVersion = version.Name
	} else {
		c.AerospikeVersion = strings.TrimRight(c.AerospikeVersion, "cf")
		versions = versions.WithName(c.AerospikeVersion)
		if len(versions) == 0 {
			return "", fmt.Errorf("aerospike version %s not found", c.AerospikeVersion)
		}
		version = versions.Latest()
		if version == nil {
			return "", fmt.Errorf("version not found")
		}
		c.AerospikeVersion = version.Name
	}

	// get the installer URL
	files, err := aerospike.GetFiles(time.Second*10, *version)
	if err != nil {
		return "", fmt.Errorf("could not get files: %s", err)
	}
	arch := aerospike.ArchitectureTypeX86_64
	if c.Arch == "arm64" {
		arch = aerospike.ArchitectureTypeAARCH64
	}
	osName := aerospike.OSName(c.Distro)
	if osName == "rocky" {
		osName = "centos"
	}
	osVersion := c.DistroVersion
	var installScript []byte
	if osVersion != "latest" {
		installScript, err = files.GetInstallScript(arch, osName, osVersion, system.logLevel >= 5, true, true, false)
		if err != nil {
			return "", fmt.Errorf("could not get install script: %s", err)
		}
	} else {
		var versionList []string
		switch osName {
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
		default:
			return "", fmt.Errorf("unsupported distro: %s", osName)
		}
		for _, version := range versionList {
			installScript, err = files.GetInstallScript(arch, osName, version, system.logLevel >= 5, true, true, false)
			if err == nil {
				osVersion = version
				break
			}
		}
		if installScript == nil {
			return "", fmt.Errorf("could not get install script: could not find a matching OS Version and Architecture for the given aerospike version %s %s", flavor, c.AerospikeVersion)
		}
	}

	// check if the template already exists
	var backendArch backends.Architecture
	err = backendArch.FromString(c.Arch)
	if err != nil {
		return "", fmt.Errorf("could not get architecture: %s", err)
	}
	images := inventory.Images.WithTags(map[string]string{"aerolab.soft.version": c.AerospikeVersion + "-" + flavor}).WithOSName(c.Distro).WithOSVersion(osVersion).WithArchitecture(backendArch)
	if images.Count() > 0 {
		return "", fmt.Errorf("template %s-%s-%s-%s already exists", c.AerospikeVersion, flavor, c.Distro, osVersion)
	}

	// check if we need to vacuum an existing template creation instance if it exists (dangling instance for image)
	needToVacuum := false
	instances := inventory.Instances.WithTags(map[string]string{"aerolab.type": "images.create", "aerolab.tmpl.version": c.AerospikeVersion + "-" + flavor}).WithNotState(backends.LifeCycleStateTerminated).WithOSName(c.Distro).WithOSVersion(osVersion).WithArchitecture(backendArch)
	if instances.Count() > 0 {
		needToVacuum = true
	}

	// build the structs for instances create and images create, instances stop and instances destroy commands (optionally vacuum too)
	instName := strings.ToLower(shortuuid.New())
	instancesCreate := &InstancesCreateCmd{
		ClusterName:        instName,
		Count:              1,
		Name:               instName,
		Owner:              c.Owner,
		Type:               "images.create",
		Tags:               []string{"aerolab.tmpl.version=" + c.AerospikeVersion + "-" + flavor},
		Description:        "temporary aerospike server instance used for template creation",
		TerminateOnStop:    false,
		ParallelSSHThreads: 1,
		SSHKeyName:         "",
		OS:                 c.Distro,
		Version:            osVersion,
		Arch:               c.Arch,
		AWS: InstancesCreateCmdAws{
			ImageID:            "",
			Expire:             20 * time.Minute,
			NetworkPlacement:   system.Opts.Config.Backend.Region,
			InstanceType:       "t3.medium",
			Disks:              []string{"type=gp2,size=20,encrypted=true"},
			Firewalls:          []string{},
			SpotInstance:       false,
			DisablePublicIP:    c.DisablePublicIP,
			IAMInstanceProfile: "",
			CustomDNS:          InstanceDNS{},
		},
		GCP: InstancesCreateCmdGcp{
			ImageName:          "",
			Expire:             20 * time.Minute,
			Zone:               system.Opts.Config.Backend.Region + "-a",
			InstanceType:       "e2-standard-2",
			Disks:              []string{"type=pd-ssd,size=20"},
			Firewalls:          []string{},
			SpotInstance:       false,
			IAMInstanceProfile: "",
			MinCPUPlatform:     "",
			CustomDNS:          InstanceDNS{},
		},
		Docker: InstancesCreateCmdDocker{
			ImageName:          "",
			NetworkName:        "",
			Disks:              []string{},
			ExposePorts:        []string{},
			StopTimeout:        nil,
			Privileged:         false,
			RestartPolicy:      "None",
			MaxRestartRetries:  0,
			ShmSize:            0,
			AdvancedConfigPath: "",
		},
		NoInstallExpiry: false,
		DryRun:          false,
	}
	imagesCreate := &ImagesCreateCmd{
		Name:         instName,
		Description:  "Aerospike Server " + c.AerospikeVersion + " " + flavor + " " + c.Distro + " " + osVersion,
		InstanceName: instName,
		SizeGiB:      20,
		Owner:        c.Owner,
		Type:         "aerospike",
		Version:      c.AerospikeVersion + "-" + flavor,
		Tags:         []string{"aerolab.soft.version=" + c.AerospikeVersion + "-" + flavor},
		DryRun:       false,
	}

	system.Logger.Info("Aerospike Version: %s, Flavor: %s, Distro: %s, OS Version: %s, Arch: %s", c.AerospikeVersion, flavor, c.Distro, osVersion, c.Arch)
	if c.DryRun {
		system.Logger.Info("Dry run, not creating the template")
		if needToVacuum {
			system.Logger.Info("Need to vacuum an existing template creation instance, would destroy the following instances:")
			for _, instance := range instances.Describe() {
				system.Logger.Info("  name=%s, zone=%s, state=%s, tags=%v", instance.Name, instance.ZoneName, instance.InstanceState, instance.Tags)
			}
		}
		y := yaml.NewEncoder(os.Stderr)
		y.SetIndent(2)
		system.Logger.Info("1. InstancesCreateCmd:")
		y.Encode(instancesCreate)
		system.Logger.Info("2. Run Install Script")
		system.Logger.Info("3. InstancesStop")
		system.Logger.Info("4. ImagesCreateCmd:")
		y.Encode(imagesCreate)
		system.Logger.Info("5. InstancesDestroy")
		y.Close()
		system.Logger.Info("Install Script (base64):")
		system.Logger.Info("%s", base64.StdEncoding.EncodeToString(installScript))
		return "", nil
	}

	if needToVacuum {
		system.Logger.Info("Vacuuming an existing template creation instance(s)")
		err := instances.Terminate(time.Minute * 10)
		if err != nil {
			return "", fmt.Errorf("could not vacuum existing template creation instance: %s", err)
		}
	}

	system.Logger.Info("Creating instances")
	inst, err := instancesCreate.CreateInstances(system, inventory, nil, "create")
	if err != nil {
		return "", fmt.Errorf("could not create instances: %s", err)
	}

	shutdown.AddEarlyCleanupJob("template-create-"+instName, func(isSignal bool) {
		if !isSignal {
			return
		}
		if !c.NoVacuum {
			c.NoVacuum = true
			system.Logger.Info("Abort: destroying temporary template creation instances")
			err := inst.Terminate(time.Minute * 10)
			if err != nil {
				system.Logger.Error("could not destroy temporary template creation instances: %s", err)
			}
		}
	})

	defer func() {
		if !c.NoVacuum {
			system.Logger.Info("Destroying temporary template creation instances on failure")
			err := inst.Terminate(time.Minute * 10)
			if err != nil {
				system.Logger.Error("could not destroy temporary template creation instances: %s", err)
			}
		}
	}()

	system.Logger.Info("Uploading install script to instances")
	confs, err := inst.GetSftpConfig("root")
	if err != nil {
		return "", fmt.Errorf("could not get sftp config: %s", err)
	}
	for _, conf := range confs {
		system.Logger.Info("Uploading install script to instance %s", conf.Host)
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return "", fmt.Errorf("could not create sftp client: %s", err)
		}
		err = cli.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/tmp/install.sh",
			Source:      bytes.NewReader(installScript),
			Permissions: 0755,
		})
		cli.Close()
		if err != nil {
			return "", fmt.Errorf("could not upload install script: %s", err)
		}
		system.Logger.Info("Uploaded install script to instance %s, running it now", conf.Host)
		outputs := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "/tmp/install.sh"},
				Terminal:       false,
				SessionTimeout: 15 * time.Minute,
				Stdin:          nil,
				Stdout:         nil,
				Stderr:         nil,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if len(outputs) == 0 {
			return "", fmt.Errorf("no output from install script")
		}
		for _, o := range outputs {
			if o.Output.Err != nil {
				if strings.Contains(o.Output.Err.Error(), "interrupted") {
					return "", fmt.Errorf("installation interrupted by user")
				}
				return "", fmt.Errorf("error running install script: %s\n%s\n%s", o.Output.Err, string(o.Output.Stdout), string(o.Output.Stderr))
			}
		}
	}

	system.Logger.Info("Stopping instances")
	err = inst.Stop(false, time.Minute*10)
	if err != nil {
		return "", fmt.Errorf("could not stop instances: %s", err)
	}

	system.Logger.Info("Creating image")
	inst.Describe()[0].AttachedVolumes = backends.VolumeList{}
	newInst := append(inventory.Instances.Describe(), inst.Describe()...)
	inventory.Instances = newInst
	image, err := imagesCreate.CreateImage(system, inventory, nil)
	if err != nil {
		return "", fmt.Errorf("could not create image: %s", err)
	}

	system.Logger.Info("Destroying temporary instances")
	err = inst.Terminate(time.Minute * 10)
	if err != nil {
		return "", fmt.Errorf("could not destroy temporary instances: %s", err)
	}
	c.NoVacuum = true
	return image.Name, nil
}

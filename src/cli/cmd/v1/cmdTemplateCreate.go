package cmd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
	"gopkg.in/yaml.v3"
)

type TemplateCreateCmd struct {
	Distro           string  `short:"d" long:"distro" description:"Distro to create the template for" default:"ubuntu"`
	DistroVersion    string  `short:"v" long:"distro-version" description:"Version of the distro to create the template for" default:"latest"`
	Arch             string  `short:"a" long:"arch" description:"Architecture to create the template for" default:"amd64"`
	AerospikeVersion string  `short:"A" long:"aerospike-version" description:"Aerospike version to create the template for" default:"latest"`
	Owner            string  `short:"o" long:"owner" description:"Owner of the template"`
	DisablePublicIP  bool    `short:"p" long:"disable-public-ip" description:"Disable public IP assignment to the instances in AWS"`
	Timeout          int     `short:"t" long:"timeout" description:"Set timeout in minutes for the template creation" default:"10"`
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
	_, err = c.CreateTemplate(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func resolveAerospikeServerVersion(aerospikeVersion string) (version *aerospike.Version, flavor string, err error) {
	// find and resolve aerospike version
	products, err := aerospike.GetProducts(time.Second * 10)
	if err != nil {
		return nil, "", fmt.Errorf("could not get products: %s", err)
	}
	products = products.WithNamePrefix("aerospike-server-")
	if strings.HasSuffix(aerospikeVersion, "c") {
		products = products.WithNameSuffix("-community")
		flavor = "community"
	} else if strings.HasSuffix(aerospikeVersion, "f") {
		products = products.WithNameSuffix("-federal")
		flavor = "federal"
	} else {
		products = products.WithNameSuffix("-enterprise")
		flavor = "enterprise"
	}
	if len(products) == 0 {
		return nil, "", fmt.Errorf("aerospike version %s not found", aerospikeVersion)
	}
	versions, err := aerospike.GetVersions(time.Second*10, products[0])
	if err != nil {
		return nil, "", fmt.Errorf("could not get versions: %s", err)
	}
	if strings.HasPrefix(aerospikeVersion, "latest") {
		version = versions.Latest()
		if version == nil {
			return nil, "", fmt.Errorf("version not found")
		}
	} else {
		aerospikeVersion = strings.TrimRight(aerospikeVersion, "cf")
		if strings.HasSuffix(aerospikeVersion, "*") || strings.HasSuffix(aerospikeVersion, ".") {
			versions = versions.WithNamePrefix(strings.TrimSuffix(aerospikeVersion, "*"))
		} else {
			versions = versions.WithName(aerospikeVersion)
		}
		if len(versions) == 0 {
			return nil, "", fmt.Errorf("aerospike version %s not found", aerospikeVersion)
		}
		version = versions.Latest()
		if version == nil {
			return nil, "", fmt.Errorf("version not found")
		}
	}
	return version, flavor, nil
}

func (c *TemplateCreateCmd) CreateTemplate(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (name string, err error) {
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

	version, flavor, err := resolveAerospikeServerVersion(c.AerospikeVersion)
	if err != nil {
		return "", err
	}
	c.AerospikeVersion = version.Name

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

	// add basic tools to the install script
	installScript, err = installers.GetInstallScript(installers.Software{
		Debug: system.logLevel >= 5,
		Optional: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
				{Command: "jq", Package: "jq"},
				{Command: "unzip", Package: "unzip"},
				{Command: "zip", Package: "zip"},
				{Command: "wget", Package: "wget"},
				{Command: "git", Package: "git"},
				{Command: "vim", Package: "vim"},
				{Command: "nano", Package: "nano"},
				{Command: "less", Package: "less"},
				{Command: "lnav", Package: "lnav"},
				{Command: "iptables", Package: "iptables"},
				{Command: "tcpdump", Package: "tcpdump"},
				{Command: "telnet", Package: "telnet"},
				{Command: "mpstat", Package: "sysstat"},
				{Command: "dig", Package: "dnsutils"},   // apt
				{Command: "dig", Package: "bind-utils"}, // yum
				{Command: "strings", Package: "binutils"},
				{Command: "which", Package: "which"},
				{Command: "ip", Package: "iproute2"},       // apt
				{Command: "ip", Package: "iproute"},        // yum
				{Command: "ip", Package: "iproute-tc"},     // yum
				{Command: "python3", Package: "python3"},   // apt and some yum
				{Command: "python3", Package: "python"},    // yum
				{Command: "nc", Package: "netcat"},         // apt
				{Command: "nc", Package: "nc"},             // yum
				{Command: "ping", Package: "iputils-ping"}, // apt
				{Command: "ping", Package: "iputils"},      // yum
				{Command: "ldapsearch", Package: "ldap-utils"},
				{Command: "netstat", Package: "net-tools"},
				{Command: "lsb_release", Package: "lsb-release"},     // apt
				{Command: "lsb_release", Package: "redhat-lsb-core"}, // yum
				{Command: "lsb_release", Package: "redhat-lsb"},      // yum
				{Command: "ps", Package: "procps"},                   // apt
				{Command: "ps", Package: "procps-ng"},                // yum
			},
			Packages: []string{"python3-setuptools", "python3-distutils", "libcurl4", "libcurl4-openssl-dev", "libldap-common", "libcurl-openssl-devel", "initscripts"},
		},
	}, installScript)
	if err != nil {
		return "", fmt.Errorf("could not add basic tools to the install script: %s", err)
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

	if c.Owner == "" {
		c.Owner = currentOwnerUser
	}

	// build the structs for instances create and images create, instances stop and instances destroy commands (optionally vacuum too)
	instName := strings.ToLower(shortuuid.New())
	// Sanitize the instance name for GCP backend to ensure it meets GCP naming requirements
	// (must start with [a-z], end with [a-z0-9], only contain lowercase letters, numbers, and hyphens)
	if system.Opts.Config.Backend.Type == "gcp" {
		instName = sanitizeGCPName(instName)
	}
	// determine instance type based on architecture
	awsInstanceType := "t3.medium"
	gcpInstanceType := "e2-standard-2"
	if c.Arch == "arm64" {
		awsInstanceType = "t4g.medium"
		gcpInstanceType = "t2a-standard-2"
	}
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
			InstanceType:       awsInstanceType,
			Disks:              []string{"type=gp2,size=20"},
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
			InstanceType:       gcpInstanceType,
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
		Timeout:      c.Timeout,
		DryRun:       false,
		IsOfficial:   true,
	}

	logger.Info("Aerospike Version: %s, Flavor: %s, Distro: %s, OS Version: %s, Arch: %s", c.AerospikeVersion, flavor, c.Distro, osVersion, c.Arch)
	if c.DryRun {
		logger.Info("Dry run, not creating the template")
		if needToVacuum {
			logger.Info("Need to vacuum an existing template creation instance, would destroy the following instances:")
			for _, instance := range instances.Describe() {
				logger.Info("  name=%s, zone=%s, state=%s, tags=%v", instance.Name, instance.ZoneName, instance.InstanceState, instance.Tags)
			}
		}
		y := yaml.NewEncoder(os.Stderr)
		y.SetIndent(2)
		logger.Info("1. InstancesCreateCmd:")
		y.Encode(instancesCreate)
		logger.Info("2. Run Install Script")
		logger.Info("3. InstancesStop")
		logger.Info("4. ImagesCreateCmd:")
		y.Encode(imagesCreate)
		logger.Info("5. InstancesDestroy")
		y.Close()
		logger.Info("Install Script (base64):")
		logger.Info("%s", base64.StdEncoding.EncodeToString(installScript))
		return "", nil
	}

	if needToVacuum {
		logger.Info("Vacuuming an existing template creation instance(s)")
		err := instances.Terminate(time.Minute * 10)
		if err != nil {
			return "", fmt.Errorf("could not vacuum existing template creation instance: %s", err)
		}
	}

	logger.Info("Creating instances")
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
			logger.Info("Abort: destroying temporary template creation instances")
			err := inst.Terminate(time.Minute * 10)
			if err != nil {
				logger.Error("could not destroy temporary template creation instances: %s", err)
			}
		}
	})

	defer func() {
		if !c.NoVacuum {
			logger.Info("Destroying temporary template creation instances on failure")
			err := inst.Terminate(time.Minute * 10)
			if err != nil {
				logger.Error("could not destroy temporary template creation instances: %s", err)
			}
		}
	}()

	logger.Info("Uploading install script to instances")
	confs, err := inst.GetSftpConfig("root")
	if err != nil {
		return "", fmt.Errorf("could not get sftp config: %s", err)
	}
	for _, conf := range confs {
		logger.Info("Uploading install script to instance %s", conf.Host)
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
		logger.Info("Uploaded install script to instance %s, running it now", conf.Host)
		var stdout, stderr *os.File
		var stdin io.ReadCloser
		terminal := false
		env := []*sshexec.Env{}
		if system.logLevel >= 5 {
			stdout = os.Stdout
			stderr = os.Stderr
			terminal = true
			stdin = io.NopCloser(os.Stdin)
		}
		execDetail := sshexec.ExecDetail{
			Command:        []string{"bash", "/tmp/install.sh"},
			Terminal:       terminal,
			SessionTimeout: 15 * time.Minute,
			Env:            env,
		}
		if stdin != nil {
			execDetail.Stdin = stdin
		}
		if stdout != nil {
			execDetail.Stdout = stdout
		}
		if stderr != nil {
			execDetail.Stderr = stderr
		}
		outputs := inst.Exec(&backends.ExecInput{
			ExecDetail:      execDetail,
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

	logger.Info("Stopping instances")
	err = inst.Stop(false, time.Minute*10)
	if err != nil {
		return "", fmt.Errorf("could not stop instances: %s", err)
	}

	logger.Info("Creating image")
	// Use the actual instance name from the created instance (in case backend sanitized it further)
	if len(inst.Describe()) > 0 {
		actualInstName := inst.Describe()[0].Name
		imagesCreate.InstanceName = actualInstName
	}
	inst.Describe()[0].AttachedVolumes = backends.VolumeList{}
	newInst := append(inventory.Instances.Describe(), inst.Describe()...)
	inventory.Instances = newInst
	image, err := imagesCreate.CreateImage(system, inventory, logger.WithPrefix("[images.create] "), nil)
	if err != nil {
		return "", fmt.Errorf("could not create image: %s", err)
	}

	logger.Info("Destroying temporary instances")
	err = inst.Terminate(time.Minute * 10)
	if err != nil {
		return "", fmt.Errorf("could not destroy temporary instances: %s", err)
	}
	c.NoVacuum = true
	return image.Name, nil
}

// sanitizeGCPName sanitizes an instance name to meet GCP naming requirements.
// GCP requires: must start with [a-z], end with [a-z0-9], only contain lowercase letters, numbers, and hyphens.
// This matches the sanitize logic in bgcp/tags.go but adapted for CLI use.
func sanitizeGCPName(s string) string {
	ret := ""
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			ret += string(c)
			continue
		}
		if c >= 'A' && c <= 'Z' {
			ret += strings.ToLower(string(c))
			continue
		}
		if c == ' ' || c == '.' || c == '_' {
			ret += "-"
			continue
		}
	}
	for strings.Contains(ret, "--") {
		ret = strings.ReplaceAll(ret, "--", "-")
	}
	// Trim leading hyphens
	ret = strings.TrimLeft(ret, "-")
	// GCP requires names to start with [a-z]
	if len(ret) == 0 || ret[0] < 'a' || ret[0] > 'z' {
		ret = "a" + ret
	}
	// GCP requires names to end with [a-z0-9]
	ret = strings.TrimRight(ret, "-")
	if len(ret) == 0 {
		ret = "a"
	}
	// Final check: ensure it ends with [a-z0-9]
	if len(ret) > 0 {
		lastChar := ret[len(ret)-1]
		if !((lastChar >= 'a' && lastChar <= 'z') || (lastChar >= '0' && lastChar <= '9')) {
			ret = ret + "a"
		}
	}
	return ret
}

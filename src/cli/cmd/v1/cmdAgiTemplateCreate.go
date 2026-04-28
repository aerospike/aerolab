//go:build !noagi

package cmd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/agi"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerolab"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike"
	"github.com/aerospike/aerolab/pkg/utils/installers/filebrowser"
	"github.com/aerospike/aerolab/pkg/utils/installers/grafana"
	"github.com/aerospike/aerolab/pkg/utils/installers/ttyd"
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	"github.com/lithammer/shortuuid"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
	"gopkg.in/yaml.v3"
)

// AgiTemplateCreateCmd creates a new AGI template image with all required software pre-installed.
// This is the most complex command in Phase 1, handling multi-step orchestration
// of software installation and image creation.
//
// The template includes:
//   - Grafana OSS
//   - ttyd (web terminal)
//   - filebrowser
//   - aerolab binary with symlinks
//   - systemd service files for all AGI services
//   - Default directory structure
//   - Default SSL certificates
type AgiTemplateCreateCmd struct {
	GrafanaVersion  string         `short:"g" long:"grafana-version" description:"Grafana version to install" default:"11.2.6"`
	ToolsVersion    string         `long:"tools-version" description:"Aerospike tools version to install ('latest' picks the newest)" default:"latest"`
	Distro          string         `short:"d" long:"distro" description:"Linux distribution to use" default:"ubuntu"`
	DistroVersion   string         `short:"i" long:"distro-version" description:"Distribution version to use" default:"latest"`
	Arch            string         `short:"a" long:"arch" description:"Architecture (amd64 or arm64)" default:"amd64"`
	Timeout         int            `short:"t" long:"timeout" description:"Timeout in minutes for template creation" default:"20"`
	NoVacuum        bool           `short:"n" long:"no-vacuum" description:"Don't cleanup temporary instance on failure"`
	DryRun          bool           `long:"dry-run" description:"Validate parameters only, don't create template"`
	Owner           string         `short:"o" long:"owner" description:"Owner tag value for the template"`
	DisablePublicIP bool           `short:"p" long:"disable-public-ip" description:"AWS: Disable public IP assignment"`
	AerolabBinary   flags.Filename `short:"b" long:"aerolab-binary" description:"Path to local aerolab binary to install (required if running unofficial build)"`
	WithEFS         bool           `short:"e" long:"with-efs" description:"AWS: Pre-install EFS utilities in template for faster AGI creation"`
	MaxRetries      int            `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep      time.Duration  `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`
	Help            HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi template create.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiTemplateCreateCmd) Execute(args []string) error {
	cmd := []string{"agi", "template", "create"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	_, err = c.CreateTemplate(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// CreateTemplate creates an AGI template image with all required software pre-installed.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - string: The name of the created template image
//   - error: nil on success, or an error describing what failed
func (c *AgiTemplateCreateCmd) CreateTemplate(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (string, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "template", "create"}, c, args...)
		if err != nil {
			return "", err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Check if running unofficial build and handle local binary
	_, _, edition, currentAerolabVersion := GetAerolabVersion()
	isUnofficial := strings.Contains(edition, "unofficial")
	useLocalBinary := c.AerolabBinary != ""

	logger.Debug("Template creation: edition=%s, currentVersion=%s, isUnofficial=%t, useLocalBinary=%t, AerolabBinary=%q",
		edition, currentAerolabVersion, isUnofficial, useLocalBinary, c.AerolabBinary)

	// For unofficial builds without explicit binary: try to use self if on Linux with matching arch
	if isUnofficial && !useLocalBinary {
		hostArch := runtime.GOARCH // "amd64" or "arm64"
		targetArch := c.Arch       // "amd64" or "arm64"
		if runtime.GOOS == "linux" && hostArch == targetArch {
			// We can use the current executable
			execPath, err := os.Executable()
			if err != nil {
				return "", fmt.Errorf("running unofficial aerolab build (%s) and failed to get executable path: %w; use --aerolab-binary to specify the path manually", currentAerolabVersion, err)
			}
			c.AerolabBinary = flags.Filename(execPath)
			useLocalBinary = true
			logger.Info("Running unofficial build on Linux with matching architecture (%s), using self: %s", hostArch, execPath)
		} else {
			return "", fmt.Errorf("running unofficial aerolab build (%s); --aerolab-binary flag is required to specify the path to a Linux aerolab binary matching the target architecture (%s) (host: %s/%s)", currentAerolabVersion, c.Arch, runtime.GOOS, hostArch)
		}
	}

	// If local binary is specified, verify it exists
	if useLocalBinary {
		if _, err := os.Stat(string(c.AerolabBinary)); os.IsNotExist(err) {
			return "", fmt.Errorf("aerolab binary not found: %s", c.AerolabBinary)
		}
		logger.Info("Using local aerolab binary: %s (skipAerolabDownload will be true)", c.AerolabBinary)
	} else {
		logger.Info("No local aerolab binary specified, will download version: %s", currentAerolabVersion)
	}

	// Resolve OS distro version (default to latest stable for the distro)
	osName := strings.ToLower(c.Distro)
	if osName == "rocky" {
		osName = "centos"
	}
	osVersion := c.DistroVersion
	if osVersion == "latest" {
		switch osName {
		case "ubuntu":
			osVersion = "24.04"
		case "centos":
			osVersion = "10"
		case "rocky":
			osVersion = "10"
		case "debian":
			osVersion = "13"
		case "amazon":
			osVersion = "2023"
		default:
			return "", fmt.Errorf("unsupported distro: %s", osName)
		}
	}

	// Check if the AGI template already exists (match by AGI version + arch)
	var backendArch backends.Architecture
	if err := backendArch.FromString(c.Arch); err != nil {
		return "", fmt.Errorf("invalid architecture: %w", err)
	}

	templateVersionTag := fmt.Sprintf("agi-%s-%d", c.Arch, agi.AGIVersion)

	// Function to check if template exists and handle duplicates
	checkTemplateExists := func() (string, bool) {
		images := inventory.Images.WithTags(map[string]string{
			"aerolab.image.type":  "agi",
			"aerolab.agi.version": strconv.Itoa(agi.AGIVersion),
		}).WithArchitecture(backendArch)
		if images.Count() > 0 {
			// Handle potential duplicates from race condition
			if images.Count() > 1 {
				img, err := CleanupDuplicateTemplates(images.Describe(), logger)
				if err != nil {
					return "", false
				}
				return img.Name, true
			}
			return images.Describe()[0].Name, true
		}
		return "", false
	}

	// Check if template already exists
	if name, exists := checkTemplateExists(); exists {
		return "", fmt.Errorf("AGI template with version %d already exists for architecture %s: %s", agi.AGIVersion, c.Arch, name)
	}

	// Initialize race handler for template creation
	raceHandler, err := newTemplateCreationRaceHandler(system.Opts.Config.Backend.Type, templateVersionTag, logger)
	if err != nil {
		return "", err
	}

	// Check for existing dangling template creation instances
	instances := inventory.Instances.WithTags(map[string]string{
		"aerolab.type":         "images.create",
		"aerolab.tmpl.version": templateVersionTag,
	}).WithNotState(backends.LifeCycleStateTerminated)

	// Handle race condition
	raceResult, err := raceHandler.CheckForRaceCondition(instances.Describe(), func() (string, bool) {
		// Refresh inventory to check for new templates
		system.Backend.ForceRefreshInventory()
		inventory = system.Backend.GetInventory()
		return checkTemplateExists()
	})
	if err != nil {
		return "", fmt.Errorf("failed to check for race condition: %w", err)
	}

	// If template was created by another process while we waited, return it
	if raceResult.TemplateExists {
		logger.Info("Template created by another process, using: %s", raceResult.TemplateName)
		return raceResult.TemplateName, nil
	}

	needToVacuum := raceResult.ShouldVacuum

	if c.Owner == "" {
		c.Owner = currentOwnerUser
	}

	// Build the install script combining all components
	logger.Debug("Building install script with skipAerolabDownload=%t", useLocalBinary)
	installScript, err := c.buildInstallScript(system, useLocalBinary)
	if err != nil {
		return "", fmt.Errorf("could not build install script: %w", err)
	}

	// Build structs for instance creation and image creation
	instName := strings.ToLower("agi-tmpl-" + shortuuid.New())
	if system.Opts.Config.Backend.Type == "gcp" {
		instName = sanitizeGCPName(instName)
	}

	// Determine instance type based on architecture
	awsInstanceType := "t3.medium"
	gcpInstanceType := "e2-standard-2"
	if c.Arch == "arm64" {
		awsInstanceType = "t4g.medium"
		gcpInstanceType = "t2a-standard-2"
	}

	// Get race handler tags for coordinating with other processes
	raceHandlerTags, err := raceHandler.GetInstanceTags()
	if err != nil {
		return "", fmt.Errorf("failed to get race handler tags: %w", err)
	}
	instanceTags := []string{fmt.Sprintf("aerolab.tmpl.version=%s", templateVersionTag)}
	for k, v := range raceHandlerTags {
		instanceTags = append(instanceTags, fmt.Sprintf("%s=%s", k, v))
	}

	instancesCreate := &InstancesCreateCmd{
		ClusterName:        instName,
		Count:              1,
		Name:               instName,
		Owner:              c.Owner,
		Type:               "images.create",
		Tags:               instanceTags,
		Description:        "temporary AGI template creation instance",
		TerminateOnStop:    false,
		ParallelSSHThreads: 1,
		SSHKeyName:         "",
		OS:                 c.Distro,
		Version:            osVersion,
		Arch:               c.Arch,
		AWS: InstancesCreateCmdAws{
			ImageID:            "",
			Expire:             TypeExpiry(time.Duration(c.Timeout) * time.Minute),
			NetworkPlacement:   system.Opts.Config.Backend.Region,
			InstanceType:       guiInstanceType(awsInstanceType),
			Disks:              []string{"type=gp2,size=30"},
			Firewalls:          []string{},
			SpotInstance:       false,
			DisablePublicIP:    c.DisablePublicIP,
			IAMInstanceProfile: "",
			CustomDNS:          InstanceDNS{},
		},
		GCP: InstancesCreateCmdGcp{
			ImageName:          "",
			Expire:             TypeExpiry(time.Duration(c.Timeout) * time.Minute),
			Zone:               guiZone(system.Opts.Config.Backend.Region + "-a"),
			InstanceType:       guiInstanceType(gcpInstanceType),
			Disks:              []string{"type=pd-ssd,size=30"},
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
		NoInstallExpiry:           false,
		DryRun:                    false,
		suppressEquivalentCommand: true,
	}

	// Build version string for the image
	_, _, _, aerolabVersion := GetAerolabVersion()
	imageVersionString := fmt.Sprintf("agi-%s-%d", aerolabVersion, agi.AGIVersion)

	imageTags := []string{
		fmt.Sprintf("aerolab.agi.version=%d", agi.AGIVersion),
		fmt.Sprintf("aerolab.agi.grafana=%s", c.GrafanaVersion),
		fmt.Sprintf("aerolab.agi.aerolab=%s", aerolabVersion),
		"aerolab.is.official=true",
	}
	// Add EFS tag if EFS utilities were pre-installed
	if c.WithEFS {
		imageTags = append(imageTags, "aerolab.agi.efs=true")
	}

	imagesCreate := &ImagesCreateCmd{
		Name:         instName,
		Description:  fmt.Sprintf("AGI Template v%d - Grafana %s, %s %s", agi.AGIVersion, c.GrafanaVersion, c.Distro, osVersion),
		InstanceName: instName,
		SizeGiB:      30,
		Owner:        c.Owner,
		Type:         "agi",
		Version:      imageVersionString,
		Tags:         imageTags,
		Timeout:      c.Timeout,
		DryRun:       false,
		IsOfficial:   true,
	}

	logger.Info("AGI Template Configuration:")
	logger.Info("  AGI Version: %d", agi.AGIVersion)
	logger.Info("  Grafana Version: %s", c.GrafanaVersion)
	logger.Info("  Distro: %s %s", c.Distro, osVersion)
	logger.Info("  Architecture: %s", c.Arch)

	if c.DryRun {
		logger.Info("Dry run, not creating template")
		if needToVacuum {
			logger.Info("Need to vacuum existing template creation instance(s):")
			for _, inst := range instances.Describe() {
				logger.Info("  name=%s, zone=%s, state=%s", inst.Name, inst.ZoneName, inst.InstanceState)
			}
		}
		y := yaml.NewEncoder(os.Stderr)
		y.SetIndent(2)
		logger.Info("1. InstancesCreateCmd:")
		//nolint:errcheck
		y.Encode(instancesCreate)
		logger.Info("2. Run Install Script")
		logger.Info("3. Stop Instance")
		logger.Info("4. ImagesCreateCmd:")
		//nolint:errcheck
		y.Encode(imagesCreate)
		logger.Info("5. Destroy Temporary Instance")
		y.Close()
		logger.Info("Install Script (base64):")
		logger.Info("%s", base64.StdEncoding.EncodeToString(installScript))
		return "", nil
	}

	if needToVacuum {
		logger.Info("Vacuuming existing template creation instance(s)")
		err := instances.Terminate(time.Minute * 10)
		if err != nil {
			return "", fmt.Errorf("could not vacuum existing template creation instance: %w", err)
		}
	}

	// Create temporary instance
	logger.Info("Creating temporary instance for AGI template")
	inst, err := instancesCreate.CreateInstances(system, inventory, nil, "create")
	if err != nil {
		return "", fmt.Errorf("could not create temporary instance: %w", err)
	}

	// Start heartbeat to signal we're actively working (for AWS/GCP race coordination)
	stopHeartbeat := raceHandler.StartHeartbeat(inst)
	defer stopHeartbeat()

	// Add early cleanup job for interrupt handling
	shutdown.AddEarlyCleanupJob("agi-template-create-"+instName, func(isSignal bool) {
		if !isSignal {
			return
		}
		if !c.NoVacuum {
			c.NoVacuum = true
			logger.Info("Abort: destroying temporary AGI template creation instance")
			err := inst.Terminate(time.Minute * 10)
			if err != nil {
				logger.Error("could not destroy temporary instance: %s", err)
			}
		}
	})

	// Defer cleanup on failure
	defer func() {
		if !c.NoVacuum {
			logger.Info("Destroying temporary template creation instance on failure")
			err := inst.Terminate(time.Minute * 10)
			if err != nil {
				logger.Error("could not destroy temporary instance: %s", err)
			}
		}
	}()

	// Upload and run install script
	logger.Info("Uploading install script to instance")
	confs, err := inst.GetSftpConfig("root")
	if err != nil {
		return "", fmt.Errorf("could not get sftp config: %w", err)
	}

	for _, conf := range confs {
		conf.MaxRetries = c.MaxRetries
		conf.RetrySleep = c.RetrySleep
		logger.Info("Uploading install script to instance %s", conf.Host)
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return "", fmt.Errorf("could not create sftp client: %w", err)
		}
		err = cli.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/opt/aerolab/scripts/agi-install.sh",
			Source:      bytes.NewReader(installScript),
			Permissions: 0755,
		})
		if err != nil {
			cli.Close()
			return "", fmt.Errorf("could not upload install script: %w", err)
		}

		// Create AGI marker file to prevent auto-downgrade from overwriting the aerolab binary
		// This must be created BEFORE uploading the binary or running any aerolab commands
		err = cli.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/opt/aerolab-agi-exec",
			Source:      bytes.NewReader([]byte("AGI_EXEC_MARKER")),
			Permissions: 0644,
		})
		if err != nil {
			cli.Close()
			return "", fmt.Errorf("could not create AGI marker file: %w", err)
		}

		// Upload local aerolab binary BEFORE running install script (if specified)
		if useLocalBinary {
			logger.Info("Uploading local aerolab binary to instance %s", conf.Host)
			binaryData, err := os.ReadFile(string(c.AerolabBinary))
			if err != nil {
				cli.Close()
				return "", fmt.Errorf("could not read local aerolab binary: %w", err)
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/usr/local/bin/aerolab",
				Source:      bytes.NewReader(binaryData),
				Permissions: 0755,
			})
			if err != nil {
				cli.Close()
				return "", fmt.Errorf("could not upload aerolab binary: %w", err)
			}
		}
		cli.Close()

		logger.Info("Running install script on instance %s", conf.Host)
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
			Command:        []string{"bash", "/opt/aerolab/scripts/agi-install.sh"},
			Terminal:       terminal,
			SessionTimeout: time.Duration(c.Timeout) * time.Minute,
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

		scriptPath := "/opt/aerolab/scripts/agi-install.sh"
		outputs := inst.Exec(&backends.ExecInput{
			ExecDetail:      execDetail,
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
			MaxRetries:      c.MaxRetries,
			RetrySleep:      c.RetrySleep,
		})
		if len(outputs) == 0 {
			return "", fmt.Errorf("no output from install script")
		}
		for _, o := range outputs {
			if o.Output.Err != nil {
				if strings.Contains(o.Output.Err.Error(), "interrupted") {
					return "", fmt.Errorf("installation interrupted by user")
				}
				// Save script failure to local machine for debugging
				failure := scriptlog.NewScriptFailureWithPath(
					instName,
					1,
					scriptPath,
					installScript,
					o.Output.Stdout,
					o.Output.Stderr,
					o.Output.Err,
				)
				logPath, saveErr := scriptlog.SaveFailure(failure)
				if saveErr != nil {
					return "", fmt.Errorf("error running install script: %s\n%s\n%s (also failed to save logs: %v)", o.Output.Err, string(o.Output.Stdout), string(o.Output.Stderr), saveErr)
				}
				return "", fmt.Errorf("%s", scriptlog.FormatError(logPath, instName, 1, o.Output.Err))
			}
		}
	}

	// Install EFS utilities if requested (AWS only)
	if c.WithEFS && system.Opts.Config.Backend.Type == "aws" {
		logger.Info("Installing EFS utilities in template")
		for _, instance := range inst.Describe() {
			err := baws.EfsInstall(&baws.EfsInstallInput{
				Instance: instance,
			})
			if err != nil {
				return "", fmt.Errorf("could not install EFS utilities: %w", err)
			}
		}
		logger.Info("EFS utilities installed successfully")
	}

	// Stop instance before creating image
	logger.Info("Stopping instance")
	err = inst.Stop(false, time.Minute*10)
	if err != nil {
		return "", fmt.Errorf("could not stop instance: %w", err)
	}

	// Create image from the instance
	logger.Info("Creating AGI template image")
	if len(inst.Describe()) > 0 {
		actualInstName := inst.Describe()[0].Name
		imagesCreate.InstanceName = actualInstName
	}
	inst.Describe()[0].AttachedVolumes = backends.VolumeList{}
	newInst := append(inventory.Instances.Describe(), inst.Describe()...)
	inventory.Instances = newInst

	image, err := imagesCreate.CreateImage(system, inventory, logger.WithPrefix("[images.create] "), nil)
	if err != nil {
		return "", fmt.Errorf("could not create image: %w", err)
	}

	// Destroy temporary instance
	logger.Info("Destroying temporary instance")
	err = inst.Terminate(time.Minute * 10)
	if err != nil {
		return "", fmt.Errorf("could not destroy temporary instance: %w", err)
	}
	c.NoVacuum = true

	// Signal successful completion (clears session file)
	raceHandler.OnSuccess()

	// Check for and cleanup any duplicate templates that may have been created
	// due to race conditions (two users starting at exactly the same time)
	system.Backend.ForceRefreshInventory()
	inventory = system.Backend.GetInventory()
	finalImages := inventory.Images.WithTags(map[string]string{
		"aerolab.image.type":  "agi",
		"aerolab.agi.version": strconv.Itoa(agi.AGIVersion),
	}).WithArchitecture(backendArch)
	if finalImages.Count() > 1 {
		finalImage, err := CleanupDuplicateTemplates(finalImages.Describe(), logger)
		if err != nil {
			logger.Warn("Failed to cleanup duplicate templates: %s", err)
		} else {
			image = finalImage
		}
	}

	logger.Info("AGI template created successfully: %s", image.Name)
	return image.Name, nil
}

// buildInstallScript creates the complete installation script for the AGI template.
// This combines all software installations and configurations into a single script.
// If skipAerolabDownload is true, the aerolab binary download is skipped (binary will be uploaded separately).
func (c *AgiTemplateCreateCmd) buildInstallScript(system *System, skipAerolabDownload bool) ([]byte, error) {
	var script bytes.Buffer

	// Script header
	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n")
	script.WriteString("echo '=== AGI Template Installation Script ==='\n")
	fmt.Fprintf(&script, "echo 'AGI Version: %d'\n", agi.AGIVersion) //nolint:errcheck
	script.WriteString("\n")

	// Install base tools
	baseToolsScript, err := installers.GetInstallScript(installers.Software{
		Debug: system.logLevel >= 5,
		Optional: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
				{Command: "wget", Package: "wget"},
				{Command: "jq", Package: "jq"},
				{Command: "vim", Package: "vim"},
				{Command: "nano", Package: "nano"},
				{Command: "less", Package: "less"},
				{Command: "lnav", Package: "lnav"},
				{Command: "unzip", Package: "unzip"},
				{Command: "git", Package: "git"},
				{Command: "netstat", Package: "net-tools"},
				{Command: "lsb_release", Package: "lsb-release"},
				{Command: "lsb_release", Package: "redhat-lsb-core"},
				{Command: "lsb_release", Package: "redhat-lsb"},
				{Command: "ps", Package: "procps"},
				{Command: "ps", Package: "procps-ng"},
			},
			Packages: []string{"ca-certificates", "gnupg", "openssl", "ssl-cert"},
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create base tools install script: %w", err)
	}

	script.WriteString("echo '=== Installing Base Tools ==='\n")
	script.Write(baseToolsScript) //nolint:errcheck // bytes.Buffer.Write never fails
	script.WriteString("\n")

	// Install Grafana
	grafanaScript, err := grafana.GetInstallScript(c.GrafanaVersion, false, false)
	if err != nil {
		return nil, fmt.Errorf("could not create grafana install script: %w", err)
	}
	script.WriteString("echo '=== Installing Grafana ==='\n")
	script.Write(grafanaScript) //nolint:errcheck // bytes.Buffer.Write never fails
	script.WriteString("\n")

	// Install ttyd
	ttydScript, err := ttyd.GetLinuxInstallScript("/usr/local/bin/ttyd", false, false)
	if err != nil {
		return nil, fmt.Errorf("could not create ttyd install script: %w", err)
	}
	script.WriteString("echo '=== Installing ttyd ==='\n")
	script.Write(ttydScript) //nolint:errcheck // bytes.Buffer.Write never fails
	script.WriteString("\n")

	// Install filebrowser
	filebrowserScript, err := filebrowser.GetLinuxInstallScript("/usr/local/bin/filebrowser", false, false)
	if err != nil {
		return nil, fmt.Errorf("could not create filebrowser install script: %w", err)
	}
	script.WriteString("echo '=== Installing filebrowser ==='\n")
	script.Write(filebrowserScript) //nolint:errcheck // bytes.Buffer.Write never fails
	script.WriteString("\n")

	// Install aerolab binary (same version as the user is running)
	script.WriteString("echo '=== Installing aerolab ==='\n")
	if skipAerolabDownload {
		// Binary was uploaded via SFTP before the script runs
		script.WriteString("echo 'Using pre-uploaded aerolab binary'\n")
	} else {
		_, _, _, currentAerolabVersion := GetAerolabVersion()
		aerolabScript, err := aerolab.GetLinuxInstallScript(currentAerolabVersion, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("could not create aerolab install script: %w", err)
		}
		script.Write(aerolabScript) //nolint:errcheck // bytes.Buffer.Write never fails
	}
	script.WriteString("\n")
	script.WriteString("# Create symlinks for aerolab commands\n")
	script.WriteString("/usr/local/bin/aerolab showcommands --destination=/usr/local/bin || true\n")
	script.WriteString("\n")
	script.WriteString("# Configure aerolab backend to 'none' since AGI exec commands don't need a backend\n")
	script.WriteString("/usr/local/bin/aerolab config backend -t none\n")
	script.WriteString("\n")

	// Install aerospike-tools (asadm, aql, asinfo, …). The Aerospike
	// server itself is intentionally NOT installed on AGI boxes —
	// AGI ingests exported data, it doesn't run a local cluster —
	// but operators still expect asadm/aql to be available for
	// inspecting collected logs and connecting to remote clusters
	// from the AGI shell. This was a feature regression when the
	// Aerospike-server install step was removed; restore tools.
	toolsScript, err := c.buildAerospikeToolsScript()
	if err != nil {
		return nil, fmt.Errorf("could not create aerospike-tools install script: %w", err)
	}
	script.WriteString("echo '=== Installing Aerospike tools ==='\n")
	script.Write(toolsScript) //nolint:errcheck // bytes.Buffer.Write never fails
	script.WriteString("\n")

	// Create directory structure
	script.WriteString("echo '=== Creating AGI Directory Structure ==='\n")
	script.WriteString(`
mkdir -p /opt/agi/files/input
mkdir -p /opt/agi/files/input/s3source
mkdir -p /opt/agi/files/input/sftpsource
mkdir -p /opt/agi/files/logs
mkdir -p /opt/agi/files/collectinfo
mkdir -p /opt/agi/files/other
mkdir -p /opt/agi/files/no-stat
mkdir -p /opt/agi/ingest
mkdir -p /opt/agi/tokens
mkdir -p /opt/agi/www
mkdir -p /var/log
`)
	script.WriteString("\n")

	// Create erro helper script
	script.WriteString("echo '=== Creating erro helper script ==='\n")
	script.WriteString(`cat > /usr/local/bin/erro << 'ERROEOF'
#!/bin/bash
grep -i 'error\|warn\|timeout' "$@"
ERROEOF
chmod 755 /usr/local/bin/erro
`)
	script.WriteString("\n")

	// Add LS_COLORS to bashrc
	script.WriteString("echo '=== Configuring bashrc ==='\n")
	script.WriteString(`
if ! grep -q "LS_COLORS" /root/.bashrc; then
    echo 'export LS_COLORS="rs=0:di=01;34:ln=01;36:mh=00:pi=40;33:so=01;35:do=01;35:bd=40;33;01:cd=40;33;01:or=40;31;01:mi=00:su=37;41:sg=30;43:ca=30;41:tw=30;42:ow=34;42:st=37;44:ex=01;32:*.tar=01;31:*.tgz=01;31:*.arc=01;31:*.arj=01;31:*.taz=01;31:*.lha=01;31:*.lz4=01;31:*.lzh=01;31:*.lzma=01;31:*.tlz=01;31:*.txz=01;31:*.tzo=01;31:*.t7z=01;31:*.zip=01;31:*.z=01;31:*.dz=01;31:*.gz=01;31:*.lrz=01;31:*.lz=01;31:*.lzo=01;31:*.xz=01;31:*.zst=01;31:*.tzst=01;31:*.bz2=01;31:*.bz=01;31:*.tbz=01;31:*.tbz2=01;31:*.tz=01;31:*.deb=01;31:*.rpm=01;31:*.jar=01;31:*.war=01;31:*.ear=01;31:*.sar=01;31:*.rar=01;31:*.alz=01;31:*.ace=01;31:*.zoo=01;31:*.cpio=01;31:*.7z=01;31:*.rz=01;31:*.cab=01;31:*.wim=01;31:*.swm=01;31:*.dwm=01;31:*.esd=01;31:*.jpg=01;35:*.jpeg=01;35:*.mjpg=01;35:*.mjpeg=01;35:*.gif=01;35:*.bmp=01;35:*.pbm=01;35:*.pgm=01;35:*.ppm=01;35:*.tga=01;35:*.xbm=01;35:*.xpm=01;35:*.tif=01;35:*.tiff=01;35:*.png=01;35:*.svg=01;35:*.svgz=01;35:*.mng=01;35:*.pcx=01;35:*.mov=01;35:*.mpg=01;35:*.mpeg=01;35:*.m2v=01;35:*.mkv=01;35:*.webm=01;35:*.ogm=01;35:*.mp4=01;35:*.m4v=01;35:*.mp4v=01;35:*.vob=01;35:*.qt=01;35:*.nuv=01;35:*.wmv=01;35:*.asf=01;35:*.rm=01;35:*.rmvb=01;35:*.flc=01;35:*.avi=01;35:*.fli=01;35:*.flv=01;35:*.gl=01;35:*.dl=01;35:*.xcf=01;35:*.xwd=01;35:*.yuv=01;35:*.cgm=01;35:*.emf=01;35:*.ogv=01;35:*.ogx=01;35:*.aac=00;36:*.au=00;36:*.flac=00;36:*.m4a=00;36:*.mid=00;36:*.midi=00;36:*.mka=00;36:*.mp3=00;36:*.mpc=00;36:*.ogg=00;36:*.ra=00;36:*.wav=00;36:*.oga=00;36:*.opus=00;36:*.spx=00;36:*.xspf=00;36:"' >> /root/.bashrc
fi
`)
	script.WriteString("\n")

	// NOTE: UUID for monitor auth is generated at runtime by agi-proxy or notifier
	// on first use, not during template creation. This ensures each AGI instance
	// has a unique secret.

	// Generate default self-signed SSL certificates
	script.WriteString("echo '=== Generating default SSL certificates ==='\n")
	script.WriteString(`
if [ ! -f /opt/agi/proxy.cert ] || [ ! -f /opt/agi/proxy.key ]; then
    openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
        -keyout /opt/agi/proxy.key \
        -out /opt/agi/proxy.cert \
        -subj "/C=US/ST=California/L=San Jose/O=Aerospike/OU=AeroLab/CN=agi.aerolab.local"
    chmod 644 /opt/agi/proxy.cert
    chmod 600 /opt/agi/proxy.key
fi
`)
	script.WriteString("\n")

	// Create systemd service files
	script.WriteString("echo '=== Creating systemd service files ==='\n")

	// agi-plugin service: runs the merged ingest+plugin service.
	// The unit file name is kept as agi-plugin.service for
	// backward compatibility with existing tooling (systemctl
	// start/stop/status scripts, log readers, etc.) but the ExecStart
	// now launches the unified "agi exec service" which shares a
	// single Pebble DB handle between the ingest pipeline and the
	// plugin HTTP server. Running them as separate processes caused
	// the second process to fail to acquire Pebble's exclusive file
	// lock on the shared data directory — hence this consolidation.
	script.WriteString(`
cat > /etc/systemd/system/agi-plugin.service << 'EOF'
[Unit]
Description=AGI Service (merged ingest + Grafana plugin backend)
After=network.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/aerolab agi exec service
Restart=always
RestartSec=5
# Give the service time to drain in-flight ingest work and flush
# the Pebble memtable on shutdown. Default of 90s is not enough
# for large log volumes; match the plugin's shutdownTimeout (60s)
# plus a safety margin.
TimeoutStopSec=120
StandardOutput=append:/var/log/agi-plugin.log
StandardError=append:/var/log/agi-plugin.log

[Install]
WantedBy=multi-user.target
EOF
`)

	// agi-proxy service
	script.WriteString(`
cat > /etc/systemd/system/agi-proxy.service << 'EOF'
[Unit]
Description=AGI Web Proxy
After=network.target grafana-server.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/opt/agi
ExecStart=/usr/local/bin/aerolab agi exec proxy -y /opt/agi/proxy.yaml
Restart=always
RestartSec=5
StandardOutput=append:/var/log/agi-proxy.log
StandardError=append:/var/log/agi-proxy.log

[Install]
WantedBy=multi-user.target
EOF
`)

	// agi-grafanafix service
	script.WriteString(`
cat > /etc/systemd/system/agi-grafanafix.service << 'EOF'
[Unit]
Description=AGI Grafana Helper
After=network.target grafana-server.service

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/aerolab agi exec grafanafix
Restart=always
RestartSec=5
StandardOutput=append:/var/log/agi-grafanafix.log
StandardError=append:/var/log/agi-grafanafix.log

[Install]
WantedBy=multi-user.target
EOF
`)

	// agi-ttyd service (customized for AGI)
	script.WriteString(`
cat > /etc/systemd/system/agi-ttyd.service << 'EOF'
[Unit]
Description=AGI Web Terminal (ttyd)
After=network.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/ttyd -p 7681 -W bash
Restart=always
RestartSec=5
StandardOutput=append:/var/log/agi-ttyd.log
StandardError=append:/var/log/agi-ttyd.log

[Install]
WantedBy=multi-user.target
EOF
`)

	// agi-filebrowser service (customized for AGI)
	script.WriteString(`
cat > /etc/systemd/system/agi-filebrowser.service << 'EOF'
[Unit]
Description=AGI File Browser
After=network.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/filebrowser -r /opt/agi/files -a 0.0.0.0 -p 8080 --noauth -d /opt/agi/filebrowser.db
Restart=always
RestartSec=5
WorkingDirectory=/opt/agi
StandardOutput=append:/var/log/agi-filebrowser.log
StandardError=append:/var/log/agi-filebrowser.log

[Install]
WantedBy=multi-user.target
EOF
`)

	// Reload systemd and enable services (but don't start them)
	script.WriteString(`
echo '=== Reloading systemd (services will be enabled during AGI instance creation) ==='
systemctl daemon-reload
# NOTE: Services are NOT enabled here - they will be enabled and started
# during AGI instance creation after configs are uploaded
`)
	script.WriteString("\n")

	// Configure Grafana basic settings
	script.WriteString("echo '=== Configuring Grafana ==='\n")
	script.WriteString(`
# Disable Grafana authentication (proxy handles auth)
if [ -f /etc/grafana/grafana.ini ]; then
    sed -i 's/^;*\(http_port\s*=\s*\).*/\13000/' /etc/grafana/grafana.ini
    # Add anonymous auth configuration if not present
    if ! grep -q "^\[auth.anonymous\]" /etc/grafana/grafana.ini; then
        echo "" >> /etc/grafana/grafana.ini
        echo "[auth.anonymous]" >> /etc/grafana/grafana.ini
        echo "enabled = true" >> /etc/grafana/grafana.ini
        echo "org_role = Admin" >> /etc/grafana/grafana.ini
    fi
    # Set root_url for proxy
    sed -i 's|^;*\(root_url\s*=\s*\).*|\1%(protocol)s://%(domain)s:%(http_port)s/grafana/|' /etc/grafana/grafana.ini
    sed -i 's|^;*\(serve_from_sub_path\s*=\s*\).*|\1true|' /etc/grafana/grafana.ini
fi

# Create /run/grafana directory with proper permissions (required for grafana-server to start)
mkdir -p /run/grafana
chown grafana:grafana /run/grafana
chmod 755 /run/grafana

# Create tmpfiles.d config to recreate /run/grafana on boot
echo 'd /run/grafana 0755 grafana grafana -' > /etc/tmpfiles.d/grafana.conf
`)
	script.WriteString("\n")

	// Cleanup and finalize
	script.WriteString("echo '=== Cleanup ==='\n")
	script.WriteString(`
rm -f /tmp/agi-install.sh
apt-get clean || yum clean all || true
`)
	script.WriteString("\n")

	script.WriteString("echo '=== AGI Template Installation Complete ==='\n")
	fmt.Fprintf(&script, "echo 'AGI Version: %d'\n", agi.AGIVersion) //nolint:errcheck

	return script.Bytes(), nil
}

// buildAerospikeToolsScript resolves the aerospike-tools artifact for
// the template's distro/arch via the Aerospike artifacts API and
// returns a self-contained bash snippet that downloads + installs
// it on the temporary template instance.
//
// We resolve the artifact at build-time (on the operator's machine)
// rather than baking the lookup into the script because:
//   - The artifacts metadata API requires HTTP access and HTML
//     scraping; doing this on the operator's host keeps the
//     in-image dependency surface minimal (the script only needs
//     curl + the tarball URL).
//   - Failure modes (e.g. unsupported distro/version, network
//     blackhole on the operator's host) surface during template
//     creation with a clear error, instead of silently bricking the
//     temporary instance partway through bash -e.
//
// The Aerospike server itself is deliberately NOT installed; AGI
// ingests exported logs and never runs a local cluster.
func (c *AgiTemplateCreateCmd) buildAerospikeToolsScript() ([]byte, error) {
	// Map AGI's distro flag onto the OS names the artifacts API
	// uses. Rocky doesn't have its own artifact line; it shares
	// CentOS RPMs (matching cmdAgiTemplateCreate's earlier alias).
	osName := strings.ToLower(c.Distro)
	if osName == "rocky" {
		osName = "centos"
	}
	osVersion := c.DistroVersion
	if osVersion == "latest" {
		switch osName {
		case "ubuntu":
			osVersion = "24.04"
		case "centos":
			osVersion = "10"
		case "debian":
			osVersion = "13"
		case "amazon":
			osVersion = "2023"
		default:
			return nil, fmt.Errorf("unsupported distro for aerospike-tools: %s", osName)
		}
	}

	var arch aerospike.ArchitectureType
	switch c.Arch {
	case "amd64", "x86_64":
		arch = aerospike.ArchitectureTypeX86_64
	case "arm64", "aarch64":
		arch = aerospike.ArchitectureTypeAARCH64
	default:
		return nil, fmt.Errorf("unsupported architecture for aerospike-tools: %s", c.Arch)
	}

	products, err := aerospike.GetProducts(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("fetch aerospike products: %w", err)
	}
	prod := products.WithName("aerospike-tools")
	if len(prod) == 0 {
		return nil, fmt.Errorf("aerospike-tools product not found in artifacts feed")
	}

	versions, err := aerospike.GetVersions(30*time.Second, prod[0])
	if err != nil {
		return nil, fmt.Errorf("fetch aerospike-tools versions: %w", err)
	}
	if c.ToolsVersion != "" && c.ToolsVersion != "latest" {
		versions = versions.WithNamePrefix(c.ToolsVersion)
	}
	chosen := versions.Latest()
	if chosen == nil {
		return nil, fmt.Errorf("no aerospike-tools version matching %q", c.ToolsVersion)
	}

	files, err := aerospike.GetFiles(30*time.Second, *chosen)
	if err != nil {
		return nil, fmt.Errorf("fetch aerospike-tools files for %s: %w", chosen.Name, err)
	}
	// download=true, install=true, upgrade=true: the tools script
	// fetches the tarball, unpacks it, runs the bundled installer,
	// and replaces any existing installation. There is no existing
	// install on a fresh template so upgrade=true is a no-op for
	// the happy path.
	installScript, err := files.GetInstallScript(arch, aerospike.OSName(osName), osVersion, false, true, true, true)
	if err != nil {
		return nil, fmt.Errorf("build aerospike-tools install script (arch=%s, os=%s/%s, version=%s): %w",
			arch, osName, osVersion, chosen.Name, err)
	}
	// Prepend a small banner so the failure log identifies which
	// version was attempted; this is invaluable when the artifacts
	// API quietly publishes a new tools build that breaks one
	// distro family.
	header := fmt.Sprintf("echo 'Aerospike tools version: %s (arch=%s, os=%s/%s)'\n", chosen.Name, arch, osName, osVersion)
	return append([]byte(header), installScript...), nil
}

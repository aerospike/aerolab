package cmd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"
	"gopkg.in/yaml.v3"
)

// ClientTemplateAMSCreateCmd creates a new AMS template image with Prometheus,
// Grafana (plugins and generic datasources) and Loki pre-installed. The
// template is tagged with a monotonically-increasing generation number so
// multiple generations can coexist; `client create ams` picks the highest
// generation automatically.
//
// Usage:
//
//	aerolab client template ams create
//	aerolab client template ams create -a arm64
type ClientTemplateAMSCreateCmd struct {
	GrafanaVersion    string        `short:"g" long:"grafana-version" description:"Grafana version to install" default:"12.4.3"`
	PrometheusVersion string        `short:"P" long:"prometheus-version" description:"Prometheus version to install" default:"latest"`
	Distro            string        `short:"d" long:"distro" description:"Linux distribution to use" default:"ubuntu"`
	DistroVersion     string        `short:"i" long:"distro-version" description:"Distribution version to use" default:"24.04"`
	Arch              string        `short:"a" long:"arch" description:"Architecture (amd64 or arm64); auto-detected when empty (docker: backend/runtime arch, cloud: amd64)"`
	Timeout           int           `short:"t" long:"timeout" description:"Timeout in minutes for template creation" default:"30"`
	NoVacuum          bool          `short:"n" long:"no-vacuum" description:"Don't cleanup temporary instance on failure"`
	DryRun            bool          `long:"dry-run" description:"Validate parameters only, don't create template"`
	Owner             string        `short:"o" long:"owner" description:"Owner tag value for the template"`
	DisablePublicIP   bool          `short:"p" long:"disable-public-ip" description:"AWS: Disable public IP assignment"`
	MaxRetries        int           `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep        time.Duration `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`
	Help              HelpCmd       `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute runs `client template ams create`.
func (c *ClientTemplateAMSCreateCmd) Execute(args []string) error {
	cmd := []string{"client", "template", "ams", "create"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	_, err = c.CreateTemplate(system, system.Backend.GetInventory(), system.Logger, args, false)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ClientTemplateAMSRefreshCmd builds a new AMS template generation and, on
// success, destroys all previous AMS template generations. If no existing
// template exists, this is equivalent to `client template ams create`.
//
// Usage:
//
//	aerolab client template ams refresh
//	aerolab client template ams refresh -a arm64
type ClientTemplateAMSRefreshCmd struct {
	ClientTemplateAMSCreateCmd
}

// Execute runs `client template ams refresh`.
func (c *ClientTemplateAMSRefreshCmd) Execute(args []string) error {
	cmd := []string{"client", "template", "ams", "refresh"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	_, err = c.CreateTemplate(system, system.Backend.GetInventory(), system.Logger, args, true)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// CreateTemplate builds a new AMS template image. When allowRefresh is false
// (`client template ams create` path, and the auto-create path from
// `client create ams`) it errors out if any AMS template already exists for
// the target architecture. When allowRefresh is true
// (`client template ams refresh` path) it always creates a new generation and
// destroys any older ones on success.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//   - allowRefresh: true for refresh semantics, false for strict create
//
// Returns:
//   - string: The name of the created (or newly-discovered) template image
//   - error: nil on success, or an error describing what failed
func (c *ClientTemplateAMSCreateCmd) CreateTemplate(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, allowRefresh bool) (string, error) {
	cmdPath := []string{"client", "template", "ams", "create"}
	if allowRefresh {
		cmdPath = []string{"client", "template", "ams", "refresh"}
	}
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, cmdPath, c, args...)
		if err != nil {
			return "", err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Auto-detect the target architecture when the user didn't pass --arch.
	// Without this, go-flags would have left c.Arch as the empty string and
	// we'd fall over in FromString; previously the struct tag had a hardcoded
	// `default:"amd64"`, which was wrong for arm64 hosts running Docker
	// (and inconsistent with `client create ams`, which resolves the host
	// arch dynamically via resolveAMSArch). Mirror the auto-detection used
	// by `template create` / AGI template create / resolveAMSArch.
	if c.Arch == "" {
		switch system.Opts.Config.Backend.Type {
		case "docker":
			ar := system.Opts.Config.Backend.Arch
			if ar == "" {
				ar = runtime.GOARCH
			}
			c.Arch = ar
		case "aws", "gcp":
			c.Arch = "amd64"
		default:
			c.Arch = runtime.GOARCH
		}
		logger.Info("Auto-detected architecture: %s", c.Arch)
	}

	var backendArch backends.Architecture
	if err := backendArch.FromString(c.Arch); err != nil {
		return "", fmt.Errorf("invalid architecture: %w", err)
	}

	// Determine the target generation. Generation is a monotonically-increasing
	// integer tag (aerolab.ams.generation). A new template always takes
	// max(existing)+1. If no templates exist, generation starts at 1.
	existingImages := inventory.Images.WithTags(map[string]string{"aerolab.image.type": "ams"}).WithArchitecture(backendArch)
	maxExistingGen := 0
	for _, img := range existingImages.Describe() {
		gen, err := strconv.Atoi(img.Tags["aerolab.ams.generation"])
		if err != nil {
			continue
		}
		if gen > maxExistingGen {
			maxExistingGen = gen
		}
	}
	targetGen := maxExistingGen + 1

	// Strict create: error if any existing template exists for this arch.
	if !allowRefresh && existingImages.Count() > 0 {
		names := []string{}
		for _, img := range existingImages.Describe() {
			names = append(names, fmt.Sprintf("%s (generation=%s)", img.Name, img.Tags["aerolab.ams.generation"]))
		}
		sort.Strings(names)
		return "", fmt.Errorf("AMS template already exists for architecture %s: %s. Use `aerolab client template ams refresh` to create a new generation, or `aerolab client template ams destroy` to remove the existing one(s) first", c.Arch, strings.Join(names, ", "))
	}

	templateVersionTag := fmt.Sprintf("ams-%s-gen%d", c.Arch, targetGen)

	checkTemplateExists := func() (string, bool) {
		images := inventory.Images.WithTags(map[string]string{
			"aerolab.image.type":      "ams",
			"aerolab.ams.generation":  strconv.Itoa(targetGen),
		}).WithArchitecture(backendArch)
		if images.Count() > 0 {
			return images.Describe()[0].Name, true
		}
		return "", false
	}

	// Initialize race handler for template creation.
	raceHandler, err := newTemplateCreationRaceHandler(system.Opts.Config.Backend.Type, templateVersionTag, logger)
	if err != nil {
		return "", err
	}

	// Detect existing dangling template-creation instances for this tag.
	instances := inventory.Instances.WithTags(map[string]string{
		"aerolab.type":         "images.create",
		"aerolab.tmpl.version": templateVersionTag,
	}).WithNotState(backends.LifeCycleStateTerminated)

	raceResult, err := raceHandler.CheckForRaceCondition(instances.Describe(), func() (string, bool) {
		system.Backend.ForceRefreshInventory()
		inventory = system.Backend.GetInventory()
		return checkTemplateExists()
	})
	if err != nil {
		return "", fmt.Errorf("failed to check for race condition: %w", err)
	}

	if raceResult.TemplateExists {
		logger.Info("Template created by another process, using: %s", raceResult.TemplateName)
		return raceResult.TemplateName, nil
	}

	needToVacuum := raceResult.ShouldVacuum

	if c.Owner == "" {
		c.Owner = currentOwnerUser
	}

	// Build the installation script that will be baked into the template.
	amsCreate := &ClientCreateAMSCmd{
		ClientCreateNoneCmd: ClientCreateNoneCmd{
			MaxRetries:         c.MaxRetries,
			RetrySleep:         c.RetrySleep,
			ParallelSSHThreads: 10,
		},
		GrafanaVersion:    c.GrafanaVersion,
		PrometheusVersion: c.PrometheusVersion,
	}
	installScript, err := amsCreate.buildAMSTemplateInstallScript(c.Arch)
	if err != nil {
		return "", fmt.Errorf("failed to build AMS template install script: %w", err)
	}

	// Build structs for instance creation and image creation.
	instName := strings.ToLower("ams-tmpl-" + shortuuid.New())
	if system.Opts.Config.Backend.Type == "gcp" {
		instName = sanitizeGCPName(instName)
	}

	awsInstanceType := "t3.medium"
	gcpInstanceType := "e2-standard-2"
	if c.Arch == "arm64" {
		awsInstanceType = "t4g.medium"
		gcpInstanceType = "t2a-standard-2"
	}

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
		Description:        "temporary AMS template creation instance",
		TerminateOnStop:    false,
		ParallelSSHThreads: 1,
		SSHKeyName:         "",
		OS:                 c.Distro,
		Version:            c.DistroVersion,
		Arch:               c.Arch,
		AWS: InstancesCreateCmdAws{
			ImageID:            "",
			Expire:             TypeExpiry(time.Duration(c.Timeout) * time.Minute),
			NetworkPlacement:   system.Opts.Config.Backend.Region,
			InstanceType:       guiInstanceType(awsInstanceType),
			Disks:              []string{"type=gp3,size=30"},
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

	_, _, _, aerolabVersion := GetAerolabVersion()
	imageVersionString := fmt.Sprintf("ams-%s-%s-%s-gen%d", c.PrometheusVersion, c.GrafanaVersion, aerolabVersion, targetGen)

	imageTags := []string{
		fmt.Sprintf("aerolab.ams.generation=%d", targetGen),
		fmt.Sprintf("aerolab.ams.prometheus=%s", c.PrometheusVersion),
		fmt.Sprintf("aerolab.ams.grafana=%s", c.GrafanaVersion),
		fmt.Sprintf("aerolab.ams.aerolab=%s", aerolabVersion),
		"aerolab.is.official=true",
	}

	imagesCreate := &ImagesCreateCmd{
		Name:         instName,
		Description:  fmt.Sprintf("AMS Template gen%d - Prometheus %s, Grafana %s, %s %s", targetGen, c.PrometheusVersion, c.GrafanaVersion, c.Distro, c.DistroVersion),
		InstanceName: instName,
		SizeGiB:      30,
		Owner:        c.Owner,
		Type:         "ams",
		Version:      imageVersionString,
		Tags:         imageTags,
		Timeout:      c.Timeout,
		DryRun:       false,
		IsOfficial:   true,
	}

	logger.Info("AMS Template Configuration:")
	logger.Info("  Generation: %d (previous max: %d)", targetGen, maxExistingGen)
	logger.Info("  Prometheus Version: %s", c.PrometheusVersion)
	logger.Info("  Grafana Version: %s", c.GrafanaVersion)
	logger.Info("  Distro: %s %s", c.Distro, c.DistroVersion)
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
		if allowRefresh && maxExistingGen > 0 {
			logger.Info("6. Refresh: destroy previous AMS template(s) with generation<%d", targetGen)
		}
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

	logger.Info("Creating temporary instance for AMS template")
	inst, err := instancesCreate.CreateInstances(system, inventory, nil, "create")
	if err != nil {
		return "", fmt.Errorf("could not create temporary instance: %w", err)
	}

	stopHeartbeat := raceHandler.StartHeartbeat(inst)
	defer stopHeartbeat()

	// Early cleanup on interrupt.
	shutdown.AddEarlyCleanupJob("ams-template-create-"+instName, func(isSignal bool) {
		if !isSignal {
			return
		}
		if !c.NoVacuum {
			c.NoVacuum = true
			logger.Info("Abort: destroying temporary AMS template creation instance")
			if err := inst.Terminate(time.Minute * 10); err != nil {
				logger.Error("could not destroy temporary instance: %s", err)
			}
		}
	})

	// Defer cleanup on failure.
	defer func() {
		if !c.NoVacuum {
			logger.Info("Destroying temporary template creation instance on failure")
			if err := inst.Terminate(time.Minute * 10); err != nil {
				logger.Error("could not destroy temporary instance: %s", err)
			}
		}
	}()

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
			DestPath:    "/opt/aerolab/scripts/ams-template-install.sh",
			Source:      bytes.NewReader(installScript),
			Permissions: 0755,
		})
		cli.Close()
		if err != nil {
			return "", fmt.Errorf("could not upload install script: %w", err)
		}

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
			Command:        []string{"bash", "/opt/aerolab/scripts/ams-template-install.sh"},
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

		scriptPath := "/opt/aerolab/scripts/ams-template-install.sh"
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

	// Bake the default Grafana dashboards into the template image. The
	// GitHub API walker is expensive, so we run it here once per template
	// rather than on every `client create ams` invocation.
	logger.Info("Baking default Grafana dashboards into template")
	tempInstances := inst.Describe()
	if len(tempInstances) == 0 {
		return "", fmt.Errorf("temporary template instance disappeared before dashboard bake step")
	}
	if err := amsCreate.installAMSDefaultDashboards(tempInstances[0], logger); err != nil {
		return "", fmt.Errorf("failed to bake default dashboards into template: %w", err)
	}

	logger.Info("Stopping instance")
	err = inst.Stop(false, time.Minute*10)
	if err != nil {
		return "", fmt.Errorf("could not stop instance: %w", err)
	}

	logger.Info("Creating AMS template image")
	if len(inst.Describe()) > 0 {
		imagesCreate.InstanceName = inst.Describe()[0].Name
	}
	inst.Describe()[0].AttachedVolumes = backends.VolumeList{}
	newInst := append(inventory.Instances.Describe(), inst.Describe()...)
	inventory.Instances = newInst

	image, err := imagesCreate.CreateImage(system, inventory, logger.WithPrefix("[images.create] "), nil)
	if err != nil {
		return "", fmt.Errorf("could not create image: %w", err)
	}

	logger.Info("Destroying temporary instance")
	err = inst.Terminate(time.Minute * 10)
	if err != nil {
		return "", fmt.Errorf("could not destroy temporary instance: %w", err)
	}
	c.NoVacuum = true

	raceHandler.OnSuccess()

	// Refresh: delete all previous AMS templates for this architecture.
	// (We deliberately keep the highest generation instead of the oldest:
	// see CleanupDuplicateTemplates, which AGI uses with opposite semantics.)
	if allowRefresh {
		system.Backend.ForceRefreshInventory()
		inventory = system.Backend.GetInventory()
		allImages := inventory.Images.WithTags(map[string]string{"aerolab.image.type": "ams"}).WithArchitecture(backendArch)
		var previous backends.ImageList
		for _, img := range allImages.Describe() {
			gen, err := strconv.Atoi(img.Tags["aerolab.ams.generation"])
			if err != nil {
				// Missing or invalid generation: treat as previous (superseded)
				if img.ImageId != image.ImageId {
					previous = append(previous, img)
				}
				continue
			}
			if gen < targetGen {
				previous = append(previous, img)
			}
		}
		if len(previous) > 0 {
			logger.Info("Refresh: destroying %d previous AMS template(s)", len(previous))
			if err := previous.DeleteImages(time.Minute * 10); err != nil {
				logger.Warn("Failed to destroy some previous AMS template(s): %s", err)
			}
		}
	}

	logger.Info("AMS template created successfully: %s (generation %d)", image.Name, targetGen)
	return image.Name, nil
}

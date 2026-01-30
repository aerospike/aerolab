package cmd

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/agi"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

// AgiStartCmd starts an AGI instance that has been stopped.
// It starts the underlying instance, waits for it to be ready,
// and verifies that AGI services are running.
//
// If no instance exists but an EFS/GCP volume with the AGI name exists,
// the command will automatically create a new instance and reattach it
// to the existing volume, preserving all AGI data.
//
// Usage:
//
//	aerolab agi start -n myagi
type AgiStartCmd struct {
	Name    TypeAgiClusterName `short:"n" long:"name" description:"AGI instance name" default:"agi"`
	NoWait  bool               `short:"w" long:"no-wait" description:"Do not wait for the instance to start"`
	DryRun  bool               `short:"d" long:"dry-run" description:"Print what would be done but don't do it"`
	Threads int                `short:"t" long:"threads" description:"Threads to use for service start" default:"1"`

	Reattach Reattach `group:"Reattach" namespace:"reattach" description:"reattach options"`

	// AWS reattach options
	AWS AgiStartCmdAws `group:"AWS" namespace:"aws" description:"backend-aws"`

	// GCP reattach options
	GCP AgiStartCmdGcp `group:"GCP" namespace:"gcp" description:"backend-gcp"`

	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type Reattach struct {
	// Override options for reattach (used by monitor for sizing/capacity)
	InstanceTypeOverride string `long:"instance-type" description:"Override instance type when reattaching"`
	NoDIMOverride        *bool  `long:"nodim" description:"Override data-in-memory setting when reattaching" no-default:"true"`
	SpotOverride         *bool  `long:"spot" description:"Override spot instance setting when reattaching" no-default:"true"`
	OwnerOverride        string `long:"owner" description:"Override owner tag when reattaching"`
}

// AgiStartCmdAws contains AWS-specific options for AGI start/reattach.
// Most settings are read from the EFS volume tags automatically.
type AgiStartCmdAws struct {
	EFSName string `long:"efs-name" description:"EFS volume name pattern (default uses AGI name)" default:"{AGI_NAME}"`
}

// AgiStartCmdGcp contains GCP-specific options for AGI start/reattach.
// Most settings are read from the volume tags automatically.
type AgiStartCmdGcp struct {
	VolName string `long:"vol-name" description:"Volume name pattern (default uses AGI name)" default:"{AGI_NAME}"`
}

// Execute implements the command execution for agi start.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiStartCmd) Execute(args []string) error {
	cmd := []string{"agi", "start"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	instance, err := c.StartAGI(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	if instance != nil && instance.Count() > 0 {
		instances := instance.Describe()
		protocol := "https"
		ip := instances[0].IP.Public
		if ip == "" {
			ip = instances[0].IP.Private
		}
		if instances[0].Tags["aerolab4ssl"] != "true" {
			protocol = "http"
		}
		system.Logger.Info("AGI instance started")
		system.Logger.Info("Access URL: %s://%s", protocol, ip)
		system.Logger.Info("")
		system.Logger.Info("Useful commands:")
		system.Logger.Info("  aerolab agi list                       - List AGI instances")
		system.Logger.Info("  aerolab agi add-auth-token -n %s --url - Add auth token and display access URL", c.Name.String())
		system.Logger.Info("  aerolab agi open -n %s                 - Open AGI in browser", c.Name.String())
		system.Logger.Info("  aerolab agi attach -n %s               - Attach to shell", c.Name.String())
		system.Logger.Info("  aerolab agi status -n %s               - Show status", c.Name.String())
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// StartAGI starts an AGI instance.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - backends.InstanceList: The started AGI instance
//   - error: nil on success, or an error describing what failed
func (c *AgiStartCmd) StartAGI(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "start"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Find AGI instance
	instances := inventory.Instances.WithTags(map[string]string{
		"aerolab.type": "agi",
	}).WithClusterName(c.Name.String()).WithState(backends.LifeCycleStateStopped).Describe()

	if instances.Count() == 0 {
		// Check if it's already running
		running := inventory.Instances.WithTags(map[string]string{
			"aerolab.type": "agi",
		}).WithClusterName(c.Name.String()).WithState(backends.LifeCycleStateRunning).Describe()
		if running.Count() > 0 {
			return running, fmt.Errorf("AGI instance %s is already running", c.Name)
		}

		// Auto-detect EFS/volume reattach scenario
		// If instance doesn't exist but volume does, automatically reattach
		backendType := system.Opts.Config.Backend.Type
		if backendType == "aws" {
			// Apply default if EFSName is empty (happens when struct is created programmatically, not via CLI)
			efsName := c.AWS.EFSName
			if efsName == "" {
				efsName = "{AGI_NAME}"
			}
			volumeName := strings.ReplaceAll(efsName, "{AGI_NAME}", c.Name.String())
			if inventory.Volumes.WithType(backends.VolumeTypeSharedDisk).WithName(volumeName).Count() > 0 {
				logger.Info("AGI instance %s not found, but EFS volume %s exists - reattaching", c.Name, volumeName)
				return c.reattachFromEFS(system, inventory, logger, args)
			}
		}
		if backendType == "gcp" {
			// Apply default if VolName is empty (happens when struct is created programmatically, not via CLI)
			volName := c.GCP.VolName
			if volName == "" {
				volName = "{AGI_NAME}"
			}
			volumeName := strings.ReplaceAll(volName, "{AGI_NAME}", c.Name.String())
			if inventory.Volumes.WithType(backends.VolumeTypeAttachedDisk).WithName(volumeName).Count() > 0 {
				logger.Info("AGI instance %s not found, but GCP volume %s exists - reattaching", c.Name, volumeName)
				return c.reattachFromGCPVolume(system, inventory, logger, args)
			}
		}

		return nil, fmt.Errorf("AGI instance %s not found (no stopped instance or persistent volume)", c.Name)
	}

	if c.DryRun {
		logger.Info("DRY-RUN: Would start AGI instance %s", c.Name)
		return instances, nil
	}

	// Start the instance
	logger.Info("Starting AGI instance %s", c.Name)
	waitDur := 10 * time.Minute
	if c.NoWait {
		waitDur = 0
	}

	err := instances.Start(waitDur)
	if err != nil {
		return nil, fmt.Errorf("failed to start instance: %w", err)
	}

	// If we didn't wait, return early
	if c.NoWait {
		return instances, nil
	}

	// Refresh inventory to get updated IPs
	inventory = system.Backend.GetInventory()
	instances = inventory.Instances.WithTags(map[string]string{
		"aerolab.type": "agi",
	}).WithClusterName(c.Name.String()).WithState(backends.LifeCycleStateRunning).Describe()

	// Start AGI services
	logger.Info("Starting AGI services")
	script := `ERRORS=""
for service in aerospike grafana-server agi-plugin agi-grafanafix agi-proxy agi-ingest; do
    if ! systemctl start "$service"; then
        ERRORS="$ERRORS $service"
    fi
done
if [ -n "$ERRORS" ]; then
    echo "Failed to start:$ERRORS" >&2
    exit 1
fi
`

	outputs := instances.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", script},
			SessionTimeout: 5 * time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: c.Threads,
	})

	var errs []string
	for _, o := range outputs {
		if o.Output.Err != nil {
			errs = append(errs, fmt.Sprintf("%v (stderr: %s)", o.Output.Err, o.Output.Stderr))
		}
	}

	if len(errs) > 0 {
		logger.Warn("Some services failed to start: %s", strings.Join(errs, "; "))
	}

	return instances, nil
}

// reattachFromEFS creates a new AGI instance and reattaches it to an existing EFS volume.
// This enables recovery of AGI data when the instance was terminated but the EFS volume persisted.
// All settings (instance type, FIPS, subnet, security groups, etc.) are read from the EFS volume tags.
func (c *AgiStartCmd) reattachFromEFS(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (backends.InstanceList, error) {
	// Resolve volume name (apply default if empty)
	efsName := c.AWS.EFSName
	if efsName == "" {
		efsName = "{AGI_NAME}"
	}
	volumeName := strings.ReplaceAll(efsName, "{AGI_NAME}", c.Name.String())

	// Check if EFS volume exists
	volumes := inventory.Volumes.WithType(backends.VolumeTypeSharedDisk).WithName(volumeName)
	if volumes.Count() == 0 {
		return nil, fmt.Errorf("AGI instance %s not found and no EFS volume '%s' exists.\n"+
			"  - Create a new AGI with: aerolab agi create -n %s --aws-with-efs ...",
			c.Name, volumeName, c.Name)
	}

	vol := volumes.Describe()[0]
	logger.Info("Found existing EFS volume %s, creating new instance to reattach", volumeName)

	if c.DryRun {
		logger.Info("DRY-RUN: Would create new instance and reattach EFS volume %s", volumeName)
		return nil, nil
	}

	// Read all settings from EFS volume tags
	instanceType := vol.Tags["agiinstance"]
	noDIM := vol.Tags["aginodim"] == "true"
	terminateOnPoweroff := vol.Tags["termonpow"] == "true"
	spotInstance := vol.Tags["isspot"] == "true"
	aerospikeVersion := vol.Tags["aerolab7agiav"]
	efsFips := vol.Tags["agifips"] == "true"
	subnetID := vol.Tags["agisubnet"]
	securityGroupID := vol.Tags["agisecgroup"]
	ebs := vol.Tags["agiebs"]
	disablePublicIP := vol.Tags["agidisablepubip"] == "true"
	spotFallback := vol.Tags["agispotfallback"] == "true"
	efsPath := vol.Tags["agiefspath"]
	route53ZoneID := vol.Tags["agiZoneID"]
	route53Domain := vol.Tags["agiDomain"]
	agiLabel := vol.Tags["agilabel"]
	sslDisable := vol.Tags["agissldisable"] == "true"
	monitorURL := vol.Tags["agimonitorurl"]
	monitorCertIgnore := vol.Tags["agimonitorcertignore"] == "true"
	expireStr := vol.Tags["agiexpire"]
	efsExpireStr := vol.Tags["agiefsexpire"]
	templateName := vol.Tags["agitemplate"]
	archStr := vol.Tags["agiarch"]
	owner := vol.Tags["owner"]

	// Apply overrides (used by monitor for sizing/capacity rotation)
	if c.Reattach.InstanceTypeOverride != "" {
		logger.Info("Override: Instance type %s -> %s", instanceType, c.Reattach.InstanceTypeOverride)
		instanceType = c.Reattach.InstanceTypeOverride
	}
	if c.Reattach.NoDIMOverride != nil {
		logger.Info("Override: NoDIM %t -> %t", noDIM, *c.Reattach.NoDIMOverride)
		noDIM = *c.Reattach.NoDIMOverride
	}
	if c.Reattach.SpotOverride != nil {
		logger.Info("Override: Spot %t -> %t", spotInstance, *c.Reattach.SpotOverride)
		spotInstance = *c.Reattach.SpotOverride
	}
	if c.Reattach.OwnerOverride != "" {
		owner = c.Reattach.OwnerOverride
	}

	// Validate we have essential settings
	if instanceType == "" {
		return nil, fmt.Errorf("EFS volume %s is missing 'agiinstance' tag - cannot determine instance type for reattach", volumeName)
	}
	if aerospikeVersion == "" {
		aerospikeVersion = "latest"
	}

	// Use EFS availability zone if subnet not stored in tags
	if subnetID == "" && vol.ZoneName != "" {
		subnetID = vol.ZoneName
	}

	// Parse instance expiry duration
	var expireDuration time.Duration
	if expireStr != "" {
		if d, err := time.ParseDuration(expireStr); err == nil {
			expireDuration = d
		}
	}
	if expireDuration == 0 {
		expireDuration = 30 * time.Hour // Default
	}

	// Parse EFS volume expiry duration
	// Note: 0 is a valid value meaning "never expire", so only use default if tag is missing
	var efsExpireDuration time.Duration
	if efsExpireStr != "" {
		if d, err := time.ParseDuration(efsExpireStr); err == nil {
			efsExpireDuration = d
		} else {
			efsExpireDuration = 96 * time.Hour // Parse error, use default
		}
	} else {
		efsExpireDuration = 96 * time.Hour // Tag missing, use default
	}

	// Default EBS size
	if ebs == "" {
		ebs = "40"
	}

	// Default EFS path
	if efsPath == "" {
		efsPath = "/"
	}

	// Default label to name
	if agiLabel == "" {
		agiLabel = c.Name.String()
	}

	// Default arch to amd64
	if archStr == "" {
		archStr = "amd64"
	}

	logger.Info("Reattach settings from EFS tags:")
	logger.Info("  Instance Type: %s", instanceType)
	logger.Info("  Aerospike Version: %s", aerospikeVersion)
	logger.Info("  Architecture: %s", archStr)
	if templateName != "" {
		logger.Info("  Preferred Template: %s", templateName)
	}
	logger.Info("  NoDIM: %t", noDIM)
	logger.Info("  Terminate on Poweroff: %t", terminateOnPoweroff)
	logger.Info("  Spot Instance: %t", spotInstance)
	logger.Info("  FIPS: %t", efsFips)
	logger.Info("  SSL Disabled: %t", sslDisable)
	if subnetID != "" {
		logger.Info("  Subnet/AZ: %s", subnetID)
	}
	if securityGroupID != "" {
		logger.Info("  Security Groups: %s", securityGroupID)
	}
	if route53Domain != "" {
		logger.Info("  Route53 Domain: %s", route53Domain)
	}
	if monitorURL != "" {
		logger.Info("  Monitor URL: %s", monitorURL)
	}

	// Build AgiCreateCmd with settings from volume tags
	createCmd := &AgiCreateCmd{
		ClusterName:       TypeAgiClusterName(c.Name.String()),
		AGILabel:          agiLabel,
		AerospikeVersion:  aerospikeVersion,
		NoDIM:             noDIM,
		NoConfigOverride:  true, // Skip source validation and config upload - configs exist on EFS
		Force:             true, // Allow creating even though EFS exists
		ProxyDisableSSL:   sslDisable,
		MonitorUrl:        monitorURL,
		MonitorCertIgnore: monitorCertIgnore,
		PreferredTemplate: templateName,
		Distro:            "ubuntu", // Default, used if template needs to be created
		DistroVersion:     "latest", // Default, used if template needs to be created
		Owner:             owner,
		AWS: AgiCreateCmdAws{
			InstanceType:        instanceType,
			Ebs:                 ebs,
			WithEFS:             true,
			EFSName:             efsName, // Use local var with default applied
			EFSPath:             efsPath,
			EFSFips:             efsFips,
			TerminateOnPoweroff: terminateOnPoweroff,
			SpotInstance:        spotInstance,
			SpotFallback:        spotFallback,
			SubnetID:            subnetID,
			SecurityGroupID:     securityGroupID,
			DisablePublicIP:     disablePublicIP,
			Route53ZoneId:       route53ZoneID,
			Route53DomainName:   route53Domain,
			Expires:             expireDuration,
			EFSExpires:          efsExpireDuration,
		},
	}

	// Create the AGI instance (this will reuse the existing EFS volume)
	logger.Info("Creating new AGI instance to attach EFS volume")
	instances, err := createCmd.CreateAGI(system, inventory, logger, args)
	if err != nil {
		return nil, fmt.Errorf("failed to create AGI instance for EFS reattach: %w", err)
	}

	// Regenerate SSL certificates if missing on EFS
	if instances.Count() > 0 {
		c.regenerateSSLIfMissing(instances, logger)
	}

	logger.Info("AGI instance reattached to EFS volume %s successfully", volumeName)
	return instances, nil
}

// reattachFromGCPVolume creates a new AGI instance and reattaches it to an existing GCP volume.
// This enables recovery of AGI data when the instance was terminated but the volume persisted.
// All settings (instance type, FIPS, zone, etc.) are read from the volume tags.
func (c *AgiStartCmd) reattachFromGCPVolume(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (backends.InstanceList, error) {
	// Resolve volume name (apply default if empty)
	volName := c.GCP.VolName
	if volName == "" {
		volName = "{AGI_NAME}"
	}
	volumeName := strings.ReplaceAll(volName, "{AGI_NAME}", c.Name.String())

	// Check if GCP volume exists
	volumes := inventory.Volumes.WithType(backends.VolumeTypeAttachedDisk).WithName(volumeName)
	if volumes.Count() == 0 {
		return nil, fmt.Errorf("AGI instance %s not found and no GCP volume '%s' exists.\n"+
			"  - Create a new AGI with: aerolab agi create -n %s --gcp-with-vol ...",
			c.Name, volumeName, c.Name)
	}

	vol := volumes.Describe()[0]
	logger.Info("Found existing GCP volume %s, creating new instance to reattach", volumeName)

	if c.DryRun {
		logger.Info("DRY-RUN: Would create new instance and reattach GCP volume %s", volumeName)
		return nil, nil
	}

	// Read all settings from GCP volume tags
	instanceType := vol.Tags["agiinstance"]
	noDIM := vol.Tags["aginodim"] == "true"
	terminateOnPoweroff := vol.Tags["termonpow"] == "true"
	spotInstance := vol.Tags["isspot"] == "true"
	aerospikeVersion := vol.Tags["aerolab7agiav"]
	volFips := vol.Tags["agifips"] == "true"
	zone := vol.Tags["agizone"]
	agiLabel := vol.Tags["agilabel"]
	sslDisable := vol.Tags["agissldisable"] == "true"
	monitorURL := vol.Tags["agimonitorurl"]
	monitorCertIgnore := vol.Tags["agimonitorcertignore"] == "true"
	expireStr := vol.Tags["agiexpire"]
	volExpireStr := vol.Tags["agivolexpire"]
	disksStr := vol.Tags["agidisks"]
	templateName := vol.Tags["agitemplate"]
	archStr := vol.Tags["agiarch"]
	owner := vol.Tags["owner"]

	// Apply overrides (used by monitor for sizing/capacity rotation)
	if c.Reattach.InstanceTypeOverride != "" {
		logger.Info("Override: Instance type %s -> %s", instanceType, c.Reattach.InstanceTypeOverride)
		instanceType = c.Reattach.InstanceTypeOverride
	}
	if c.Reattach.NoDIMOverride != nil {
		logger.Info("Override: NoDIM %t -> %t", noDIM, *c.Reattach.NoDIMOverride)
		noDIM = *c.Reattach.NoDIMOverride
	}
	if c.Reattach.SpotOverride != nil {
		logger.Info("Override: Spot %t -> %t", spotInstance, *c.Reattach.SpotOverride)
		spotInstance = *c.Reattach.SpotOverride
	}
	if c.Reattach.OwnerOverride != "" {
		owner = c.Reattach.OwnerOverride
	}

	// Validate we have essential settings
	if instanceType == "" {
		return nil, fmt.Errorf("GCP volume %s is missing 'agiinstance' tag - cannot determine instance type for reattach", volumeName)
	}
	if aerospikeVersion == "" {
		aerospikeVersion = "latest"
	}

	// Use volume's zone if not stored in tags
	if zone == "" && vol.ZoneName != "" {
		zone = vol.ZoneName
	}

	// Parse instance expiry duration
	var expireDuration time.Duration
	if expireStr != "" {
		if d, err := time.ParseDuration(expireStr); err == nil {
			expireDuration = d
		}
	}
	if expireDuration == 0 {
		expireDuration = 30 * time.Hour // Default
	}

	// Parse GCP volume expiry duration
	// Note: 0 is a valid value meaning "never expire", so only use default if tag is missing
	var volExpireDuration time.Duration
	if volExpireStr != "" {
		if d, err := time.ParseDuration(volExpireStr); err == nil {
			volExpireDuration = d
		} else {
			volExpireDuration = 96 * time.Hour // Parse error, use default
		}
	} else {
		volExpireDuration = 96 * time.Hour // Tag missing, use default
	}

	// Parse disks configuration
	var disks []string
	if disksStr != "" {
		disks = strings.Split(disksStr, ";")
	}
	if len(disks) == 0 {
		disks = []string{"type=pd-ssd,size=40"} // Default
	}

	// Default label to name
	if agiLabel == "" {
		agiLabel = c.Name.String()
	}

	// Default arch to amd64
	if archStr == "" {
		archStr = "amd64"
	}

	logger.Info("Reattach settings from GCP volume tags:")
	logger.Info("  Instance Type: %s", instanceType)
	logger.Info("  Aerospike Version: %s", aerospikeVersion)
	logger.Info("  Architecture: %s", archStr)
	if templateName != "" {
		logger.Info("  Preferred Template: %s", templateName)
	}
	logger.Info("  NoDIM: %t", noDIM)
	logger.Info("  Terminate on Poweroff: %t", terminateOnPoweroff)
	logger.Info("  Spot Instance: %t", spotInstance)
	logger.Info("  FIPS: %t", volFips)
	logger.Info("  SSL Disabled: %t", sslDisable)
	if zone != "" {
		logger.Info("  Zone: %s", zone)
	}
	if monitorURL != "" {
		logger.Info("  Monitor URL: %s", monitorURL)
	}

	// Build AgiCreateCmd with settings from volume tags
	createCmd := &AgiCreateCmd{
		ClusterName:       TypeAgiClusterName(c.Name.String()),
		AGILabel:          agiLabel,
		AerospikeVersion:  aerospikeVersion,
		NoDIM:             noDIM,
		NoConfigOverride:  true, // Skip source validation and config upload - configs exist on volume
		Force:             true, // Allow creating even though volume exists
		ProxyDisableSSL:   sslDisable,
		MonitorUrl:        monitorURL,
		MonitorCertIgnore: monitorCertIgnore,
		PreferredTemplate: templateName,
		Distro:            "ubuntu", // Default, used if template needs to be created
		DistroVersion:     "latest", // Default, used if template needs to be created
		Owner:             owner,
		GCP: AgiCreateCmdGcp{
			InstanceType:        instanceType,
			Disks:               disks,
			WithVol:             true,
			VolName:             volName, // Use local var with default applied
			VolFips:             volFips,
			TerminateOnPoweroff: terminateOnPoweroff,
			SpotInstance:        spotInstance,
			Zone:                zone,
			Expires:             expireDuration,
			VolExpires:          volExpireDuration,
		},
	}

	// Create the AGI instance (this will reuse the existing GCP volume)
	logger.Info("Creating new AGI instance to attach GCP volume")
	instances, err := createCmd.CreateAGI(system, inventory, logger, args)
	if err != nil {
		return nil, fmt.Errorf("failed to create AGI instance for GCP volume reattach: %w", err)
	}

	// Regenerate SSL certificates if missing on volume
	if instances.Count() > 0 {
		c.regenerateSSLIfMissing(instances, logger)
	}

	logger.Info("AGI instance reattached to GCP volume %s successfully", volumeName)
	return instances, nil
}

// regenerateSSLIfMissing checks if SSL certificates exist on the AGI instance and regenerates them if missing.
// This handles the case where the volume was created with SSL disabled or the certs were deleted.
func (c *AgiStartCmd) regenerateSSLIfMissing(instances backends.InstanceList, logger *logger.Logger) {
	if instances.Count() == 0 {
		return
	}

	// Check if SSL is expected (from instance tags)
	inst := instances.Describe()[0]
	sslEnabled := inst.Tags["aerolab4ssl"] == "true"
	if !sslEnabled {
		logger.Debug("SSL is disabled for this AGI instance, skipping certificate check")
		return
	}

	// Check if certificates exist
	outputs := instances.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "[ -f /opt/agi/proxy.cert ] && [ -f /opt/agi/proxy.key ] && echo 'EXISTS' || echo 'MISSING'"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if len(outputs) == 0 || outputs[0].Output.Err != nil {
		logger.Warn("Failed to check SSL certificate status")
		return
	}

	if strings.TrimSpace(string(outputs[0].Output.Stdout)) == "EXISTS" {
		logger.Debug("SSL certificates exist on AGI instance")
		return
	}

	// Regenerate self-signed certificates
	logger.Info("SSL certificates missing, regenerating self-signed certificates")
	outputs = instances.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command: []string{"bash", "-c", `openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
				-keyout /opt/agi/proxy.key \
				-out /opt/agi/proxy.cert \
				-subj "/C=US/ST=California/L=San Jose/O=Aerospike/OU=AeroLab/CN=agi.aerolab.local" && \
				chmod 644 /opt/agi/proxy.cert && \
				chmod 600 /opt/agi/proxy.key`},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if len(outputs) > 0 && outputs[0].Output.Err != nil {
		logger.Warn("Failed to regenerate SSL certificates: %s", outputs[0].Output.Err)
	} else {
		logger.Info("SSL certificates regenerated successfully")
	}
}

// readDeploymentJSON reads the deployment.json.gz from an AGI instance and decodes it.
// This is used to restore the original AgiCreateCmd settings during reattach.
func (c *AgiStartCmd) readDeploymentJSON(instances backends.InstanceList) (*AgiCreateCmd, error) {
	if instances.Count() == 0 {
		return nil, fmt.Errorf("no instances to read deployment JSON from")
	}

	// Read deployment.json.gz via exec
	outputs := instances.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"cat", "/opt/agi/deployment.json.gz"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if len(outputs) == 0 || outputs[0].Output.Err != nil {
		return nil, fmt.Errorf("failed to read deployment.json.gz")
	}

	// Decompress gzip
	reader, err := gzip.NewReader(bytes.NewReader(outputs[0].Output.Stdout))
	if err != nil {
		return nil, fmt.Errorf("failed to decompress deployment.json.gz: %w", err)
	}
	defer reader.Close()

	jsonData, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read decompressed data: %w", err)
	}

	// Unmarshal JSON
	createCmd := &AgiCreateCmd{}
	if err := json.Unmarshal(jsonData, createCmd); err != nil {
		return nil, fmt.Errorf("failed to parse deployment.json: %w", err)
	}

	return createCmd, nil
}

// resolveTemplateForReattach finds an existing AGI template for the given architecture.
// Returns the template name or empty string if not found.
func (c *AgiStartCmd) resolveTemplateForReattach(inventory *backends.Inventory, arch backends.Architecture) string {
	images := inventory.Images.WithTags(map[string]string{
		"aerolab.image.type":  "agi",
		"aerolab.agi.version": strconv.Itoa(agi.AGIVersion),
	}).WithArchitecture(arch)

	if images.Count() > 0 {
		return images.Describe()[0].Name
	}
	return ""
}

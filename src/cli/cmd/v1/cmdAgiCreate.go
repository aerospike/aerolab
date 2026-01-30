package cmd

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/agi"
	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	rtypes "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lithammer/shortuuid"
	"github.com/pkg/sftp"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

// ClusterFeatureAGI is the feature flag for AGI instances
const ClusterFeatureAGI = 1

// AgiCreateCmd creates a new AGI instance from an AGI template.
// This is the second most complex command, handling volume management,
// config generation, and multi-source log ingestion.
//
// The command supports:
//   - Local, SFTP, S3, and cluster sources for logs
//   - EFS/GCP volume persistence
//   - Docker volumes and bind mounts
//   - SSL certificate configuration
//   - Authentication via basic auth or tokens
//   - Monitor integration for auto-scaling
type AgiCreateCmd struct {
	// Instance naming
	ClusterName TypeAgiClusterName `short:"n" long:"name" description:"AGI name (use ~auto~ for auto-generated name)" default:"agi"`
	AGILabel    string             `long:"agi-label" description:"Friendly label for the AGI instance"`

	// Source options
	LocalSource   flags.Filename  `long:"source-local" description:"Get logs from a local directory; Docker: use 'bind:/path' prefix to bind-mount instead of copying"`
	BindFilesDir  flags.Filename  `long:"bind-files-dir" description:"Docker only: bind mount a host directory to /opt/agi/files (rw) for AGI output; use with --source-local bind:/path for input"`
	ClusterSource TypeClusterName `long:"source-cluster" description:"Cluster name to use as the source for logs"`

	// SFTP source options
	SftpEnable    bool           `long:"source-sftp-enable" description:"Enable SFTP source"`
	SftpThreads   int            `long:"source-sftp-threads" description:"Number of concurrent downloader threads" default:"1"`
	SftpHost      string         `long:"source-sftp-host" description:"SFTP host"`
	SftpPort      int            `long:"source-sftp-port" description:"SFTP port" default:"22"`
	SftpUser      string         `long:"source-sftp-user" description:"SFTP user"`
	SftpPass      string         `long:"source-sftp-pass" description:"SFTP password (supports ENV::VAR_NAME)" webtype:"password"`
	SftpKey       flags.Filename `long:"source-sftp-key" description:"Key file for SFTP login"`
	SftpPath      string         `long:"source-sftp-path" description:"Path on SFTP to download logs from"`
	SftpRegex     string         `long:"source-sftp-regex" description:"Regex to filter files to download"`
	SftpSkipCheck bool           `long:"source-sftp-skipcheck" description:"Skip SFTP accessibility check"`

	// S3 source options
	S3Enable    bool   `long:"source-s3-enable" description:"Enable S3 source"`
	S3Threads   int    `long:"source-s3-threads" description:"Number of concurrent downloader threads" default:"4"`
	S3Region    string `long:"source-s3-region" description:"AWS region where S3 bucket is located"`
	S3Bucket    string `long:"source-s3-bucket" description:"S3 bucket name"`
	S3KeyID     string `long:"source-s3-key-id" description:"AWS access key ID (supports ENV::VAR_NAME)"`
	S3Secret    string `long:"source-s3-secret-key" description:"AWS secret key (supports ENV::VAR_NAME)" webtype:"password"`
	S3Path      string `long:"source-s3-path" description:"Path prefix in S3 bucket"`
	S3Regex     string `long:"source-s3-regex" description:"Regex to filter files to download"`
	S3SkipCheck bool   `long:"source-s3-skipcheck" description:"Skip S3 accessibility check"`
	S3Endpoint  string `long:"source-s3-endpoint" description:"Custom S3 endpoint URL"`

	// Memory/storage options
	NoDIM         bool `long:"no-dim" description:"Disable data-in-memory, use read-page-cache instead (less RAM, slower)"`
	NoDIMFileSize int  `long:"no-dim-filesize" description:"File size in GB when using --no-dim (default: auto-calculated)"`

	// SSL options
	ProxyDisableSSL bool           `long:"proxy-ssl-disable" description:"Disable TLS on the proxy"`
	ProxyCert       flags.Filename `long:"proxy-ssl-cert" description:"SSL certificate file (default: self-signed)"`
	ProxyKey        flags.Filename `long:"proxy-ssl-key" description:"SSL private key file (default: self-signed)"`

	// Proxy timeouts
	ProxyMaxInactive time.Duration `long:"proxy-max-inactive" description:"Max inactivity before shutdown" default:"1h"`
	ProxyMaxUptime   time.Duration `long:"proxy-max-uptime" description:"Max uptime before shutdown" default:"24h"`

	// Time range filtering
	TimeRanges     bool   `long:"ingest-timeranges-enable" description:"Enable time range filtering for log ingestion"`
	TimeRangesFrom string `long:"ingest-timeranges-from" description:"Time range start (format: 2006-01-02T15:04:05Z07:00)"`
	TimeRangesTo   string `long:"ingest-timeranges-to" description:"Time range end (format: 2006-01-02T15:04:05Z07:00)"`

	// Custom ingest options
	CustomSourceName string         `long:"ingest-custom-source-name" description:"Custom source name to display in Grafana"`
	PatternsFile     flags.Filename `long:"ingest-patterns-file" description:"Custom patterns YAML file for log ingest"`
	FeaturesFilePath flags.Filename `short:"f" long:"featurefile" description:"Features file to install, overriding the template's features file"`
	IngestLogLevel   int            `long:"ingest-log-level" description:"Log level: 1=CRITICAL,2=ERROR,3=WARN,4=INFO,5=DEBUG,6=DETAIL" default:"4"`
	IngestCpuProfile bool           `long:"ingest-cpu-profiling" description:"Enable CPU profiling for ingest"`
	PluginCpuProfile bool           `long:"plugin-cpu-profiling" description:"Enable CPU profiling for plugin"`
	PluginLogLevel   int            `long:"plugin-log-level" description:"Plugin log level" default:"4"`

	// Notification options
	SlackToken   string `long:"notify-slack-token" description:"Slack token for notifications (supports ENV::VAR_NAME)"`
	SlackChannel string `long:"notify-slack-channel" description:"Slack channel for notifications"`

	// Monitor options
	MonitorUrl        string `long:"monitor-url" description:"AGI Monitor URL for sizing notifications"`
	MonitorCertIgnore bool   `long:"monitor-ignore-cert" description:"Ignore invalid monitor SSL certificate"`

	// Configuration options
	NoConfigOverride bool   `long:"no-config-override" description:"Don't override existing config when restarting with EFS"`
	Owner            string `long:"owner" description:"Owner tag value"`

	// Version options
	AerospikeVersion string `short:"v" long:"aerospike-version" description:"Aerospike server version" default:"latest"`
	GrafanaVersion   string `long:"grafana-version" description:"Grafana version" default:"11.2.6"`
	Distro           string `short:"d" long:"distro" description:"Linux distribution" default:"ubuntu"`
	DistroVersion    string `long:"distro-version" description:"Distribution version" default:"latest"`

	// Timeout
	Timeout  int  `short:"t" long:"timeout" description:"Creation timeout in minutes" default:"30"`
	NoVacuum bool `long:"no-vacuum" description:"Don't cleanup on failure"`

	// Force flag
	Force bool `long:"force" description:"Force create even if EFS volume already exists (will overwrite existing data)"`

	// Preferred template for reattach (internal use, not a CLI flag)
	// If set and the template exists, it will be used instead of resolving a new one
	PreferredTemplate string `json:"-"`

	// Aerolab binary option
	AerolabBinary flags.Filename `long:"aerolab-binary" description:"Path to local aerolab binary to install (required if running unofficial build)"`

	// AWS-specific options
	AWS AgiCreateCmdAws `group:"AWS" namespace:"aws" description:"backend-aws"`

	// GCP-specific options
	GCP AgiCreateCmdGcp `group:"GCP" namespace:"gcp" description:"backend-gcp"`

	// Docker-specific options
	Docker AgiCreateCmdDocker `group:"Docker" namespace:"docker" description:"backend-docker"`

	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// AgiCreateCmdAws contains AWS-specific options for AGI instance creation.
type AgiCreateCmdAws struct {
	InstanceType        string        `short:"I" long:"instance-type" description:"Instance type (min 12GB RAM); empty=auto-select"`
	Ebs                 string        `short:"E" long:"ebs" description:"EBS volume size in GB" default:"40"`
	SecurityGroupID     string        `short:"S" long:"secgroup-id" description:"Security group IDs (comma-separated)"`
	SubnetID            string        `short:"U" long:"subnet-id" description:"Subnet ID or availability zone"`
	Tags                []string      `long:"tags" description:"Custom tags (key=value)"`
	WithEFS             bool          `long:"with-efs" description:"Use EFS for persistent storage"`
	EFSName             string        `long:"efs-name" description:"EFS volume name" default:"{AGI_NAME}"`
	EFSPath             string        `long:"efs-path" description:"EFS mount path" default:"/"`
	EFSMultiZone        bool          `long:"efs-multizone" description:"Enable multi-AZ EFS (higher cost)"`
	EFSExpires          time.Duration `long:"efs-expire" description:"EFS expiry after last use" default:"96h"`
	EFSFips             bool          `long:"efs-fips" description:"Enable FIPS mode for the EFS mount"`
	TerminateOnPoweroff bool          `long:"terminate-on-poweroff" description:"Terminate instance on poweroff"`
	SpotInstance        bool          `long:"spot-instance" description:"Request spot instance"`
	SpotFallback        bool          `long:"spot-fallback" description:"Fall back to on-demand if spot unavailable"`
	Expires             time.Duration `long:"expire" description:"Instance expiry time" default:"30h"`
	Route53ZoneId       string        `long:"route53-zoneid" description:"Route53 zone ID for DNS"`
	Route53DomainName   string        `long:"route53-domain" description:"Route53 domain name"`
	DisablePublicIP     bool          `long:"disable-public-ip" description:"Disable public IP assignment"`
}

// AgiCreateCmdGcp contains GCP-specific options for AGI instance creation.
type AgiCreateCmdGcp struct {
	InstanceType        string        `long:"instance" description:"Instance type" default:"c2d-highmem-4"`
	Disks               []string      `long:"disk" description:"Disk configuration (type=X,size=Y)" default:"type=pd-ssd,size=40"`
	Zone                string        `long:"zone" description:"GCP zone"`
	Tags                []string      `long:"tag" description:"Network tags"`
	Labels              []string      `long:"label" description:"Labels (key=value)"`
	SpotInstance        bool          `long:"spot-instance" description:"Request spot instance"`
	Expires             time.Duration `long:"expire" description:"Instance expiry time" default:"30h"`
	WithVol             bool          `long:"with-vol" description:"Use persistent volume for storage"`
	VolName             string        `long:"vol-name" description:"Volume name" default:"{AGI_NAME}"`
	VolExpires          time.Duration `long:"vol-expire" description:"Volume expiry after last use" default:"96h"`
	VolFips             bool          `long:"vol-fips" description:"Enable FIPS mode for the volume mount"`
	TerminateOnPoweroff bool          `long:"terminate-on-poweroff" description:"Terminate instance on poweroff"`
}

// AgiCreateCmdDocker contains Docker-specific options for AGI instance creation.
type AgiCreateCmdDocker struct {
	ExposePortsToHost string `short:"e" long:"expose-ports" description:"Port forwarding (HOST_PORT:NODE_PORT)"`
	Privileged        bool   `short:"B" long:"privileged" description:"Run in privileged mode"`
	NetworkName       string `long:"network" description:"Docker network name"`
}

// Execute implements the command execution for agi create.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiCreateCmd) Execute(args []string) error {
	cmd := []string{"agi", "create"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	_, err = c.CreateAGI(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// CreateAGI creates a new AGI instance from an AGI template.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - backends.InstanceList: The created AGI instance(s)
//   - error: nil on success, or an error describing what failed
func (c *AgiCreateCmd) CreateAGI(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "create"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Process ENV:: variables for secrets
	c.processEnvVariables()

	// Handle cluster source - get logs from cluster
	if c.ClusterSource != "" {
		if c.LocalSource != "" {
			return nil, fmt.Errorf("cannot specify both --source-cluster and --source-local")
		}
		localSource, err := c.getLogsFromCluster(system, inventory, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to get logs from cluster %s: %w", c.ClusterSource, err)
		}
		c.LocalSource = flags.Filename(localSource)
		defer os.RemoveAll(string(localSource))
	}

	// Handle ~auto~ naming
	if c.ClusterName == "~auto~" {
		c.ClusterName = TypeAgiClusterName(c.generateAutoName())
	}

	// Check for existing EFS volume (AWS only)
	if system.Opts.Config.Backend.Type == "aws" && c.AWS.WithEFS {
		volumeName := strings.ReplaceAll(c.AWS.EFSName, "{AGI_NAME}", string(c.ClusterName))
		existingVol := inventory.Volumes.WithType(backends.VolumeTypeSharedDisk).WithName(volumeName)
		if existingVol.Count() > 0 && !c.Force {
			return nil, fmt.Errorf("EFS volume '%s' already exists.\n"+
				"  - Use 'aerolab agi start -n %s --aws-with-efs' to start with existing EFS data\n"+
				"  - Use --force to create a new AGI and overwrite existing EFS data",
				volumeName, c.ClusterName)
		}
	}

	// Check for existing GCP volume
	if system.Opts.Config.Backend.Type == "gcp" && c.GCP.WithVol {
		volumeName := strings.ReplaceAll(c.GCP.VolName, "{AGI_NAME}", string(c.ClusterName))
		existingVol := inventory.Volumes.WithType(backends.VolumeTypeAttachedDisk).WithName(volumeName)
		if existingVol.Count() > 0 && !c.Force {
			return nil, fmt.Errorf("GCP volume '%s' already exists.\n"+
				"  - Use 'aerolab agi start -n %s --gcp-with-vol' to start with existing volume data\n"+
				"  - Use --force to create a new AGI and overwrite existing volume data",
				volumeName, c.ClusterName)
		}
	}

	// Validate parameters
	if err := c.validateParameters(); err != nil {
		return nil, err
	}

	// Set defaults
	if c.AGILabel == "" {
		c.AGILabel = string(c.ClusterName)
	}
	if c.Owner == "" {
		c.Owner = currentOwnerUser
	}

	// Get backend type
	backendType := system.Opts.Config.Backend.Type

	// Set default GCP zone from configured region if not specified
	if backendType == "gcp" && c.GCP.Zone == "" {
		c.GCP.Zone = system.Opts.Config.Backend.Region + "-a"
		logger.Info("Using default zone %s", c.GCP.Zone)
	}

	// Docker doesn't support monitor
	if backendType == "docker" && c.MonitorUrl != "" {
		return nil, fmt.Errorf("AGI monitor is not supported on Docker")
	}

	// Bind mount is only supported on Docker
	if isBindMountSource(string(c.LocalSource)) && backendType != "docker" {
		return nil, fmt.Errorf("bind mount (bind:/path) for --source-local is only supported on Docker backend")
	}

	// --bind-files-dir is only supported on Docker
	if c.BindFilesDir != "" && backendType != "docker" {
		return nil, fmt.Errorf("--bind-files-dir is only supported on Docker backend")
	}

	// Validate --bind-files-dir directory exists
	if c.BindFilesDir != "" {
		info, err := os.Stat(string(c.BindFilesDir))
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("--bind-files-dir directory not found: %s", c.BindFilesDir)
		}
		if err != nil {
			return nil, fmt.Errorf("--bind-files-dir error: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("--bind-files-dir must be a directory: %s", c.BindFilesDir)
		}
	}

	// Verify local binary exists if specified
	if c.AerolabBinary != "" {
		if _, err := os.Stat(string(c.AerolabBinary)); os.IsNotExist(err) {
			return nil, fmt.Errorf("aerolab binary not found: %s", c.AerolabBinary)
		}
		logger.Info("Will use local aerolab binary: %s", c.AerolabBinary)
	}

	// Start background task to ensure expiry system has CleanupDNS enabled (AWS with Route53 only)
	var expiryCleanupWg sync.WaitGroup
	if backendType == "aws" && c.AWS.Route53ZoneId != "" && c.AWS.Route53DomainName != "" {
		expiryCleanupWg.Add(1)
		go func() {
			defer expiryCleanupWg.Done()
			if err := c.ensureExpiryCleanupDNS(system, logger); err != nil {
				logger.Warn("Failed to enable CleanupDNS in expiry system: %s", err)
				logger.Warn("You may need to manually enable CleanupDNS via: aerolab config aws expiry-install -c")
			}
		}()
	}
	defer expiryCleanupWg.Wait()

	// Check for existing volume and restore settings if found
	if err := c.checkAndRestoreVolumeSettings(system, inventory, logger); err != nil {
		return nil, err
	}

	// Determine architecture based on backend and instance type
	var arch backends.Architecture
	switch backendType {
	case "docker":
		ar := system.Opts.Config.Backend.Arch
		if ar == "" {
			ar = runtime.GOARCH
		}
		arch.FromString(ar)
	case "aws":
		// If instance type specified, get arch from it; otherwise default to amd64
		if c.AWS.InstanceType != "" {
			itypes, err := system.Backend.GetInstanceTypes(backends.BackendTypeAWS)
			if err == nil {
				for _, i := range itypes {
					if i.Name == c.AWS.InstanceType && len(i.Arch) > 0 {
						arch.FromString(i.Arch[0].String())
						break
					}
				}
			}
		}
		if arch.String() == "" {
			arch.FromString("amd64")
		}
	case "gcp":
		// If instance type specified, get arch from it; otherwise default to amd64
		if c.GCP.InstanceType != "" {
			itypes, err := system.Backend.GetInstanceTypes(backends.BackendTypeGCP)
			if err == nil {
				for _, i := range itypes {
					if i.Name == c.GCP.InstanceType && len(i.Arch) > 0 {
						arch.FromString(i.Arch[0].String())
						break
					}
				}
			}
		}
		if arch.String() == "" {
			arch.FromString("amd64")
		}
	default:
		arch.FromString("amd64")
	}
	logger.Info("Using architecture: %s", arch.String())

	// Resolve AGI template
	templateName, templateCreated, err := c.resolveTemplate(system, inventory, logger, args, arch)
	if err != nil {
		return nil, err
	}
	logger.Info("Using AGI template: %s", templateName)

	// Ensure AGI firewall exists (AWS/GCP only)
	var agiFirewallName string
	if backendType == "aws" || backendType == "gcp" {
		var err error
		agiFirewallName, err = c.ensureAGIFirewall(system, inventory, logger, backendType)
		if err != nil {
			return nil, fmt.Errorf("failed to ensure AGI firewall: %w", err)
		}
		// Refresh inventory to include new firewall
		inventory, err = system.Backend.GetRefreshedInventory()
		if err != nil {
			return nil, fmt.Errorf("failed to refresh inventory after firewall creation: %w", err)
		}
	}

	// Create instance from template
	logger.Info("Creating AGI instance from template")
	instance, err := c.createInstance(system, inventory, logger, templateName, args, arch, agiFirewallName)
	if err != nil {
		return nil, fmt.Errorf("failed to create AGI instance: %w", err)
	}

	// Add cleanup job for interrupt handling
	instName := string(c.ClusterName)
	shutdown.AddEarlyCleanupJob("agi-create-"+instName, func(isSignal bool) {
		if !isSignal {
			return
		}
		if !c.NoVacuum {
			c.NoVacuum = true
			logger.Info("Abort: destroying AGI instance")
			err := instance.Terminate(time.Minute * 10)
			if err != nil {
				logger.Error("could not destroy instance: %s", err)
			}
		}
	})

	// Defer cleanup on failure
	defer func() {
		if !c.NoVacuum && err != nil {
			logger.Info("Destroying AGI instance on failure")
			err := instance.Terminate(time.Minute * 10)
			if err != nil {
				logger.Error("could not destroy instance: %s", err)
			}
		}
	}()

	// Configure Route53 DNS if specified (AWS only)
	if backendType == "aws" && c.AWS.Route53ZoneId != "" && c.AWS.Route53DomainName != "" {
		logger.Info("Configuring Route53 DNS")
		if err := c.configureAGIDNS(system, instance, logger); err != nil {
			logger.Warn("Failed to configure DNS: %s", err)
		}
	}

	// Get available memory on the instance
	memSize, err := c.getAvailableMemory(instance, backendType)
	if err != nil {
		return nil, err
	}
	logger.Info("Available memory for Aerospike: %d GB", memSize/1024/1024/1024)

	// Generate configuration files
	logger.Info("Generating AGI configuration files")
	configs, err := c.generateConfigs(system, memSize, backendType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate configs: %w", err)
	}

	// Upload configuration files via SFTP
	logger.Info("Uploading configuration to AGI instance")
	if err := c.uploadConfigs(instance, configs, logger); err != nil {
		return nil, fmt.Errorf("failed to upload configs: %w", err)
	}

	// Upload local source files if specified
	if c.LocalSource != "" {
		logger.Info("Uploading local source files")
		if err := c.uploadLocalSource(instance, logger); err != nil {
			return nil, fmt.Errorf("failed to upload local source: %w", err)
		}
	}

	// Upload local aerolab binary if specified (overrides template's aerolab)
	// Skip if template was just created - it already has the correct binary
	if c.AerolabBinary != "" && !templateCreated {
		logger.Info("Uploading local aerolab binary")
		if err := c.uploadAerolabBinary(instance, logger); err != nil {
			return nil, fmt.Errorf("failed to upload aerolab binary: %w", err)
		}
	}

	// Generate and upload aerospike.conf
	logger.Info("Configuring Aerospike")
	if err := c.configureAerospike(instance, memSize, backendType, logger); err != nil {
		return nil, fmt.Errorf("failed to configure Aerospike: %w", err)
	}

	// Start AGI services
	logger.Info("Starting AGI services")
	if err := c.startServices(instance, logger); err != nil {
		return nil, fmt.Errorf("failed to start services: %w", err)
	}

	// Create marker file
	instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "date > /opt/agi-installed"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	// Mark as successful (don't vacuum)
	c.NoVacuum = true

	// Get access information
	logger.Info("AGI instance created successfully!")
	logger.Info("")
	logger.Info("Useful commands:")
	logger.Info("  aerolab agi list                       - List AGI instances")
	logger.Info("  aerolab agi add-auth-token -n %s --url - Add auth token and display access URL", c.ClusterName)
	logger.Info("  aerolab agi open -n %s                 - Open AGI in browser", c.ClusterName)
	logger.Info("  aerolab agi attach -n %s               - Attach to shell", c.ClusterName)
	logger.Info("  aerolab agi status -n %s               - Show status", c.ClusterName)

	return instance, nil
}

// processEnvVariables processes ENV::VAR_NAME patterns in configuration fields.
func (c *AgiCreateCmd) processEnvVariables() {
	fields := []*string{
		&c.SlackToken,
		&c.SftpUser,
		&c.SftpPass,
		&c.S3KeyID,
		&c.S3Secret,
	}
	for _, field := range fields {
		if strings.HasPrefix(*field, "ENV::") {
			envVar := strings.TrimPrefix(*field, "ENV::")
			*field = os.Getenv(envVar)
		}
	}
}

// generateAutoName generates an auto name based on source parameters.
func (c *AgiCreateCmd) generateAutoName() string {
	nName := ""
	if c.LocalSource != "" {
		nName = string(c.LocalSource)
	}
	nName += "\nS3"
	if c.S3Enable {
		nName += "\n" + c.S3Bucket + "\n" + c.S3Path + "\n" + c.S3Regex
	}
	nName += "\nSFTP"
	if c.SftpEnable {
		nName += "\n" + c.SftpHost + "\n" + strconv.Itoa(c.SftpPort) + "\n" + c.SftpUser + "\n" + c.SftpPath + "\n" + c.SftpRegex
	}
	return shortuuid.NewWithNamespace(nName)
}

// isBindMountSource checks if the local source uses bind mount syntax (bind:/path).
func isBindMountSource(source string) bool {
	return strings.HasPrefix(source, "bind:")
}

// getBindMountPath extracts the path from a bind mount source (bind:/path -> /path).
func getBindMountPath(source string) string {
	return strings.TrimPrefix(source, "bind:")
}

// validateParameters validates the command parameters.
func (c *AgiCreateCmd) validateParameters() error {
	// Skip source validation when reusing existing EFS config (reattach/resize scenario)
	if !c.NoConfigOverride {
		// At least one source must be specified
		hasSource := c.LocalSource != "" || c.SftpEnable || c.S3Enable
		if !hasSource {
			return fmt.Errorf("at least one source must be specified (--source-local, --source-sftp-enable, or --source-s3-enable)")
		}
	}

	// Validate S3 configuration (still validate S3 path if S3 is enabled, even with NoConfigOverride)
	if c.S3Enable && c.S3Path == "" {
		return fmt.Errorf("--source-s3-path is required when S3 source is enabled")
	}

	// Validate SSL cert/key
	if (c.ProxyCert != "" && c.ProxyKey == "") || (c.ProxyCert == "" && c.ProxyKey != "") {
		return fmt.Errorf("both --proxy-ssl-cert and --proxy-ssl-key must be specified together")
	}

	// Validate Route53 configuration
	if (c.AWS.Route53ZoneId != "" && c.AWS.Route53DomainName == "") ||
		(c.AWS.Route53ZoneId == "" && c.AWS.Route53DomainName != "") {
		return fmt.Errorf("both --route53-zoneid and --route53-domain must be specified together")
	}

	// Validate time ranges
	if c.TimeRanges {
		if c.TimeRangesFrom == "" || c.TimeRangesTo == "" {
			return fmt.Errorf("both --ingest-timeranges-from and --ingest-timeranges-to are required when time ranges are enabled")
		}
	}

	// Skip file existence checks when reusing existing EFS config (reattach/resize scenario)
	if !c.NoConfigOverride {
		// Validate files exist (handle bind: prefix for local source)
		localSourcePath := string(c.LocalSource)
		if isBindMountSource(localSourcePath) {
			localSourcePath = getBindMountPath(localSourcePath)
		}

		filesToCheck := []string{
			localSourcePath,
			string(c.SftpKey),
			string(c.ProxyCert),
			string(c.ProxyKey),
			string(c.PatternsFile),
		}
		for _, f := range filesToCheck {
			if f == "" {
				continue
			}
			if _, err := os.Stat(f); err != nil {
				return fmt.Errorf("%s is not accessible: %w", f, err)
			}
		}
	}

	// Validate S3 accessibility if enabled (unless skipped)
	if c.S3Enable && !c.S3SkipCheck && !c.NoConfigOverride {
		if err := c.validateS3Access(); err != nil {
			return fmt.Errorf("S3 accessibility check failed: %w\n  Use --source-s3-skipcheck to bypass this check", err)
		}
	}

	// Validate SFTP accessibility if enabled (unless skipped)
	if c.SftpEnable && !c.SftpSkipCheck && !c.NoConfigOverride {
		if err := c.validateSFTPAccess(); err != nil {
			return fmt.Errorf("SFTP accessibility check failed: %w\n  Use --source-sftp-skipcheck to bypass this check", err)
		}
	}

	return nil
}

// validateS3Access validates that the S3 bucket and path are accessible with the provided credentials.
func (c *AgiCreateCmd) validateS3Access() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build AWS config with provided credentials
	var cfgParams []func(*config.LoadOptions) error
	if c.S3Region != "" {
		cfgParams = append(cfgParams, config.WithRegion(c.S3Region))
	}
	if c.S3KeyID != "" {
		cfgParams = append(cfgParams, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.S3KeyID, c.S3Secret, "")))
	}

	cfg, err := config.LoadDefaultConfig(ctx, cfgParams...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	var client *s3.Client
	if c.S3Endpoint != "" {
		client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(c.S3Endpoint)
		})
	} else {
		client = s3.NewFromConfig(cfg)
	}

	// Check bucket access using HeadBucket
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.S3Bucket),
	})
	if err != nil {
		return fmt.Errorf("cannot access bucket '%s': %w", c.S3Bucket, err)
	}

	// If path is specified and not empty/root, check if objects exist at that path
	if c.S3Path != "" && c.S3Path != "/" {
		// Use ListObjectsV2 with MaxKeys=1 to check if any objects exist at the path
		result, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:  aws.String(c.S3Bucket),
			Prefix:  aws.String(c.S3Path),
			MaxKeys: aws.Int32(1),
		})
		if err != nil {
			return fmt.Errorf("cannot list objects at path '%s': %w", c.S3Path, err)
		}
		if len(result.Contents) == 0 {
			return fmt.Errorf("no objects found at path '%s' in bucket '%s'", c.S3Path, c.S3Bucket)
		}
	}

	return nil
}

// validateSFTPAccess validates that the SFTP server and path are accessible with the provided credentials.
func (c *AgiCreateCmd) validateSFTPAccess() error {
	// Build SSH config
	var authMethods []ssh.AuthMethod

	if c.SftpPass != "" {
		// Password authentication
		authMethods = append(authMethods, ssh.Password(c.SftpPass))
	}

	if c.SftpKey != "" {
		// Key file authentication
		keyData, err := os.ReadFile(string(c.SftpKey))
		if err != nil {
			return fmt.Errorf("cannot read SSH key file '%s': %w", c.SftpKey, err)
		}
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return fmt.Errorf("cannot parse SSH key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if len(authMethods) == 0 {
		return fmt.Errorf("no authentication method provided (password or key file required)")
	}

	sshConfig := &ssh.ClientConfig{
		User:            c.SftpUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	// Connect to SSH
	addr := fmt.Sprintf("%s:%d", c.SftpHost, c.SftpPort)
	sshClient, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("cannot connect to SFTP server '%s': %w", addr, err)
	}
	defer sshClient.Close()

	// Create SFTP client
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("cannot establish SFTP session: %w", err)
	}
	defer sftpClient.Close()

	// If path is specified and not empty/root, check if it exists
	if c.SftpPath != "" && c.SftpPath != "/" {
		fi, err := sftpClient.Stat(c.SftpPath)
		if err != nil {
			return fmt.Errorf("cannot access path '%s': %w", c.SftpPath, err)
		}
		// If it's a directory, check if it has any files
		if fi.IsDir() {
			files, err := sftpClient.ReadDir(c.SftpPath)
			if err != nil {
				return fmt.Errorf("cannot list directory '%s': %w", c.SftpPath, err)
			}
			if len(files) == 0 {
				return fmt.Errorf("directory '%s' is empty", c.SftpPath)
			}
		}
	}

	return nil
}

// checkAndRestoreVolumeSettings checks for existing volume and restores settings.
// When Force=true (reattach scenario), settings are already populated by cmdAgiStart.go
// with any overrides applied, so we skip the restore to avoid overwriting overrides.
func (c *AgiCreateCmd) checkAndRestoreVolumeSettings(system *System, inventory *backends.Inventory, logger *logger.Logger) error {
	backendType := system.Opts.Config.Backend.Type

	// Only applicable for cloud backends with volumes
	if backendType == "docker" {
		return nil
	}

	var volumeName string
	if backendType == "aws" && c.AWS.WithEFS {
		volumeName = strings.ReplaceAll(c.AWS.EFSName, "{AGI_NAME}", string(c.ClusterName))
	} else if backendType == "gcp" && c.GCP.WithVol {
		volumeName = strings.ReplaceAll(c.GCP.VolName, "{AGI_NAME}", string(c.ClusterName))
	} else {
		return nil
	}

	// Check for existing volume
	volumes := inventory.Volumes.WithName(volumeName)
	if volumes.Count() == 0 {
		return nil
	}

	vol := volumes.Describe()[0]

	// When Force=true (reattach scenario from agi start), cmdAgiStart.go has already
	// read all settings from volume tags and applied any overrides. Skip restore to
	// avoid overwriting overrides (e.g., instance type sizing, spot->on-demand for capacity).
	if c.Force {
		logger.Debug("Reattach mode (Force=true), skipping volume settings restore")
		return nil
	}

	logger.Info("Found existing volume %s, restoring settings", volumeName)

	// Restore settings from volume tags (only for direct agi create, not reattach)
	// Check for empty or default "latest" - if user specified a specific version, keep it
	if c.AerospikeVersion == "" || c.AerospikeVersion == "latest" {
		if v, ok := vol.Tags["aerolab7agiav"]; ok && v != "" {
			c.AerospikeVersion = v
		}
	}
	if backendType == "aws" && c.AWS.InstanceType == "" {
		if v, ok := vol.Tags["agiinstance"]; ok && v != "" {
			c.AWS.InstanceType = v
		}
	} else if backendType == "gcp" && c.GCP.InstanceType == "" {
		if v, ok := vol.Tags["agiinstance"]; ok && v != "" {
			c.GCP.InstanceType = v
		}
	}
	if vol.Tags["aginodim"] == "true" {
		c.NoDIM = true
	}
	if vol.Tags["termonpow"] == "true" {
		if backendType == "aws" {
			c.AWS.TerminateOnPoweroff = true
		} else {
			c.GCP.TerminateOnPoweroff = true
		}
	}
	if vol.Tags["isspot"] == "true" {
		if backendType == "aws" {
			c.AWS.SpotInstance = true
		} else {
			c.GCP.SpotInstance = true
		}
	}

	return nil
}

// resolveTemplate finds or creates an AGI template.
// Returns the template name and a boolean indicating if a new template was created.
func (c *AgiCreateCmd) resolveTemplate(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, arch backends.Architecture) (string, bool, error) {
	// Check for preferred template first (used by reattach to use the same template)
	if c.PreferredTemplate != "" {
		preferredImages := inventory.Images.WithName(c.PreferredTemplate)
		if preferredImages.Count() > 0 {
			logger.Info("Using preferred template from volume tags: %s", c.PreferredTemplate)
			return c.PreferredTemplate, false, nil
		}
		logger.Warn("Preferred template %s not found, resolving new template", c.PreferredTemplate)
	}

	// Look for existing template
	images := inventory.Images.WithTags(map[string]string{
		"aerolab.image.type":  "agi",
		"aerolab.agi.version": strconv.Itoa(agi.AGIVersion),
	}).WithArchitecture(arch)

	if images.Count() > 0 {
		// Template exists - use it as-is (custom binary upload happens later in CreateAGI if specified)
		return images.Describe()[0].Name, false, nil
	}

	// No template found, need to create one
	// Note: CreateTemplate handles unofficial build logic (using self on Linux if arch matches)
	logger.Info("No AGI template found, creating one...")

	// if AWS, make EFS utils install in the template
	withEFS := system.Opts.Config.Backend.Type == "aws"

	templateCreate := &AgiTemplateCreateCmd{
		AerospikeVersion: c.AerospikeVersion,
		GrafanaVersion:   c.GrafanaVersion,
		Distro:           c.Distro,
		DistroVersion:    c.DistroVersion,
		Arch:             arch.String(),
		Timeout:          c.Timeout,
		NoVacuum:         c.NoVacuum,
		Owner:            c.Owner,
		DisablePublicIP:  c.AWS.DisablePublicIP,
		AerolabBinary:    c.AerolabBinary,
		WithEFS:          withEFS,
	}

	templateName, err := templateCreate.CreateTemplate(system, inventory, logger.WithPrefix("[template] "), args)
	if err != nil {
		return "", false, fmt.Errorf("failed to create AGI template: %w", err)
	}

	return templateName, true, nil
}

// createInstance creates the AGI instance from the template.
// The agiFirewallName parameter is the VPC-specific firewall name for cloud backends (empty for Docker).
func (c *AgiCreateCmd) createInstance(system *System, inventory *backends.Inventory, logger *logger.Logger, templateName string, args []string, arch backends.Architecture, agiFirewallName string) (backends.InstanceList, error) {
	backendType := system.Opts.Config.Backend.Type

	// Sanitize instance name for GCP
	instName := string(c.ClusterName)
	if backendType == "gcp" {
		instName = sanitizeGCPName(instName)
	}

	// Build source strings for volume and instance tags
	// This is done early so the tags can be applied to volumes
	sourceStringLocal := ""
	sourceStringSftp := ""
	sourceStringS3 := ""
	if c.LocalSource != "" {
		sourceStringLocal = "local:" + string(c.LocalSource)
		if len(sourceStringLocal) > 191 {
			sourceStringLocal = sourceStringLocal[:188] + "..."
		}
	}
	if c.SftpEnable {
		sourceStringSftp = c.SftpHost + ":" + c.SftpPath
		if c.SftpRegex != "" {
			sourceStringSftp += "^" + c.SftpRegex
		}
		if len(sourceStringSftp) > 191 {
			sourceStringSftp = sourceStringSftp[:188] + "..."
		}
	}
	if c.S3Enable {
		sourceStringS3 = c.S3Bucket + ":" + c.S3Path
		if c.S3Regex != "" {
			sourceStringS3 += "^" + c.S3Regex
		}
		if len(sourceStringS3) > 191 {
			sourceStringS3 = sourceStringS3[:188] + "..."
		}
	}
	// Base64 encode source strings for tags
	sourceStringLocalB64 := base64.RawStdEncoding.EncodeToString([]byte(sourceStringLocal))
	sourceStringSftpB64 := base64.RawStdEncoding.EncodeToString([]byte(sourceStringSftp))
	sourceStringS3B64 := base64.RawStdEncoding.EncodeToString([]byte(sourceStringS3))
	agiLabelB64 := base64.RawStdEncoding.EncodeToString([]byte(c.AGILabel))

	// Create EFS/GCP volume if requested and doesn't exist
	var volumeName string
	var volumeCreated bool
	var volFips bool
	if backendType == "aws" && c.AWS.WithEFS {
		volFips = c.AWS.EFSFips
		volumeName = strings.ReplaceAll(c.AWS.EFSName, "{AGI_NAME}", string(c.ClusterName))
		if inventory.Volumes.WithType(backends.VolumeTypeSharedDisk).WithName(volumeName).Count() == 0 {
			logger.Info("Creating EFS volume: %s", volumeName)
			volumeCreated = true
			// Build volume tags with AGI metadata (includes settings needed for reattach)
			volumeTags := []string{
				fmt.Sprintf("agiinstance=%s", c.AWS.InstanceType),
				fmt.Sprintf("aginodim=%t", c.NoDIM),
				fmt.Sprintf("termonpow=%t", c.AWS.TerminateOnPoweroff),
				fmt.Sprintf("isspot=%t", c.AWS.SpotInstance),
				fmt.Sprintf("aerolab7agiav=%s", c.AerospikeVersion),
				fmt.Sprintf("agifips=%t", c.AWS.EFSFips),
				fmt.Sprintf("agisubnet=%s", c.AWS.SubnetID),
				fmt.Sprintf("agisecgroup=%s", c.AWS.SecurityGroupID),
				fmt.Sprintf("agiefsexpire=%s", c.AWS.EFSExpires.String()),
				fmt.Sprintf("agiLabel=%s", agiLabelB64),
				fmt.Sprintf("agiSrcLocal=%s", sourceStringLocalB64),
				fmt.Sprintf("agiSrcSftp=%s", sourceStringSftpB64),
				fmt.Sprintf("agiSrcS3=%s", sourceStringS3B64),
			}
			volume := &VolumesCreateCmd{
				Name:            volumeName,
				Description:     fmt.Sprintf("EFS volume for AGI %s", c.ClusterName),
				Owner:           c.Owner,
				Tags:            volumeTags,
				VolumeType:      "shared",
				NoInstallExpiry: false,
				AWS: VolumesCreateCmdAws{
					SizeGiB:           0,
					Placement:         c.AWS.SubnetID,
					DiskType:          "shared",
					Iops:              0,
					Throughput:        0,
					Encrypted:         true,
					SharedDiskOneZone: !c.AWS.EFSMultiZone,
					Expire:            c.AWS.EFSExpires,
				},
				DryRun: false,
			}
			_, err := volume.CreateVolumes(system, inventory, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create EFS volume: %w", err)
			}
			// Refresh inventory to include new volume
			inventory, err = system.Backend.GetRefreshedInventory()
			if err != nil {
				return nil, fmt.Errorf("failed to refresh inventory after volume creation: %w", err)
			}
		} else {
			logger.Info("Using existing EFS volume: %s", volumeName)
			// Reset volume expiry to the specified duration from now (zero duration = never expire)
			vol := inventory.Volumes.WithType(backends.VolumeTypeSharedDisk).WithName(volumeName)
			if c.AWS.EFSExpires == 0 {
				logger.Debug("Removing EFS volume expiry (never expire)")
				if err := vol.ChangeExpiry(time.Time{}); err != nil {
					logger.Warn("Failed to remove EFS volume expiry: %s", err)
				}
			} else {
				newExpiry := time.Now().Add(c.AWS.EFSExpires)
				logger.Debug("Resetting EFS volume expiry to %s", newExpiry.Format(time.RFC3339))
				if err := vol.ChangeExpiry(newExpiry); err != nil {
					logger.Warn("Failed to reset EFS volume expiry: %s", err)
				}
			}
			// Update EFS tags with current instance settings
			// This ensures tags reflect the latest settings (important for monitor sizing and reattach)
			newTags := map[string]string{
				"agiinstance":   c.AWS.InstanceType,
				"aginodim":      fmt.Sprintf("%t", c.NoDIM),
				"termonpow":     fmt.Sprintf("%t", c.AWS.TerminateOnPoweroff),
				"isspot":        fmt.Sprintf("%t", c.AWS.SpotInstance),
				"aerolab7agiav": c.AerospikeVersion,
				"agifips":       fmt.Sprintf("%t", c.AWS.EFSFips),
				"agisubnet":     c.AWS.SubnetID,
				"agisecgroup":   c.AWS.SecurityGroupID,
				"agiefsexpire":  c.AWS.EFSExpires.String(),
				"agiLabel":      agiLabelB64,
				"agiSrcLocal":   sourceStringLocalB64,
				"agiSrcSftp":    sourceStringSftpB64,
				"agiSrcS3":      sourceStringS3B64,
			}
			if err := vol.AddTags(newTags, 5*time.Minute); err != nil {
				logger.Warn("Failed to update EFS volume tags: %s", err)
			} else {
				logger.Debug("Updated EFS volume tags with current instance settings")
			}
		}
	} else if backendType == "gcp" && c.GCP.WithVol {
		volFips = c.GCP.VolFips
		volumeName = strings.ReplaceAll(c.GCP.VolName, "{AGI_NAME}", string(c.ClusterName))
		if inventory.Volumes.WithType(backends.VolumeTypeAttachedDisk).WithName(volumeName).Count() == 0 {
			logger.Info("Creating GCP volume: %s", volumeName)
			volumeCreated = true
			// Build volume tags with AGI metadata (includes settings needed for reattach)
			volumeTags := []string{
				fmt.Sprintf("agiinstance=%s", c.GCP.InstanceType),
				fmt.Sprintf("aginodim=%t", c.NoDIM),
				fmt.Sprintf("termonpow=%t", c.GCP.TerminateOnPoweroff),
				fmt.Sprintf("isspot=%t", c.GCP.SpotInstance),
				fmt.Sprintf("aerolab7agiav=%s", c.AerospikeVersion),
				fmt.Sprintf("agifips=%t", c.GCP.VolFips),
				fmt.Sprintf("agizone=%s", c.GCP.Zone),
				fmt.Sprintf("agivolexpire=%s", c.GCP.VolExpires.String()),
				fmt.Sprintf("agiLabel=%s", agiLabelB64),
				fmt.Sprintf("agiSrcLocal=%s", sourceStringLocalB64),
				fmt.Sprintf("agiSrcSftp=%s", sourceStringSftpB64),
				fmt.Sprintf("agiSrcS3=%s", sourceStringS3B64),
			}
			volume := &VolumesCreateCmd{
				Name:            volumeName,
				Description:     fmt.Sprintf("GCP volume for AGI %s - %s", c.ClusterName, c.AGILabel),
				Owner:           c.Owner,
				Tags:            volumeTags,
				VolumeType:      "attached",
				NoInstallExpiry: false,
				GCP: VolumesCreateCmdGcp{
					SizeGiB:    50, // Default size for AGI GCP volume
					Zone:       c.GCP.Zone,
					DiskType:   "pd-ssd",
					Iops:       0,
					Throughput: 0,
					Expire:     c.GCP.VolExpires,
				},
				DryRun: false,
			}
			_, err := volume.CreateVolumes(system, inventory, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create GCP volume: %w", err)
			}
			// Refresh inventory to include new volume
			inventory, err = system.Backend.GetRefreshedInventory()
			if err != nil {
				return nil, fmt.Errorf("failed to refresh inventory after volume creation: %w", err)
			}
		} else {
			logger.Info("Using existing GCP volume: %s", volumeName)
			// Reset volume expiry to the specified duration from now (zero duration = never expire)
			vol := inventory.Volumes.WithType(backends.VolumeTypeAttachedDisk).WithName(volumeName)
			if c.GCP.VolExpires == 0 {
				logger.Debug("Removing GCP volume expiry (never expire)")
				if err := vol.ChangeExpiry(time.Time{}); err != nil {
					logger.Warn("Failed to remove GCP volume expiry: %s", err)
				}
			} else {
				newExpiry := time.Now().Add(c.GCP.VolExpires)
				logger.Debug("Resetting GCP volume expiry to %s", newExpiry.Format(time.RFC3339))
				if err := vol.ChangeExpiry(newExpiry); err != nil {
					logger.Warn("Failed to reset GCP volume expiry: %s", err)
				}
			}
			// Update GCP volume tags with current instance settings
			// This ensures tags reflect the latest settings (important for monitor sizing and reattach)
			newTags := map[string]string{
				"agiinstance":   c.GCP.InstanceType,
				"aginodim":      fmt.Sprintf("%t", c.NoDIM),
				"termonpow":     fmt.Sprintf("%t", c.GCP.TerminateOnPoweroff),
				"isspot":        fmt.Sprintf("%t", c.GCP.SpotInstance),
				"aerolab7agiav": c.AerospikeVersion,
				"agifips":       fmt.Sprintf("%t", c.GCP.VolFips),
				"agizone":       c.GCP.Zone,
				"agivolexpire":  c.GCP.VolExpires.String(),
				"agiLabel":      agiLabelB64,
				"agiSrcLocal":   sourceStringLocalB64,
				"agiSrcSftp":    sourceStringSftpB64,
				"agiSrcS3":      sourceStringS3B64,
			}
			if err := vol.AddTags(newTags, 5*time.Minute); err != nil {
				logger.Warn("Failed to update GCP volume tags: %s", err)
			} else {
				logger.Debug("Updated GCP volume tags with current instance settings")
			}
		}
	}

	// Determine instance type based on backend
	var awsInstanceType, gcpInstanceType string
	switch backendType {
	case "aws":
		awsInstanceType = c.AWS.InstanceType
		if awsInstanceType == "" {
			if arch.String() == "arm64" {
				awsInstanceType = "r7g.xlarge"
			} else {
				switch system.Opts.Config.Backend.Region {
				case "af-south-1", "ap-east-1", "ca-west-1", "cn-north-1", "cn-northwest-1", "eu-central-2", "il-central-1", "me-south-1", "me-central-1":
					// change the default instance family for regions that don't support the r7 yet
					awsInstanceType = "r6i.xlarge"
				default:
					awsInstanceType = "r7i.xlarge"
				}
			}
		}
	case "gcp":
		gcpInstanceType = c.GCP.InstanceType
	}

	// Determine exposed port
	var exposePort string
	if c.Docker.ExposePortsToHost != "" {
		exposePort = c.Docker.ExposePortsToHost
	} else if backendType == "docker" {
		// Dynamic port allocation for Docker
		exposePort = findNextAvailableAGIPort(inventory, !c.ProxyDisableSSL)
	} else if !c.ProxyDisableSSL {
		exposePort = "8443:443"
	} else {
		exposePort = "8080:80"
	}

	// Build tags list
	instanceTags := []string{
		fmt.Sprintf("aerolab4features=%d", ClusterFeatureAGI),
		fmt.Sprintf("aerolab4ssl=%t", !c.ProxyDisableSSL),
		fmt.Sprintf("agiLabel=%s", agiLabelB64),
		fmt.Sprintf("agiSrcLocal=%s", sourceStringLocalB64),
		fmt.Sprintf("agiSrcSftp=%s", sourceStringSftpB64),
		fmt.Sprintf("agiSrcS3=%s", sourceStringS3B64),
		fmt.Sprintf("aerolab7agiav=%s", c.AerospikeVersion),
	}
	// Add Route53 tags if configured
	if c.AWS.Route53ZoneId != "" && c.AWS.Route53DomainName != "" {
		instanceTags = append(instanceTags, fmt.Sprintf("agiDomain=%s", c.AWS.Route53DomainName))
		instanceTags = append(instanceTags, fmt.Sprintf("agiZoneID=%s", c.AWS.Route53ZoneId))
	}

	// Add user-provided custom tags/labels
	switch backendType {
	case "aws":
		instanceTags = append(instanceTags, c.AWS.Tags...)
	case "gcp":
		instanceTags = append(instanceTags, c.GCP.Labels...)
	}

	// Determine terminate-on-stop based on backend
	var terminateOnStop bool
	switch backendType {
	case "aws":
		terminateOnStop = c.AWS.TerminateOnPoweroff
	case "gcp":
		terminateOnStop = c.GCP.TerminateOnPoweroff
	}

	// Build instance creation command
	instancesCreate := &InstancesCreateCmd{
		ClusterName:        instName,
		Count:              1,
		Name:               instName,
		Owner:              c.Owner,
		Type:               "agi",
		Tags:               instanceTags,
		Description:        fmt.Sprintf("AGI: %s", c.AGILabel),
		TerminateOnStop:    terminateOnStop,
		ParallelSSHThreads: 1,
		ImageType:          "agi",
		ImageVersion:       fmt.Sprintf("agi-%s-%s-%d", c.AerospikeVersion, "latest", agi.AGIVersion),
		Arch:               arch.String(),
		AWS: InstancesCreateCmdAws{
			ImageID:          templateName,
			Expire:           c.AWS.Expires,
			NetworkPlacement: system.Opts.Config.Backend.Region,
			InstanceType:     awsInstanceType,
			Disks:            []string{fmt.Sprintf("type=gp2,size=%s", c.AWS.Ebs)},
			Firewalls:        []string{agiFirewallName},
			SpotInstance:     c.AWS.SpotInstance,
			DisablePublicIP:  c.AWS.DisablePublicIP,
		},
		GCP: InstancesCreateCmdGcp{
			ImageName:    templateName,
			Expire:       c.GCP.Expires,
			Zone:         c.GCP.Zone,
			InstanceType: gcpInstanceType,
			Disks:        c.GCP.Disks,
			Firewalls:    append([]string{agiFirewallName}, c.GCP.Tags...),
			SpotInstance: c.GCP.SpotInstance,
		},
		Docker: InstancesCreateCmdDocker{
			ImageName:   templateName,
			NetworkName: c.Docker.NetworkName,
			ExposePorts: []string{exposePort},
			Privileged:  c.Docker.Privileged,
		},
	}

	// Add bind mount for files directory if specified (must be added BEFORE input mount)
	if c.BindFilesDir != "" && backendType == "docker" {
		// Mount the files directory as read-write so AGI can write output
		instancesCreate.Docker.Disks = append(instancesCreate.Docker.Disks,
			fmt.Sprintf("%s:/opt/agi/files:rw", c.BindFilesDir))
		logger.Info("Using bind mount for files directory: %s -> /opt/agi/files (read-write)", c.BindFilesDir)
	}

	// Add bind mount for local source if specified with bind: prefix
	if isBindMountSource(string(c.LocalSource)) && backendType == "docker" {
		bindPath := getBindMountPath(string(c.LocalSource))
		// Mount as read-only to protect the source logs
		instancesCreate.Docker.Disks = append(instancesCreate.Docker.Disks,
			fmt.Sprintf("%s:/opt/agi/files/input:ro", bindPath))
		logger.Info("Using bind mount for local source: %s -> /opt/agi/files/input (read-only)", bindPath)
	}

	// Add security group ID if specified
	if c.AWS.SecurityGroupID != "" {
		instancesCreate.AWS.Firewalls = append(instancesCreate.AWS.Firewalls, strings.Split(c.AWS.SecurityGroupID, ",")...)
	}
	if c.AWS.SubnetID != "" {
		instancesCreate.AWS.NetworkPlacement = c.AWS.SubnetID
	}

	// Create the instance with spot fallback support
	inst, err := instancesCreate.CreateInstances(system, inventory, nil, "create")
	if err != nil {
		// Check if spot fallback is enabled and this was a spot instance request
		isSpotRequest := (backendType == "aws" && c.AWS.SpotInstance) || (backendType == "gcp" && c.GCP.SpotInstance)
		spotFallbackEnabled := backendType == "aws" && c.AWS.SpotFallback // Only AWS supports spot fallback currently

		if isSpotRequest && spotFallbackEnabled {
			// Check if error is related to spot capacity
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "capacity") || strings.Contains(errStr, "spot") || strings.Contains(errStr, "insufficient") {
				logger.Warn("Spot instance creation failed, falling back to on-demand: %s", err)
				instancesCreate.AWS.SpotInstance = false
				inst, err = instancesCreate.CreateInstances(system, inventory, nil, "create")
			}
		}

		if err != nil {
			// If we created a volume and instance creation failed, clean it up
			if volumeCreated && volumeName != "" {
				logger.Warn("Instance creation failed, cleaning up created volume: %s", volumeName)
				cleanupErr := inventory.Volumes.WithName(volumeName).DeleteVolumes(inventory.Firewalls.Describe(), 10*time.Minute)
				if cleanupErr != nil {
					logger.Error("Failed to clean up volume %s: %s", volumeName, cleanupErr)
				}
			}
			return nil, err
		}
	}

	// Attach volume to instance if volume was used
	if volumeName != "" {
		logger.Info("Attaching volume %s to AGI instance", volumeName)
		volumes := inventory.Volumes.WithName(volumeName)
		if volumes.Count() == 0 {
			return nil, fmt.Errorf("volume %s not found for attachment", volumeName)
		}

		// Get the instance for attachment
		if inst.Count() == 0 {
			return nil, fmt.Errorf("no instance created for volume attachment")
		}
		instance := inst.Describe()[0]

		// AGI always mounts to /opt/agi
		mountPath := "/opt/agi"

		// Before mounting, move /opt/agi to /opt/agi.orig to preserve template files
		// These will be restored after the mount (only files that don't exist in the volume)
		logger.Debug("Moving /opt/agi to /opt/agi.orig before volume mount")
		inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"mv", "/opt/agi", "/opt/agi.orig"},
				SessionTimeout: time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})

		err = volumes.Attach(instance, &backends.VolumeAttachShared{
			MountTargetDirectory: mountPath,
			FIPS:                 volFips,
		}, 10*time.Minute)
		if err != nil {
			// If volume attachment fails, terminate the instance and optionally clean up volume
			logger.Error("Failed to attach volume, terminating instance: %s", err)
			termErr := inst.Terminate(10 * time.Minute)
			if termErr != nil {
				logger.Error("Failed to terminate instance after volume attachment failure: %s", termErr)
			}
			if volumeCreated {
				logger.Warn("Cleaning up created volume after attachment failure: %s", volumeName)
				cleanupErr := inventory.Volumes.WithName(volumeName).DeleteVolumes(inventory.Firewalls.Describe(), 10*time.Minute)
				if cleanupErr != nil {
					logger.Error("Failed to clean up volume %s: %s", volumeName, cleanupErr)
				}
			}
			return nil, fmt.Errorf("failed to attach volume %s: %w", volumeName, err)
		}

		// After successful mount, restore template files from /opt/agi.orig to /opt/agi
		// Using cp -rn to not overwrite existing files (preserves volume content on subsequent mounts)
		logger.Debug("Restoring template files from /opt/agi.orig to /opt/agi")
		outputs := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				// cp -rn: recursive, no-clobber (don't overwrite existing files)
				// This preserves existing volume content while adding any missing template files
				// The hidden files copy may fail if none exist, so we use || true for that part
				Command:        []string{"bash", "-c", "cp -rn /opt/agi.orig/* /opt/agi/ && (cp -rn /opt/agi.orig/.[!.]* /opt/agi/ 2>/dev/null || true) && rm -rf /opt/agi.orig"},
				SessionTimeout: 2 * time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if len(outputs) > 0 && outputs[0].Output.Err != nil && os.Getenv("AEROLAB_DEBUG") != "1" {
			// Failed to restore template files - AGI instance won't work properly
			logger.Error("Failed to restore template files from /opt/agi.orig: %s", outputs[0].Output.Err)
			logger.Error("Terminating instance due to template restore failure")
			termErr := inst.Terminate(10 * time.Minute)
			if termErr != nil {
				logger.Error("Failed to terminate instance: %s", termErr)
			}
			if volumeCreated {
				logger.Warn("Cleaning up created volume after restore failure: %s", volumeName)
				cleanupErr := inventory.Volumes.WithName(volumeName).DeleteVolumes(inventory.Firewalls.Describe(), 10*time.Minute)
				if cleanupErr != nil {
					logger.Error("Failed to clean up volume %s: %s", volumeName, cleanupErr)
				}
			}
			return nil, fmt.Errorf("failed to restore template files after volume mount: %w\n%s\n%s", outputs[0].Output.Err, string(outputs[0].Output.Stdout), string(outputs[0].Output.Stderr))
		} else if len(outputs) > 0 && outputs[0].Output.Err != nil {
			logger.Error("Failed to restore template files from /opt/agi.orig: %s", outputs[0].Output.Err)
			logger.Error("stdout: %s", string(outputs[0].Output.Stdout))
			logger.Error("stderr: %s", string(outputs[0].Output.Stderr))
			return nil, fmt.Errorf("not terminating instance as AEROLAB_DEBUG is set")
		}
	}

	// Update volume tags with resolved values and all parameters needed for reattach
	// This must happen AFTER instance creation so we have the resolved instance type, subnet, etc.
	if volumeName != "" && backendType != "docker" {
		c.updateVolumeTagsWithResolvedValues(system, inventory, logger, volumeName, backendType, awsInstanceType, gcpInstanceType, sourceStringLocal, sourceStringSftp, sourceStringS3, agiFirewallName, templateName, arch)
	}

	return inst, nil
}

// updateVolumeTagsWithResolvedValues updates the volume tags with all resolved values after instance creation.
// This ensures tags have the actual values used (not empty strings from unset parameters) and includes
// all parameters needed for the reattach flow.
func (c *AgiCreateCmd) updateVolumeTagsWithResolvedValues(system *System, inventory *backends.Inventory, logger *logger.Logger, volumeName string, backendType string, awsInstanceType string, gcpInstanceType string, sourceStringLocal string, sourceStringSftp string, sourceStringS3 string, agiFirewallName string, templateName string, arch backends.Architecture) {
	var vol backends.Volumes
	if backendType == "aws" {
		vol = inventory.Volumes.WithType(backends.VolumeTypeSharedDisk).WithName(volumeName)
	} else if backendType == "gcp" {
		vol = inventory.Volumes.WithType(backends.VolumeTypeAttachedDisk).WithName(volumeName)
	}
	if vol == nil || vol.Count() == 0 {
		logger.Warn("Could not find volume %s to update tags", volumeName)
		return
	}

	// Build comprehensive tag set with all values needed for reattach
	tags := map[string]string{
		// Core instance settings (now with resolved values)
		"aginodim":      fmt.Sprintf("%t", c.NoDIM),
		"aerolab7agiav": c.AerospikeVersion,

		// Template and architecture for consistent reattach
		"agitemplate": templateName,
		"agiarch":     arch.String(),

		// Source information (base64 encoded)
		"agisrclocal": sourceStringLocal,
		"agisrcs3":    sourceStringS3,
		"agisrcsftp":  sourceStringSftp,

		// Display/metadata
		"agilabel": c.AGILabel,

		// SSL settings
		"agissldisable": fmt.Sprintf("%t", c.ProxyDisableSSL),

		// Monitor settings
		"agimonitorurl":        c.MonitorUrl,
		"agimonitorcertignore": fmt.Sprintf("%t", c.MonitorCertIgnore),
	}

	// Add backend-specific tags
	if backendType == "aws" {
		tags["agiinstance"] = awsInstanceType // Resolved value, not input
		tags["termonpow"] = fmt.Sprintf("%t", c.AWS.TerminateOnPoweroff)
		tags["isspot"] = fmt.Sprintf("%t", c.AWS.SpotInstance)
		tags["agifips"] = fmt.Sprintf("%t", c.AWS.EFSFips)
		tags["agisubnet"] = c.AWS.SubnetID
		tags["agisecgroup"] = c.AWS.SecurityGroupID
		tags["agiebs"] = c.AWS.Ebs
		tags["agiexpire"] = c.AWS.Expires.String()
		tags["agidisablepubip"] = fmt.Sprintf("%t", c.AWS.DisablePublicIP)
		tags["agispotfallback"] = fmt.Sprintf("%t", c.AWS.SpotFallback)
		tags["agiefspath"] = c.AWS.EFSPath
		tags["agifirewall"] = agiFirewallName

		// Route53 settings
		if c.AWS.Route53ZoneId != "" {
			tags["agiZoneID"] = c.AWS.Route53ZoneId
		}
		if c.AWS.Route53DomainName != "" {
			tags["agiDomain"] = c.AWS.Route53DomainName
		}
	} else if backendType == "gcp" {
		tags["agiinstance"] = gcpInstanceType // Resolved value, not input
		tags["termonpow"] = fmt.Sprintf("%t", c.GCP.TerminateOnPoweroff)
		tags["isspot"] = fmt.Sprintf("%t", c.GCP.SpotInstance)
		tags["agifips"] = fmt.Sprintf("%t", c.GCP.VolFips)
		tags["agizone"] = c.GCP.Zone
		tags["agiexpire"] = c.GCP.Expires.String()
		tags["agifirewall"] = agiFirewallName

		// GCP disk configuration (join array to string)
		if len(c.GCP.Disks) > 0 {
			tags["agidisks"] = strings.Join(c.GCP.Disks, ";")
		}
	}

	if err := vol.AddTags(tags, 5*time.Minute); err != nil {
		logger.Warn("Failed to update volume tags with resolved values: %s", err)
	} else {
		logger.Debug("Updated volume tags with resolved instance settings")
	}
}

// getAvailableMemory gets the available memory on the instance.
func (c *AgiCreateCmd) getAvailableMemory(instance backends.InstanceList, backendType string) (int64, error) {
	var lastErr error
	var lastStdout, lastStderr string

	// Retry up to 10 times with 3 second delay to handle instance startup delay
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			time.Sleep(3 * time.Second)
		}

		outputs := instance.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"free", "-b"},
				SessionTimeout: time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})

		if len(outputs) == 0 {
			lastErr = fmt.Errorf("no output from exec")
			continue
		}
		if outputs[0].Output.Err != nil {
			lastErr = outputs[0].Output.Err
			lastStdout = string(outputs[0].Output.Stdout)
			lastStderr = string(outputs[0].Output.Stderr)
			continue
		}

		stdout := string(outputs[0].Output.Stdout)
		lines := strings.Split(stdout, "\n")
		if len(lines) < 2 || !strings.HasPrefix(lines[1], "Mem:") {
			lastErr = fmt.Errorf("malformed output")
			lastStdout = stdout
			lastStderr = string(outputs[0].Output.Stderr)
			continue
		}

		// Success - parse the memory value
		fields := strings.Fields(lines[1])
		if len(fields) < 2 {
			return 0, fmt.Errorf("malformed memory line: %q", lines[1])
		}

		totalMem, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse memory: %w", err)
		}

		// Reserve memory: 6GB for cloud, 3GB for docker
		var reserved int64
		if backendType == "docker" {
			reserved = 3 * 1024 * 1024 * 1024
		} else {
			reserved = 6 * 1024 * 1024 * 1024
		}

		memSize := totalMem - reserved
		if memSize < 1024*1024*1024 {
			return 0, fmt.Errorf("not enough RAM (min 1GB after reservation)")
		}

		return memSize, nil
	}

	// All retries failed
	return 0, fmt.Errorf("failed to get memory info after 10 attempts: %w (stdout=%q stderr=%q)", lastErr, lastStdout, lastStderr)
}

// generateConfigs generates all AGI configuration files.
func (c *AgiCreateCmd) generateConfigs(system *System, memSize int64, backendType string) (map[string][]byte, error) {
	configs := make(map[string][]byte)

	// Generate ingest.yaml
	ingestConfig, err := c.generateIngestConfig(backendType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ingest config: %w", err)
	}
	configs["/opt/agi/ingest.yaml"] = ingestConfig

	// Generate plugin.yaml
	pluginConfig := c.generatePluginConfig(backendType)
	configs["/opt/agi/plugin.yaml"] = pluginConfig

	// Generate grafanafix.yaml
	grafanafixConfig := c.generateGrafanafixConfig()
	configs["/opt/agi/grafanafix.yaml"] = grafanafixConfig

	// Generate notifier.yaml
	notifierConfig := c.generateNotifierConfig()
	configs["/opt/agi/notifier.yaml"] = notifierConfig

	// Generate proxy.yaml
	proxyConfig := c.generateProxyConfig(backendType)
	configs["/opt/agi/proxy.yaml"] = proxyConfig

	// Generate label and name files
	configs["/opt/agi/label"] = []byte(c.AGILabel)
	configs["/opt/agi/name"] = []byte(string(c.ClusterName))
	configs["/opt/agi/owner"] = []byte(c.Owner)

	// Generate deployment.json.gz
	deploymentConfig, err := c.generateDeploymentConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to generate deployment config: %w", err)
	}
	configs["/opt/agi/deployment.json.gz"] = deploymentConfig

	// Create nodim marker if needed
	if c.NoDIM {
		configs["/opt/agi/nodim"] = []byte("")
	}

	return configs, nil
}

// generateIngestConfig generates the ingest.yaml configuration.
func (c *AgiCreateCmd) generateIngestConfig(backendType string) ([]byte, error) {
	config, err := ingest.MakeConfigReader(true, nil, false)
	if err != nil {
		return nil, err
	}

	// Configure based on backend
	config.Aerospike.MaxPutThreads = 128
	if backendType == "docker" {
		config.Aerospike.MaxPutThreads = 64
	}
	config.Aerospike.WaitForSindexes = true

	// Pre-processor settings
	config.PreProcess.FileThreads = 6
	config.PreProcess.UnpackerFileThreads = 4

	// Processor settings
	config.Processor.MaxConcurrentLogFiles = 6

	// Progress file settings
	config.ProgressFile.DisableWrite = false
	config.ProgressFile.Compress = true
	config.ProgressFile.WriteInterval = 10 * time.Second
	config.ProgressFile.OutputFilePath = "/opt/agi/ingest"

	// Progress print settings
	config.ProgressPrint.Enable = true
	config.ProgressPrint.PrintDetailProgress = true
	config.ProgressPrint.PrintOverallProgress = true
	config.ProgressPrint.UpdateInterval = 10 * time.Second

	// Custom patterns file
	if c.PatternsFile != "" {
		config.PatternsFile = "/opt/agi/patterns.yaml"
	}

	// Directory configuration
	config.Directories.CollectInfo = "/opt/agi/files/collectinfo"
	config.Directories.DirtyTmp = "/opt/agi/files/input"
	config.Directories.Logs = "/opt/agi/files/logs"
	config.Directories.NoStatLogs = "/opt/agi/files/no-stat"
	config.Directories.OtherFiles = "/opt/agi/files/other"

	// Enable read-only input mode when using bind mount
	if isBindMountSource(string(c.LocalSource)) {
		config.Directories.ReadOnlyInput = true
	}

	// Log level
	config.LogLevel = c.IngestLogLevel

	// CPU profiling
	if c.IngestCpuProfile {
		config.CPUProfilingOutputFile = "/opt/agi/cpu.ingest.pprof"
	}

	// Custom source name
	config.CustomSourceName = c.CustomSourceName

	// Time ranges
	if c.TimeRanges {
		config.IngestTimeRanges.Enabled = true
		from, err := time.Parse("2006-01-02T15:04:05Z07:00", c.TimeRangesFrom)
		if err != nil {
			from, err = time.Parse("2006/01/02 15:04:05", c.TimeRangesFrom)
			if err != nil {
				return nil, fmt.Errorf("invalid time range from: %w", err)
			}
		}
		to, err := time.Parse("2006-01-02T15:04:05Z07:00", c.TimeRangesTo)
		if err != nil {
			to, err = time.Parse("2006/01/02 15:04:05", c.TimeRangesTo)
			if err != nil {
				return nil, fmt.Errorf("invalid time range to: %w", err)
			}
		}
		config.IngestTimeRanges.From = from
		config.IngestTimeRanges.To = to
	}

	// SFTP source
	if c.SftpEnable {
		config.Downloader.SftpSource = &ingest.SftpSource{
			Enabled:     true,
			Threads:     c.SftpThreads,
			Host:        c.SftpHost,
			Port:        c.SftpPort,
			Username:    c.SftpUser,
			Password:    c.SftpPass,
			PathPrefix:  c.SftpPath,
			SearchRegex: c.SftpRegex,
		}
		if c.SftpKey != "" {
			config.Downloader.SftpSource.KeyFile = "/opt/agi/sftp.key"
		}
	}

	// S3 source
	if c.S3Enable {
		config.Downloader.S3Source = &ingest.S3Source{
			Enabled:     true,
			Threads:     c.S3Threads,
			Region:      c.S3Region,
			BucketName:  c.S3Bucket,
			KeyID:       c.S3KeyID,
			SecretKey:   c.S3Secret,
			PathPrefix:  c.S3Path,
			SearchRegex: c.S3Regex,
			Endpoint:    c.S3Endpoint,
		}
	}

	// Marshal to YAML
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(config); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// generatePluginConfig generates the plugin.yaml configuration.
func (c *AgiCreateCmd) generatePluginConfig(backendType string) []byte {
	maxDp := 34560000
	if backendType == "docker" {
		maxDp = maxDp / 2
	}

	cpuProfiling := ""
	if c.PluginCpuProfile {
		cpuProfiling = "cpuProfilingOutputFile: \"/opt/agi/cpu.plugin.pprof\"\n"
	}

	config := fmt.Sprintf(`maxDataPointsReceived: %d
logLevel: %d
addNoneToLabels:
  - Histogram
  - HistogramDev
  - HistogramUs
  - HistogramSize
  - HistogramCount
%s`, maxDp, c.PluginLogLevel, cpuProfiling)

	return []byte(config)
}

// generateGrafanafixConfig generates the grafanafix.yaml configuration.
func (c *AgiCreateCmd) generateGrafanafixConfig() []byte {
	config := `dashboards:
  fromDir: ""
  loadEmbedded: true
grafanaURL: "http://127.0.0.1:8850"
annotationFile: "/opt/agi/annotations.json"
labelFiles:
  - "/opt/agi/label"
  - "/opt/agi/name"
`
	return []byte(config)
}

// generateNotifierConfig generates the notifier.yaml configuration.
// Returns the YAML-encoded configuration or nil if marshaling fails.
func (c *AgiCreateCmd) generateNotifierConfig() []byte {
	notifier := map[string]interface{}{
		"agiMonitor":           c.MonitorUrl,
		"agiMonitorCertIgnore": c.MonitorCertIgnore,
		"slackToken":           c.SlackToken,
		"slackChannel":         c.SlackChannel,
	}

	data, err := yaml.Marshal(notifier)
	if err != nil {
		// This should never happen with a simple map[string]interface{}
		// but log it if it does for debugging purposes
		return nil
	}
	return data
}

// generateProxyConfig generates the proxy.yaml configuration.
func (c *AgiCreateCmd) generateProxyConfig(backendType string) []byte {
	// Determine port and TLS settings
	listenPort := 443
	https := true
	certFile := "/opt/agi/proxy.cert"
	keyFile := "/opt/agi/proxy.key"
	shutdownCmd := "/usr/bin/systemctl stop aerospike; /usr/bin/sync; /sbin/poweroff -p || /sbin/poweroff"

	if c.ProxyDisableSSL {
		listenPort = 80
		https = false
		certFile = ""
		keyFile = ""
	}

	if backendType == "docker" {
		// Docker doesn't need poweroff
		shutdownCmd = "/usr/bin/systemctl stop aerospike; /usr/bin/sync"
	}

	proxyConfig := map[string]interface{}{
		"agiName":         string(c.ClusterName),
		"label":           c.AGILabel,
		"listenPort":      listenPort,
		"https":           https,
		"certFile":        certFile,
		"keyFile":         keyFile,
		"maxInactivity":   c.ProxyMaxInactive.String(),
		"maxUptime":       c.ProxyMaxUptime.String(),
		"authType":        "token",
		"tokenPath":       "/opt/agi/tokens",
		"shutdownCommand": shutdownCmd,
	}

	data, err := yaml.Marshal(proxyConfig)
	if err != nil {
		return nil
	}
	return data
}

// generateDeploymentConfig generates the deployment.json.gz metadata.
func (c *AgiCreateCmd) generateDeploymentConfig() ([]byte, error) {
	// Create a copy of the command with secrets and local paths removed
	deployCmd := *c
	deployCmd.SftpPass = ""
	deployCmd.S3Secret = ""
	deployCmd.SlackToken = ""
	// Clear AerolabBinary - the template already has aerolab installed and
	// this local path won't exist on the monitor when recreating for sizing
	deployCmd.AerolabBinary = ""

	jsonData, err := json.Marshal(deployCmd)
	if err != nil {
		return nil, err
	}

	// Compress with gzip
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(jsonData); err != nil {
		gz.Close()
		return nil, err
	}
	gz.Close()

	return buf.Bytes(), nil
}

// uploadAerolabBinary uploads a local aerolab binary to the AGI instance.
// This overrides the aerolab binary that was installed during template creation.
func (c *AgiCreateCmd) uploadAerolabBinary(instance backends.InstanceList, logger *logger.Logger) error {
	// Read the local binary
	binaryData, err := os.ReadFile(string(c.AerolabBinary))
	if err != nil {
		return fmt.Errorf("could not read local aerolab binary: %w", err)
	}

	confs, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("could not get SFTP config: %w", err)
	}

	for _, conf := range confs {
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("could not create SFTP client: %w", err)
		}

		// Remove existing binary first (ignore errors if it doesn't exist)
		_ = cli.RawClient().Remove("/usr/local/bin/aerolab")

		// Upload the binary
		err = cli.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/usr/local/bin/aerolab",
			Source:      bytes.NewReader(binaryData),
			Permissions: 0755,
		})
		cli.Close()
		if err != nil {
			return fmt.Errorf("could not upload aerolab binary: %w", err)
		}

		logger.Debug("Uploaded aerolab binary to %s", conf.Host)
	}

	return nil
}

// uploadConfigs uploads configuration files to the AGI instance.
func (c *AgiCreateCmd) uploadConfigs(instance backends.InstanceList, configs map[string][]byte, logger *logger.Logger) error {
	confs, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("could not get SFTP config: %w", err)
	}

	for _, conf := range confs {
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("could not create SFTP client: %w", err)
		}
		defer cli.Close()

		// Create directories - including aerospike data/smd which are needed when EFS/volume is mounted
		dirs := []string{
			"/opt/agi/files/input",
			"/opt/agi/files/input/s3source",
			"/opt/agi/files/input/sftpsource",
			"/opt/agi/files/logs",
			"/opt/agi/files/collectinfo",
			"/opt/agi/files/other",
			"/opt/agi/files/no-stat",
			"/opt/agi/ingest",
			"/opt/agi/tokens",
			"/opt/agi/aerospike/data",
			"/opt/agi/aerospike/smd",
		}
		for _, dir := range dirs {
			_ = cli.RawClient().MkdirAll(dir)
		}

		// Upload each config file
		for path, content := range configs {
			// Check if we should skip due to NoConfigOverride
			if c.NoConfigOverride {
				if cli.IsExists(path) {
					logger.Debug("Skipping existing config: %s", path)
					continue
				}
			}

			perm := os.FileMode(0644)
			if strings.HasSuffix(path, ".key") {
				perm = 0600
			}

			err := cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    path,
				Source:      bytes.NewReader(content),
				Permissions: perm,
			})
			if err != nil {
				return fmt.Errorf("failed to upload %s: %w", path, err)
			}
		}

		// Upload SSL certificates if provided, or generate self-signed ones if missing
		if c.ProxyCert != "" {
			certData, err := os.ReadFile(string(c.ProxyCert))
			if err != nil {
				return fmt.Errorf("failed to read SSL cert: %w", err)
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/opt/agi/proxy.cert",
				Source:      bytes.NewReader(certData),
				Permissions: 0644,
			})
			if err != nil {
				return fmt.Errorf("failed to upload SSL cert: %w", err)
			}
		}
		if c.ProxyKey != "" {
			keyData, err := os.ReadFile(string(c.ProxyKey))
			if err != nil {
				return fmt.Errorf("failed to read SSL key: %w", err)
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/opt/agi/proxy.key",
				Source:      bytes.NewReader(keyData),
				Permissions: 0600,
			})
			if err != nil {
				return fmt.Errorf("failed to upload SSL key: %w", err)
			}
		}

		// Generate self-signed SSL certificates if they don't exist and SSL is enabled
		// This handles the case where EFS/volume mount overwrites template content
		if !c.ProxyDisableSSL && c.ProxyCert == "" {
			if !cli.IsExists("/opt/agi/proxy.cert") || !cli.IsExists("/opt/agi/proxy.key") {
				logger.Debug("Generating self-signed SSL certificates as fallback")
				instance.Exec(&backends.ExecInput{
					ExecDetail: sshexec.ExecDetail{
						Command:        []string{"bash", "-c", `openssl req -x509 -nodes -days 3650 -newkey rsa:2048 -keyout /opt/agi/proxy.key -out /opt/agi/proxy.cert -subj "/C=US/ST=California/L=San Jose/O=Aerospike/OU=AeroLab/CN=agi.aerolab.local" && chmod 644 /opt/agi/proxy.cert && chmod 600 /opt/agi/proxy.key`},
						SessionTimeout: time.Minute,
					},
					Username:        "root",
					ConnectTimeout:  30 * time.Second,
					ParallelThreads: 1,
				})
			}
		}

		// Upload SFTP key if provided
		if c.SftpKey != "" {
			keyData, err := os.ReadFile(string(c.SftpKey))
			if err != nil {
				return fmt.Errorf("failed to read SFTP key: %w", err)
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/opt/agi/sftp.key",
				Source:      bytes.NewReader(keyData),
				Permissions: 0600,
			})
			if err != nil {
				return fmt.Errorf("failed to upload SFTP key: %w", err)
			}
		}

		// Upload patterns file if provided
		if c.PatternsFile != "" {
			patternData, err := os.ReadFile(string(c.PatternsFile))
			if err != nil {
				return fmt.Errorf("failed to read patterns file: %w", err)
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/opt/agi/patterns.yaml",
				Source:      bytes.NewReader(patternData),
				Permissions: 0644,
			})
			if err != nil {
				return fmt.Errorf("failed to upload patterns file: %w", err)
			}
		}
	}

	return nil
}

// uploadLocalSource uploads local source files to the AGI instance.
// If the source uses bind mount (bind:/path), this function skips the upload
// since the files are already available via the bind mount.
func (c *AgiCreateCmd) uploadLocalSource(instance backends.InstanceList, logger *logger.Logger) error {
	// Skip upload if using bind mount - files are already available via mount
	if isBindMountSource(string(c.LocalSource)) {
		logger.Info("Skipping file upload - using bind mount for local source")
		return nil
	}

	confs, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("could not get SFTP config: %w", err)
	}

	sourcePath := string(c.LocalSource)
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("could not stat local source: %w", err)
	}

	for _, conf := range confs {
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("could not create SFTP client: %w", err)
		}
		defer cli.Close()

		if stat.IsDir() {
			// Upload directory contents by using the files upload command
			// For now, we'll use exec to handle directory uploads via tar
			outputs := instance.Exec(&backends.ExecInput{
				ExecDetail: sshexec.ExecDetail{
					Command:        []string{"mkdir", "-p", "/opt/agi/files/input"},
					SessionTimeout: time.Minute,
				},
				Username:        "root",
				ConnectTimeout:  30 * time.Second,
				ParallelThreads: 1,
			})
			if len(outputs) > 0 && outputs[0].Output.Err != nil {
				return fmt.Errorf("failed to create input directory: %w", outputs[0].Output.Err)
			}

			// Walk the directory and upload each file
			err = uploadDirectory(cli, sourcePath, "/opt/agi/files/input/")
			if err != nil {
				return fmt.Errorf("failed to upload directory: %w", err)
			}
		} else {
			// Upload single file
			f, err := os.Open(sourcePath)
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			defer f.Close()

			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/opt/agi/files/input/" + stat.Name(),
				Source:      f,
				Permissions: 0644,
			})
			if err != nil {
				return fmt.Errorf("failed to upload file: %w", err)
			}
		}
	}

	return nil
}

// configureAerospike generates and uploads the aerospike.conf file.
func (c *AgiCreateCmd) configureAerospike(instance backends.InstanceList, memSize int64, backendType string, logger *logger.Logger) error {
	// Calculate storage parameters
	memSizeGB := memSize / (1024 * 1024 * 1024)
	var memSizeStr, storEngine, dataSizeStr, rpcStr, wbs, maxWriteCache string
	var fileSizeInt int64

	// Parse Aerospike version to determine config format
	// Default to version 8 (latest) since "latest" resolves to newest version
	// and newer versions don't support memory-size
	majorVersion := 8
	versionParts := strings.Split(c.AerospikeVersion, ".")
	if len(versionParts) > 0 {
		if v, err := strconv.Atoi(versionParts[0]); err == nil {
			majorVersion = v
		}
	}
	logger.Info("Aerospike version: %s (major: %d)", c.AerospikeVersion, majorVersion)

	if c.NoDIM {
		// No data-in-memory mode - use storage-engine device
		storEngine = "device"
		if c.NoDIMFileSize != 0 {
			fileSizeInt = int64(c.NoDIMFileSize)
		} else {
			fileSizeInt = 2000
		}
		rpcStr = "read-page-cache true"
		if majorVersion < 7 || (majorVersion == 7 && len(versionParts) > 1 && versionParts[1] == "0") {
			wbs = "write-block-size 8M"
		}
	} else {
		// Data-in-memory mode
		if majorVersion >= 7 {
			// Aerospike 7+: use storage-engine memory with NO memory-size or data-size
			// Memory is managed automatically
			storEngine = "memory"
			fileSizeInt = int64(float64(memSizeGB) / 1.25)
		} else {
			// Aerospike < 7: use storage-engine device with memory-size and data-in-memory
			storEngine = "device"
			memSizeStr = fmt.Sprintf("memory-size %dG", memSizeGB)
			dataSizeStr = fmt.Sprintf("data-in-memory %t", true)
			fileSizeInt = memSizeGB
			wbs = "write-block-size 8M"
		}
	}

	if backendType != "docker" {
		maxWriteCache = "max-write-cache 1024M"
	}

	// Get SFTP config first - we need to check for features file before building the config
	confs, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("could not get SFTP config: %w", err)
	}

	for _, conf := range confs {
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("could not create SFTP client: %w", err)
		}
		defer cli.Close()

		// Ensure features file exists before building config
		// This handles the case where EFS/volume mount overwrites template content
		hasFeatureFile := false

		// Ensure /opt/agi/aerospike/ directory exists
		_ = cli.RawClient().MkdirAll("/opt/agi/aerospike")
		_ = cli.RawClient().MkdirAll("/opt/agi/aerospike/data")
		_ = cli.RawClient().MkdirAll("/opt/agi/aerospike/smd")

		// If user provided a features file, upload it
		if c.FeaturesFilePath != "" {
			featuresContent, err := os.ReadFile(string(c.FeaturesFilePath))
			if err != nil {
				return fmt.Errorf("could not read features file %s: %w", c.FeaturesFilePath, err)
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/opt/agi/aerospike/features.conf",
				Source:      bytes.NewReader(featuresContent),
				Permissions: 0644,
			})
			if err != nil {
				return fmt.Errorf("could not upload features file: %w", err)
			}
			hasFeatureFile = true
			logger.Info("Uploaded custom features file from %s", c.FeaturesFilePath)
		} else if cli.IsExists("/opt/agi/aerospike/features.conf") {
			// Check if features.conf already exists in /opt/agi/aerospike/
			hasFeatureFile = true
		} else {
			// Try to copy from /etc/aerospike/
			outputs := instance.Exec(&backends.ExecInput{
				ExecDetail: sshexec.ExecDetail{
					Command:        []string{"cp", "-n", "/etc/aerospike/features.conf", "/opt/agi/aerospike/features.conf"},
					SessionTimeout: time.Minute,
				},
				Username:        "root",
				ConnectTimeout:  30 * time.Second,
				ParallelThreads: 1,
			})

			// Check if copy succeeded
			copyFailed := false
			if len(outputs) > 0 && outputs[0].Output.Err != nil {
				copyFailed = true
			}

			// Re-check if file exists now
			if !copyFailed && cli.IsExists("/opt/agi/aerospike/features.conf") {
				hasFeatureFile = true
			} else {
				logger.Warn("No features file found at /opt/agi/aerospike/features.conf or /etc/aerospike/features.conf")
				logger.Warn("Aerospike will start without a feature-key-file directive.")
				logger.Warn("  - Aerospike Community Edition 6.1+ works without a features file")
				logger.Warn("  - Aerospike Enterprise requires a valid features file to start")
			}
		}

		// Build aerospike.conf with conditional feature-key-file directive
		var aerospikeConf bytes.Buffer
		aerospikeConf.WriteString("service {\n")
		aerospikeConf.WriteString("    proto-fd-max 15000\n")
		aerospikeConf.WriteString("    work-directory /opt/agi/aerospike\n")
		aerospikeConf.WriteString("    cluster-name agi\n")
		if hasFeatureFile {
			aerospikeConf.WriteString("    feature-key-file /opt/agi/aerospike/features.conf\n")
		}
		aerospikeConf.WriteString("}\n\n")

		aerospikeConf.WriteString("logging {\n")
		aerospikeConf.WriteString("    file /var/log/agi-aerospike.log {\n")
		aerospikeConf.WriteString("        context any info\n")
		aerospikeConf.WriteString("    }\n")
		aerospikeConf.WriteString("}\n\n")

		aerospikeConf.WriteString("network {\n")
		aerospikeConf.WriteString("    service {\n")
		aerospikeConf.WriteString("        address any\n")
		aerospikeConf.WriteString("        port 3000\n")
		aerospikeConf.WriteString("    }\n")
		aerospikeConf.WriteString("    heartbeat {\n")
		aerospikeConf.WriteString("        interval 150\n")
		aerospikeConf.WriteString("        mode mesh\n")
		aerospikeConf.WriteString("        port 3002\n")
		aerospikeConf.WriteString("        timeout 10\n")
		aerospikeConf.WriteString("    }\n")
		aerospikeConf.WriteString("    fabric {\n")
		aerospikeConf.WriteString("        port 3001\n")
		aerospikeConf.WriteString("    }\n")
		aerospikeConf.WriteString("    info {\n")
		aerospikeConf.WriteString("        port 3003\n")
		aerospikeConf.WriteString("    }\n")
		aerospikeConf.WriteString("}\n\n")

		aerospikeConf.WriteString("namespace agi {\n")
		aerospikeConf.WriteString("    default-ttl 0\n")
		if memSizeStr != "" {
			// Aerospike < 7: memory-size at namespace level
			aerospikeConf.WriteString(fmt.Sprintf("    %s\n", memSizeStr))
		}
		aerospikeConf.WriteString("    replication-factor 2\n")
		aerospikeConf.WriteString(fmt.Sprintf("    storage-engine %s {\n", storEngine))
		aerospikeConf.WriteString("        file /opt/agi/aerospike/data/agi.dat\n")
		aerospikeConf.WriteString(fmt.Sprintf("        filesize %dG\n", fileSizeInt))
		if dataSizeStr != "" {
			// Aerospike < 7: data-in-memory inside storage-engine device
			aerospikeConf.WriteString(fmt.Sprintf("        %s\n", dataSizeStr))
		}
		if rpcStr != "" {
			aerospikeConf.WriteString(fmt.Sprintf("        %s\n", rpcStr))
		}
		if wbs != "" {
			aerospikeConf.WriteString(fmt.Sprintf("        %s\n", wbs))
		}
		if maxWriteCache != "" {
			aerospikeConf.WriteString(fmt.Sprintf("        %s\n", maxWriteCache))
		}
		aerospikeConf.WriteString("    }\n")
		aerospikeConf.WriteString("}\n")

		// Upload aerospike.conf to /opt/agi so it can be persisted on EFS
		err = cli.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/opt/agi/aerospike.conf",
			Source:      bytes.NewReader(aerospikeConf.Bytes()),
			Permissions: 0644,
		})
		if err != nil {
			return fmt.Errorf("failed to upload aerospike.conf: %w", err)
		}
	}

	return nil
}

// startServices starts all AGI services.
func (c *AgiCreateCmd) startServices(instance backends.InstanceList, logger *logger.Logger) error {
	logger.Debug("Enabling and starting all AGI services")

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

	outputs := instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", script},
			SessionTimeout: 5 * time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	var errs []string
	for _, o := range outputs {
		if o.Output.Err != nil {
			errs = append(errs, fmt.Sprintf("%v (stderr: %s)", o.Output.Err, o.Output.Stderr))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to start services: %s", strings.Join(errs, "; "))
	}

	return nil
}

// getAccessURL returns the access URL for the AGI instance.
func (c *AgiCreateCmd) getAccessURL(instance backends.InstanceList, backendType string) string {
	instances := instance.Describe()
	if len(instances) == 0 {
		return "unknown"
	}

	ip := instances[0].IP.Public
	if ip == "" {
		ip = instances[0].IP.Private
	}
	if ip == "" {
		ip = instances[0].Name
	}

	protocol := "https"
	containerPort := "443"
	if c.ProxyDisableSSL {
		protocol = "http"
		containerPort = "80"
	}

	if backendType == "docker" {
		// Extract actual host port from firewall rules
		hostPort := 0
		for _, fw := range instances[0].Firewalls {
			// Format: host=0.0.0.0:9443,container=443
			parts := strings.Split(fw, ",")
			if len(parts) != 2 {
				continue
			}
			var hp, cp string
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "host=") {
					hostPart := strings.TrimPrefix(part, "host=")
					if colonIdx := strings.LastIndex(hostPart, ":"); colonIdx >= 0 {
						hp = hostPart[colonIdx+1:]
					}
				} else if strings.HasPrefix(part, "container=") {
					cp = strings.TrimPrefix(part, "container=")
				}
			}
			if cp == containerPort && hp != "" {
				if p, err := strconv.Atoi(hp); err == nil {
					hostPort = p
					break
				}
			}
		}
		if hostPort > 0 {
			return fmt.Sprintf("%s://localhost:%d", protocol, hostPort)
		}
		return fmt.Sprintf("%s://localhost", protocol)
	}

	return fmt.Sprintf("%s://%s", protocol, ip)
}

// getLogsFromCluster retrieves logs from a source cluster.
func (c *AgiCreateCmd) getLogsFromCluster(system *System, inventory *backends.Inventory, logger *logger.Logger) (string, error) {
	clusterName := string(c.ClusterSource)

	// Get cluster instances
	cluster := inventory.Instances.WithClusterName(clusterName).WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		return "", fmt.Errorf("cluster %s not found or has no running instances", clusterName)
	}

	// Create temp directory for logs
	tmpDir, err := os.MkdirTemp("", "aerolab-logs-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Download logs from each node
	logger.Info("Downloading logs from cluster %s (%d nodes)", clusterName, cluster.Count())

	parallelize.ForEachLimit(cluster.Describe(), 4, func(inst *backends.Instance) {
		// Get log location
		logLocation := c.getLogLocation(inst)
		if logLocation == "" {
			logger.Warn("Could not determine log location for node %d", inst.NodeNo)
			return
		}

		nodeDir := fmt.Sprintf("%s/node-%d", tmpDir, inst.NodeNo)
		os.MkdirAll(nodeDir, 0755)

		confs, err := backends.InstanceList{inst}.GetSftpConfig("root")
		if err != nil {
			logger.Warn("Could not get SFTP config for node %d: %s", inst.NodeNo, err)
			return
		}

		for _, conf := range confs {
			cli, err := sshexec.NewSftp(conf)
			if err != nil {
				logger.Warn("Could not create SFTP client for node %d: %s", inst.NodeNo, err)
				return
			}
			defer cli.Close()

			if logLocation == "JOURNALCTL" {
				// Get logs from journalctl
				outputs := backends.InstanceList{inst}.Exec(&backends.ExecInput{
					ExecDetail: sshexec.ExecDetail{
						Command:        []string{"journalctl", "-u", "aerospike", "--no-pager"},
						SessionTimeout: 5 * time.Minute,
					},
					Username:        "root",
					ConnectTimeout:  30 * time.Second,
					ParallelThreads: 1,
				})
				if len(outputs) > 0 && outputs[0].Output.Err == nil {
					os.WriteFile(fmt.Sprintf("%s/aerospike.log", nodeDir), outputs[0].Output.Stdout, 0644)
				}
			} else {
				// Download log file
				destFile, err := os.Create(fmt.Sprintf("%s/aerospike.log", nodeDir))
				if err != nil {
					logger.Warn("Could not create log file for node %d: %s", inst.NodeNo, err)
					return
				}
				defer destFile.Close()

				err = cli.ReadFile(&sshexec.FileReader{
					SourcePath:  logLocation,
					Destination: destFile,
				})
				if err != nil {
					logger.Warn("Could not download log from node %d: %s", inst.NodeNo, err)
				}
			}
		}
	})

	return tmpDir, nil
}

// getLogLocation determines the log file location for an Aerospike instance.
func (c *AgiCreateCmd) getLogLocation(inst *backends.Instance) string {
	outputs := backends.InstanceList{inst}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"cat", "/etc/aerospike/aerospike.conf"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if len(outputs) == 0 || outputs[0].Output.Err != nil {
		return ""
	}

	confContent := string(outputs[0].Output.Stdout)

	// Check for console logging (journalctl)
	if strings.Contains(confContent, "console") && strings.Contains(confContent, "logging") {
		return "JOURNALCTL"
	}

	// Look for file logging
	lines := strings.Split(confContent, "\n")
	inLogging := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "logging") {
			inLogging = true
			continue
		}
		if inLogging && strings.HasPrefix(line, "file ") {
			// Extract file path
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return strings.Trim(parts[1], "{}")
			}
		}
		if inLogging && line == "}" {
			break
		}
	}

	return "JOURNALCTL"
}

// uploadDirectory recursively uploads a directory to the remote server.
func uploadDirectory(cli *sshexec.Sftp, localPath, remotePath string) error {
	return uploadDirectoryRecursive(cli, localPath, remotePath, "")
}

func uploadDirectoryRecursive(cli *sshexec.Sftp, localBase, remoteBase, subPath string) error {
	localPath := localBase
	if subPath != "" {
		localPath = localBase + "/" + subPath
	}

	entries, err := os.ReadDir(localPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", localPath, err)
	}

	for _, entry := range entries {
		entryPath := entry.Name()
		if subPath != "" {
			entryPath = subPath + "/" + entry.Name()
		}

		localEntryPath := localBase + "/" + entryPath
		remoteEntryPath := remoteBase + entryPath

		if entry.IsDir() {
			// Create remote directory and recurse
			_ = cli.RawClient().MkdirAll(remoteEntryPath)
			err := uploadDirectoryRecursive(cli, localBase, remoteBase, entryPath)
			if err != nil {
				return err
			}
		} else {
			// Upload file
			f, err := os.Open(localEntryPath)
			if err != nil {
				return fmt.Errorf("failed to open %s: %w", localEntryPath, err)
			}

			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    remoteEntryPath,
				Source:      f,
				Permissions: 0644,
			})
			f.Close()
			if err != nil {
				return fmt.Errorf("failed to upload %s: %w", localEntryPath, err)
			}
		}
	}

	return nil
}

// getRoute53DomainInfo queries Route53 to get the hosted zone's domain name
// and computes the DNS record name (subdomain) from the FQDN.
//
// Returns:
//   - dnsName: The subdomain part (e.g., "myagi" from "myagi.example.com")
//   - domainName: The zone's domain name (e.g., "example.com")
//   - error: nil on success, or an error describing what failed
func (c *AgiCreateCmd) getRoute53DomainInfo(system *System, logger *logger.Logger) (dnsName string, domainName string, err error) {
	// Get a region for the Route53 client
	regions, err := system.Backend.ListEnabledRegions(backends.BackendTypeAWS)
	if err != nil || len(regions) == 0 {
		return "", "", fmt.Errorf("failed to get AWS region: %w", err)
	}
	region := regions[0]

	// Get Route53 client
	cli, err := baws.GetRoute53Client(system.Backend.GetCredentials(), &region)
	if err != nil {
		return "", "", fmt.Errorf("failed to get Route53 client: %w", err)
	}

	// Get the hosted zone to determine its domain name
	hostedZone, err := cli.GetHostedZone(context.Background(), &route53.GetHostedZoneInput{
		Id: aws.String(c.AWS.Route53ZoneId),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to get hosted zone: %w", err)
	}

	// The hosted zone name includes a trailing dot, remove it
	domainName = strings.TrimSuffix(aws.ToString(hostedZone.HostedZone.Name), ".")

	// Compute the subdomain by removing the domain from the FQDN
	fqdn := strings.TrimSuffix(c.AWS.Route53DomainName, ".")
	if strings.HasSuffix(fqdn, "."+domainName) {
		dnsName = strings.TrimSuffix(fqdn, "."+domainName)
	} else if fqdn == domainName {
		// The FQDN is the domain itself (apex record)
		dnsName = ""
	} else {
		// The FQDN doesn't match the zone - use the full FQDN as the name
		logger.Warn("FQDN %s doesn't appear to be in zone %s", fqdn, domainName)
		dnsName = fqdn
	}

	return dnsName, domainName, nil
}

// configureAGIDNS creates a Route53 DNS record for the AGI instance.
// The DNS name format is: {prefix}.{region}.agi.{domain}
// Where prefix is:
//   - fs-{first8charsOfEFSId} if an EFS volume is attached
//   - agi-{shortuuid} otherwise
func (c *AgiCreateCmd) configureAGIDNS(system *System, instance backends.InstanceList, logger *logger.Logger) error {
	if len(instance.Describe()) == 0 {
		return fmt.Errorf("no instances to configure DNS for")
	}
	inst := instance.Describe()[0]

	// Get the instance's public IP
	publicIP := inst.IP.Public
	if publicIP == "" {
		publicIP = inst.IP.Private
	}
	if publicIP == "" {
		return fmt.Errorf("instance has no IP address")
	}

	// Determine the DNS name prefix
	var prefix string
	if inst.AttachedVolumes != nil && inst.AttachedVolumes.Count() > 0 {
		// Use the first attached EFS volume's ID
		for _, vol := range inst.AttachedVolumes.Describe() {
			if vol.VolumeType == backends.VolumeTypeSharedDisk && vol.FileSystemId != "" {
				// Use first 8 characters of the EFS filesystem ID
				fsId := vol.FileSystemId
				if len(fsId) > 8 {
					fsId = fsId[:8]
				}
				prefix = "fs-" + fsId
				break
			}
		}
	}
	if prefix == "" {
		// Generate a shortuuid-based prefix
		uuid := shortuuid.New()
		if len(uuid) > 8 {
			uuid = uuid[:8]
		}
		// Ensure it starts with a letter (DNS requirement)
		prefix = "agi-" + strings.ToLower(uuid)
	}

	// Build the FQDN: {prefix}.{region}.agi.{domain}
	region := system.Opts.Config.Backend.Region
	domain := strings.TrimSuffix(c.AWS.Route53DomainName, ".")
	fqdn := fmt.Sprintf("%s.%s.agi.%s", prefix, region, domain)

	logger.Info("Creating DNS record: %s -> %s", fqdn, publicIP)

	// Get Route53 client
	regions, err := system.Backend.ListEnabledRegions(backends.BackendTypeAWS)
	if err != nil || len(regions) == 0 {
		return fmt.Errorf("failed to get AWS region: %w", err)
	}
	cli, err := baws.GetRoute53Client(system.Backend.GetCredentials(), &regions[0])
	if err != nil {
		return fmt.Errorf("failed to get Route53 client: %w", err)
	}

	// Create the A record with TTL of 10 seconds
	_, err = cli.ChangeResourceRecordSets(context.Background(), &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(c.AWS.Route53ZoneId),
		ChangeBatch: &rtypes.ChangeBatch{
			Changes: []rtypes.Change{
				{
					Action: rtypes.ChangeActionUpsert,
					ResourceRecordSet: &rtypes.ResourceRecordSet{
						Name: aws.String(fqdn),
						Type: rtypes.RRTypeA,
						TTL:  aws.Int64(10),
						ResourceRecords: []rtypes.ResourceRecord{
							{Value: aws.String(publicIP)},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create DNS record: %w", err)
	}

	// Update instance tags with DNS information
	err = instance.AddTags(map[string]string{
		"agiDNSName": fqdn,
	})
	if err != nil {
		logger.Warn("Failed to update instance tags with DNS name: %s", err)
	}

	logger.Info("DNS record created: %s", fqdn)
	return nil
}

// ensureExpiryCleanupDNS checks if the expiry system has CleanupDNS enabled and enables it if not.
// This is necessary when AGI creates DNS records via Route53, so the expiry system can clean up
// stale DNS records when instances are terminated.
func (c *AgiCreateCmd) ensureExpiryCleanupDNS(system *System, logger *logger.Logger) error {
	// Get the current expiry system configuration
	expiryList, err := system.Backend.ExpiryList()
	if err != nil {
		return fmt.Errorf("failed to get expiry list: %w", err)
	}

	// Check each AWS expiry system and enable CleanupDNS if needed
	// We process each zone individually to preserve its specific settings
	var updateErrors error

	for _, expiry := range expiryList.ExpirySystems {
		if expiry.BackendType != backends.BackendTypeAWS {
			continue
		}
		if !expiry.InstallationSuccess {
			continue
		}
		detail, ok := expiry.BackendSpecific.(*baws.ExpiryDetail)
		if !ok {
			continue
		}
		// If CleanupDNS is already enabled, skip
		if detail.CleanupDNS {
			continue
		}

		// Enable CleanupDNS for this zone while preserving other settings
		logger.Info("Enabling CleanupDNS in expiry system for zone: %s", expiry.Zone)
		err := system.Backend.ExpiryChangeConfiguration(
			backends.BackendTypeAWS,
			detail.LogLevel,
			detail.ExpireEksctl,
			true, // enable CleanupDNS
			expiry.Zone,
		)
		if err != nil {
			updateErrors = errors.Join(updateErrors, fmt.Errorf("zone %s: %w", expiry.Zone, err))
		}
	}

	return updateErrors
}

// ensureAGIFirewall ensures the AGI firewall exists for the specified VPC, creating it if necessary.
// The firewall allows inbound TCP traffic on ports 80 (HTTP) and 443 (HTTPS) from anywhere.
// This function handles race conditions gracefully - if another process creates the firewall
// concurrently, the "already exists" error is ignored.
//
// The firewall name is VPC-specific:
//   - AWS: AEROLAB_AGI_{project}_{vpc-id}
//   - GCP: aerolab-agi-{vpc-name} (sanitized)
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - backendType: The backend type ("aws" or "gcp")
//
// Returns:
//   - string: The firewall name that was created or found
//   - error: nil on success, or an error describing what failed
func (c *AgiCreateCmd) ensureAGIFirewall(system *System, inventory *backends.Inventory, logger *logger.Logger, backendType string) (string, error) {
	// Get the default network (VPC)
	networks := inventory.Networks.WithDefault(true)
	if networks == nil || networks.Count() == 0 {
		return "", fmt.Errorf("no default network found for firewall creation")
	}
	vpc := networks.Describe()[0]

	// Generate VPC-specific firewall name based on backend type
	var firewallName string
	switch backendType {
	case "aws":
		// AWS: AEROLAB_AGI_{project}_{vpc-id}
		// Use AEROLAB_PROJECT env var (aerolab project), not Backend.Project (GCP project)
		project := os.Getenv("AEROLAB_PROJECT")
		if project == "" {
			project = "default"
		}
		firewallName = "AEROLAB_AGI_" + project + "_" + vpc.NetworkId
	case "gcp":
		// GCP: aerolab-agi-{vpc-name} (sanitized)
		firewallName = sanitizeGCPName("aerolab-agi-" + vpc.Name)
	default:
		return "", fmt.Errorf("unsupported backend type for firewall: %s", backendType)
	}

	// Check if firewall already exists
	fws := inventory.Firewalls.WithName(firewallName)
	if fws.Count() > 0 {
		logger.Debug("Firewall %s already exists", firewallName)
		return firewallName, nil
	}

	logger.Info("Creating %s firewall rule for AGI access (ports 22, 80, 443)", firewallName)

	// Create firewall rule for ports 22 (SSH), 80, and 443
	_, err := system.Backend.CreateFirewall(&backends.CreateFirewallInput{
		BackendType: backends.BackendType(backendType),
		Name:        firewallName,
		Description: "AeroLab AGI access (ports 22, 80, 443)",
		Owner:       c.Owner,
		Ports: []*backends.Port{
			{FromPort: 22, ToPort: 22, SourceCidr: "0.0.0.0/0", Protocol: backends.ProtocolTCP},
			{FromPort: 80, ToPort: 80, SourceCidr: "0.0.0.0/0", Protocol: backends.ProtocolTCP},
			{FromPort: 443, ToPort: 443, SourceCidr: "0.0.0.0/0", Protocol: backends.ProtocolTCP},
		},
		Network: vpc,
	}, time.Minute)
	if err != nil {
		// Handle race condition - if firewall was created by another process, that's fine
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "AlreadyExists") || strings.Contains(err.Error(), "InvalidGroup.Duplicate") {
			logger.Debug("Firewall %s was created by another process, continuing", firewallName)
			return firewallName, nil
		}
		return "", fmt.Errorf("failed to create %s firewall: %w", firewallName, err)
	}

	logger.Info("Firewall %s created successfully", firewallName)
	return firewallName, nil
}

// findNextAvailableAGIPort finds the next available host port for Docker AGI instances.
// It scans existing AGI instances and finds an unused port starting from the base port.
//
// Parameters:
//   - inventory: The current backend inventory
//   - useSSL: If true, starts from port 9443 (for container port 443), otherwise 9080 (for container port 80)
//
// Returns:
//   - string: Port mapping in format "HOST_PORT:CONTAINER_PORT"
func findNextAvailableAGIPort(inventory *backends.Inventory, useSSL bool) string {
	basePort := 9080
	containerPort := "80"
	if useSSL {
		basePort = 9443
		containerPort = "443"
	}

	// Collect all used ports from existing AGI instances
	usedPorts := make(map[int]bool)

	if inventory != nil {
		agiInstances := inventory.Instances.WithTags(map[string]string{
			"aerolab.type": "agi",
		}).WithNotState(backends.LifeCycleStateTerminated, backends.LifeCycleStateTerminating).Describe()

		for _, inst := range agiInstances {
			for _, fw := range inst.Firewalls {
				// Format: host=0.0.0.0:8443,container=443
				parts := strings.Split(fw, ",")
				if len(parts) != 2 {
					continue
				}

				var hostPort string
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(part, "host=") {
						hostPart := strings.TrimPrefix(part, "host=")
						// Extract port from IP:PORT
						if colonIdx := strings.LastIndex(hostPart, ":"); colonIdx >= 0 {
							hostPort = hostPart[colonIdx+1:]
						}
					}
				}

				if hostPort != "" {
					if port, err := strconv.Atoi(hostPort); err == nil {
						usedPorts[port] = true
					}
				}
			}
		}
	}

	// Find next available port
	port := basePort
	for usedPorts[port] {
		port++
	}

	return fmt.Sprintf("%d:%s", port, containerPort)
}

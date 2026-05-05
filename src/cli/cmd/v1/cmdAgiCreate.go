//go:build !noagi

package cmd

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/agi"
	"github.com/aerospike/aerolab/pkg/agi/db"
	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
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
	EnableLiveIngest bool           `long:"enable-live-ingest" description:"Expose HTTPS live log streaming; forces WAL on and disables post-ingest compaction"`
	// liveDispatcherToken is set when generating configs with --enable-live-ingest (logged once on success).
	liveDispatcherToken string `json:"-"`
	// Ingest pipeline tuning. Both default to 0 = use the
	// embedded ingest-config defaults (which themselves auto-scale
	// where appropriate; see pkg/agi/ingest/struct.go). Setting a
	// positive value here pins the corresponding knob explicitly.
	IngestMaxConcurrentLogFiles int  `long:"ingest-max-concurrent-log-files" description:"Override max concurrent log files (0 = auto = clamp(GOMAXPROCS, 4, 16))" default:"0"`
	IngestMaxPutThreads         int  `long:"ingest-max-put-threads" description:"Override max put-threads worker pool (0 = use yaml default 128, or set explicitly to override)" default:"0"`
	PluginCpuProfile            bool `long:"plugin-cpu-profiling" description:"Enable CPU profiling for plugin"`
	PluginLogLevel              int  `long:"plugin-log-level" description:"Plugin log level" default:"4"`

	// Notification options
	SlackToken   string `long:"notify-slack-token" description:"Slack token for notifications (supports ENV::VAR_NAME)"`
	SlackChannel string `long:"notify-slack-channel" description:"Slack channel for notifications"`

	// Monitor options
	MonitorUrl        string `long:"monitor-url" description:"AGI Monitor URL for sizing notifications"`
	MonitorCertIgnore bool   `long:"monitor-ignore-cert" description:"Ignore invalid monitor SSL certificate"`

	// Configuration options
	NoConfigOverride bool `long:"no-config-override" description:"Don't override existing config when restarting with EFS"`
	// RefreshEngineConfigs forces re-upload of engine-tuning configs
	// (ingest.yaml, plugin.yaml) even when NoConfigOverride is set.
	// Set by the monitor on sizing-driven reattach so that the new
	// instance's larger memSize is reflected in Pebble's CacheBytes /
	// MemTableSizeBytes and the plugin's concurrency knobs. Internal,
	// not exposed on the CLI.
	RefreshEngineConfigs bool   `json:"-"`
	Owner                string `long:"owner" description:"Owner tag value"`

	// Version options
	GrafanaVersion string `long:"grafana-version" description:"Grafana version" default:"11.2.6"`
	Distro         string `short:"d" long:"distro" description:"Linux distribution" default:"ubuntu"`
	DistroVersion  string `long:"distro-version" description:"Distribution version" default:"latest"`

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

	// Retry options
	MaxRetries int           `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep time.Duration `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`

	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// AgiCreateCmdAws contains AWS-specific options for AGI instance creation.
type AgiCreateCmdAws struct {
	InstanceType        guiInstanceType `short:"I" long:"instance-type" description:"Instance type (min 8GB RAM); empty=auto-select (m7i.xlarge / m6i.xlarge / m7g.xlarge by region+arch)" webchoice:"method::List"`
	Ebs                 string          `short:"E" long:"ebs" description:"EBS volume size in GB" default:"40"`
	SecurityGroupID     string          `short:"S" long:"secgroup-id" description:"Security group IDs (comma-separated)"`
	SubnetID            string          `short:"U" long:"subnet-id" description:"Subnet ID or availability zone"`
	Tags                []string        `long:"tags" description:"Custom tags (key=value)"`
	WithEFS             bool            `long:"with-efs" description:"Use EFS for persistent storage"`
	EFSName             string          `long:"efs-name" description:"EFS volume name" default:"{AGI_NAME}"`
	EFSPath             string          `long:"efs-path" description:"EFS mount path" default:"/"`
	EFSMultiZone        bool            `long:"efs-multizone" description:"Enable multi-AZ EFS (higher cost)"`
	EFSExpires          TypeExpiry      `long:"efs-expire" description:"EFS expiry after last use" default:"96h"`
	EFSFips             bool            `long:"efs-fips" description:"Enable FIPS mode for the EFS mount"`
	TerminateOnPoweroff bool            `long:"terminate-on-poweroff" description:"Terminate instance on poweroff"`
	SpotInstance        bool            `long:"spot-instance" description:"Request spot instance"`
	SpotFallback        bool            `long:"spot-fallback" description:"Fall back to on-demand if spot unavailable"`
	Expires             TypeExpiry      `long:"expire" description:"Instance expiry time" default:"30h"`
	Route53ZoneId       string          `long:"route53-zoneid" description:"Route53 zone ID for DNS"`
	Route53DomainName   string          `long:"route53-domain" description:"Route53 domain name"`
	DisablePublicIP     bool            `long:"disable-public-ip" description:"Disable public IP assignment"`
}

// AgiCreateCmdGcp contains GCP-specific options for AGI instance creation.
//
// Default instance: c2d-standard-4 (16 GiB) post-Pebble. The previous
// default (c2d-highmem-4, 32 GiB) was sized for an in-memory primary
// index that no longer exists; the new sizing model needs floor + 10%
// of log size + headroom, which fits comfortably in 16 GiB for log
// bundles up to ~80 GiB. Larger workloads ladder up via the monitor.
type AgiCreateCmdGcp struct {
	InstanceType        guiInstanceType `long:"instance" description:"Instance type" default:"c2d-standard-4" webchoice:"method::List"`
	Disks               []string        `long:"disk" description:"Disk configuration (type=X,size=Y)" default:"type=pd-ssd,size=40"`
	Zone                guiZone         `long:"zone" description:"GCP zone" webchoice:"method::List"`
	Tags                []string        `long:"tag" description:"Network tags"`
	Labels              []string        `long:"label" description:"Labels (key=value)"`
	SpotInstance        bool            `long:"spot-instance" description:"Request spot instance"`
	Expires             TypeExpiry      `long:"expire" description:"Instance expiry time" default:"30h"`
	WithVol             bool            `long:"with-vol" description:"Use persistent volume for storage"`
	VolName             string          `long:"vol-name" description:"Volume name" default:"{AGI_NAME}"`
	VolExpires          TypeExpiry      `long:"vol-expire" description:"Volume expiry after last use" default:"96h"`
	VolFips             bool            `long:"vol-fips" description:"Enable FIPS mode for the volume mount"`
	TerminateOnPoweroff bool            `long:"terminate-on-poweroff" description:"Terminate instance on poweroff"`
}

// AgiCreateCmdDocker contains Docker-specific options for AGI instance creation.
type AgiCreateCmdDocker struct {
	ExposePortsToHost string   `short:"e" long:"expose-ports" description:"Port forwarding (HOST_PORT:NODE_PORT)"`
	Privileged        bool     `short:"B" long:"privileged" description:"Run in privileged mode"`
	NetworkName       string   `long:"network" description:"Docker network name"`
	Disks             []string `long:"disk" description:"Mount a host path or named volume into the container; format: {volumeName|/hostPath}:{mountTargetDirectory}[:ro|:rw]; example: /host/data:/mnt/data or myvol:/data:ro; can be specified multiple times"`
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
		defer os.RemoveAll(localSource)
	}

	// Handle ~auto~ naming
	if c.ClusterName == "~auto~" {
		c.ClusterName = TypeAgiClusterName(c.generateAutoName())
	}

	// Check if AGI with the same name already exists
	for {
		existingAGI := inventory.Instances.
			WithNotState(backends.LifeCycleStateTerminated, backends.LifeCycleStateTerminating).
			WithClusterName(string(c.ClusterName)).
			WithTags(map[string]string{"aerolab.type": "agi"})
		if existingAGI.Count() == 0 {
			break
		}
		if !IsInteractive() {
			return nil, fmt.Errorf("AGI '%s' already exists", c.ClusterName)
		}
		logger.Warn("AGI '%s' already exists", c.ClusterName)
		newName, err := AskForString("Enter a new AGI name: ")
		if err != nil {
			return nil, fmt.Errorf("failed to read new name: %w", err)
		}
		newName = strings.TrimSpace(newName)
		if newName == "" {
			return nil, errors.New("AGI name cannot be empty")
		}
		c.ClusterName = TypeAgiClusterName(newName)
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
		c.GCP.Zone = guiZone(system.Opts.Config.Backend.Region + "-a")
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

	// Bind mount source is incompatible with S3/SFTP sources because the
	// downloader writes into DirtyTmp/{s3source,sftpsource}, which lives on
	// the read-only bind mount and would fail to create files at download
	// time.
	if isBindMountSource(string(c.LocalSource)) && (c.S3Enable || c.SftpEnable) {
		return nil, fmt.Errorf("bind mount (bind:/path) for --source-local cannot be combined with --source-s3-enable or --source-sftp-enable; the bind-mounted directory is read-only and cannot receive downloaded files")
	}

	// Resolve bind mount source to an absolute path; Docker rejects relative
	// paths (interpreting them as named volumes).
	if isBindMountSource(string(c.LocalSource)) {
		bindPath := getBindMountPath(string(c.LocalSource))
		absBindPath, err := filepath.Abs(bindPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve --source-local bind path %q to absolute: %w", bindPath, err)
		}
		if absBindPath != bindPath {
			logger.Info("Resolved --source-local bind path %s -> %s", bindPath, absBindPath)
		}
		c.LocalSource = flags.Filename("bind:" + absBindPath)
	}

	// --bind-files-dir is only supported on Docker
	if c.BindFilesDir != "" && backendType != "docker" {
		return nil, fmt.Errorf("--bind-files-dir is only supported on Docker backend")
	}

	// Validate --bind-files-dir directory exists
	if c.BindFilesDir != "" {
		// Resolve to absolute path for the same Docker reason as above.
		absBindFilesDir, err := filepath.Abs(string(c.BindFilesDir))
		if err != nil {
			return nil, fmt.Errorf("failed to resolve --bind-files-dir %q to absolute: %w", c.BindFilesDir, err)
		}
		if absBindFilesDir != string(c.BindFilesDir) {
			logger.Info("Resolved --bind-files-dir %s -> %s", c.BindFilesDir, absBindFilesDir)
		}
		c.BindFilesDir = flags.Filename(absBindFilesDir)

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
		expiryCleanupWg.Go(func() {
			if err := c.ensureExpiryCleanupDNS(system, logger); err != nil {
				logger.Warn("Failed to enable CleanupDNS in expiry system: %s", err)
				logger.Warn("You may need to manually enable CleanupDNS via: aerolab config aws expiry-install -c")
			}
		})
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
					if i.Name == string(c.AWS.InstanceType) && len(i.Arch) > 0 {
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
					if i.Name == string(c.GCP.InstanceType) && len(i.Arch) > 0 {
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

	// Refresh inventory if a new template was just created, so that
	// CreateInstances can find the image and recognise it as official.
	// Without this, Docker instances get tagged aerolab.custom.image=true
	// (because the stale inventory doesn't contain the new template),
	// which causes Exec to use docker-exec instead of SSH.
	if templateCreated {
		inventory, err = system.Backend.GetRefreshedInventory()
		if err != nil {
			return nil, fmt.Errorf("failed to refresh inventory after template creation: %w", err)
		}
	}

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
	totalMem, memSize, err := c.getAvailableMemory(instance, backendType)
	if err != nil {
		return nil, err
	}
	logger.Info("Available memory: %d GB total, %d GB after OS reserve", totalMem/1024/1024/1024, memSize/1024/1024/1024)

	// Generate configuration files
	logger.Info("Generating AGI configuration files")
	configs, err := c.generateConfigs(system, totalMem, memSize, backendType)
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
		MaxRetries:      c.MaxRetries,
		RetrySleep:      c.RetrySleep,
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
		if after, ok := strings.CutPrefix(*field, "ENV::"); ok {
			envVar := after
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
	if backendType == "aws" && c.AWS.InstanceType == "" {
		if v, ok := vol.Tags["agiinstance"]; ok && v != "" {
			c.AWS.InstanceType = guiInstanceType(v)
		}
	} else if backendType == "gcp" && c.GCP.InstanceType == "" {
		if v, ok := vol.Tags["agiinstance"]; ok && v != "" {
			c.GCP.InstanceType = guiInstanceType(v)
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
		GrafanaVersion:  c.GrafanaVersion,
		Distro:          c.Distro,
		DistroVersion:   c.DistroVersion,
		Arch:            arch.String(),
		Timeout:         c.Timeout,
		NoVacuum:        c.NoVacuum,
		Owner:           c.Owner,
		DisablePublicIP: c.AWS.DisablePublicIP,
		AerolabBinary:   c.AerolabBinary,
		WithEFS:         withEFS,
	}

	templateName, err := templateCreate.CreateTemplate(system, inventory, logger.WithPrefix("[template] "), args)
	if err != nil {
		return "", false, fmt.Errorf("failed to create AGI template: %w", err)
	}

	// Refresh inventory so the newly created image is visible to downstream
	// instance creation, then resolve the image's canonical inventory name.
	// (The Docker backend stores images with a `:latest` tag suffix, which
	// CreateTemplate does not include in the value it returns. Using the
	// raw returned name would cause `WithName` exact-match lookups in
	// InstancesCreateCmd to miss and fall through to a placeholder
	// `Image{}` with a zero-value Architecture, which in turn makes Docker
	// try to run the arm64 template as linux/amd64.)
	if err := system.Backend.ForceRefreshInventory(); err != nil {
		return "", false, fmt.Errorf("failed to refresh inventory after AGI template creation: %w", err)
	}
	canonicalName := resolveCanonicalAGITemplateName(system.Backend.GetInventory(), arch, templateName)
	if canonicalName == "" {
		return "", false, fmt.Errorf("auto-created AGI template %q not visible in inventory after refresh", templateName)
	}
	return canonicalName, true, nil
}

// resolveCanonicalAGITemplateName finds the canonical inventory name for an
// AGI template image we just auto-created. It accepts the name that
// CreateTemplate returned (e.g. `agi-tmpl-xxx`) and matches it against
// inventory entries for the given architecture, tolerating backend-imposed
// tag suffixes like Docker's `:latest`. Returns "" if no match was found.
func resolveCanonicalAGITemplateName(inventory *backends.Inventory, arch backends.Architecture, createdName string) string {
	images := inventory.Images.
		WithTags(map[string]string{"aerolab.image.type": "agi"}).
		WithArchitecture(arch).
		Describe()
	for _, img := range images {
		if img.Name == createdName || strings.HasPrefix(img.Name, createdName+":") {
			return img.Name
		}
	}
	return ""
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
				"aerolab.agi.volume=true",
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
				newExpiry := time.Now().Add(c.AWS.EFSExpires.Duration())
				logger.Debug("Resetting EFS volume expiry to %s", newExpiry.Format(time.RFC3339))
				if err := vol.ChangeExpiry(newExpiry); err != nil {
					logger.Warn("Failed to reset EFS volume expiry: %s", err)
				}
			}
			// Update EFS tags with current instance settings
			// This ensures tags reflect the latest settings (important for monitor sizing and reattach)
			newTags := map[string]string{
				"agiinstance":        string(c.AWS.InstanceType),
				"aginodim":           fmt.Sprintf("%t", c.NoDIM),
				"termonpow":          fmt.Sprintf("%t", c.AWS.TerminateOnPoweroff),
				"isspot":             fmt.Sprintf("%t", c.AWS.SpotInstance),
				"aerolab.agi.volume": "true",
				"agifips":            fmt.Sprintf("%t", c.AWS.EFSFips),
				"agisubnet":          c.AWS.SubnetID,
				"agisecgroup":        c.AWS.SecurityGroupID,
				"agiefsexpire":       c.AWS.EFSExpires.String(),
				"agiLabel":           agiLabelB64,
				"agiSrcLocal":        sourceStringLocalB64,
				"agiSrcSftp":         sourceStringSftpB64,
				"agiSrcS3":           sourceStringS3B64,
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
				"aerolab.agi.volume=true",
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
				newExpiry := time.Now().Add(c.GCP.VolExpires.Duration())
				logger.Debug("Resetting GCP volume expiry to %s", newExpiry.Format(time.RFC3339))
				if err := vol.ChangeExpiry(newExpiry); err != nil {
					logger.Warn("Failed to reset GCP volume expiry: %s", err)
				}
			}
			// Update GCP volume tags with current instance settings
			// This ensures tags reflect the latest settings (important for monitor sizing and reattach)
			newTags := map[string]string{
				"agiinstance":        string(c.GCP.InstanceType),
				"aginodim":           fmt.Sprintf("%t", c.NoDIM),
				"termonpow":          fmt.Sprintf("%t", c.GCP.TerminateOnPoweroff),
				"isspot":             fmt.Sprintf("%t", c.GCP.SpotInstance),
				"aerolab.agi.volume": "true",
				"agifips":            fmt.Sprintf("%t", c.GCP.VolFips),
				"agizone":            string(c.GCP.Zone),
				"agivolexpire":       c.GCP.VolExpires.String(),
				"agiLabel":           agiLabelB64,
				"agiSrcLocal":        sourceStringLocalB64,
				"agiSrcSftp":         sourceStringSftpB64,
				"agiSrcS3":           sourceStringS3B64,
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
		awsInstanceType = string(c.AWS.InstanceType)
		if awsInstanceType == "" {
			// Post-Pebble defaults: drop from the r-family
			// (memory-optimized, 32 GiB at xlarge) to the m-family
			// (general-purpose, 16 GiB at xlarge). The new sizing
			// model needs ~10% of log size + a small floor; 16 GiB
			// fits log bundles up to ~80 GiB without scaling.
			// Larger workloads ladder up via the monitor's
			// pre-process completion callback.
			if arch.String() == "arm64" {
				awsInstanceType = "m7g.xlarge"
			} else {
				switch system.Opts.Config.Backend.Region {
				case "af-south-1", "ap-east-1", "ca-west-1", "cn-north-1", "cn-northwest-1", "eu-central-2", "il-central-1", "me-south-1", "me-central-1":
					// change the default instance family for regions that don't support the m7 yet
					awsInstanceType = "m6i.xlarge"
				default:
					awsInstanceType = "m7i.xlarge"
				}
			}
		}
	case "gcp":
		gcpInstanceType = string(c.GCP.InstanceType)
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
		ImageVersion:       fmt.Sprintf("agi-%d", agi.AGIVersion),
		Arch:               arch.String(),
		AWS: InstancesCreateCmdAws{
			ImageID:          templateName,
			Expire:           c.AWS.Expires,
			NetworkPlacement: system.Opts.Config.Backend.Region,
			InstanceType:     guiInstanceType(awsInstanceType),
			Disks:            []string{fmt.Sprintf("type=gp3,size=%s", c.AWS.Ebs)},
			Firewalls:        []string{agiFirewallName},
			SpotInstance:     c.AWS.SpotInstance,
			DisablePublicIP:  c.AWS.DisablePublicIP,
		},
		GCP: InstancesCreateCmdGcp{
			ImageName:    templateName,
			Expire:       c.GCP.Expires,
			Zone:         c.GCP.Zone,
			InstanceType: guiInstanceType(gcpInstanceType),
			Disks:        c.GCP.Disks,
			Firewalls:    append([]string{agiFirewallName}, c.GCP.Tags...),
			SpotInstance: c.GCP.SpotInstance,
		},
		Docker: InstancesCreateCmdDocker{
			ImageName:   templateName,
			NetworkName: c.Docker.NetworkName,
			Disks:       c.Docker.Disks,
			ExposePorts: []string{exposePort},
			Privileged:  c.Docker.Privileged,
		},
		suppressEquivalentCommand: true,
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
			MaxRetries:      c.MaxRetries,
			RetrySleep:      c.RetrySleep,
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
			MaxRetries:      c.MaxRetries,
			RetrySleep:      c.RetrySleep,
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

	if c.EnableLiveIngest {
		desc := inst.Describe()
		if len(desc) > 0 {
			host := desc[0].IP.Public
			if host == "" {
				host = desc[0].IP.Private
			}
			scheme := "https"
			if c.ProxyDisableSSL {
				scheme = "http"
			}
			if c.liveDispatcherToken != "" {
				logger.Info("Live log ingest is enabled. Dispatcher bearer token: %s", c.liveDispatcherToken)
			}
			if host != "" {
				logger.Info("Live ingest stream URL pattern: %s://%s/agi/ingest/stream?cluster=CLUSTER&node=NODE_ID&source-id=STABLE_ID (Authorization: Bearer <token>)", scheme, host)
			} else {
				logger.Info("Live ingest is enabled; use the AGI instance IP or DNS with path /agi/ingest/stream and query params cluster, node, source-id")
			}
		}
	}

	return inst, nil
}

// updateVolumeTagsWithResolvedValues updates the volume tags with all resolved values after instance creation.
// This ensures tags have the actual values used (not empty strings from unset parameters) and includes
// all parameters needed for the reattach flow.
func (c *AgiCreateCmd) updateVolumeTagsWithResolvedValues(system *System, inventory *backends.Inventory, logger *logger.Logger, volumeName string, backendType string, awsInstanceType string, gcpInstanceType string, sourceStringLocal string, sourceStringSftp string, sourceStringS3 string, agiFirewallName string, templateName string, arch backends.Architecture) {
	var vol backends.Volumes
	switch backendType {
	case "aws":
		vol = inventory.Volumes.WithType(backends.VolumeTypeSharedDisk).WithName(volumeName)
	case "gcp":
		vol = inventory.Volumes.WithType(backends.VolumeTypeAttachedDisk).WithName(volumeName)
	}
	if vol == nil || vol.Count() == 0 {
		logger.Warn("Could not find volume %s to update tags", volumeName)
		return
	}

	// Build comprehensive tag set with all values needed for reattach
	tags := map[string]string{
		// Core instance settings (now with resolved values)
		"aginodim":           fmt.Sprintf("%t", c.NoDIM),
		"aerolab.agi.volume": "true",

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
	switch backendType {
	case "aws":
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
	case "gcp":
		tags["agiinstance"] = gcpInstanceType // Resolved value, not input
		tags["termonpow"] = fmt.Sprintf("%t", c.GCP.TerminateOnPoweroff)
		tags["isspot"] = fmt.Sprintf("%t", c.GCP.SpotInstance)
		tags["agifips"] = fmt.Sprintf("%t", c.GCP.VolFips)
		tags["agizone"] = string(c.GCP.Zone)
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
//
// Returns the total host RAM (totalMem, as reported by /proc/meminfo
// MemTotal) and the post-reservation budget (memSize = totalMem -
// reserved). Both are exposed because the Pebble sizing helpers cap
// against totalMem (the OOM guardrail is a fraction of the *whole*
// host) while the rest of the pipeline budgets against memSize (the
// portion AGI may safely allocate after carving out the OS reserve).
func (c *AgiCreateCmd) getAvailableMemory(instance backends.InstanceList, backendType string) (totalMem int64, memSize int64, err error) {
	var lastErr error
	var lastStdout, lastStderr string

	// Retry up to 10 times with 3 second delay to handle instance startup delay
	for attempt := range 10 {
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
			MaxRetries:      c.MaxRetries,
			RetrySleep:      c.RetrySleep,
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
			return 0, 0, fmt.Errorf("malformed memory line: %q", lines[1])
		}

		total, perr := strconv.ParseInt(fields[1], 10, 64)
		if perr != nil {
			return 0, 0, fmt.Errorf("failed to parse memory: %w", perr)
		}

		// Reserve memory: 6GB for cloud, 3GB for docker
		reserved := agiOSReserveBytes(backendType)
		mSize := total - reserved
		// Minimum viable memSize differs by deployment:
		//   - Cloud uses the 50%-of-host Pebble budget cap and the
		//     full ingest/plugin concurrency; below 5 GiB memSize
		//     the generated config pegs at 100% memory and OOMs.
		//   - Docker uses the legacy "memSize/2" Pebble heuristic
		//     and lower concurrency (MaxPutThreads halved); the
		//     classic 1 GiB floor still works.
		var minViable int64
		if backendType == "docker" {
			minViable = int64(1) << 30
		} else {
			minViable = agiNonPebbleOverheadBytes + (int64(1) << 30)
		}
		if mSize < minViable {
			return 0, 0, fmt.Errorf("not enough RAM after %d GiB OS reserve: have %d GiB, need at least %d GiB",
				reserved/(1<<30), mSize/(1<<30), minViable/(1<<30))
		}

		return total, mSize, nil
	}

	// All retries failed
	return 0, 0, fmt.Errorf("failed to get memory info after 10 attempts: %w (stdout=%q stderr=%q)", lastErr, lastStdout, lastStderr)
}

// pebbleConfig collects the Pebble DB tuning values that get embedded
// into both ingest.yaml and plugin.yaml. They MUST agree across both
// configs because the merged service (cmdAgiExecService) opens one
// shared Pebble handle and applies whichever set of options the
// caller built first; mismatched yaml lets one half silently win.
type pebbleConfig struct {
	CacheBytes            int64
	MemTableBytes         uint64
	MaxCompactions        int
	StopWritesThreshold   int
	BlockSize             int
	Compression           string
	TargetFileSizeL0      int64
	BytesPerSync          int
	LBaseMaxBytes         int64
	L0StopWritesThreshold int
	EnableBloomFilter     bool
	// PostIngestCompact only flows into ingest.yaml; the plugin
	// never ingests so this knob has no meaning there. Kept in the
	// shared struct so generateConfigs has one place to derive every
	// per-deployment Pebble decision before fanning out to the two
	// generators.
	PostIngestCompact bool
}

// generateConfigs generates all AGI configuration files.
func (c *AgiCreateCmd) generateConfigs(system *System, totalMem, memSize int64, backendType string) (map[string][]byte, error) {
	configs := make(map[string][]byte)

	// Compute Pebble DB tuning from the instance's memory profile and
	// storage shape. totalMem is the host's total RAM (used for the
	// 50%-of-host guardrail); memSize is the post-reservation budget
	// (used for the per-process budget that ingest+plugin+Go also
	// share); backendType picks the EFS-/NFS-tuned vs local-FS tuned
	// branch for every helper that has one.
	pcfg := pebbleConfig{
		CacheBytes:            computePebbleCacheBytes(totalMem, memSize, backendType),
		MemTableBytes:         computePebbleMemTableBytes(totalMem, memSize, backendType),
		MaxCompactions:        computePebbleMaxConcurrentCompactions(backendType),
		BlockSize:             computePebbleBlockSize(backendType),
		Compression:           computePebbleCompression(backendType),
		TargetFileSizeL0:      computePebbleTargetFileSizeL0(backendType),
		BytesPerSync:          computePebbleBytesPerSync(backendType),
		LBaseMaxBytes:         0, // set below — depends on MemTableBytes
		L0StopWritesThreshold: computePebbleL0StopWritesThreshold(backendType),
		EnableBloomFilter:     computePebbleEnableBloomFilter(backendType),
		PostIngestCompact:     computePebblePostIngestCompact(backendType),
	}
	pcfg.StopWritesThreshold = computePebbleStopWritesThreshold(totalMem, memSize, pcfg.MemTableBytes, backendType)
	// LBaseMaxBytes scales with the memtable size we picked so that
	// LBase always has room for several flushes before triggering
	// the LBase→L1 compaction cascade. Must come after MemTableBytes
	// is set; see computePebbleLBaseMaxBytes for the ratio rationale.
	pcfg.LBaseMaxBytes = computePebbleLBaseMaxBytes(backendType, pcfg.MemTableBytes)

	if c.EnableLiveIngest {
		pcfg.PostIngestCompact = false
	}

	// Generate ingest.yaml
	ingestConfig, err := c.generateIngestConfig(backendType, pcfg)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ingest config: %w", err)
	}
	configs["/opt/agi/ingest.yaml"] = ingestConfig

	// Generate plugin.yaml
	pluginConfig := c.generatePluginConfig(backendType, pcfg, c.EnableLiveIngest)
	configs["/opt/agi/plugin.yaml"] = pluginConfig

	if c.EnableLiveIngest {
		c.liveDispatcherToken = ""
		tok := make([]byte, 24)
		if _, err := rand.Read(tok); err != nil {
			system.Logger.Warn("could not generate dispatcher token: %v", err)
		} else {
			tokHex := hex.EncodeToString(tok)
			configs["/opt/agi/tokens/dispatcher"] = []byte(tokHex + "\n")
			c.liveDispatcherToken = tokHex
		}
	}

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

// agiNonPebbleOverheadBytes is the amount of memSize that AGI sets
// aside for non-Pebble process needs: the ingest pipeline (PutBatch
// shards × batch size × row size, scanner read buffers, decompressors),
// the plugin pipeline (in-flight queries' decoded rows + result
// buffers), the Go runtime (heap + goroutine stacks + GC overhead),
// and the in-process Grafana / grafanafix bits. Empirically ~4 GiB at
// the configured concurrency limits. Pebble's budget is computed as
// memSize - this constant so all three layers fit at peak load.
const agiNonPebbleOverheadBytes = int64(4) << 30

// agiOSReserveBytes is the OS / system reserve carved out of the
// host's total RAM before AGI may allocate. It is mirrored in
// getAvailableMemory and the AGI monitor sizing formula; keep all
// three in lockstep or instances generated here will fail the
// monitor's pre-process sizing check.
func agiOSReserveBytes(backendType string) int64 {
	if backendType == "docker" {
		return int64(3) << 30
	}
	return int64(6) << 30
}

// computePebbleTotalBudget returns the maximum number of host RAM
// bytes Pebble may consume across its block cache and memtables
// combined. It is the joint constraint of two limits:
//
//  1. **50% of total host RAM** — a hard guardrail against OOM under
//     sudden plugin-query load, regardless of memSize. The remaining
//     50% covers the OS reserve, ingest/plugin pipeline buffers, the
//     Go runtime, Grafana, and the OS page cache that itself helps
//     Pebble's L1+ reads.
//  2. **memSize − agiNonPebbleOverheadBytes** — leaves at least
//     agiNonPebbleOverheadBytes inside the post-reservation budget
//     for the merged process's non-Pebble parts.
//
// On Docker the original "half of memSize" heuristic is preserved so
// the existing dev path is unchanged.
func computePebbleTotalBudget(totalMem, memSize int64, backendType string) int64 {
	if backendType == "docker" {
		return memSize / 2
	}
	halfHost := totalMem / 2
	afterOverhead := memSize - agiNonPebbleOverheadBytes
	if afterOverhead < 0 {
		afterOverhead = 0
	}
	budget := halfHost
	if afterOverhead < budget {
		budget = afterOverhead
	}
	// Floor: even on very small cloud hosts (which AGI is not
	// targeted at, but which we should not crash on at config
	// generation), keep enough room for a minimal cache + a couple
	// of memtables so the LSM remains functional.
	const minBudget = int64(512) << 20
	if budget < minBudget {
		budget = minBudget
	}
	return budget
}

// computePebbleCacheBytes returns the recommended Pebble block cache
// size.
//
// Cloud (AWS/GCP): half of the Pebble budget (see
// computePebbleTotalBudget) goes to the explicit block cache; the
// other half is reserved for peak memtable RAM. The value is
// floored at 256 MiB and capped at 16 GiB — beyond 16 GiB, marginal
// value drops sharply because the OS page cache is just as
// effective for cold reads and cheaper to evict during compaction.
// When the cache cap binds, the unused portion of the budget flows
// to memtables (see computePebbleMemTableBytes).
//
// Docker: keeps the original "half of memSize" heuristic. Docker
// AGIs use small fixed-size memtables (64–256 MiB × default 4
// threshold) whose peak RAM is well under 1 GiB, so the cache and
// memtable budgets do not need to share a pool — the original
// formula is unchanged from the pre-budget-cap behaviour.
func computePebbleCacheBytes(totalMem, memSize int64, backendType string) int64 {
	const floor = int64(256) << 20
	const cap = int64(16) << 30
	var cache int64
	if backendType == "docker" {
		cache = memSize / 2
	} else {
		cache = computePebbleTotalBudget(totalMem, memSize, backendType) / 2
	}
	if cache < floor {
		cache = floor
	}
	if cache > cap {
		cache = cap
	}
	return cache
}

// computePebbleMemTableBytes returns the recommended per-memtable
// size (Pebble's write buffer). On cloud (AWS/GCP, EFS-backed) the
// size is auto-picked so that the EFS-jitter buffer floor of 8
// memtables fits in the half of the Pebble budget reserved for
// memtables — i.e. 8 × memTableBytes ≤ memtableBudget.
//
// Docker AGIs run on local FS where SSTable creation is sub-
// millisecond; the original tuning (256 MiB on >=6 GiB hosts,
// 64 MiB below) is throughput-bound on the actual write rather
// than file metadata, so we leave it alone.
//
// Cloud rationale: each memtable flush on EFS opens a new SSTable
// (O_CREAT → write → fsync → atomic rename → MANIFEST update + sync),
// each metadata step costing 20–100 ms (vs <1 ms on NVMe). The
// dominant ingest cost is therefore the *number* of flushes, not the
// bytes-per-flush. Larger memtables linearly reduce flush count, so
// we pick the largest size that still lets 8 memtables fit in the
// memtable budget, capped at 1 GiB (beyond which a single flush
// duration starts producing user-visible PutBatch stalls when the
// stop-writes queue overruns during a slow EFS moment).
//
// Choice ladder on cloud (per-memtable budget = memtableBudget / 8):
//
//	≥ 1024 MiB → 1 GiB      // r7a.2xlarge and bigger
//	≥  512 MiB → 512 MiB    // mid-size hosts
//	≥  256 MiB → 256 MiB    // m7i.xlarge / 16 GiB cloud
//	≥  128 MiB → 128 MiB    // tight hosts
//	otherwise   → 64 MiB    // floor
func computePebbleMemTableBytes(totalMem, memSize int64, backendType string) uint64 {
	if backendType == "docker" {
		if memSize >= int64(6)<<30 {
			return uint64(256) << 20
		}
		return uint64(64) << 20
	}
	cacheBytes := computePebbleCacheBytes(totalMem, memSize, backendType)
	pebbleBudget := computePebbleTotalBudget(totalMem, memSize, backendType)
	memtableBudget := pebbleBudget - cacheBytes
	if memtableBudget < int64(64)<<20 {
		return uint64(64) << 20
	}
	const target = int64(8) // EFS-jitter floor — match computePebbleStopWritesThreshold
	per := memtableBudget / target
	switch {
	case per >= int64(1)<<30:
		return uint64(1) << 30
	case per >= int64(512)<<20:
		return uint64(512) << 20
	case per >= int64(256)<<20:
		return uint64(256) << 20
	case per >= int64(128)<<20:
		return uint64(128) << 20
	default:
		return uint64(64) << 20
	}
}

// computePebbleMaxConcurrentCompactions returns the upper bound on
// concurrent Pebble compactions for the deployment.
//
// Docker AGIs use local FS where parallel compactions saturate
// disk bandwidth and serialize harmlessly behind it; we return 0
// so the db package's 4-way default applies (db.DefaultOptions).
//
// Cloud AGIs back /opt/agi with EFS. We allow exactly two
// concurrent compactions: one memtable flush pipelining with one
// L0→Lbase compaction. The earlier "1" choice was made when the
// rest of the LSM was un-tuned (small SSTables, default
// BytesPerSync, default LBaseMaxBytes) and parallel compactions
// thrashed the MANIFEST + directory inode metadata path. After
// the BlockSize / TargetFileSize / BytesPerSync=disabled /
// LBaseMaxBytes=4×memTable changes, each compaction produces far
// fewer, much larger SSTables, so the metadata-RTT cost is
// amortised across enough bytes that two streams of fsync round-
// trips can pipeline at the EFS server in parallel without
// over-subscribing the per-NFS-client cap. The expected win is
// flush-while-compacting overlap, removing the "memtable
// stalls until compaction drains" dead time that "1" forces.
// Higher values (4-6) regressed in earlier testing because the
// LSM was not yet shaped to absorb them; if a future change moves
// the ingest off EFS the budget should be re-evaluated. Operators
// that observe write stalls can override via the
// maxConcurrentCompactions yaml key.
func computePebbleMaxConcurrentCompactions(backendType string) int {
	if backendType == "docker" {
		return 0 // db default = 4
	}
	return 2
}

// computePebbleStopWritesThreshold returns the upper bound on the
// number of memtables Pebble keeps alive (active + queued +
// flushing) before blocking incoming PutBatch calls.
//
// Docker AGIs run on local FS where memtable flushes are fast and
// the writer rarely backs up; we return 0 so the db package's
// default of 4 applies (db.DefaultOptions). Worst-case memtable
// RAM at default Docker memtable sizes is well under 1 GiB.
//
// Cloud AGIs back /opt/agi with EFS, where a single memtable flush
// can take 30+ seconds (EFS bandwidth + metadata RTTs). Any
// transient EFS slowdown stalls the writer the moment the queue
// hits the default of 4 — we therefore set a floor of 8 to absorb
// short EFS hiccups without leaking flushes back to the foreground
// put path. On bigger hosts where the memtable budget exceeds
// 8 × memTableBytes the threshold scales with the budget up to a
// ceiling of 32 (beyond which the post-EFS-recovery flush queue
// takes pathologically long to drain).
//
// computePebbleMemTableBytes already auto-picks memTableBytes so
// that 8 × memTableBytes ≤ memtableBudget, so the floor of 8 is
// always satisfiable on cloud — no danger of peak memtable RAM
// blowing the budget on small hosts.
//
// Worked examples (cloud only, with the auto-picked memTableBytes):
//
//	 16 GiB host: memtableBudget=3 GiB, memTableBytes=256 MiB → 12 → floor=8 wins (peak 2 GiB)
//	 32 GiB host: memtableBudget=8 GiB, memTableBytes=1 GiB   → 8  → floor=8 wins (peak 8 GiB)
//	 64 GiB host: memtableBudget=16 GiB, memTableBytes=1 GiB  → 16 → budget wins  (peak 16 GiB)
//	128 GiB host: memtableBudget=32+ GiB, memTableBytes=1 GiB → 32 → ceiling wins (peak 32 GiB)
func computePebbleStopWritesThreshold(totalMem, memSize int64, memTableBytes uint64, backendType string) int {
	if backendType == "docker" {
		return 0 // db default = 4
	}
	if memTableBytes == 0 {
		return 8
	}
	cacheBytes := computePebbleCacheBytes(totalMem, memSize, backendType)
	pebbleBudget := computePebbleTotalBudget(totalMem, memSize, backendType)
	memtableBudget := pebbleBudget - cacheBytes
	if memtableBudget < 0 {
		memtableBudget = 0
	}
	const floor = 8
	const ceiling = 32
	n := int(uint64(memtableBudget) / memTableBytes)
	if n < floor {
		n = floor
	}
	if n > ceiling {
		n = ceiling
	}
	return n
}

// computePebbleBlockSize returns the SSTable data-block size for the
// deployment.
//
// Docker AGIs use local FS where read amplification on point Gets
// dominates the trade-off; we return 0 so the db package leaves
// Pebble's default of 4 KiB in place.
//
// Cloud AGIs back /opt/agi with EFS. Every block-sized write to an
// SSTable is one NFS WRITE RPC; EFS bills "≥4 KiB per request" and
// each request pays a fixed ~2.7 ms write latency. A 128 KiB block
// pays the same per-RPC overhead as a 32 KiB block while moving 4×
// the bytes per round-trip, so the streaming write rate scales
// roughly linearly with BlockSize until the per-NFS-client byte
// cap (500 MiBps on One Zone) becomes the next ceiling. We're
// nowhere near that ceiling at AGI's measured ~24 MiB/s, so
// bigger blocks should translate fairly directly into ingest
// wall-clock savings.
//
// On the read side: AGI's hot path is timestamp-range scans where
// successive iterator reads land in the same block; bigger blocks
// are nearly free because the same prefetched bytes serve more
// rows. Cold point Gets decompress 4× more bytes per call than at
// 32 KiB, but the plugin is scan-heavy and the extra Snappy/Zstd
// CPU per cold block is sub-millisecond at modern instance sizes.
//
// The compounding effect with "balanced" compression (Snappy /
// MinLZ on upper levels, Zstd1 on the bottom 1-2 levels) is where
// the bulk of the on-disk savings come from for the int-heavy AGI
// metric workload — bigger blocks give the codec more
// cross-row redundancy to fold out.
func computePebbleBlockSize(backendType string) int {
	if backendType == "docker" {
		return 0 // db default → pebble default = 4 KiB
	}
	return 128 << 10 // 128 KiB
}

// computePebbleCompression returns the SSTable compression profile
// for the deployment.
//
// Docker AGIs use local FS where Snappy is already the right
// trade-off (cheap CPU, quick flushes, low write amplification);
// we return "" so the db package leaves Pebble's uniform-Snappy
// default in place.
//
// Cloud AGIs back /opt/agi with EFS, where on-disk byte volume is
// the bottleneck and CPU has slack on the deployed instance shapes.
// "balanced" maps to Pebble's DBCompressionBalanced: cheap codecs
// (FastestCompression — MinLZ on x86_64 / Snappy on arm64) on the
// upper LSM levels so flushes and minor compactions stay on the
// hot path, plus Zstd level 1 on the bottom 1-2 levels where the
// bulk of bytes settle. This concentrates Zstd CPU on the level
// where it amortises best (data is rewritten there once and
// stays) while never adding Zstd cost to the foreground writer.
//
// On the AGI metric workload (varint-encoded integer columns,
// repeated colID/type tag bytes per row, timestamp-clustered
// blocks) the realistic on-disk improvement vs uniform Snappy is
// roughly 25-40%, which translates almost linearly into ingest
// wall-clock when EFS-bandwidth-bound.
func computePebbleCompression(backendType string) string {
	if backendType == "docker" {
		return "" // db default → pebble default = uniform Snappy
	}
	return "balanced"
}

// computePebbleTargetFileSizeL0 returns the target SSTable size at L0.
// All higher levels double from this value (Pebble cascades the
// chain in options.go::EnsureDefaults: TargetFileSizes[i] =
// TargetFileSizes[i-1] * 2 for any unset entry).
//
// Docker AGIs use local FS where SSTable creation is sub-millisecond;
// Pebble's default 2 MiB L0 target is fine and we return 0 to leave it
// alone.
//
// Cloud AGIs back /opt/agi with EFS, where each new SSTable costs 2-3
// metadata RTTs (~60-90 ms) for the open / close / atomic-rename +
// MANIFEST update path. With Pebble's default 2 MiB target, a 1 GiB
// memtable flush splits into ~500 files, paying tens of seconds of
// pure metadata cost per flush. We previously bumped to 32 MiB
// (~32 files per flush); 64 MiB halves that again to ~16 files
// per flush. Combined with the 128 KiB BlockSize change, each of
// those files is also written in 4× larger NFS RPCs.
//
// The cascaded chain at 64 MiB L0 looks like:
//
//	L0 = 64 MiB         (memtable flush output)
//	L1 = 128 MiB        (LBase compaction target)
//	L2 = 256 MiB
//	L3 = 512 MiB
//	L4 = 1 GiB
//	L5 = 2 GiB
//	L6 = 4 GiB
//
// All of these still fit inside LBaseMaxBytes (4 × memTableBytes ≈
// 4 GiB on the typical AGI shape), so L0→Lbase compaction never
// has to split across multiple LBase files for size reasons —
// keeping the per-compaction MANIFEST update count to one. Read-
// side: indexed range scans (the plugin's hot path) prefer fewer,
// larger files because each open() is one NFS round-trip; cold
// Gets pay the same per-block decompression cost regardless of
// file size.
func computePebbleTargetFileSizeL0(backendType string) int64 {
	if backendType == "docker" {
		return 0 // db default → pebble default 2 MiB
	}
	return int64(64) << 20 // 64 MiB
}

// computePebbleBytesPerSync returns the periodic-sync cadence in
// bytes for SSTable writes.
//
// Docker AGIs use local FS where Pebble's default 512 KiB cadence
// "smooths writes" by issuing sync_file_range to bound the dirty
// page-cache window per file. We return 0 (db default → pebble
// default) so Docker stays unchanged.
//
// Cloud AGIs back /opt/agi with EFS, where each sync_file_range call
// becomes a synchronous NFS COMMIT (~30 ms each). Default cadence
// fires one COMMIT per 512 KiB of SSTable data — thousands of round
// trips per ingest run, all of them inline on the writer goroutine.
// We return db.BytesPerSyncDisabled to set Pebble's BytesPerSync to 0,
// disabling the periodic sync entirely. This is durability-neutral
// for AGI: the WAL is already off (re-ingest from source on crash)
// and close() still flushes the file.
func computePebbleBytesPerSync(backendType string) int {
	if backendType == "docker" {
		return 0 // db default → pebble default 512 KiB
	}
	return db.BytesPerSyncDisabled
}

// computePebbleLBaseMaxBytes returns the max size of LBase, the
// level into which L0 compacts. **Critical**: this MUST be larger
// than the memtable size, ideally several times larger.
//
// Why: a memtable flush dumps `memTableBytes` of data into L0. If
// LBase < memTableBytes, every single flush overflows LBase the
// moment its L0 sublevel compacts down — which immediately fires
// an LBase→L1 compaction, which fires L1→L2, etc. Each step in
// that cascade is more file create / rename / MANIFEST RTTs on
// EFS, exactly what we are trying to avoid.
//
// Pebble's defaults reflect the right design: 4 MiB memtable +
// 64 MiB LBase = 16× ratio, so several flushes land before
// triggering cascade. We mirror that: LBase = 4 × memTableBytes
// on cloud. Concrete numbers (cloud only):
//
//	memTable=64 MiB   → LBase=256 MiB
//	memTable=128 MiB  → LBase=512 MiB
//	memTable=256 MiB  → LBase=1 GiB
//	memTable=512 MiB  → LBase=2 GiB
//	memTable=1 GiB    → LBase=4 GiB
//
// 4× balances three things:
//  1. Big enough that 4 flushes land before LBase fills (so
//     cascade fires once per ~4 flushes, not per flush);
//  2. Small enough that LBase compactions still run in bounded
//     time and bounded memory;
//  3. For typical AGI ingest sizes (10-50 GiB), the cascade
//     fires only a handful of times across the whole run
//     instead of after every flush.
//
// Docker AGIs use local FS where the cascade is cheap (compactions
// are quick), and the default 64 MiB ÷ 64-256 MiB memtable ratio
// is already fine. We return 0 to leave Pebble's default in place.
func computePebbleLBaseMaxBytes(backendType string, memTableBytes uint64) int64 {
	if backendType == "docker" {
		return 0 // db default → pebble default 64 MiB
	}
	const ratio = 4
	return int64(memTableBytes) * ratio
}

// computePebbleL0StopWritesThreshold returns the L0-sublevel count
// at which Pebble stalls foreground writers (the L0 analogue of
// MemTableStopWritesThreshold).
//
// Docker AGIs use local FS where compactions drain L0 quickly; the
// default 12 is plenty. We return 0 to leave it alone.
//
// Cloud AGIs back /opt/agi with EFS and run with
// MaxConcurrentCompactions=1 (serialized to keep EFS metadata RTTs
// from amplifying), so the L0 sublevel queue can spike during slow
// EFS moments. Stopping at 12 sublevels would stall the ingest
// pipeline whenever EFS hiccups; 36 absorbs the spike. This is the
// L0 counterpart to the 8-32 EFS-jitter buffer we apply at the
// memtable layer.
func computePebbleL0StopWritesThreshold(backendType string) int {
	if backendType == "docker" {
		return 0 // db default → pebble default 12
	}
	return 36
}

// computePebbleEnableBloomFilter returns whether to install a
// 10-bits-per-key bloom filter on every level.
//
// Bloom filters are pure write-side tax: each key is hashed and
// folded into the per-SSTable filter bytes during flush /
// compaction (extra CPU on the writer goroutine), the filter
// itself adds ~1.25 % to the on-disk byte count (extra EFS
// bandwidth on every flush and compaction), and there is *no*
// write-path benefit. The payoff is read-only — negative point
// lookups skip the block fetch — and AGI's plugin reads on the
// post-Pebble path are already 3× the legacy system, so we
// optimise for ingest throughput here and leave bloom filters
// off everywhere by default. Operators with a read-heavy cold
// query pattern can flip enableBloomFilter: true in the yaml.
func computePebbleEnableBloomFilter(backendType string) bool {
	_ = backendType
	return false
}

// computePebblePostIngestCompact returns whether to trigger a
// synchronous full-keyspace db.Compact() at the end of ProcessLogs.
//
// Always false. Post-ingest compaction is disabled for every
// backend (Docker, AWS, GCP) because the synchronous Compact()
// stalls the "ingest finished" signal for an unbounded amount of
// time on EFS-backed DBs and has been observed to wedge the
// service on large runs. The yaml field (db.postIngestCompact)
// and the ProcessLogs implementation are retained so an operator
// can still flip it on by hand for a one-off run, but the create
// command never opts in.
func computePebblePostIngestCompact(backendType string) bool {
	_ = backendType
	return false
}

// generateIngestConfig generates the ingest.yaml configuration.
func (c *AgiCreateCmd) generateIngestConfig(backendType string, pcfg pebbleConfig) ([]byte, error) {
	config, err := ingest.MakeConfigReader(true, nil, false)
	if err != nil {
		return nil, err
	}

	// Pebble DB tuning. The merged service (cmdAgiExecService) opens
	// one Pebble handle and shares it between ingest and plugin; both
	// YAML files must therefore declare the same settings or the
	// operator-set value from one would silently lose to the other
	// depending on which call site builds the options first.
	config.DB.CacheBytes = pcfg.CacheBytes
	config.DB.MemTableSizeBytes = pcfg.MemTableBytes
	config.DB.MaxConcurrentCompactions = pcfg.MaxCompactions
	config.DB.MemTableStopWritesThreshold = pcfg.StopWritesThreshold
	config.DB.BlockSize = pcfg.BlockSize
	config.DB.Compression = pcfg.Compression
	config.DB.TargetFileSizeL0 = pcfg.TargetFileSizeL0
	config.DB.BytesPerSync = pcfg.BytesPerSync
	config.DB.LBaseMaxBytes = pcfg.LBaseMaxBytes
	config.DB.L0StopWritesThreshold = pcfg.L0StopWritesThreshold
	config.DB.EnableBloomFilter = pcfg.EnableBloomFilter
	config.DB.PostIngestCompact = pcfg.PostIngestCompact

	// MaxPutThreads: explicit CLI override takes precedence over
	// every other source. Otherwise fall through to the embedded
	// ingest-config default in pkg/agi/ingest/struct.go (currently
	// 128). The historical Docker special-case of 64 was removed
	// after head-to-head pprofs showed no measurable difference
	// between 64 and 128 worker pools at the same throughput —
	// parked workers consume zero CPU, so the deeper pool is free
	// and survives single-shard commit-window stalls a little
	// better. Anyone genuinely constrained can still pin a
	// smaller value via --ingest-max-put-threads.
	if c.IngestMaxPutThreads > 0 {
		config.MaxPutThreads = c.IngestMaxPutThreads
	}

	// Pre-processor settings
	config.PreProcess.FileThreads = 6
	config.PreProcess.UnpackerFileThreads = 4

	// MaxConcurrentLogFiles: same precedence rule. Default of 0
	// in the ingest config selects the auto formula
	// (clamp(GOMAXPROCS, 4, 16) — see processLogsFeed). The
	// previous hard-coded value of 6 was removed because it
	// silently undersized parser parallelism on 16+ vCPU cloud
	// boxes (the agi pprofs had ~30% idle CPU even with the
	// downstream lock-skip + 16-shard batcher fixes in place).
	if c.IngestMaxConcurrentLogFiles > 0 {
		config.Processor.MaxConcurrentLogFiles = c.IngestMaxConcurrentLogFiles
	}

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

	if c.EnableLiveIngest {
		config.Live.Enabled = true
		config.Live.ListenAddr = "127.0.0.1:18080"
		config.DB.EnableWAL = true
		config.DB.PostIngestCompact = false
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
func (c *AgiCreateCmd) generatePluginConfig(backendType string, pcfg pebbleConfig, enableWAL bool) []byte {
	maxDp := 34560000
	if backendType == "docker" {
		maxDp = maxDp / 2
	}

	// Plugin concurrency: the post-Pebble plugin path is snapshot-
	// isolated, so multiple in-flight queries no longer compete for an
	// in-memory primary index. Bump the default fan-out so a Grafana
	// dashboard refresh with several panels is not serialized by the
	// 4-deep semaphore. On Docker the box is typically tiny — keep
	// the upstream defaults there.
	maxRequests := 16
	maxJobs := 8
	if backendType == "docker" {
		maxRequests = 4
		maxJobs = 4
	}

	cpuProfiling := ""
	if c.PluginCpuProfile {
		cpuProfiling = "cpuProfilingOutputFile: \"/opt/agi/cpu.plugin.pprof\"\n"
	}

	config := fmt.Sprintf(`maxDataPointsReceived: %d
maxConcurrentRequests: %d
maxConcurrentJobs: %d
logLevel: %d
addNoneToLabels:
  - Histogram
  - HistogramDev
  - HistogramUs
  - HistogramSize
  - HistogramCount
db:
  cacheBytes: %d
  memTableSizeBytes: %d
  maxConcurrentCompactions: %d
  memTableStopWritesThreshold: %d
  blockSize: %d
  compression: %q
  targetFileSizeL0: %d
  bytesPerSync: %d
  lBaseMaxBytes: %d
  l0StopWritesThreshold: %d
  enableBloomFilter: %t
  enableWAL: %t
%s`, maxDp, maxRequests, maxJobs, c.PluginLogLevel,
		pcfg.CacheBytes, pcfg.MemTableBytes, pcfg.MaxCompactions, pcfg.StopWritesThreshold,
		pcfg.BlockSize, pcfg.Compression,
		pcfg.TargetFileSizeL0, pcfg.BytesPerSync, pcfg.LBaseMaxBytes, pcfg.L0StopWritesThreshold,
		pcfg.EnableBloomFilter,
		enableWAL,
		cpuProfiling)

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
	notifier := map[string]any{
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
	shutdownCmd := "/usr/bin/sync; /sbin/poweroff -p || /sbin/poweroff"

	if c.ProxyDisableSSL {
		listenPort = 80
		https = false
		certFile = ""
		keyFile = ""
	}

	if backendType == "docker" {
		// Docker doesn't need poweroff
		shutdownCmd = "/usr/bin/sync"
	}

	proxyConfig := map[string]any{
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
		conf.MaxRetries = c.MaxRetries
		conf.RetrySleep = c.RetrySleep
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
		conf.MaxRetries = c.MaxRetries
		conf.RetrySleep = c.RetrySleep
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("could not create SFTP client: %w", err)
		}
		defer cli.Close()

		// Create AGI working directories (created here so they exist when EFS/volume is mounted)
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
		}
		for _, dir := range dirs {
			_ = cli.RawClient().MkdirAll(dir)
		}

		// engineTuningConfigs are configs whose contents depend on the
		// instance's available memory (memSize) and on memSize-derived
		// concurrency defaults. When the monitor reattaches the volume
		// to a different instance type for sizing, these MUST be
		// refreshed even though NoConfigOverride is set, otherwise the
		// new (larger) instance keeps running with the old (smaller)
		// instance's Pebble cache size and plugin concurrency limits,
		// leaving the extra RAM unused.
		engineTuningConfigs := map[string]bool{
			"/opt/agi/ingest.yaml": true,
			"/opt/agi/plugin.yaml": true,
		}

		// Upload each config file
		for path, content := range configs {
			// Check if we should skip due to NoConfigOverride
			if c.NoConfigOverride {
				if cli.IsExists(path) {
					if c.RefreshEngineConfigs && engineTuningConfigs[path] {
						logger.Info("Refreshing memSize-derived config: %s", path)
					} else {
						logger.Debug("Skipping existing config: %s", path)
						continue
					}
				}
			}

			perm := os.FileMode(0644)
			if strings.HasSuffix(path, ".key") || strings.HasPrefix(path, "/opt/agi/tokens/") {
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
					MaxRetries:      c.MaxRetries,
					RetrySleep:      c.RetrySleep,
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
		conf.MaxRetries = c.MaxRetries
		conf.RetrySleep = c.RetrySleep
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
				MaxRetries:      c.MaxRetries,
				RetrySleep:      c.RetrySleep,
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

// startServices starts all AGI services.
func (c *AgiCreateCmd) startServices(instance backends.InstanceList, logger *logger.Logger) error {
	logger.Debug("Enabling and starting all AGI services")

	script := `ERRORS=""
for service in grafana-server agi-plugin agi-grafanafix agi-proxy; do
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
		MaxRetries:      c.MaxRetries,
		RetrySleep:      c.RetrySleep,
	})

	var errs []string
	for _, o := range outputs {
		if o.Output.Err != nil {
			// Save script failure to local machine for debugging
			failure := scriptlog.NewScriptFailureWithPath(
				o.Instance.ClusterName,
				o.Instance.NodeNo,
				"start-services.sh",
				[]byte(script),
				o.Output.Stdout,
				o.Output.Stderr,
				o.Output.Err,
			)
			logPath, saveErr := scriptlog.SaveFailure(failure)
			if saveErr != nil {
				errs = append(errs, fmt.Sprintf("%v (stderr: %s) (failed to save logs: %v)", o.Output.Err, o.Output.Stderr, saveErr))
			} else {
				errs = append(errs, scriptlog.FormatError(logPath, o.Instance.ClusterName, o.Instance.NodeNo, o.Output.Err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to start services: %s", strings.Join(errs, "; "))
	}

	return nil
}

/* unused function
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
				if after, ok := strings.CutPrefix(part, "host="); ok {
					hostPart := after
					if colonIdx := strings.LastIndex(hostPart, ":"); colonIdx >= 0 {
						hp = hostPart[colonIdx+1:]
					}
				} else if after, ok := strings.CutPrefix(part, "container="); ok {
					cp = after
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
*/

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
		//nolint:errcheck
		os.MkdirAll(nodeDir, 0755)

		confs, err := backends.InstanceList{inst}.GetSftpConfig("root")
		if err != nil {
			logger.Warn("Could not get SFTP config for node %d: %s", inst.NodeNo, err)
			return
		}

		for _, conf := range confs {
			conf.MaxRetries = c.MaxRetries
			conf.RetrySleep = c.RetrySleep
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
					MaxRetries:      c.MaxRetries,
					RetrySleep:      c.RetrySleep,
				})
				if len(outputs) > 0 && outputs[0].Output.Err == nil {
					//nolint:errcheck
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
		MaxRetries:      c.MaxRetries,
		RetrySleep:      c.RetrySleep,
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

/* unused function
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
	if before, ok := strings.CutSuffix(fqdn, "."+domainName); ok {
		dnsName = before
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
*/

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
					if after, ok := strings.CutPrefix(part, "host="); ok {
						hostPart := after
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

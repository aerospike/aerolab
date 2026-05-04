//go:build !noagi

package cmd

import (
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/notifier"
	flags "github.com/rglonek/go-flags"
)

// AgiMonitorConfigCmd contains all runtime configuration for the AGI monitor
// logic — sizing thresholds, notifications, capacity rotation, etc. These
// fields are relevant both when running the standalone listener (agi monitor
// listen) and when the monitor is embedded inside the WebUI (webui
// --agi-monitor-enable). Server/listener fields (address, TLS, autocert) live
// in AgiMonitorListenCmd instead.
//
// Sizing model (post-Pebble migration, post-budget-cap update):
//
//	required_RAM_GB = RAMFloorGB
//	                 + CachePeakMultiplier * cache_target_GB   // see below
//	                 + RAMThresMinFreeGB                       // operator-visible headroom
//
//	cache_target_GB = clamp(CacheTargetPct/100 * logSize_GB, CacheMinGB, CacheMaxGB)
//
// The CachePeakMultiplier (default 4) reflects the post-budget-cap
// allocation in cmdAgiCreate: Pebble is hard-capped at 50% of total
// host RAM, of which roughly half goes to the block cache and half
// to peak memtable RAM. So for the cache to actually reach
// `cache_target_GB`, the host needs at least 4 × cache_target_GB
// just for the Pebble portion. RAMFloorGB on top of that covers the
// OS reservation, the merged process's non-Pebble overhead
// (ingest/plugin/Go/Grafana), and is sized to mirror the
// agiOSReserveBytes + agiNonPebbleOverheadBytes constants in
// cmdAgiCreate. Keep the two in lockstep or the monitor's
// pre-process sizing will mis-predict.
//
// Worked examples (cloud, with defaults RAMFloorGB=10, multiplier=4,
// MinFree=2):
//
//	  9 GiB logs (cache_target=1):   10 + 4   + 2 = 16 GiB → m7i.xlarge
//	100 GiB logs (cache_target=10):  10 + 40  + 2 = 52 GiB → r7a.2xlarge / 64 GiB
//	500 GiB logs (cache_target=16):  10 + 64  + 2 = 76 GiB → SizingMaxRamGB caps at 48 GiB; warned
//
// The old DIM/NoDIM split is gone — there is no in-memory primary index
// anymore; everything serves out of the Pebble LSM with a configurable block
// cache plus the OS page cache. The DIM-specific flags below
// (DimMultiplier, NoDimMultiplier, SizingNoDIMFirst) are accepted for
// backward compatibility with old monitor YAML / CLI invocations but are
// no longer consulted by the sizing logic.
type AgiMonitorConfigCmd struct {
	// TLS verification for callbacks to AGI instances
	StrictAGITLS bool `long:"strict-agi-tls" description:"If set, AGI-Monitor will expect AGI instances to have a valid TLS certificate"`

	// Disk sizing thresholds (GCP)
	GCPDiskThresholdPct int `long:"gcp-disk-thres-pct" description:"Usage threshold pct at which the disk will be increased" default:"80"`
	GCPDiskIncreaseGB   int `long:"gcp-disk-grow-gb" description:"When threshold is breached, grow by these many GB" default:"100"`

	// RAM sizing thresholds (live monitoring; threshold-driven sizing).
	// Threshold-driven sizing is rare in the Pebble model because the steady
	// state is heavily disk-resident; the bigger trigger is the
	// pre-process completion event, which uses the formula above directly.
	RAMThresUsedPct   int `long:"ram-thres-used-pct" description:"Max used PCT of RAM (MemAvailable-based) before instance gets sized" default:"90"`
	RAMThresMinFreeGB int `long:"ram-thres-minfree-gb" description:"Minimum free GB of RAM before instance gets sized; doubles as 'headroom' in the pre-process sizing formula" default:"2"`

	// New sizing model knobs (Pebble).
	//
	// RAMFloorGB default 10 = 6 GiB OS reserve (matches
	// agiOSReserveBytes("aws"|"gcp")) + 4 GiB merged-process
	// non-Pebble overhead (matches agiNonPebbleOverheadBytes) in
	// cmdAgiCreate.go. CachePeakMultiplier default 4 matches the
	// "Pebble = 50% of host RAM, half cache + half memtables"
	// budget split also in cmdAgiCreate.go. Keep these defaults in
	// lockstep with cmdAgiCreate's constants or pre-process sizing
	// will under- or over-provision relative to what AGI actually
	// allocates.
	RAMFloorGB           int `long:"ram-floor-gb" description:"Always-on baseline RAM (OS reserve + merged-process non-Pebble overhead). Added on top of (multiplier × cache target) and headroom" default:"10"`
	CachePeakMultiplier  int `long:"cache-peak-multiplier" description:"Multiplier on the Pebble cache target to predict total host RAM needed; 4 reflects 'Pebble = 50% of host, half cache + half memtables'" default:"4"`
	CacheTargetPct       int `long:"cache-target-pct" description:"Pebble block cache target as % of LogProcessorTotalSize; combined with the OS page cache, this is the 'fast in-memory query' surface" default:"10"`
	CacheMinGB           int `long:"cache-min-gb" description:"Floor on the Pebble block cache target, even for small ingests" default:"1"`
	CacheMaxGB           int `long:"cache-max-gb" description:"Ceiling on the Pebble block cache target; beyond this the OS page cache is just as effective and cheaper" default:"16"`

	// Sizing options
	DisableSizing   bool `long:"sizing-disable" description:"Set to disable sizing of instances for more resources"`
	SizingMaxRamGB  int  `long:"sizing-max-ram-gb" description:"Will not size above these many GB" default:"48"`
	SizingMaxDiskGB int  `long:"sizing-max-disk-gb" description:"Will not size above these many GB" default:"400"`

	// Spot capacity rotation
	DisableCapacity bool `long:"capacity-disable" description:"Set to disable rotation of spot instances with capacity issues to ondemand"`

	// Deprecated: DIM mode is gone in the Pebble backend. Kept on the
	// struct for backward compatibility with existing YAML/CLI; the
	// fields are no longer read by the sizing logic.
	SizingNoDIMFirst bool    `long:"sizing-nodim" description:"DEPRECATED: DIM mode no longer exists post-Pebble migration; flag is ignored" hidden:"true"`
	DimMultiplier    float64 `long:"sizing-multiplier-dim" description:"DEPRECATED: pre-Pebble DIM RAM multiplier; flag is ignored" hidden:"true"`
	NoDimMultiplier  float64 `long:"sizing-multiplier-nodim" description:"DEPRECATED: pre-Pebble NoDIM RAM multiplier; flag is ignored" hidden:"true"`

	// Debugging
	DebugEvents bool `long:"debug-events" description:"Log all events for debugging purposes"`

	// Pricing API
	DisablePricingAPI bool `long:"disable-pricing-api" description:"Set to disable pricing queries for cost tracking"`

	// Notifications
	NotifyURL    string `long:"notify-url" description:"Optional: specify a notification URL to send action notifications to"`
	NotifyHeader string `long:"notify-header" description:"Optional: set a header in the notification; format: key=value"`
	SlackToken   string `long:"notify-slack-token" description:"Set to enable slack notifications for events"`
	SlackChannel string `long:"notify-slack-channel" description:"Set to the channel to notify to"`

	// Internal state (not exposed as flags)
	invLock  *sync.Mutex           `yaml:"-" json:"-"`
	execLock *sync.Mutex           `yaml:"-" json:"-"`
	notifier *notifier.HTTPSNotify `yaml:"-" json:"-"`
}

// AgiMonitorListenCmd starts the monitor listener that handles AGI instance events.
// The monitor provides auto-sizing, spot instance rotation, and notification forwarding.
//
// The monitor implements a challenge-response authentication system:
//  1. AGI instance sends notification with secret to monitor
//  2. Monitor calls back to AGI /agi/monitor-challenge endpoint
//  3. Validates secret matches before processing event
//
// Supported events:
//   - SPOT_INSTANCE_CAPACITY_SHUTDOWN: Rotate spot to on-demand
//   - SYS_RESOURCE_USAGE_MONITOR: Check RAM/disk sizing
//   - INGEST_STEP_*: Check sizing after processing steps
//   - SERVICE_DOWN: Handle crashed services
type AgiMonitorListenCmd struct {
	// Server configuration
	ListenAddress string `long:"address" description:"Address to listen on; if autocert is enabled, will also listen on :80" default:"0.0.0.0:443"`
	NoTLS         bool   `long:"no-tls" description:"Disable TLS"`

	// TLS configuration
	AutoCertDomains []string `long:"autocert" description:"TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains, can be used more than once"`
	AutoCertEmail   string   `long:"autocert-email" description:"TLS: if autocert is specified, specify a valid email address to use with letsencrypt"`
	CertFile        string   `long:"cert-file" description:"TLS: certificate file to use if not using letsencrypt; default: generate self-signed"`
	KeyFile         string   `long:"key-file" description:"TLS: key file to use if not using letsencrypt; default: generate self-signed"`

	// Embed monitor config (sizing thresholds, notifications, etc.)
	AgiMonitorConfigCmd

	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// AgiMonitorCreateCmd creates a monitor instance on AWS or GCP.
// The monitor is deployed as a client instance with an instance role that allows
// it to manage other AGI instances in the same region/project.
//
// The monitor will:
//  1. Create a client instance with the appropriate IAM role
//  2. Install aerolab binary
//  3. Configure systemd service (agimonitor.service)
//  4. Apply aerolab-agi firewall for ports 80/443
//  5. Start and enable the monitor listener service
type AgiMonitorCreateCmd struct {
	Name  string `short:"n" long:"name" description:"Monitor client name" default:"agimonitor"`
	Owner string `long:"owner" description:"AWS/GCP only: create owner tag with this value"`

	// Embed listen command flags for configuration
	AgiMonitorListenCmd

	// Aerolab binary option
	AerolabBinary flags.Filename `long:"aerolab-binary" description:"Path to local aerolab binary to install (required if running unofficial build)"`

	// AWS-specific options
	AWS AgiMonitorCreateCmdAws `group:"AWS" namespace:"aws" description:"backend-aws"`

	// GCP-specific options
	GCP AgiMonitorCreateCmdGcp `group:"GCP" namespace:"gcp" description:"backend-gcp"`

	// Retry configuration
	MaxRetries int           `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep time.Duration `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`

	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// AgiMonitorCreateCmdAws contains AWS-specific options for monitor creation.
type AgiMonitorCreateCmdAws struct {
	InstanceType      guiInstanceType `long:"instance" description:"Instance type to use" default:"t3a.medium" webchoice:"method::List"`
	SecurityGroupID   string        `short:"S" long:"secgroup-id" description:"Security group IDs to use, comma-separated; default: empty: create and auto-manage"`
	SubnetID          string        `short:"U" long:"subnet-id" description:"Subnet-id, availability-zone name, or empty; default: empty: first found in default VPC"`
	NamePrefix        []string      `long:"secgroup-name" description:"Name prefix to use for the security groups, can be specified multiple times" default:"AeroLab"`
	InstanceRole      string        `long:"role" description:"Instance role to assign to the instance; the role must allow at least EC2 and EFS access; and must be manually precreated" default:"agimonitor"`
	ElasticIP         string        `long:"elastic-ip" description:"Pre-allocated Elastic IP to associate with the monitor instance; can be the allocation ID (eipalloc-xxx) or the IP address itself; useful when DNS is already configured to point to this IP for autocert"`
	Route53ZoneId     string        `long:"route53-zoneid" description:"If set, will automatically update a route53 DNS domain with the monitor URL; expiry system will also be updated accordingly"`
	Route53DomainName string        `long:"route53-fqdn" description:"The route domain the zone refers to; eg monitor.eu-west-1.myagi.org"`
	Expires           TypeExpiry     `long:"expire" description:"Instance expiry (0 for never)" default:"0"`
	// AWS credentials - alternative to using instance profile
	AWSKeyId     string `long:"key-id" description:"AWS Access Key ID; alternative to using --role instance profile; use ENV::VARNAME to read from environment variable"`
	AWSSecretKey string `long:"secret-key" description:"AWS Secret Access Key; alternative to using --role instance profile; use ENV::VARNAME to read from environment variable"`
}

// AgiMonitorCreateCmdGcp contains GCP-specific options for monitor creation.
type AgiMonitorCreateCmdGcp struct {
	InstanceType guiInstanceType `long:"instance" description:"Instance type to use" default:"e2-medium" webchoice:"method::List"`
	Zone         guiZone         `long:"zone" description:"Zone name to deploy to" webrequired:"true" webchoice:"method::List"`
	NamePrefix   []string      `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external"`
	InstanceRole string        `long:"role" description:"Instance role to assign to the instance; the role must allow at least compute access; and must be manually precreated" default:"agimonitor"`
	Expires      TypeExpiry     `long:"expire" description:"Instance expiry (0 for never)" default:"0"`
}

// Monitor notification constants
const (
	agiMonitorNotifyActionSpotCapacity = "spot-capacity"
	agiMonitorNotifyActionRAM          = "sizing-ram"
	agiMonitorNotifyActionDisk         = "sizing-disk"
	agiMonitorNotifyActionDiskRAM      = "sizing-disk-ram"
	agiMonitorNotifyStageStart         = "start"
	agiMonitorNotifyStageDone          = "done"
	agiMonitorNotifyStageError         = "error"
)

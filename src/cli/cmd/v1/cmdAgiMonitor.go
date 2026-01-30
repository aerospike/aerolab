package cmd

import (
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/notifier"
	flags "github.com/rglonek/go-flags"
)

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
	StrictAGITLS  bool   `long:"strict-agi-tls" description:"If set, AGI-Monitor will expect AGI instances to have a valid TLS certificate"`

	// TLS configuration
	AutoCertDomains []string `long:"autocert" description:"TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains, can be used more than once"`
	AutoCertEmail   string   `long:"autocert-email" description:"TLS: if autocert is specified, specify a valid email address to use with letsencrypt"`
	CertFile        string   `long:"cert-file" description:"TLS: certificate file to use if not using letsencrypt; default: generate self-signed"`
	KeyFile         string   `long:"key-file" description:"TLS: key file to use if not using letsencrypt; default: generate self-signed"`

	// Disk sizing thresholds (GCP)
	GCPDiskThresholdPct int `long:"gcp-disk-thres-pct" description:"Usage threshold pct at which the disk will be increased" default:"80"`
	GCPDiskIncreaseGB   int `long:"gcp-disk-grow-gb" description:"When threshold is breached, grow by these many GB" default:"100"`

	// RAM sizing thresholds
	RAMThresUsedPct   int `long:"ram-thres-used-pct" description:"Max used PCT of RAM before instance gets sized" default:"95"`
	RAMThresMinFreeGB int `long:"ram-thres-minfree-gb" description:"Minimum free GB of RAM before instance gets sized" default:"8"`

	// Sizing options
	SizingNoDIMFirst bool `long:"sizing-nodim" description:"If set, the system will first stop using data-in-memory as a sizing option before resorting to changing instance sizes"`
	DisableSizing    bool `long:"sizing-disable" description:"Set to disable sizing of instances for more resources"`
	SizingMaxRamGB   int  `long:"sizing-max-ram-gb" description:"Will not size above these many GB" default:"130"`
	SizingMaxDiskGB  int  `long:"sizing-max-disk-gb" description:"Will not size above these many GB" default:"400"`

	// Spot capacity rotation
	DisableCapacity bool `long:"capacity-disable" description:"Set to disable rotation of spot instances with capacity issues to ondemand"`

	// RAM multipliers for sizing calculations
	DimMultiplier   float64 `long:"sizing-multiplier-dim" description:"Log size * multiplier = how much RAM is needed" default:"1.8"`
	NoDimMultiplier float64 `long:"sizing-multiplier-nodim" description:"Log size * multiplier = how much RAM is needed" default:"0.4"`

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

	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// AgiMonitorCreateCmdAws contains AWS-specific options for monitor creation.
type AgiMonitorCreateCmdAws struct {
	InstanceType      string        `long:"instance" description:"Instance type to use" default:"t3a.medium"`
	SecurityGroupID   string        `short:"S" long:"secgroup-id" description:"Security group IDs to use, comma-separated; default: empty: create and auto-manage"`
	SubnetID          string        `short:"U" long:"subnet-id" description:"Subnet-id, availability-zone name, or empty; default: empty: first found in default VPC"`
	NamePrefix        []string      `long:"secgroup-name" description:"Name prefix to use for the security groups, can be specified multiple times" default:"AeroLab"`
	InstanceRole      string        `long:"role" description:"Instance role to assign to the instance; the role must allow at least EC2 and EFS access; and must be manually precreated" default:"agimonitor"`
	ElasticIP         string        `long:"elastic-ip" description:"Pre-allocated Elastic IP to associate with the monitor instance; can be the allocation ID (eipalloc-xxx) or the IP address itself; useful when DNS is already configured to point to this IP for autocert"`
	Route53ZoneId     string        `long:"route53-zoneid" description:"If set, will automatically update a route53 DNS domain with the monitor URL; expiry system will also be updated accordingly"`
	Route53DomainName string        `long:"route53-fqdn" description:"The route domain the zone refers to; eg monitor.eu-west-1.myagi.org"`
	Expires           time.Duration `long:"expire" description:"Instance expiry (0 for never)" default:"0"`
	// AWS credentials - alternative to using instance profile
	AWSKeyId     string `long:"key-id" description:"AWS Access Key ID; alternative to using --role instance profile; use ENV::VARNAME to read from environment variable"`
	AWSSecretKey string `long:"secret-key" description:"AWS Secret Access Key; alternative to using --role instance profile; use ENV::VARNAME to read from environment variable"`
}

// AgiMonitorCreateCmdGcp contains GCP-specific options for monitor creation.
type AgiMonitorCreateCmdGcp struct {
	InstanceType string        `long:"instance" description:"Instance type to use" default:"e2-medium"`
	Zone         string        `long:"zone" description:"Zone name to deploy to" webrequired:"true"`
	NamePrefix   []string      `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external"`
	InstanceRole string        `long:"role" description:"Instance role to assign to the instance; the role must allow at least compute access; and must be manually precreated" default:"agimonitor"`
	Expires      time.Duration `long:"expire" description:"Instance expiry (0 for never)" default:"0"`
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

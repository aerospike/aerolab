//go:build noagi

package cmd

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	flags "github.com/rglonek/go-flags"
)

const ClusterFeatureAGI = 1

func GetSSHAuthorizedKeysGzB64() string { return "" }

func PutSSHAuthorizedKeys(_ string) {}

var errNoAGI = fmt.Errorf("AGI is not available in this build")

// generateRandomToken is defined in cmdAgiAddToken.go for full builds; duplicated here for noagi
// because cmdWebUIAgiTokens.go calls it.
func generateRandomToken(n int, src rand.Source) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const (
		letterIdxBits = 6
		letterIdxMask = 1<<letterIdxBits - 1
		letterIdxMax  = 63 / letterIdxBits
	)

	sb := strings.Builder{}
	sb.Grow(n)

	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			sb.WriteByte(letterBytes[idx])
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return sb.String()
}

// --- Root AGI command (tags from cmdAgi.go) ---

type AgiCmd struct {
	Template  AgiTemplateCmd  `command:"template" subcommands-optional:"true" description:"AGI template management" webicon:"fas fa-file-image"`
	Create    AgiCreateCmd    `command:"create" subcommands-optional:"true" description:"Create AGI instance" webicon:"fas fa-plus" invwebforce:"true"`
	List      AgiListCmd      `command:"list" subcommands-optional:"true" description:"List AGI instances" webicon:"fas fa-list"`
	Start     AgiStartCmd     `command:"start" subcommands-optional:"true" description:"Start AGI instance" webicon:"fas fa-play" invwebforce:"true"`
	Stop      AgiStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop AGI instance" webicon:"fas fa-stop" invwebforce:"true"`
	Status    AgiStatusCmd    `command:"status" subcommands-optional:"true" description:"Show AGI status" webicon:"fas fa-info-circle"`
	Details   AgiDetailsCmd   `command:"details" subcommands-optional:"true" description:"Show ingest details" webicon:"fas fa-magnifying-glass"`
	Destroy   AgiDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy AGI instance" webicon:"fas fa-trash" invwebforce:"true"`
	Delete    AgiDeleteCmd    `command:"delete" subcommands-optional:"true" description:"Destroy instance and volume" webicon:"fas fa-trash-can" invwebforce:"true"`
	Attach    AgiAttachCmd    `command:"attach" subcommands-optional:"true" description:"Attach to AGI shell" webicon:"fas fa-terminal"`
	Open      AgiOpenCmd      `command:"open" subcommands-optional:"true" description:"Open AGI in browser" webicon:"fas fa-globe"`
	AddToken  AgiAddTokenCmd  `command:"add-auth-token" subcommands-optional:"true" description:"Add auth token" webicon:"fas fa-key"`
	Relabel   AgiRelabelCmd   `command:"change-label" subcommands-optional:"true" description:"Change label" webicon:"fas fa-tag"`
	Retrigger AgiRetriggerCmd `command:"run-ingest" subcommands-optional:"true" description:"Retrigger ingest" webicon:"fas fa-rotate"`
	Share     AgiShareCmd     `command:"share" subcommands-optional:"true" description:"Share via SSH key" webicon:"fas fa-share"`
	Monitor   AgiMonitorCmd   `command:"monitor" subcommands-optional:"true" description:"Monitor system" webicon:"fas fa-gauge"`
	Exec      AgiExecCmd      `command:"exec" subcommands-optional:"true" description:"AGI subsystems" hidden:"true"`
	Help      HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiCmd) Execute(args []string) error { return errNoAGI }

type AgiTemplateCmd struct {
	Create  AgiTemplateCreateCmd  `command:"create" subcommands-optional:"true" description:"Create AGI template" webicon:"fas fa-plus" invwebforce:"true"`
	List    AgiTemplateListCmd    `command:"list" subcommands-optional:"true" description:"List AGI templates" webicon:"fas fa-list"`
	Destroy AgiTemplateDestroyCmd `command:"destroy" subcommands-optional:"true" description:"Destroy AGI template" webicon:"fas fa-trash" invwebforce:"true"`
	Vacuum  AgiTemplateVacuumCmd  `command:"vacuum" subcommands-optional:"true" description:"Clean dangling templates" webicon:"fas fa-broom"`
	Help    HelpCmd               `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiTemplateCmd) Execute(args []string) error { return errNoAGI }

type AgiMonitorCmd struct {
	Create AgiMonitorCreateCmd `command:"create" subcommands-optional:"true" description:"Create monitor instance" webicon:"fas fa-plus" invwebforce:"true"`
	Listen AgiMonitorListenCmd `command:"listen" subcommands-optional:"true" description:"Start monitor listener" webicon:"fas fa-headphones"`
	Help   HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiMonitorCmd) Execute(args []string) error { return errNoAGI }

type AgiExecCmd struct {
	Plugin       AgiExecPluginCmd       `command:"plugin" subcommands-optional:"true" description:"Run plugin backend"`
	GrafanaFix   AgiExecGrafanaFixCmd   `command:"grafanafix" subcommands-optional:"true" description:"Run Grafana helper"`
	Ingest       AgiExecIngestCmd       `command:"ingest" subcommands-optional:"true" description:"Run ingest service"`
	Proxy        AgiExecProxyCmd        `command:"proxy" subcommands-optional:"true" description:"Run web proxy"`
	IngestStatus AgiExecIngestStatusCmd `command:"ingest-status" subcommands-optional:"true" description:"Get ingest status"`
	IngestDetail AgiExecIngestDetailCmd `command:"ingest-detail" subcommands-optional:"true" description:"Get ingest details"`
	Simulate     AgiExecSimulateCmd     `command:"simulate" subcommands-optional:"true" description:"Simulate spot termination"`
	Help         HelpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecCmd) Execute(args []string) error { return errNoAGI }

type AgiMonitorConfigCmd struct {
	StrictAGITLS bool `long:"strict-agi-tls" description:"If set, AGI-Monitor will expect AGI instances to have a valid TLS certificate"`

	GCPDiskThresholdPct int `long:"gcp-disk-thres-pct" description:"Usage threshold pct at which the disk will be increased" default:"80"`
	GCPDiskIncreaseGB   int `long:"gcp-disk-grow-gb" description:"When threshold is breached, grow by these many GB" default:"100"`

	RAMThresUsedPct   int `long:"ram-thres-used-pct" description:"Max used PCT of RAM before instance gets sized" default:"95"`
	RAMThresMinFreeGB int `long:"ram-thres-minfree-gb" description:"Minimum free GB of RAM before instance gets sized" default:"8"`

	SizingNoDIMFirst bool `long:"sizing-nodim" description:"If set, the system will first stop using data-in-memory as a sizing option before resorting to changing instance sizes"`
	DisableSizing    bool `long:"sizing-disable" description:"Set to disable sizing of instances for more resources"`
	SizingMaxRamGB   int  `long:"sizing-max-ram-gb" description:"Will not size above these many GB" default:"130"`
	SizingMaxDiskGB  int  `long:"sizing-max-disk-gb" description:"Will not size above these many GB" default:"400"`

	DisableCapacity bool `long:"capacity-disable" description:"Set to disable rotation of spot instances with capacity issues to ondemand"`

	DimMultiplier   float64 `long:"sizing-multiplier-dim" description:"Log size * multiplier = how much RAM is needed" default:"1.8"`
	NoDimMultiplier float64 `long:"sizing-multiplier-nodim" description:"Log size * multiplier = how much RAM is needed" default:"0.4"`

	DebugEvents bool `long:"debug-events" description:"Log all events for debugging purposes"`

	DisablePricingAPI bool `long:"disable-pricing-api" description:"Set to disable pricing queries for cost tracking"`

	NotifyURL    string `long:"notify-url" description:"Optional: specify a notification URL to send action notifications to"`
	NotifyHeader string `long:"notify-header" description:"Optional: set a header in the notification; format: key=value"`
	SlackToken   string `long:"notify-slack-token" description:"Set to enable slack notifications for events"`
	SlackChannel string `long:"notify-slack-channel" description:"Set to the channel to notify to"`

	invLock  any `yaml:"-" json:"-"`
	execLock any `yaml:"-" json:"-"`
	notifier any `yaml:"-" json:"-"`
}

type AgiMonitorListenCmd struct {
	ListenAddress   string   `long:"address" description:"Address to listen on; if autocert is enabled, will also listen on :80" default:"0.0.0.0:443"`
	NoTLS           bool     `long:"no-tls" description:"Disable TLS"`
	AutoCertDomains []string `long:"autocert" description:"TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains, can be used more than once"`
	AutoCertEmail   string   `long:"autocert-email" description:"TLS: if autocert is specified, specify a valid email address to use with letsencrypt"`
	CertFile        string   `long:"cert-file" description:"TLS: certificate file to use if not using letsencrypt; default: generate self-signed"`
	KeyFile         string   `long:"key-file" description:"TLS: key file to use if not using letsencrypt; default: generate self-signed"`
	AgiMonitorConfigCmd
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiMonitorListenCmd) Execute(args []string) error { return errNoAGI }

type AgiMonitorCreateCmd struct {
	Name  string `short:"n" long:"name" description:"Monitor client name" default:"agimonitor"`
	Owner string `long:"owner" description:"AWS/GCP only: create owner tag with this value"`
	AgiMonitorListenCmd
	AerolabBinary flags.Filename         `long:"aerolab-binary" description:"Path to local aerolab binary to install (required if running unofficial build)"`
	AWS           AgiMonitorCreateCmdAws `group:"AWS" namespace:"aws" description:"backend-aws"`
	GCP           AgiMonitorCreateCmdGcp `group:"GCP" namespace:"gcp" description:"backend-gcp"`
	MaxRetries    int                    `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep    time.Duration          `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`
	Help          HelpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiMonitorCreateCmd) Execute(args []string) error { return errNoAGI }

type AgiMonitorCreateCmdAws struct {
	InstanceType      guiInstanceType `long:"instance" description:"Instance type to use" default:"t3a.medium" webchoice:"method::List"`
	SecurityGroupID   string          `short:"S" long:"secgroup-id" description:"Security group IDs to use, comma-separated; default: empty: create and auto-manage"`
	SubnetID          string          `short:"U" long:"subnet-id" description:"Subnet-id, availability-zone name, or empty; default: empty: first found in default VPC"`
	NamePrefix        []string        `long:"secgroup-name" description:"Name prefix to use for the security groups, can be specified multiple times" default:"AeroLab"`
	InstanceRole      string          `long:"role" description:"Instance role to assign to the instance; the role must allow at least EC2 and EFS access; and must be manually precreated" default:"agimonitor"`
	ElasticIP         string          `long:"elastic-ip" description:"Pre-allocated Elastic IP to associate with the monitor instance; can be the allocation ID (eipalloc-xxx) or the IP address itself; useful when DNS is already configured to point to this IP for autocert"`
	Route53ZoneId     string          `long:"route53-zoneid" description:"If set, will automatically update a route53 DNS domain with the monitor URL; expiry system will also be updated accordingly"`
	Route53DomainName string          `long:"route53-fqdn" description:"The route domain the zone refers to; eg monitor.eu-west-1.myagi.org"`
	Expires           TypeExpiry      `long:"expire" description:"Instance expiry (0 for never)" default:"0"`
	AWSKeyId          string          `long:"key-id" description:"AWS Access Key ID; alternative to using --role instance profile; use ENV::VARNAME to read from environment variable"`
	AWSSecretKey      string          `long:"secret-key" description:"AWS Secret Access Key; alternative to using --role instance profile; use ENV::VARNAME to read from environment variable"`
}

type AgiMonitorCreateCmdGcp struct {
	InstanceType guiInstanceType `long:"instance" description:"Instance type to use" default:"e2-medium" webchoice:"method::List"`
	Zone         guiZone         `long:"zone" description:"Zone name to deploy to" webrequired:"true" webchoice:"method::List"`
	NamePrefix   []string        `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external"`
	InstanceRole string          `long:"role" description:"Instance role to assign to the instance; the role must allow at least compute access; and must be manually precreated" default:"agimonitor"`
	Expires      TypeExpiry      `long:"expire" description:"Instance expiry (0 for never)" default:"0"`
}

type Reattach struct {
	InstanceTypeOverride string `long:"instance-type" description:"Override instance type when reattaching"`
	NoDIMOverride        *bool  `long:"nodim" description:"Override data-in-memory setting when reattaching" no-default:"true"`
	SpotOverride         *bool  `long:"spot" description:"Override spot instance setting when reattaching" no-default:"true"`
	OwnerOverride        string `long:"owner" description:"Override owner tag when reattaching"`
}

type AgiStartCmdAws struct {
	EFSName string `long:"efs-name" description:"EFS volume name pattern (default uses AGI name)" default:"{AGI_NAME}"`
}

type AgiStartCmdGcp struct {
	VolName string `long:"vol-name" description:"Volume name pattern (default uses AGI name)" default:"{AGI_NAME}"`
}

type AgiCreateCmdAws struct{}

type AgiCreateCmdGcp struct{}

type AgiCreateCmdDocker struct{}

type AgiCreateCmd struct {
	AWS    AgiCreateCmdAws    `group:"AWS" namespace:"aws" description:"backend-aws"`
	GCP    AgiCreateCmdGcp    `group:"GCP" namespace:"gcp" description:"backend-gcp"`
	Docker AgiCreateCmdDocker `group:"Docker" namespace:"docker" description:"backend-docker"`
	Help   HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiCreateCmd) Execute(args []string) error { return errNoAGI }

type AgiStartCmd struct {
	Reattach Reattach       `group:"Reattach" namespace:"reattach" description:"reattach options"`
	AWS      AgiStartCmdAws `group:"AWS" namespace:"aws" description:"backend-aws"`
	GCP      AgiStartCmdGcp `group:"GCP" namespace:"gcp" description:"backend-gcp"`
	Help     HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiStartCmd) Execute(args []string) error { return errNoAGI }

type AgiListCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiListCmd) Execute(args []string) error { return errNoAGI }

type AgiStopCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiStopCmd) Execute(args []string) error { return errNoAGI }

type AgiStatusCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiStatusCmd) Execute(args []string) error { return errNoAGI }

type AgiDetailsCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiDetailsCmd) Execute(args []string) error { return errNoAGI }

type AgiDestroyCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiDestroyCmd) Execute(args []string) error { return errNoAGI }

type AgiDeleteCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiDeleteCmd) Execute(args []string) error { return errNoAGI }

type AgiAttachCmd struct {
	Help AgiAttachCmdHelp `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiAttachCmd) Execute(args []string) error { return errNoAGI }

type AgiAttachCmdHelp struct{}

func (c *AgiAttachCmdHelp) Execute(args []string) error { return errNoAGI }

type AgiOpenCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiOpenCmd) Execute(args []string) error { return errNoAGI }

type AgiAddTokenCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiAddTokenCmd) Execute(args []string) error { return errNoAGI }

type AgiRelabelCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiRelabelCmd) Execute(args []string) error { return errNoAGI }

type AgiRetriggerCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiRetriggerCmd) Execute(args []string) error { return errNoAGI }

type AgiShareCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiShareCmd) Execute(args []string) error { return errNoAGI }

type AgiTemplateCreateCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiTemplateCreateCmd) Execute(args []string) error { return errNoAGI }

type AgiTemplateListCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiTemplateListCmd) Execute(args []string) error { return errNoAGI }

type AgiTemplateDestroyCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiTemplateDestroyCmd) Execute(args []string) error { return errNoAGI }

type AgiTemplateVacuumCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiTemplateVacuumCmd) Execute(args []string) error { return errNoAGI }

type AgiExecPluginCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecPluginCmd) Execute(args []string) error { return errNoAGI }

type AgiExecGrafanaFixCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecGrafanaFixCmd) Execute(args []string) error { return errNoAGI }

type AgiExecIngestCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecIngestCmd) Execute(args []string) error { return errNoAGI }

type AgiExecProxyCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecProxyCmd) Execute(args []string) error { return errNoAGI }

type AgiExecIngestStatusCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecIngestStatusCmd) Execute(args []string) error { return errNoAGI }

type AgiExecIngestDetailCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecIngestDetailCmd) Execute(args []string) error { return errNoAGI }

type AgiExecSimulateCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecSimulateCmd) Execute(args []string) error { return errNoAGI }

type AgiStatusOutput struct{}

type AgiServiceStatus struct{}

type AgiSystemStatus struct{}

type AgiIngestStatus struct{}

type AgiDetailsOutput struct{}

type AgiIngestSteps struct{}

type AgiDownloadProgress struct{}

type AgiProcessProgress struct{}

type AgiError struct{}

type AgiListOutput struct{}

type AgiVolumeOutput struct{}

type AgiListFullOutput struct{}

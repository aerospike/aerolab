package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "embed"

	"github.com/aerospike/aerolab/gcplabels"
	"github.com/aerospike/aerolab/ingest"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/bestmethod/inslice"
	"github.com/lithammer/shortuuid"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
	flags "github.com/rglonek/jeddevdk-goflags"
	"gopkg.in/yaml.v3"
)

// copy of notier.HTTPSNotify
type hTTPSNotify struct {
	AGIMonitorUrl        string   `long:"agi-monitor-url" description:"AWS/GCP: AGI Monitor endpoint url to send the notifications to for sizing" yaml:"agiMonitor" simplemode:"false"`
	AGIMonitorCertIgnore bool     `long:"agi-monitor-ignore-cert" description:"set to make https calls ignore invalid server certificate" simplemode:"false"`
	Endpoint             string   `long:"notify-web-endpoint" description:"http(s) URL to contact with a notification" yaml:"endpoint" simplemode:"false"`
	Headers              []string `long:"notify-web-header" description:"a header to set for notification; for example to use Authorization tokens; format: Name=value" yaml:"headers" simplemode:"false"`
	AbortOnFail          bool     `long:"notify-web-abort-on-fail" description:"if set, ingest will be aborted if the notification system receives an error response or no response" yaml:"abortOnFail" simplemode:"false"`
	AbortOnCode          []int    `long:"notify-web-abort-code" description:"set to status codes on which to abort the operation" yaml:"abortStatusCodes" simplemode:"false"`
	IgnoreInvalidCert    bool     `long:"notify-web-ignore-cert" description:"set to make https calls ignore invalid server certificate" simplemode:"false"`
	SlackToken           string   `long:"notify-slack-token" description:"set to enable slack notifications for events" simplemode:"false"`
	SlackChannel         string   `long:"notify-slack-channel" description:"set to the channel to notify to" simplemode:"false"`
	SlackEvents          string   `long:"notify-slack-events" description:"comma-separated list of events to notify for" default:"INGEST_FINISHED,SERVICE_DOWN,SERVICE_UP,MAX_AGE_REACHED,MAX_INACTIVITY_REACHED,SPOT_INSTANCE_CAPACITY_SHUTDOWN" simplemode:"false"`
}

type agiCreateCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	AGILabel         string          `long:"agi-label" description:"friendly label"`
	NoDIM            bool            `long:"no-dim" description:"set to disable data-in-memory and enable read-page-cache in aerospike; much less RAM used, but slower"`
	NoDIMFileSize    int             `long:"no-dim-filesize" description:"if using --no-dim, optionally specify a filesize in GB for data storage; default: memory size calculation"`
	ClusterSource    TypeClusterName `long:"source-cluster" description:"cluster name to use as the source for the AGI"`
	LocalSource      flags.Filename  `long:"source-local" description:"get logs from a local directory"`
	SftpEnable       bool            `long:"source-sftp-enable" description:"enable sftp source" simplemode:"false"`
	SftpThreads      int             `long:"source-sftp-threads" description:"number of concurrent downloader threads" default:"1" simplemode:"false"`
	SftpHost         string          `long:"source-sftp-host" description:"sftp host" simplemode:"false"`
	SftpPort         int             `long:"source-sftp-port" description:"sftp port" default:"22" simplemode:"false"`
	SftpUser         string          `long:"source-sftp-user" description:"sftp user" simplemode:"false"`
	SftpPass         string          `long:"source-sftp-pass" description:"sftp password" webtype:"password" simplemode:"false"`
	SftpKey          flags.Filename  `long:"source-sftp-key" description:"key to use for sftp login for log download, alternative to password" simplemode:"false"`
	SftpPath         string          `long:"source-sftp-path" description:"path on sftp to download logs from" simplemode:"false"`
	SftpRegex        string          `long:"source-sftp-regex" description:"regex to apply for choosing what to download, the regex is applied on paths AFTER the sftp-path specification, not the whole path" simplemode:"false"`
	SftpSkipCheck    bool            `long:"source-sftp-skipcheck" description:"set to prevent aerolab for checking from this machine if sftp is accessible with the given credentials" simplemode:"false"`
	SftpFullCheck    bool            `long:"source-sftp-listfiles" description:"set this to make aerolab login to sftp and list files prior to starting AGI; this will interactively prompt to continue" simplemode:"false"`
	S3Enable         bool            `long:"source-s3-enable" description:"enable s3 source" simplemode:"false"`
	S3Threads        int             `long:"source-s3-threads" description:"number of concurrent downloader threads" default:"4" simplemode:"false"`
	S3Region         string          `long:"source-s3-region" description:"aws region where the s3 bucket is located" simplemode:"false"`
	S3Bucket         string          `long:"source-s3-bucket" description:"s3 bucket name" simplemode:"false"`
	S3KeyID          string          `long:"source-s3-key-id" description:"(optional) access key ID" simplemode:"false"`
	S3Secret         string          `long:"source-s3-secret-key" description:"(optional) secret key" webtype:"password" simplemode:"false"`
	S3path           string          `long:"source-s3-path" description:"path on s3 to download logs from" simplemode:"false"`
	S3Regex          string          `long:"source-s3-regex" description:"regex to apply for choosing what to download, the regex is applied on paths AFTER the s3-path specification, not the whole path" simplemode:"false"`
	S3SkipCheck      bool            `long:"source-s3-skipcheck" description:"set to prevent aerolab for checking from this machine if s3 is accessible with the given credentials" simplemode:"false"`
	S3Endpoint       string          `long:"source-s3-endpoint" description:"specify a custom endpoint for the S3 source bucket"`
	ProxyDisableSSL  bool            `long:"proxy-ssl-disable" description:"switch to disable TLS on the proxy" simplemode:"false"`
	ProxyCert        flags.Filename  `long:"proxy-ssl-cert" description:"if not provided snakeoil will be used" simplemode:"false"`
	ProxyKey         flags.Filename  `long:"proxy-ssl-key" description:"if not provided snakeoil will be used" simplemode:"false"`
	ProxyMaxInactive time.Duration   `long:"proxy-max-inactive" description:"maximum duration of inactivity by the user over which the server will poweroff" default:"1h"`
	ProxyMaxUptime   time.Duration   `long:"proxy-max-uptime" description:"maximum uptime of the instance, after which the server will poweroff" default:"24h"`
	TimeRanges       bool            `long:"ingest-timeranges-enable" description:"enable importing statistics only on a specified time range found in the logs" simplemode:"false"`
	TimeRangesFrom   string          `long:"ingest-timeranges-from" description:"time range from, format: 2006-01-02T15:04:05Z07:00 or '2006/01/02 15:03:05'" simplemode:"false" web-input-mask:"yyyy/mm/dd HH:MM:ss"`
	TimeRangesTo     string          `long:"ingest-timeranges-to" description:"time range to, format: 2006-01-02T15:04:05Z07:00 or '2006/01/02 15:03:05'" simplemode:"false" web-input-mask:"yyyy/mm/dd HH:MM:ss"`
	CustomSourceName string          `long:"ingest-custom-source-name" description:"custom source name to disaplay in grafana" simplemode:"false"`
	PatternsFile     flags.Filename  `long:"ingest-patterns-file" description:"provide a custom patterns YAML file to the log ingest system" simplemode:"false"`
	IngestLogLevel   int             `long:"ingest-log-level" description:"1-CRITICAL,2-ERROR,3-WARN,4-INFO,5-DEBUG,6-DETAIL" default:"4" simplemode:"false"`
	IngestCpuProfile bool            `long:"ingest-cpu-profiling" description:"enable log ingest cpu profiling" simplemode:"false"`
	PluginCpuProfile bool            `long:"plugin-cpu-profiling" description:"enable CPU profiling for the grafana plugin" simplemode:"false"`
	PluginLogLevel   int             `long:"plugin-log-level" description:"1-CRITICAL,2-ERROR,3-WARN,4-INFO,5-DEBUG,6-DETAIL" default:"4" simplemode:"false"`
	NoConfigOverride bool            `long:"no-config-override" description:"if set, existing configuration will not be overridden; useful when restarting EFS-based AGIs" simplemode:"false"`
	NoToolsOverride  bool            `long:"no-tools-override" description:"by default agi will install the latest tools package; set this to disable tools package upgrade" simplemode:"false"`
	SendClusterInfo  string          `long:"send-cluster-info" description:"URL to send cluster info to that is discovered in collectinfos, ex https://user:pass@example.com/clusterinfo" simplemode:"false" webhidden:"true"`
	hTTPSNotify
	WithAGIMonitorAuto      bool                 `long:"with-monitor" description:"if set, system will look for agimonitor client; if not present, one will be created; will also auto-fill the monitor URL" simplemode:"false"`
	MonitorAutoCertDomains  []string             `long:"monitor-autocert" description:"Monitor Creation: TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains, can be used more than once" yaml:"autocertDomains" simplemode:"false"` // TLS: if specified, will attempt to auto-obtain certificates from letsencrypt for given domains
	MonitorAutoCertEmail    string               `long:"monitor-autocert-email" description:"Monitor Creation: TLS: if autocert is specified, specify a valid email address to use with letsencrypt" simplemode:"false"`
	MonitorCertFile         string               `long:"monitor-cert-file" description:"Monitor Creation: TLS: certificate file to use if not using letsencrypt; default: generate self-signed" yaml:"certFile" simplemode:"false"` // TLS: cert file (if not using autocert), default: snakeoil
	MonitorKeyFile          string               `long:"monitor-key-file" description:"Monitor Creation: TLS: key file to use if not using letsencrypt; default: generate self-signed" yaml:"keyFile" simplemode:"false"`           // TLS: key file (if not using autocert), default: snakeoil
	AerospikeVersion        TypeAerospikeVersion `short:"v" long:"aerospike-version" description:"Custom Aerospike server version" default:"8.0.0.*"`
	Distro                  TypeDistro           `short:"d" long:"distro" description:"Custom distro" default:"ubuntu" simplemode:"false"`
	FeaturesFilePath        flags.Filename       `short:"f" long:"featurefile" description:"Features file to install, or directory containing feature files"`
	FeaturesFilePrintDetail bool                 `long:"featurefile-printdetail" description:"Print details of discovered features files" hidden:"true"`
	chDirCmd
	NoVacuumOnFail                bool   `long:"no-vacuum" description:"if set, will not remove the template instance/container should it fail installation" simplemode:"false"`
	Owner                         string `long:"owner" description:"AWS/GCP only: create owner tag with this value"`
	NonInteractive                bool   `long:"non-interactive" description:"set to disable interactive mode" webdisable:"true" webset:"true"`
	GrafanaVersion                string `long:"grafana-version" description:"grafana version to install" default:"11.2.6" simplemode:"false"`
	uploadAuthorizedContentsGzB64 string
	Aws                           agiCreateCmdAws    `no-flag:"true"`
	Gcp                           agiCreateCmdGcp    `no-flag:"true"`
	Docker                        agiCreateCmdDocker `no-flag:"true"`
}

type agiCreateCmdAws struct {
	InstanceType        string        `short:"I" long:"instance-type" description:"optional instance type to use; min RAM: 16GB; default in order, as available: edition: g/a/i, family:r7/r6/r5, size:xlarge"`
	InstanceTypeArm     bool          `long:"instance-arch-arm" description:"if not specifying InstanceType, optionally specify this parameter to prefer ARM over amd64 type instances"`
	Ebs                 string        `short:"E" long:"ebs" description:"EBS volume size GB" default:"40"`
	SecurityGroupID     string        `short:"S" long:"secgroup-id" description:"security group IDs to use, comma-separated; default: empty: create and auto-manage" simplemode:"false"`
	SubnetID            string        `short:"U" long:"subnet-id" description:"subnet-id, availability-zone name, or empty; default: empty: first found in default VPC" simplemode:"false"`
	Tags                []string      `long:"tags" description:"apply custom tags to instances; format: key=value; this parameter can be specified multiple times"`
	NamePrefix          []string      `long:"secgroup-name" description:"Name prefix to use for the security groups, can be specified multiple times" default:"AeroAGI" simplemode:"false"`
	WithEFS             bool          `long:"aws-with-efs" description:"set to enable EFS as the storage medium for the AGI stack" simplemode:"false"`
	EFSName             string        `long:"aws-efs-name" description:"set to change the default name of the EFS volume" default:"{AGI_NAME}" simplemode:"false"`
	EFSPath             string        `long:"aws-efs-path" description:"set to change the default path of the EFS directory to be mounted" default:"/" simplemode:"false"`
	EFSMultiZone        bool          `long:"aws-efs-multizone" description:"by default the EFS volume will be one-zone to save on costs; set this to enable multi-AZ support" simplemode:"false"`
	TerminateOnPoweroff bool          `long:"aws-terminate-on-poweroff" description:"if set, when shutdown or poweroff is executed from the instance itself (or it reaches max inactive/uptime), it will be stopped AND terminated" simplemode:"false"`
	SpotInstance        bool          `long:"aws-spot-instance" description:"set to request a spot instance in place of on-demand"`
	SpotFallback        bool          `long:"aws-spot-fallback" description:"if set, and spot instance errors with capacity not available, request an on-demand instead"`
	Expires             time.Duration `long:"aws-expire" description:"length of life of nodes prior to expiry; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry; grow default: match existing cluster" default:"30h"`
	EFSExpires          time.Duration `long:"aws-efs-expire" description:"if EFS is not remounted using aerolab for this amount of time, it will be expired" default:"96h" simplemode:"false"`
	Route53ZoneId       string        `long:"route53-zoneid" description:"if set, will automatically update a route53 DNS domain with an entry of {instanceId}.{region}.agi.; expiry system will also be updated accordingly" simplemode:"false"`
	Route53DomainName   string        `long:"route53-domain" description:"the route domain the zone refers to; eg myagi.org" simplemode:"false"`
}

type agiCreateCmdGcp struct {
	InstanceType        string        `long:"instance" description:"instance type to use" default:"c2d-highmem-4"`
	Disks               []string      `long:"disk" description:"format type:sizeGB, ex: pd-ssd:20 ex: pd-balanced:40" default:"pd-ssd:40"`
	Zone                guiZone       `long:"zone" description:"zone name to deploy to" webrequired:"true"`
	Tags                []string      `long:"tag" description:"apply custom tags to instances; this parameter can be specified multiple times"`
	Labels              []string      `long:"label" description:"apply custom labels to instances; format: key=value; this parameter can be specified multiple times"`
	NamePrefix          []string      `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"agi-managed-external" simplemode:"false"`
	SpotInstance        bool          `long:"gcp-spot-instance" description:"set to request a spot instance in place of on-demand"`
	Expires             time.Duration `long:"gcp-expire" description:"length of life of nodes prior to expiry; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry; grow default: match existing cluster" default:"30h"`
	WithVol             bool          `long:"gcp-with-vol" description:"set to enable extra volume as the storage medium for the AGI stack" simplemode:"false"`
	VolName             string        `long:"gcp-vol-name" description:"set to change the default name of the volume" default:"{AGI_NAME}" simplemode:"false"`
	VolExpires          time.Duration `long:"gcp-vol-expire" description:"if the volume is not remounted using aerolab for this amount of time, it will be expired" default:"96h" simplemode:"false"`
	TerminateOnPoweroff bool          `long:"gcp-terminate-on-poweroff" description:"if set, when shutdown or poweroff is executed from the instance itself, it will be stopped AND terminated" simplemode:"false"`
}

type agiCreateCmdDocker struct {
	ExposePortsToHost string `short:"e" long:"expose-ports" description:"If a single machine is being deployed, port forward. Format: HOST_PORT:NODE_PORT,HOST_PORT:NODE_PORT"`
	CpuLimit          string `short:"l" long:"cpu-limit" description:"Impose CPU speed limit. Values acceptable could be '1' or '2' or '0.5' etc." default:"" simplemode:"false"`
	RamLimit          string `short:"t" long:"ram-limit" description:"Limit RAM available to each node, e.g. 500m, or 1g." default:"" simplemode:"false"`
	SwapLimit         string `short:"w" long:"swap-limit" description:"Limit the amount of total memory (ram+swap) each node can use, e.g. 600m. If ram-limit==swap-limit, no swap is available." default:"" simplemode:"false"`
	Privileged        bool   `short:"B" long:"privileged" description:"Docker only: run container in privileged mode"`
	NetworkName       string `long:"network" description:"specify a network name to use for non-default docker network; for more info see: aerolab config docker help" default:"" simplemode:"false"`
}

func init() {
	addBackendSwitch("agi.create", "aws", &a.opts.AGI.Create.Aws)
	addBackendSwitch("agi.create", "docker", &a.opts.AGI.Create.Docker)
	addBackendSwitch("agi.create", "gcp", &a.opts.AGI.Create.Gcp)
}

func (c *agiCreateCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.ClusterSource != "" {
		if c.LocalSource != "" {
			return errors.New("local source cannot be specified when using --source-cluster")
		}
		localSource, err := agiLogsGet(c.ClusterSource)
		if err != nil {
			return err
		}
		c.LocalSource = localSource
		defer os.RemoveAll(string(localSource))
	}
	c.S3path = strings.Trim(c.S3path, "\t\n\r ")
	if strings.HasPrefix(c.SlackToken, "ENV::") {
		c.SlackToken = os.Getenv(strings.Split(c.SlackToken, "::")[1])
	}
	if strings.HasPrefix(c.SftpUser, "ENV::") {
		c.SftpUser = os.Getenv(strings.Split(c.SftpUser, "::")[1])
	}
	if strings.HasPrefix(c.SftpPass, "ENV::") {
		c.SftpPass = os.Getenv(strings.Split(c.SftpPass, "::")[1])
	}
	if strings.HasPrefix(c.S3KeyID, "ENV::") {
		c.S3KeyID = os.Getenv(strings.Split(c.S3KeyID, "::")[1])
	}
	if strings.HasPrefix(c.S3Secret, "ENV::") {
		c.S3Secret = os.Getenv(strings.Split(c.S3Secret, "::")[1])
	}
	if c.ClusterName == "~auto~" {
		nName := ""
		if c.LocalSource != "" {
			nName = string(c.LocalSource)
		}
		nName = nName + "\nS3"
		if c.S3Enable {
			nName = nName + "\n" + c.S3Bucket + "\n" + c.S3path + "\n" + c.S3Regex
		}
		nName = nName + "\nSFTP"
		if c.SftpEnable {
			nName = nName + "\n" + c.SftpHost + "\n" + strconv.Itoa(c.SftpPort) + "\n" + c.SftpUser + "\n" + c.SftpPath + "\n" + c.SftpRegex
		}
		c.ClusterName = TypeClusterName(shortuuid.NewWithNamespace(nName))
	}
	if c.S3Enable && c.S3path == "" {
		return errors.New("S3 path cannot be left empty")
	}
	if a.opts.Config.Backend.Type == "aws" {
		if (c.Aws.Route53DomainName == "" && c.Aws.Route53ZoneId != "") || (c.Aws.Route53DomainName != "" && c.Aws.Route53ZoneId == "") {
			return errors.New("either both route53-zoneid and route53-domain must be fills or both must be empty")
		}
	}
	if c.Owner == "" {
		c.Owner = currentOwnerUser
	}
	if a.opts.Config.Backend.Type == "docker" && (c.WithAGIMonitorAuto || c.hTTPSNotify.AGIMonitorUrl != "") {
		return errors.New("AGI monitor is not supported on docker; sizing would not be possible either way")
	}
	if (c.WithAGIMonitorAuto || c.hTTPSNotify.AGIMonitorUrl != "") && a.opts.Config.Backend.Type == "aws" && !c.Aws.WithEFS {
		return errors.New("AGI monitor can only be enabled for instances with EFS storage enabled (use --aws-with-efs)")
	}
	if (c.WithAGIMonitorAuto || c.hTTPSNotify.AGIMonitorUrl != "") && a.opts.Config.Backend.Type == "gcp" && !c.Gcp.WithVol {
		return errors.New("AGI monitor can only be enabled for instances with extra Volume storage enabled (use --gcp-with-vol)")
	}
	// upload file list start - add logtags
	flist := []fileListReader{
		{
			filePath:     "/usr/local/bin/logtags",
			fileContents: strings.NewReader(agiCreateLogTags),
			fileSize:     len(agiCreateLogTags),
		},
	}
	// generate ingest.yaml
	var tfrom, tto time.Time
	var err error
	if c.TimeRangesFrom != "" {
		tfrom, err = time.Parse("2006-01-02T15:04:05Z07:00", c.TimeRangesFrom)
		if err != nil {
			tfrom, err = time.Parse("2006/01/02 15:04:05 GMT", c.TimeRangesFrom+" GMT")
			if err != nil {
				return fmt.Errorf("from time range invalid: %s", err)
			}
		}
	}
	if c.TimeRangesTo != "" {
		tto, err = time.Parse("2006-01-02T15:04:05Z07:00", c.TimeRangesTo)
		if err != nil {
			tto, err = time.Parse("2006/01/02 15:04:05 GMT", c.TimeRangesTo+" GMT")
			if err != nil {
				return fmt.Errorf("to time range invalid: %s", err)
			}
		}
	}
	config, err := ingest.MakeConfigReader(true, nil, true)
	if err != nil {
		return fmt.Errorf("create.ingest.MakeConfig: %s", err)
	}
	config.Aerospike.MaxPutThreads = 128
	if a.opts.Config.Backend.Type == "docker" {
		config.Aerospike.MaxPutThreads = 64
	}
	config.Aerospike.WaitForSindexes = true
	config.PreProcess.FileThreads = 6
	config.PreProcess.UnpackerFileThreads = 4
	config.Processor.MaxConcurrentLogFiles = 6
	config.ProgressFile.DisableWrite = false
	config.ProgressFile.Compress = true
	config.ProgressFile.WriteInterval = 10 * time.Second
	config.ProgressFile.OutputFilePath = "/opt/agi/ingest"
	config.ProgressPrint.Enable = true
	config.ProgressPrint.PrintDetailProgress = true
	config.ProgressPrint.PrintOverallProgress = true
	config.ProgressPrint.UpdateInterval = 10 * time.Second
	if c.PatternsFile != "" {
		config.PatternsFile = "/opt/agi/patterns.yaml"
	}
	config.Directories.CollectInfo = "/opt/agi/files/collectinfo"
	config.Directories.DirtyTmp = "/opt/agi/files/input"
	config.Directories.Logs = "/opt/agi/files/logs"
	config.Directories.NoStatLogs = "/opt/agi/files/no-stat"
	config.Directories.OtherFiles = "/opt/agi/files/other"
	config.LogLevel = c.IngestLogLevel
	if c.IngestCpuProfile {
		config.CPUProfilingOutputFile = "/opt/agi/cpu.ingest.pprof"
	}
	config.CustomSourceName = c.CustomSourceName
	config.IngestTimeRanges.Enabled = c.TimeRanges
	if c.TimeRanges {
		config.IngestTimeRanges.From = tfrom
		config.IngestTimeRanges.To = tto
	}
	// sources - sftp
	config.Downloader.SftpSource = new(ingest.SftpSource)
	config.Downloader.SftpSource.Enabled = c.SftpEnable
	config.Downloader.SftpSource.Threads = c.SftpThreads
	config.Downloader.SftpSource.Host = c.SftpHost
	config.Downloader.SftpSource.Port = c.SftpPort
	config.Downloader.SftpSource.Username = c.SftpUser
	config.Downloader.SftpSource.Password = c.SftpPass
	if c.SftpKey != "" {
		config.Downloader.SftpSource.KeyFile = "/opt/agi/sftp.key"
	}
	config.Downloader.SftpSource.PathPrefix = c.SftpPath
	config.Downloader.SftpSource.SearchRegex = c.SftpRegex
	// sources - s3
	config.Downloader.S3Source = new(ingest.S3Source)
	config.Downloader.S3Source.Enabled = c.S3Enable
	config.Downloader.S3Source.Threads = c.S3Threads
	config.Downloader.S3Source.Region = c.S3Region
	config.Downloader.S3Source.BucketName = c.S3Bucket
	config.Downloader.S3Source.KeyID = c.S3KeyID
	config.Downloader.S3Source.SecretKey = c.S3Secret
	config.Downloader.S3Source.PathPrefix = c.S3path
	config.Downloader.S3Source.SearchRegex = c.S3Regex
	config.Downloader.S3Source.Endpoint = c.S3Endpoint
	config.SendClusterInfo = c.SendClusterInfo
	var encBuf bytes.Buffer
	enc := yaml.NewEncoder(&encBuf)
	enc.SetIndent(2)
	err = enc.Encode(config)
	if err != nil {
		return fmt.Errorf("could not marshal ingest configuration to yaml: %s", err)
	}
	conf := encBuf.Bytes()
	flist = append(flist, fileListReader{
		filePath:     "/tmp/ingest.yaml",
		fileContents: bytes.NewReader(conf),
		fileSize:     len(conf),
	})

	// test s3 access and directory
	if c.S3Enable && !c.S3SkipCheck {
		var s3creds *credentials.Credentials
		if c.S3KeyID != "" || c.S3Secret != "" {
			s3creds = credentials.NewStaticCredentials(c.S3KeyID, c.S3Secret, "")
		}
		var endpoint *string
		if c.S3Endpoint != "" {
			endpoint = aws.String(c.S3Endpoint)
		}
		sess, err := session.NewSession(&aws.Config{
			Region:      aws.String(c.S3Region),
			Credentials: s3creds,
			Endpoint:    endpoint,
		})
		if err != nil {
			return fmt.Errorf("failed to test s3 credentials: %s", err)
		}
		svc := s3.New(sess)
		obj, err := svc.ListObjectsV2(&s3.ListObjectsV2Input{
			Bucket:  aws.String(c.S3Bucket),
			MaxKeys: aws.Int64(5),
			Prefix:  aws.String(c.S3path),
		})
		if err != nil {
			return fmt.Errorf("failed to list s3 objects: %s", err)
		}
		if len(obj.Contents) == 0 {
			return fmt.Errorf("directory empty or path doesn't exist")
		}
	}

	// test sftp access and directory
	if c.SftpEnable && !c.SftpSkipCheck {
		log.Println("Checking sftp access...")
		sftpFiles, err := ingest.SftpCheckLogin(config, c.SftpFullCheck)
		if err != nil {
			return fmt.Errorf("failed to validate sftp: %s", err)
		}
		if c.SftpFullCheck {
			log.Println("=-=-=-= Starting sftp directory listing =-=-=-=")
			for sftpName, sftpFile := range sftpFiles {
				fmt.Printf("==> %s (%s)\n", sftpName, convSize(sftpFile.Size))
			}
			log.Println("=-=-=-= End sftp directory listing =-=-=-=")
			if !c.NonInteractive {
				fmt.Println("Press ENTER to continue, or ctrl+c to exit")
				reader := bufio.NewReader(os.Stdin)
				_, err := reader.ReadString('\n')
				if err != nil {
					logExit(err)
				}
			}
		} else if len(sftpFiles) == 0 {
			if !c.NonInteractive {
				fmt.Println("WARNING: Directory appears to be empty, press ENTER to continue, ot ctrl+c to exit")
				reader := bufio.NewReader(os.Stdin)
				_, err := reader.ReadString('\n')
				if err != nil {
					logExit(err)
				}
			} else {
				fmt.Println("WARNING: Directory appears to be empty!")
			}
		}
	}

	// agi monitor
	if c.WithAGIMonitorAuto {
		b.WorkOnClients()
		clients, err := b.ClusterList()
		if err != nil {
			return err
		}
		if !inslice.HasString(clients, a.opts.AGI.Monitor.Create.Name) {
			a.opts.AGI.Monitor.Create.Owner = c.Owner
			a.opts.AGI.Monitor.Create.Aws.NamePrefix = c.Aws.NamePrefix
			a.opts.AGI.Monitor.Create.Aws.SecurityGroupID = c.Aws.SecurityGroupID
			a.opts.AGI.Monitor.Create.Aws.SubnetID = c.Aws.SubnetID
			a.opts.AGI.Monitor.Create.Gcp.NamePrefix = c.Gcp.NamePrefix
			a.opts.AGI.Monitor.Create.Gcp.Zone = c.Gcp.Zone.String()
			a.opts.AGI.Monitor.Create.AutoCertDomains = c.MonitorAutoCertDomains
			a.opts.AGI.Monitor.Create.AutoCertEmail = c.MonitorAutoCertEmail
			a.opts.AGI.Monitor.Create.CertFile = c.MonitorCertFile
			a.opts.AGI.Monitor.Create.KeyFile = c.MonitorKeyFile
			if c.ProxyCert != "" && c.ProxyKey != "" && !c.ProxyDisableSSL {
				a.opts.AGI.Monitor.Create.StrictAGITLS = true
			}
			err := a.opts.AGI.Monitor.Create.create(nil)
			if err != nil {
				return err
			}
			if len(c.MonitorAutoCertDomains) == 0 && c.MonitorCertFile == "" && c.MonitorKeyFile == "" {
				c.AGIMonitorCertIgnore = true // should notifier on AGI side expect AGI monitor to have a valid certificate
			}
		}
		b.WorkOnClients()
		agimUrl := ""
		if a.opts.Config.Backend.Type == "aws" {
			// get agimUrl (agimUrl aws tag) and use that as c.AGIMonitorUrl = "https://" + agimUrl
			tags, err := b.GetInstanceTags(a.opts.AGI.Monitor.Create.Name)
			if err == nil {
				for _, tgs := range tags {
					if tg, ok := tgs["agimUrl"]; ok && tg != "" {
						agimUrl = tg
						c.AGIMonitorUrl = "https://" + agimUrl
					}
				}
			}
		}
		if agimUrl == "" {
			ips, err := b.GetNodeIpMap(a.opts.AGI.Monitor.Create.Name, true)
			if err != nil {
				return err
			}
			if len(ips) == 0 {
				return errors.New("could not get private IP of AGI monitor client, ensure it is running")
			}
			if nip, ok := ips[1]; !ok || nip == "" {
				return errors.New("could not resolve private IP of AGI monitor client, ensure it is running")
			}
			c.AGIMonitorUrl = "https://" + ips[1]
		}
		b.WorkOnServers()
	}
	// if files specified do not exist, bail early
	for _, fn := range []string{
		string(c.LocalSource),
		string(c.SftpKey),
		string(c.ProxyCert),
		string(c.ProxyKey),
		string(c.PatternsFile),
	} {
		if fn == "" {
			continue
		}
		if _, err := os.Stat(fn); err != nil {
			return fmt.Errorf("%s is not accessible: %s", fn, err)
		}
	}
	log.Println("Getting volume information")
	if (a.opts.Config.Backend.Type == "aws" && c.Aws.InstanceType == "") || (a.opts.Config.Backend.Type == "gcp" && c.Gcp.InstanceType == "c2d-highmem-4") {
		inv, err := b.Inventory("", []int{InventoryItemVolumes})
		if err != nil {
			return err
		}
		var foundVol *inventoryVolume
		for _, vol := range inv.Volumes {
			if vol.Name != string(c.ClusterName) {
				continue
			}
			foundVol = &vol
			break
		}
		if foundVol != nil {
			if v, ok := foundVol.Tags["aerolab7agiav"]; ok {
				c.AerospikeVersion = TypeAerospikeVersion(v)
			} else {
				c.AerospikeVersion = "6.4.0.26" // do not break the existing behaviour, if we are missing a version sticker, default to old version on AGI restarts with existing volumes
			}
			c.Aws.InstanceType = foundVol.Tags["agiinstance"]
			c.Gcp.InstanceType = foundVol.Tags["agiinstance"]
			if foundVol.Tags["aginodim"] == "true" {
				c.NoDIM = true
			}
			if foundVol.Tags["termonpow"] == "true" {
				c.Aws.TerminateOnPoweroff = true
				c.Gcp.TerminateOnPoweroff = true
			}
			if foundVol.Tags["isspot"] == "true" {
				c.Aws.SpotInstance = true
				c.Gcp.SpotInstance = true
			}
		}
	}
	if a.opts.Config.Backend.Type == "aws" && c.Aws.InstanceType == "" {
		log.Println("Resolving supported Instance Types")
		sup := make([]string, 8)
		cnt := 6
		if c.Aws.InstanceTypeArm {
			cnt = 0
		}
		itypes, err := b.GetInstanceTypes(0, 0, 0, 0, 0, 0, true, "")
		if err != nil {
			sup[cnt] = "r7g.xlarge"
		} else {
			for _, itype := range itypes {
				switch itype.InstanceName {
				case "r7g.xlarge":
					sup[cnt] = "r7g.xlarge"
				case "r6g.xlarge":
					sup[cnt+1] = "r6g.xlarge"
				}
			}
		}
		cnt = 0
		if c.Aws.InstanceTypeArm {
			cnt = 2
		}
		itypes, err = b.GetInstanceTypes(0, 0, 0, 0, 0, 0, false, "")
		if err != nil {
			sup[cnt] = "r7a.xlarge"
		} else {
			for _, itype := range itypes {
				switch itype.InstanceName {
				case "r7a.xlarge":
					sup[cnt] = "r7a.xlarge"
				case "r7i.xlarge":
					sup[cnt+1] = "r7i.xlarge"
				case "r6a.xlarge":
					sup[cnt+2] = "r6a.xlarge"
				case "r6i.xlarge":
					sup[cnt+3] = "r6i.xlarge"
				case "r5a.xlarge":
					sup[cnt+4] = "r5a.xlarge"
				case "r5.xlarge":
					sup[cnt+5] = "r5.xlarge"
				}
			}
		}
		for i := range sup {
			if sup[i] == "" {
				continue
			}
			c.Aws.InstanceType = sup[i]
			break
		}
	} else if (a.opts.Config.Backend.Type == "aws" && c.Aws.InstanceType != "") || (a.opts.Config.Backend.Type == "gcp" && c.Gcp.InstanceType != "") {
		ntype := c.Aws.InstanceType
		if a.opts.Config.Backend.Type == "gcp" {
			ntype = c.Gcp.InstanceType
		}
		log.Println("Resolving supported Instance Types")
		found := false
		for _, narm := range []bool{true, false} {
			itypes, err := b.GetInstanceTypes(0, 0, 0, 0, 0, 0, narm, c.Gcp.Zone.String())
			if err != nil {
				log.Printf("WARNING: Could not check instance size, ensure instance has 12GB RAM or more (%s)", err)
			} else {
				for _, itype := range itypes {
					if itype.InstanceName != ntype {
						continue
					}
					if itype.RamGB < 12 {
						return fmt.Errorf("instance %s is too small (min=12G instance=%d)", itype.InstanceName, int(itype.RamGB))
					}
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			log.Printf("WARNING: Instance Type not found in instance listing, could not verify the instance has at least 12GB RAM")
		}
	}
	log.Println("Starting AGI deployment...")
	if c.AGILabel == "" {
		c.AGILabel = string(c.ClusterName)
	}
	sourceStringLocal := ""
	sourceStringSftp := ""
	sourceStringS3 := ""
	if c.LocalSource != "" {
		sourceStringLocal = "local:" + string(c.LocalSource)
	}
	if c.SftpEnable {
		sourceStringSftp = c.SftpHost + ":" + c.SftpPath + "^" + c.SftpRegex
	}
	if c.S3Enable {
		sourceStringS3 = c.S3Bucket + ":" + c.S3path + "^" + c.S3Regex
	}
	sourceStringSftp = strings.TrimSuffix(sourceStringSftp, "^")
	sourceStringS3 = strings.TrimSuffix(sourceStringS3, "^")
	if len(sourceStringLocal) > 191 { // 255 with base64 overhead
		sourceStringLocal = sourceStringLocal[0:188] + "..."
	}
	if len(sourceStringSftp) > 191 { // 255 with base64 overhead
		sourceStringSftp = sourceStringSftp[0:188] + "..."
	}
	if len(sourceStringS3) > 191 { // 255 with base64 overhead
		sourceStringS3 = sourceStringS3[0:188] + "..."
	}
	if a.opts.Config.Backend.Type == "aws" && c.Aws.Route53ZoneId != "" {
		agiDomainFound := false
		agiZoneIDFound := false
		for itag, ntag := range c.Aws.Tags {
			if strings.HasPrefix(ntag, "agiDomain=") {
				agiDomainFound = true
				c.Aws.Tags[itag] = "agiDomain=" + c.Aws.Route53DomainName
			}
			if strings.HasPrefix(ntag, "agiZoneID=") {
				agiZoneIDFound = true
				c.Aws.Tags[itag] = "agiZoneID=" + c.Aws.Route53ZoneId
			}
		}
		if !agiDomainFound {
			c.Aws.Tags = append(c.Aws.Tags, "agiDomain="+c.Aws.Route53DomainName)
		}
		if !agiZoneIDFound {
			c.Aws.Tags = append(c.Aws.Tags, "agiZoneID="+c.Aws.Route53ZoneId)
		}
	}
	sourceStringLocal = base64.RawStdEncoding.EncodeToString([]byte(sourceStringLocal))
	sourceStringSftp = base64.RawStdEncoding.EncodeToString([]byte(sourceStringSftp))
	sourceStringS3 = base64.RawStdEncoding.EncodeToString([]byte(sourceStringS3))
	if a.opts.Config.Backend.Type == "aws" && c.Aws.WithEFS {
		a.opts.Cluster.Create.NoSetDNS = true
	}
	a.opts.Cluster.Create.ClusterName = c.ClusterName
	a.opts.Cluster.Create.NodeCount = 1
	a.opts.Cluster.Create.CustomConfigFilePath = ""
	a.opts.Cluster.Create.CustomToolsFilePath = ""
	a.opts.Cluster.Create.FeaturesFilePath = c.FeaturesFilePath
	a.opts.Cluster.Create.FeaturesFilePrintDetail = c.FeaturesFilePrintDetail
	a.opts.Cluster.Create.HeartbeatMode = "mesh"
	a.opts.Cluster.Create.MulticastAddress = ""
	a.opts.Cluster.Create.MulticastPort = ""
	a.opts.Cluster.Create.AutoStartAerospike = "n"
	a.opts.Cluster.Create.NoOverrideClusterName = false
	a.opts.Cluster.Create.NoSetHostname = false
	a.opts.Cluster.Create.ScriptEarly = ""
	a.opts.Cluster.Create.ScriptLate = ""
	a.opts.Cluster.Create.NoVacuumOnFail = c.NoVacuumOnFail
	a.opts.Cluster.Create.Owner = c.Owner
	a.opts.Cluster.Create.PriceOnly = false
	a.opts.Cluster.Create.ChDir = c.ChDir
	a.opts.Cluster.Create.DistroName = c.Distro
	a.opts.Cluster.Create.DistroVersion = "latest"
	a.opts.Cluster.Create.AerospikeVersion = c.AerospikeVersion
	a.opts.Cluster.Create.Username = ""
	a.opts.Cluster.Create.Password = ""
	a.opts.Cluster.Create.useAgiFirewall = true
	a.opts.Cluster.Create.Aws.AMI = ""
	a.opts.Cluster.Create.Aws.InstanceType = guiInstanceType(c.Aws.InstanceType)
	a.opts.Cluster.Create.Aws.Ebs = c.Aws.Ebs
	a.opts.Cluster.Create.Aws.SecurityGroupID = c.Aws.SecurityGroupID
	a.opts.Cluster.Create.Aws.SubnetID = c.Aws.SubnetID
	a.opts.Cluster.Create.Aws.Tags = append(c.Aws.Tags, "aerolab7agiav="+c.AerospikeVersion.String(), "aerolab4features="+strconv.Itoa(int(ClusterFeatureAGI)), fmt.Sprintf("aerolab4ssl=%t", !c.ProxyDisableSSL), fmt.Sprintf("agiLabel=%s", c.AGILabel), fmt.Sprintf("agiinstance=%s", c.Aws.InstanceType), fmt.Sprintf("aginodim=%t", c.NoDIM), fmt.Sprintf("termonpow=%t", c.Aws.TerminateOnPoweroff), fmt.Sprintf("isspot=%t", c.Aws.SpotInstance), fmt.Sprintf("agiSrcLocal=%s", sourceStringLocal), fmt.Sprintf("agiSrcSftp=%s", sourceStringSftp), fmt.Sprintf("agiSrcS3=%s", sourceStringS3))
	a.opts.Cluster.Create.Aws.NamePrefix = c.Aws.NamePrefix
	a.opts.Cluster.Create.Aws.Expires = c.Aws.Expires
	a.opts.Cluster.Create.Aws.PublicIP = false
	a.opts.Cluster.Create.Aws.IsArm = false
	a.opts.Cluster.Create.Aws.NoBestPractices = false
	if c.Aws.WithEFS {
		c.Aws.EFSName = strings.ReplaceAll(c.Aws.EFSName, "{AGI_NAME}", string(c.ClusterName))
		a.opts.Cluster.Create.Aws.EFSCreate = true
		a.opts.Cluster.Create.Aws.EFSOneZone = !c.Aws.EFSMultiZone
		a.opts.Cluster.Create.Aws.EFSMount = c.Aws.EFSName + ":" + c.Aws.EFSPath + ":" + "/opt/agi"
		a.opts.Cluster.Create.Aws.EFSExpires = c.Aws.EFSExpires
	}
	if a.opts.Config.Backend.Type == "aws" {
		a.opts.Cluster.Create.volExtraTags = map[string]string{
			"agiinstance":   c.Aws.InstanceType,
			"aginodim":      fmt.Sprintf("%t", c.NoDIM),
			"termonpow":     fmt.Sprintf("%t", c.Aws.TerminateOnPoweroff),
			"isspot":        fmt.Sprintf("%t", c.Aws.SpotInstance),
			"aerolab7agiav": c.AerospikeVersion.String(),
		}
	} else if a.opts.Config.Backend.Type == "gcp" {
		a.opts.Cluster.Create.volExtraTags = map[string]string{
			"agiinstance": c.Gcp.InstanceType,
			"aginodim":    fmt.Sprintf("%t", c.NoDIM),
			"termonpow":   fmt.Sprintf("%t", c.Gcp.TerminateOnPoweroff),
			"isspot":      fmt.Sprintf("%t", c.Gcp.SpotInstance),
		}
	}
	if c.Gcp.WithVol {
		c.Gcp.VolName = strings.ReplaceAll(c.Aws.EFSName, "{AGI_NAME}", string(c.ClusterName))
		a.opts.Cluster.Create.Gcp.VolCreate = true
		a.opts.Cluster.Create.Gcp.VolExpires = c.Gcp.VolExpires
		a.opts.Cluster.Create.Gcp.VolMount = c.Gcp.VolName + ":/opt/agi"
	}
	a.opts.Cluster.Create.Aws.TerminateOnPoweroff = c.Aws.TerminateOnPoweroff
	a.opts.Cluster.Create.Gcp.TerminateOnPoweroff = c.Gcp.TerminateOnPoweroff
	a.opts.Cluster.Create.Aws.SpotInstance = c.Aws.SpotInstance
	a.opts.Cluster.Create.spotFallback = c.Aws.SpotFallback
	a.opts.Cluster.Create.Gcp.SpotInstance = c.Gcp.SpotInstance
	a.opts.Cluster.Create.Gcp.Image = ""
	a.opts.Cluster.Create.Gcp.InstanceType = guiInstanceType(c.Gcp.InstanceType)
	a.opts.Cluster.Create.Gcp.Disks = c.Gcp.Disks
	a.opts.Cluster.Create.Gcp.PublicIP = false
	a.opts.Cluster.Create.Gcp.Zone = guiZone(c.Gcp.Zone)
	a.opts.Cluster.Create.Gcp.IsArm = false
	a.opts.Cluster.Create.Gcp.NoBestPractices = false
	a.opts.Cluster.Create.Gcp.Tags = c.Gcp.Tags
	a.opts.Cluster.Create.Gcp.Labels = append(c.Gcp.Labels, "aerolab4features="+strconv.Itoa(int(ClusterFeatureAGI)), fmt.Sprintf("aerolab4ssl=%t", !c.ProxyDisableSSL), "agilabel=set")
	a.opts.Cluster.Create.gcpMeta = map[string]string{
		"agiLabel":    c.AGILabel,
		"agiSrcLocal": sourceStringLocal,
		"agiSrcSftp":  sourceStringSftp,
		"agiSrcS3":    sourceStringS3,
	}
	a.opts.Cluster.Create.Gcp.VolLabels = append(gcplabels.PackToKV("agilabel", c.AGILabel), "agilabel=set", fmt.Sprintf("agiinstance=%s", c.Gcp.InstanceType), fmt.Sprintf("aginodim=%t", c.NoDIM), fmt.Sprintf("termonpow=%t", c.Gcp.TerminateOnPoweroff), fmt.Sprintf("isspot=%t", c.Gcp.SpotInstance))
	a.opts.Cluster.Create.Gcp.VolDescription = c.AGILabel
	a.opts.Cluster.Create.Gcp.NamePrefix = c.Gcp.NamePrefix
	a.opts.Cluster.Create.Gcp.Expires = c.Gcp.Expires
	if !c.ProxyDisableSSL {
		c.Docker.ExposePortsToHost = "443"
	} else {
		c.Docker.ExposePortsToHost = "80"
	}
	a.opts.Cluster.Create.Docker.ExposePortsToHost = c.Docker.ExposePortsToHost
	a.opts.Cluster.Create.Docker.NoAutoExpose = true
	a.opts.Cluster.Create.Docker.CpuLimit = c.Docker.CpuLimit
	a.opts.Cluster.Create.Docker.RamLimit = c.Docker.RamLimit
	a.opts.Cluster.Create.Docker.SwapLimit = c.Docker.SwapLimit
	a.opts.Cluster.Create.Docker.Privileged = c.Docker.Privileged
	a.opts.Cluster.Create.Docker.NetworkName = c.Docker.NetworkName
	a.opts.Cluster.Create.Docker.ClientType = strconv.Itoa(int(ClusterFeatureAGI))
	a.opts.Cluster.Create.Docker.Labels = []string{"agiLabel=" + c.AGILabel}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get current working directory: %s", err)
	}
	err = a.opts.Cluster.Create.realExecute2(args, false)
	if err != nil {
		return err
	}
	err = os.Chdir(cwd)
	if err != nil {
		return fmt.Errorf("could not recover current working directory: %s", err)
	}

	wgDns := new(sync.WaitGroup)
	if a.opts.Config.Backend.Type == "aws" && c.Aws.Route53ZoneId != "" {
		instIps, err := b.GetInstanceIpMap(string(c.ClusterName), false)
		if err != nil {
			log.Printf("ERROR: Could not get node IPs, DNS will not be updated: %s", err)
		} else {
			wgDns.Add(2)
			go func() {
				defer wgDns.Done()
				if c.Aws.Expires != 0 {
					err := b.ExpiriesUpdateZoneID(c.Aws.Route53ZoneId)
					if err != nil {
						log.Printf("ERROR Route53 ZoneID not updated in expiry system, zones will be not cleaned up on expiry: %s", err)
					}
				}
			}()
			go func() {
				defer wgDns.Done()
				for inst, ip := range instIps {
					err := b.DomainCreate(c.Aws.Route53ZoneId, fmt.Sprintf("%s.%s.agi.%s", inst, a.opts.Config.Backend.Region, c.Aws.Route53DomainName), ip, true)
					if err != nil {
						log.Printf("ERROR creating domain in route53: %s", err)
					}
				}
			}()
		}
	}
	log.Println("Cluster Node created, continuing AGI deployment...")

	// docker will use max 2GB on plugin, aws/gcp 4GB; for aws/gcp we should leave 6GB of RAM unused (4GB-plugin 2GB-OS); for docker: 3GB (2GB-plugin 1GB-everything else)
	out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"free", "-b"}}, []int{1})
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("could not get available memory on node: %s: %s", err, string(out[0]))
		} else {
			return fmt.Errorf("could not get available memory on node: %s", err)
		}
	}
	outn := strings.Split(string(out[0]), "\n")
	if len(outn) < 2 || !strings.HasPrefix(outn[1], "Mem:") || !strings.Contains(outn[0], "available") {
		return fmt.Errorf("malformed output from free -b: %s", string(out[0]))
	}
	memstr := strings.Split(outn[1], " ")
	avail, err := strconv.Atoi(memstr[len(memstr)-1])
	if err != nil {
		return fmt.Errorf("could not convert available memory to int:%s", err)
	}
	memSize := avail
	if a.opts.Config.Backend.Type == "docker" {
		memSize = memSize - (1024 * 1024 * 1024 * 3)
	} else {
		memSize = memSize - (1024 * 1024 * 1024 * 6)
	}
	if memSize < 1024*1024*1024 {
		if a.opts.Config.Backend.Type == "docker" {
			return fmt.Errorf("not enough RAM to continue, give docker more memory, minMemSize:%d asdMemSize:%d avail:%d restrict:3GiB", 1024*1024*1024, memSize, avail)
		}
		return fmt.Errorf("not enough RAM to continue, choose an instance which has at least 7GiB memory available after startup, minMemSize:%d asdMemSize:%d avail:%d restrict:6GiB", 1024*1024*1024, memSize, avail)
	}

	// create directories
	_, err = b.RunCommands(c.ClusterName.String(), [][]string{{"mkdir", "-p", "/opt/agi/files/input"}}, []int{1})
	if err != nil {
		return fmt.Errorf("could not create /opt/agi/files/input on remote: %s", err)
	}

	// upload local source logs
	if c.LocalSource != "" {
		a.opts.Files.Upload.ClusterName = c.ClusterName
		a.opts.Files.Upload.Nodes = "1"
		a.opts.Files.Upload.IsClient = false
		a.opts.Files.Upload.Files.Source = c.LocalSource
		a.opts.Files.Upload.Files.Destination = "/opt/agi/files/input/"
		err = a.opts.Files.Upload.runUpload(nil)
		if err != nil {
			return fmt.Errorf("failed to upload local source to remote: %s", err)
		}
	}

	// we need to know if node is arm
	isArm, err := b.IsNodeArm(c.ClusterName.String(), 1)
	if err != nil {
		return fmt.Errorf("could not identify node architecture: %s", err)
	}

	// upload aerolab to remote
	_, err = b.RunCommands(string(c.ClusterName), [][]string{{"ls", "/usr/local/bin/aerolab"}}, []int{1})
	if err != nil {
		nLinuxBinary := nLinuxBinaryX64
		if isArm {
			nLinuxBinary = nLinuxBinaryArm64
		}
		if len(nLinuxBinary) == 0 {
			xtail := ""
			if isArm {
				xtail = ".arm"
			} else {
				xtail = ".amd"
			}
			if _, err := os.Stat("/usr/local/bin/aerolab" + xtail); err == nil {
				nLinuxBinary, _ = os.ReadFile("/usr/local/bin/aerolab" + xtail)
			}
		}
		if len(nLinuxBinary) == 0 {
			execName, err := findExec()
			if err != nil {
				return err
			}
			nLinuxBinary, err = os.ReadFile(execName)
			if err != nil {
				return err
			}
		}
		flist = append(flist, fileListReader{
			filePath:     "/usr/local/bin/aerolab",
			fileContents: bytes.NewReader(nLinuxBinary),
			fileSize:     len(nLinuxBinary),
		})
	}

	// upload custom patterns file
	if c.PatternsFile != "" {
		stat, err := os.Stat(string(c.PatternsFile))
		if err != nil {
			return fmt.Errorf("could not access patterns file: %s", err)
		}
		f, err := os.Open(string(c.PatternsFile))
		if err != nil {
			return fmt.Errorf("failed to open patterns file: %s", err)
		}
		defer f.Close()
		flist = append(flist, fileListReader{
			filePath:     "/opt/agi/patterns.yaml",
			fileContents: f,
			fileSize:     int(stat.Size()),
		})
	}

	// upload proxy key
	if c.ProxyKey != "" {
		stat, err := os.Stat(string(c.ProxyKey))
		if err != nil {
			return fmt.Errorf("could not access proxy key file: %s", err)
		}
		f, err := os.Open(string(c.ProxyKey))
		if err != nil {
			return fmt.Errorf("failed to open proxy key file: %s", err)
		}
		defer f.Close()
		flist = append(flist, fileListReader{
			filePath:     "/opt/agi/proxy.key",
			fileContents: f,
			fileSize:     int(stat.Size()),
		})
	}

	// upload proxy cert
	if c.ProxyCert != "" {
		stat, err := os.Stat(string(c.ProxyCert))
		if err != nil {
			return fmt.Errorf("could not access proxy cert file: %s", err)
		}
		f, err := os.Open(string(c.ProxyCert))
		if err != nil {
			return fmt.Errorf("failed to open proxy cert file: %s", err)
		}
		defer f.Close()
		flist = append(flist, fileListReader{
			filePath:     "/opt/agi/proxy.cert",
			fileContents: f,
			fileSize:     int(stat.Size()),
		})
	}

	// upload sftp key
	if c.SftpKey != "" {
		stat, err := os.Stat(string(c.SftpKey))
		if err != nil {
			return fmt.Errorf("could not access sftp key file: %s", err)
		}
		f, err := os.Open(string(c.SftpKey))
		if err != nil {
			return fmt.Errorf("failed to open sftp key file: %s", err)
		}
		defer f.Close()
		flist = append(flist, fileListReader{
			filePath:     "/opt/agi/sftp.key",
			fileContents: f,
			fileSize:     int(stat.Size()),
		})
	}

	// make install script
	edition := "amd64"
	if isArm {
		edition = "arm64"
	}
	maxDp := 34560000
	if a.opts.Config.Backend.Type == "docker" {
		maxDp = maxDp / 2
	}
	cpuProfiling := ""
	if c.PluginCpuProfile {
		cpuProfiling = "cpuProfilingOutputFile: \"/opt/agi/cpu.plugin.pprof\""
	}
	proxyPort := 80
	proxySSL := ""
	if !c.ProxyDisableSSL {
		proxySSL = "-S"
		proxyPort = 443
	}
	proxyCert := "\"\""
	proxyKey := proxyCert
	if c.ProxyCert != "" {
		proxyCert = "/opt/agi/proxy.cert"
	} else if c.ProxyCert == "" && !c.ProxyDisableSSL {
		proxyCert = "/opt/agi/proxy.cert" //"/etc/ssl/certs/ssl-cert-snakeoil.pem"
	}
	if c.ProxyKey != "" {
		proxyKey = "/opt/agi/proxy.key"
	} else if c.ProxyKey == "" && !c.ProxyDisableSSL {
		proxyKey = "/opt/agi/proxy.key" //"/etc/ssl/private/ssl-cert-snakeoil.key"
	}
	proxyMaxInactive := c.ProxyMaxInactive.String()
	proxyMaxUptime := c.ProxyMaxUptime.String()
	installScript := ""
	notifierYaml, _ := yaml.Marshal(c.hTTPSNotify)
	override := "1"
	if c.NoConfigOverride {
		override = "0"
	}
	nver := strings.Split(c.AerospikeVersion.String(), ".")
	//memory-size %dG
	var memSizeStr, storEngine, dimStr, rpcStr, wbs, maxWriteCache string
	var fileSizeInt int
	maxWriteCache = "max-write-cache 1024M"
	if inslice.HasString([]string{"6", "5", "4", "3"}, nver[0]) {
		memSizeStr = "memory-size " + strconv.Itoa(memSize/1024/1024/1024) + "G"
		storEngine = "device"
		fileSizeInt = memSize / 1024 / 1024 / 1024
		if c.NoDIM && c.NoDIMFileSize != 0 {
			fileSizeInt = c.NoDIMFileSize
		} else if c.NoDIM {
			fileSizeInt = 2000
		}
		dimStr = fmt.Sprintf("data-in-memory %t", !c.NoDIM)
		rpcStr = fmt.Sprintf("read-page-cache %t", c.NoDIM)
		wbs = "write-block-size 8M"
	} else {
		if c.NoDIM {
			storEngine = "device"
			fileSizeInt = 2000
			if c.NoDIMFileSize != 0 {
				fileSizeInt = c.NoDIMFileSize
			}
			rpcStr = fmt.Sprintf("read-page-cache %t", c.NoDIM)
			wbs = "write-block-size 8M"
		} else {
			storEngine = "memory"
			fileSizeInt = int(float64(memSize/1024/1024/1024) / 1.25)
		}
	}
	nveri, _ := strconv.Atoi(nver[0])
	if (nver[0] == "7" && len(nver) > 1 && nver[1] != "0") || nveri > 7 {
		wbs = ""
	}
	cedition := "x86_64"
	if edition == "arm64" {
		cedition = "aarch64"
	}

	// upgrade tools package
	toolsUpgrade := ""
	if !c.NoToolsOverride {
		toolsUpgrade = fmt.Sprintf("mkdir /tmp/toolsupgrade && pushd /tmp/toolsupgrade && aerolab installer download -d %s -i %s && tar -zxvf aerospike-server* && rm -rf *tgz && cd aerospike-server* && rm -f aerospike-server* && ./asinstall && popd", a.opts.Cluster.Create.DistroName, a.opts.Cluster.Create.DistroVersion)
	}

	if a.opts.Config.Backend.Type == "docker" {
		installScript = fmt.Sprintf(agiCreateScriptDocker, override, c.NoDIM, c.Owner, c.GrafanaVersion, edition, c.GrafanaVersion, edition, c.GrafanaVersion, cedition, toolsUpgrade, memSizeStr, storEngine, fileSizeInt, dimStr, rpcStr, wbs, c.AGILabel, c.ClusterName, c.ClusterName, c.ClusterName, c.AGILabel, proxyPort, proxySSL, proxyCert, proxyKey, proxyMaxInactive, proxyMaxUptime, maxDp, c.PluginLogLevel, cpuProfiling, notifierYaml)
	} else {
		shutdownCmd := "/usr/bin/systemctl stop aerospike; /usr/bin/sync; /sbin/poweroff -p || /sbin/poweroff"
		installScript = fmt.Sprintf(agiCreateScript, override, c.NoDIM, c.Owner, c.GrafanaVersion, edition, c.GrafanaVersion, edition, c.GrafanaVersion, cedition, toolsUpgrade, memSizeStr, storEngine, fileSizeInt, dimStr, rpcStr, wbs, maxWriteCache, c.AGILabel, c.ClusterName, c.ClusterName, shutdownCmd, c.ClusterName, c.AGILabel, proxyPort, proxySSL, proxyCert, proxyKey, proxyMaxInactive, proxyMaxUptime, maxDp, c.PluginLogLevel, cpuProfiling, notifierYaml)
	}
	flist = append(flist, fileListReader{filePath: "/root/agiinstaller.sh", fileContents: strings.NewReader(installScript), fileSize: len(installScript)})

	// upload agiCreate config
	c.SftpPass = ""
	c.S3Secret = ""
	c.SftpKey = ""
	c.ProxyCert = ""
	c.ProxyKey = ""
	c.LocalSource = ""
	c.ClusterSource = ""
	c.PatternsFile = ""
	c.ChDir = ""
	c.FeaturesFilePath = ""
	c.NoConfigOverride = true
	deploymentDetail, _ := json.Marshal(c)
	deploymentDetail, _ = gz(deploymentDetail)
	flist = append(flist, fileListReader{
		filePath:     "/tmp/deployment.json.gz",
		fileContents: bytes.NewReader(deploymentDetail),
		fileSize:     len(deploymentDetail),
	})
	if c.uploadAuthorizedContentsGzB64 != "" {
		flist = append(flist, fileListReader{
			filePath:     "/tmp/aerolab.install.ssh",
			fileContents: strings.NewReader(c.uploadAuthorizedContentsGzB64),
			fileSize:     len(c.uploadAuthorizedContentsGzB64),
		})
	}

	// upload all files and run installer
	err = b.CopyFilesToClusterReader(c.ClusterName.String(), flist, []int{1})
	if err != nil {
		return fmt.Errorf("could not upload configuration to instance: %s", err)
	}
	out, err = b.RunCommands(c.ClusterName.String(), [][]string{{"/bin/bash", "/root/agiinstaller.sh"}}, []int{1})
	if err != nil {
		return fmt.Errorf("failed to run install script: %s\n%s", err, string(out[0]))
	}
	if a.opts.Config.Backend.Type == "docker" {
		a.opts.Cluster.Stop.ClusterName = c.ClusterName
		a.opts.Cluster.Stop.Nodes = "1"
		err = a.opts.Cluster.Stop.doStop("agi")
		if err != nil {
			return err
		}
		a.opts.Cluster.Start.ClusterName = c.ClusterName
		a.opts.Cluster.Start.Nodes = "1"
		a.opts.Cluster.Start.NoStart = true
		err = a.opts.Cluster.Start.doStart("agi")
		if err != nil {
			return err
		}
	}
	if a.opts.Config.Backend.Type == "aws" && c.Aws.Route53ZoneId != "" {
		log.Println("Waiting on Route53 update to complete...")
		wgDns.Wait()
	}
	log.Println("Done")
	log.Println("* aerolab agi help                 - list of available AGI commands")
	log.Println("* aerolab agi list                 - get web URL")
	log.Printf("* aerolab agi add-auth-token -n %s       - generate an authentication token", c.ClusterName.String())
	log.Printf("* aerolab agi add-auth-token -n %s --url - generate an authentication token and display a quick-access url", c.ClusterName.String())
	log.Printf("* aerolab agi attach -n %s               - attach to the shell; log files are at /opt/agi/files/", c.ClusterName.String())
	return nil
}

//go:embed cmdAgiCreate.script.cloud.sh
var agiCreateScript string

//go:embed cmdAgiCreate.script.docker.sh
var agiCreateScriptDocker string

//go:embed cmdAgiCreateLogtags.py
var agiCreateLogTags string

func gz(p []byte) (r []byte, err error) {
	buf := &bytes.Buffer{}
	g := gzip.NewWriter(buf)
	_, err = g.Write(p)
	if err != nil {
		g.Close()
		return nil, err
	}
	g.Close()
	return buf.Bytes(), nil
}

func agiLogsGet(clusterName TypeClusterName) (destination flags.Filename, err error) {
	destination = flags.Filename(strconv.Itoa(int(time.Now().UnixMilli())))
	location := agiGetLogLocation(clusterName, 1)
	if location == "" {
		return "", errors.New("no log location found")
	}
	a.opts.Logs.Get.ClusterName = clusterName
	a.opts.Logs.Get.Nodes = ""
	a.opts.Logs.Get.Destination = destination
	a.opts.Logs.Get.Force = true
	a.opts.Logs.Get.Journal = location == "JOURNALCTL"
	a.opts.Logs.Get.LogLocation = location
	err = a.opts.Logs.Get.Execute(nil)
	if err != nil {
		return "", err
	}
	return destination, nil
}

// returns "JOURNALCTL" if the cluster is using journalctl, otherwise returns the log location
func agiGetLogLocation(clusterName TypeClusterName, node int) string {
	out, err := b.RunCommands(clusterName.String(), [][]string{{"cat", "/etc/aerospike/aerospike.conf"}}, []int{node})
	if err != nil {
		return ""
	}
	conf, err := aeroconf.Parse(bytes.NewReader(out[0]))
	if err != nil {
		return ""
	}
	if conf.Type("logging") == aeroconf.ValueNil {
		return "JOURNALCTL"
	}
	conf = conf.Stanza("logging")
	if conf.Type("console") != aeroconf.ValueNil {
		return "JOURNALCTL"
	}
	for _, key := range conf.ListKeys() {
		if strings.HasPrefix(key, "file ") {
			return strings.TrimPrefix(key, "file ")
		}
	}
	return ""
}

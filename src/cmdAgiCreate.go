package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "embed"

	"github.com/aerospike/aerolab/ingest"
	flags "github.com/rglonek/jeddevdk-goflags"
	"gopkg.in/yaml.v2"
)

type agiCreateCmd struct {
	ClusterName             TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	AGILabel                string          `long:"agi-label" description:"friendly label"`
	LocalSource             flags.Filename  `long:"source-local" description:"get logs from a local directory"`
	SftpEnable              bool            `long:"source-sftp-enable" description:"enable sftp source"`
	SftpThreads             int             `long:"source-sftp-threads" description:"number of concurrent downloader threads" default:"6"`
	SftpHost                string          `long:"source-sftp-host" description:"sftp host"`
	SftpPort                int             `long:"source-sftp-port" description:"sftp port" default:"22"`
	SftpUser                string          `long:"source-sftp-user" description:"sftp user"`
	SftpPass                string          `long:"source-sftp-pass" description:"sftp password"`
	SftpKey                 flags.Filename  `long:"source-sftp-key" description:"key to use for sftp login for log download, alternative to password"`
	SftpPath                string          `long:"source-sftp-path" description:"path on sftp to download logs from"`
	SftpRegex               string          `long:"source-sftp-regex" description:"regex to apply for choosing what to download, the regex is applied on paths AFTER the sftp-path specification, not the whole path; start wih ^"`
	S3Enable                bool            `long:"source-s3-enable" description:"enable s3 source"`
	S3Threads               int             `long:"source-s3-threads" description:"number of concurrent downloader threads" default:"6"`
	S3Region                string          `long:"source-s3-region" description:"aws region where the s3 bucket is located"`
	S3Bucket                string          `long:"source-s3-bucket" description:"s3 bucket name"`
	S3KeyID                 string          `long:"source-s3-key-id" description:"(optional) access key ID"`
	S3Secret                string          `long:"source-s3-secret-key" description:"(optional) secret key"`
	S3path                  string          `long:"source-s3-path" description:"path on s3 to download logs from"`
	S3Regex                 string          `long:"source-s3-regex" description:"regex to apply for choosing what to download, the regex is applied on paths AFTER the s3-path specification, not the whole path; start wih ^"`
	ProxyEnableSSL          bool            `long:"proxy-ssl-enable" description:"switch to enable TLS on the proxy"`
	ProxyCert               flags.Filename  `long:"proxy-ssl-cert" description:"if not provided snakeoil will be used"`
	ProxyKey                flags.Filename  `long:"proxy-ssl-key" description:"if not provided snakeoil will be used"`
	ProxyMaxInactive        time.Duration   `long:"proxy-max-inactive" description:"maximum duration of inactivity by the user over which the server will poweroff" default:"1h"`
	ProxyMaxUptime          time.Duration   `long:"proxy-max-uptime" description:"maximum uptime of the instance, after which the server will poweroff" default:"24h"`
	TimeRanges              bool            `long:"ingest-timeranges-enable" description:"enable importing statistics only on a specified time range found in the logs"`
	TimeRangesFrom          time.Time       `long:"ingest-timeranges-from" description:"time range from, format: 2006-01-02T15:04:05Z07:00"`
	TimeRangesTo            time.Time       `long:"ingest-timeranges-to" description:"time range to, format: 2006-01-02T15:04:05Z07:00"`
	CustomSourceName        string          `long:"ingest-custom-source-name" description:"custom source name to disaplay in grafana"`
	PatternsFile            flags.Filename  `long:"ingest-patterns-file" description:"provide a custom patterns YAML file to the log ingest system"`
	IngestLogLevel          int             `long:"ingest-log-level" description:"1-CRITICAL,2-ERROR,3-WARN,4-INFO,5-DEBUG,6-DETAIL" default:"4"`
	IngestCpuProfile        bool            `long:"ingest-cpu-profiling" description:"enable log ingest cpu profiling"`
	PluginCpuProfile        bool            `long:"plugin-cpu-profiling" description:"enable CPU profiling for the grafana plugin"`
	PluginLogLevel          int             `long:"plugin-log-level" description:"1-CRITICAL,2-ERROR,3-WARN,4-INFO,5-DEBUG,6-DETAIL" default:"4"`
	FeaturesFilePath        flags.Filename  `short:"f" long:"featurefile" description:"Features file to install, or directory containing feature files"`
	FeaturesFilePrintDetail bool            `long:"featurefile-printdetail" description:"Print details of discovered features files" hidden:"true"`
	chDirCmd
	NoVacuumOnFail bool               `long:"no-vacuum" description:"if set, will not remove the template instance/container should it fail installation"`
	Aws            agiCreateCmdAws    `no-flag:"true"`
	Gcp            agiCreateCmdGcp    `no-flag:"true"`
	Docker         agiCreateCmdDocker `no-flag:"true"`
	Owner          string             `long:"owner" description:"AWS/GCP only: create owner tag with this value"`
}

type agiCreateCmdAws struct {
	InstanceType    string        `short:"I" long:"instance-type" description:"instance type to use" default:"r5a.xlarge"`
	Ebs             string        `short:"E" long:"ebs" description:"EBS volume size GB" default:"40"`
	SecurityGroupID string        `short:"S" long:"secgroup-id" description:"security group IDs to use, comma-separated; default: empty: create and auto-manage"`
	SubnetID        string        `short:"U" long:"subnet-id" description:"subnet-id, availability-zone name, or empty; default: empty: first found in default VPC"`
	Tags            []string      `long:"tags" description:"apply custom tags to instances; format: key=value; this parameter can be specified multiple times"`
	NamePrefix      []string      `long:"secgroup-name" description:"Name prefix to use for the security groups, can be specified multiple times" default:"AeroLab"`
	Expires         time.Duration `long:"aws-expire" description:"length of life of nodes prior to expiry; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry; grow default: match existing cluster" default:"30h"`
}

type agiCreateCmdGcp struct {
	InstanceType string        `long:"instance" description:"instance type to use" default:"e2-highmem-4"`
	Disks        []string      `long:"disk" description:"format type:sizeGB, ex: pd-ssd:20 ex: pd-balanced:40" default:"pd-ssd:40"`
	Zone         string        `long:"zone" description:"zone name to deploy to"`
	Tags         []string      `long:"tag" description:"apply custom tags to instances; this parameter can be specified multiple times"`
	Labels       []string      `long:"label" description:"apply custom labels to instances; format: key=value; this parameter can be specified multiple times"`
	NamePrefix   []string      `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external"`
	Expires      time.Duration `long:"gcp-expire" description:"length of life of nodes prior to expiry; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry; grow default: match existing cluster" default:"30h"`
}

type agiCreateCmdDocker struct {
	ExposePortsToHost string `short:"e" long:"expose-ports" description:"If a single machine is being deployed, port forward. Format: HOST_PORT:NODE_PORT,HOST_PORT:NODE_PORT"`
	CpuLimit          string `short:"l" long:"cpu-limit" description:"Impose CPU speed limit. Values acceptable could be '1' or '2' or '0.5' etc." default:""`
	RamLimit          string `short:"t" long:"ram-limit" description:"Limit RAM available to each node, e.g. 500m, or 1g." default:""`
	SwapLimit         string `short:"w" long:"swap-limit" description:"Limit the amount of total memory (ram+swap) each node can use, e.g. 600m. If ram-limit==swap-limit, no swap is available." default:""`
	Privileged        bool   `short:"B" long:"privileged" description:"Docker only: run container in privileged mode"`
	NetworkName       string `long:"network" description:"specify a network name to use for non-default docker network; for more info see: aerolab config docker help" default:""`
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
	log.Println("Starting AGI deployment...")
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
	a.opts.Cluster.Create.DistroName = "ubuntu"
	a.opts.Cluster.Create.DistroVersion = "22.04"
	a.opts.Cluster.Create.AerospikeVersion = "6.4.0.*"
	a.opts.Cluster.Create.Username = ""
	a.opts.Cluster.Create.Password = ""
	a.opts.Cluster.Create.Aws.AMI = ""
	a.opts.Cluster.Create.Aws.InstanceType = c.Aws.InstanceType
	a.opts.Cluster.Create.Aws.Ebs = c.Aws.Ebs
	a.opts.Cluster.Create.Aws.SecurityGroupID = c.Aws.SecurityGroupID
	a.opts.Cluster.Create.Aws.SubnetID = c.Aws.SubnetID
	a.opts.Cluster.Create.Aws.Tags = append(c.Aws.Tags, "aerolab4features="+strconv.Itoa(int(ClusterFeatureAGI)), fmt.Sprintf("aerolab4ssl=%t", c.ProxyEnableSSL), fmt.Sprintf("agiLabel=%s", c.AGILabel))
	a.opts.Cluster.Create.Aws.NamePrefix = c.Aws.NamePrefix
	a.opts.Cluster.Create.Aws.Expires = c.Aws.Expires
	a.opts.Cluster.Create.Aws.PublicIP = false
	a.opts.Cluster.Create.Aws.IsArm = false
	a.opts.Cluster.Create.Aws.NoBestPractices = false
	a.opts.Cluster.Create.Gcp.Image = ""
	a.opts.Cluster.Create.Gcp.InstanceType = c.Gcp.InstanceType
	a.opts.Cluster.Create.Gcp.Disks = c.Gcp.Disks
	a.opts.Cluster.Create.Gcp.PublicIP = false
	a.opts.Cluster.Create.Gcp.Zone = c.Gcp.Zone
	a.opts.Cluster.Create.Gcp.IsArm = false
	a.opts.Cluster.Create.Gcp.NoBestPractices = false
	a.opts.Cluster.Create.Gcp.Tags = c.Gcp.Tags
	a.opts.Cluster.Create.Gcp.Labels = append(c.Gcp.Labels, "aerolab4features="+strconv.Itoa(int(ClusterFeatureAGI)), fmt.Sprintf("aerolab4ssl=%t", c.ProxyEnableSSL))
	a.opts.Cluster.Create.gcpMeta = map[string]string{
		"agiLabel": c.AGILabel,
	}
	a.opts.Cluster.Create.Gcp.NamePrefix = c.Gcp.NamePrefix
	a.opts.Cluster.Create.Gcp.Expires = c.Gcp.Expires
	if c.ProxyEnableSSL {
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
	err := a.opts.Cluster.Create.realExecute2(args, false)
	if err != nil {
		return err
	}

	log.Println("Cluster Node created, continuing AGI deployment...")

	// docker will use max 2GB on plugin, aws/gcp 4GB; for aws/gcp we should leave 6GB of RAM unused (4GB-plugin 2GB-OS); for docker: 3GB (2GB-plugin 1GB-everything else)
	out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"free", "-b"}}, []int{1})
	if err != nil {
		return fmt.Errorf("could not get available memory on node: %s: %s", err, string(out[0]))
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
	nLinuxBinary := nLinuxBinaryX64
	if isArm {
		nLinuxBinary = nLinuxBinaryArm64
	}
	flist := []fileListReader{
		{
			filePath:     "/usr/local/bin/aerolab",
			fileContents: bytes.NewReader(nLinuxBinary),
			fileSize:     len(nLinuxBinary),
		},
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

	// upload sftp key
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

	// upload proxy key
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

	// generate ingest.yaml
	config, err := ingest.MakeConfigReader(true, nil, true)
	if err != nil {
		return fmt.Errorf("create.ingest.MakeConfig: %s", err)
	}
	config.Aerospike.MaxPutThreads = 1024
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
		config.IngestTimeRanges.From = c.TimeRangesFrom
		config.IngestTimeRanges.To = c.TimeRangesTo
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
	conf, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("could not marshal ingest configuration to yaml: %s", err)
	}
	flist = append(flist, fileListReader{
		filePath:     "/opt/agi/ingest.yaml",
		fileContents: bytes.NewReader(conf),
		fileSize:     len(conf),
	})

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
	if c.ProxyEnableSSL {
		proxySSL = "-S"
		proxyPort = 443
	}
	proxyCert := "\"\""
	proxyKey := proxyCert
	if c.ProxyCert != "" {
		proxyCert = "/opt/agi/proxy.cert"
	} else if c.ProxyCert == "" && c.ProxyEnableSSL {
		proxyCert = "/etc/ssl/certs/ssl-cert-snakeoil.pem"
	}
	if c.ProxyKey != "" {
		proxyKey = "/opt/agi/proxy.key"
	} else if c.ProxyKey == "" && c.ProxyEnableSSL {
		proxyKey = "/etc/ssl/private/ssl-cert-snakeoil.key"
	}
	proxyMaxInactive := c.ProxyMaxInactive.String()
	proxyMaxUptime := c.ProxyMaxUptime.String()
	installScript := ""
	if a.opts.Config.Backend.Type == "docker" {
		installScript = fmt.Sprintf(agiCreateScriptDocker, edition, edition, memSize/1024/1024/1024, memSize/1024/1024/1024, c.AGILabel, proxyPort, proxySSL, proxyCert, proxyKey, proxyMaxInactive, proxyMaxUptime, maxDp, c.PluginLogLevel, cpuProfiling)
	} else {
		installScript = fmt.Sprintf(agiCreateScript, edition, edition, memSize/1024/1024/1024, memSize/1024/1024/1024, c.AGILabel, proxyPort, proxySSL, proxyCert, proxyKey, proxyMaxInactive, proxyMaxUptime, maxDp, c.PluginLogLevel, cpuProfiling)
	}
	flist = append(flist, fileListReader{filePath: "/root/agiinstaller.sh", fileContents: strings.NewReader(installScript), fileSize: len(installScript)})

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
		a.opts.Cluster.Stop.Execute(nil)
		a.opts.Cluster.Start.ClusterName = c.ClusterName
		a.opts.Cluster.Start.Nodes = "1"
		a.opts.Cluster.Start.NoStart = true
		a.opts.Cluster.Start.Execute(nil)
	}
	log.Println("Done")
	log.Println("* aerolab agi help                 - list of available AGI commands")
	log.Println("* aerolab agi list                 - get web URL")
	log.Printf("* aerolab agi add-auth-token -n %s - generate an authentication token", c.ClusterName.String())
	log.Printf("* aerolab agi attach -n %s         - attach to the shell; log files are at /opt/agi/files/", c.ClusterName.String())
	return nil
}

//go:embed cmdAgiCreate.script.cloud.sh
var agiCreateScript string

//go:embed cmdAgiCreate.script.docker.sh
var agiCreateScriptDocker string

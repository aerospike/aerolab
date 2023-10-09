package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	_ "embed"

	flags "github.com/rglonek/jeddevdk-goflags"
)

type agiCreateCmd struct {
	ClusterName             TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
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
	InstanceType    string        `short:"I" long:"instance-type" description:"instance type to use" default:""`
	Ebs             string        `short:"E" long:"ebs" description:"EBS volume size GB" default:"40"`
	SecurityGroupID string        `short:"S" long:"secgroup-id" description:"security group IDs to use, comma-separated; default: empty: create and auto-manage"`
	SubnetID        string        `short:"U" long:"subnet-id" description:"subnet-id, availability-zone name, or empty; default: empty: first found in default VPC"`
	Tags            []string      `long:"tags" description:"apply custom tags to instances; format: key=value; this parameter can be specified multiple times"`
	NamePrefix      []string      `long:"secgroup-name" description:"Name prefix to use for the security groups, can be specified multiple times" default:"AeroLab"`
	Expires         time.Duration `long:"aws-expire" description:"length of life of nodes prior to expiry; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry; grow default: match existing cluster" default:"30h"`
}

type agiCreateCmdGcp struct {
	InstanceType string        `long:"instance" description:"instance type to use" default:""`
	Disks        []string      `long:"disk" description:"format type:sizeGB, ex: pd-ssd:20 ex: pd-balanced:40" default:"pd-ssd:40"`
	Zone         string        `long:"zone" description:"zone name to deploy to"`
	Tags         []string      `long:"tag" description:"apply custom tags to instances; this parameter can be specified multiple times"`
	Labels       []string      `long:"label" description:"apply custom labels to instances; format: key=value; this parameter can be specified multiple times"`
	NamePrefix   []string      `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external"`
	Expires      time.Duration `long:"gcp-expire" description:"length of life of nodes prior to expiry; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry; grow default: match existing cluster" default:"30h"`
}

type agiCreateCmdDocker struct {
	ExposePortsToHost string `short:"e" long:"expose-ports" description:"If a single machine is being deployed, port forward. Format: HOST_PORT:NODE_PORT,HOST_PORT:NODE_PORT" default:"4433:443"`
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
	a.opts.Cluster.Create.Aws.Tags = append(c.Aws.Tags, "aerolab4features="+strconv.Itoa(int(ClusterFeatureAGI)))
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
	a.opts.Cluster.Create.Gcp.Labels = append(c.Gcp.Labels, "aerolab4features="+strconv.Itoa(int(ClusterFeatureAGI)))
	a.opts.Cluster.Create.Gcp.NamePrefix = c.Gcp.NamePrefix
	a.opts.Cluster.Create.Gcp.Expires = c.Gcp.Expires
	a.opts.Cluster.Create.Docker.ExposePortsToHost = c.Docker.ExposePortsToHost
	a.opts.Cluster.Create.Docker.NoAutoExpose = true
	a.opts.Cluster.Create.Docker.CpuLimit = c.Docker.CpuLimit
	a.opts.Cluster.Create.Docker.RamLimit = c.Docker.RamLimit
	a.opts.Cluster.Create.Docker.SwapLimit = c.Docker.SwapLimit
	a.opts.Cluster.Create.Docker.Privileged = c.Docker.Privileged
	a.opts.Cluster.Create.Docker.NetworkName = c.Docker.NetworkName
	a.opts.Cluster.Create.Docker.ClientType = strconv.Itoa(int(ClusterFeatureAGI))
	err := a.opts.Cluster.Create.realExecute2(args, false)
	if err != nil {
		return err
	}
	log.Println("Cluster Node created, continuing AGI deployment...")
	// TODO deploy aerolab to /usr/local/bin/aerolab
	// TODO upload custom patterns file if specified, to /opt/agi/patterns.yaml
	// TODO upload custom sftp key file if specified, to /opt/agi/sftp.key
	// TODO if local source, mkdir and upload files directly to /opt/agi/files/input
	// TODO below need to become command line parameters in this function
	isArm := false
	memSizeGB := 24
	fileSizeGB := 24
	agiLabel := "initial label"
	authType := "basic"
	proxyExecParams := "-u admin -p secure"
	pluginCpuProfiling := false
	pluginLogLevel := 6
	docker := false
	ingestLogLevel := 6
	maxPutThreads := 1024
	ingestCpuProfiling := false
	timeRangesEnable := false
	timeRangeFrom := "#from: \"\" #2006-01-02T15:04:05Z07:00"
	timeRangeTo := "#from: \"\" #2006-01-02T15:04:05Z07:00"
	customSourceName := ""
	sftpEnabled := false
	sftpThreads := 6
	sftpHost := "asftp.aerospike.com"
	sftpPort := 22
	sftpUser := ""
	sftpPass := ""
	sftpPath := ""
	sftpRegex := ""
	s3Enabled := false
	s3Threads := 6
	s3Region := ""
	s3Bucket := ""
	s3KeyID := ""
	s3Secret := ""
	s3Path := ""
	s3Regex := ""
	patternsFile := ""
	sftpKeyFile := ""

	maxDp := 34560000
	if docker {
		maxDp = maxDp / 2
		maxPutThreads = 64
	}
	edition := "amd64"
	if isArm {
		edition = "arm64"
	}
	cpuProfiling := ""
	if pluginCpuProfiling {
		cpuProfiling = "cpuProfilingOutputFile: \"/opt/agi/cpu.plugin.pprof\""
	}
	ingestProfiling := ""
	if ingestCpuProfiling {
		ingestProfiling = "cpuProfilingOutputFile: \"/opt/agi/cpu.ingest.pprof\""
	}
	patternsFileRemote := ""
	if patternsFile != "" {
		patternsFileRemote = "/opt/agi/patterns.yaml"
	}
	sftpRemote := ""
	if sftpKeyFile != "" {
		sftpRemote = "/opt/agi/sftp.key"
	}
	installScript := fmt.Sprintf(agiCreateScript, edition, edition, memSizeGB, fileSizeGB, agiLabel, authType, proxyExecParams, maxDp, pluginLogLevel, cpuProfiling, ingestLogLevel, maxPutThreads, ingestProfiling, patternsFileRemote, timeRangesEnable, timeRangeFrom, timeRangeTo, customSourceName, sftpEnabled, sftpThreads, sftpHost, sftpPort, sftpUser, sftpPass, sftpRemote, sftpPath, sftpRegex, s3Enabled, s3Threads, s3Region, s3Bucket, s3KeyID, s3Secret, s3Path, s3Regex)
	err = b.CopyFilesToClusterReader(c.ClusterName.String(), []fileListReader{{filePath: "/root/agiinstaller.sh", fileContents: strings.NewReader(installScript), fileSize: len(installScript)}}, []int{1})
	if err != nil {
		return fmt.Errorf("could not copy install script to instance: %s", err)
	}
	out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"/bin/bash", "/root/agiinstaller.sh"}}, []int{1})
	if err != nil {
		return fmt.Errorf("failed to run install script: %s\n%s", err, string(out[0]))
	}
	log.Println("Done, run `aerolab agi list` to get web URL, or attach with `aerolab agi attach` to continue.")
	return nil
}

//go:embed cmdAgiCreate.script.sh
var agiCreateScript string

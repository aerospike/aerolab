package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/diff"
	"github.com/aerospike/aerolab/ingest"
	flags "github.com/rglonek/jeddevdk-goflags"
	"gopkg.in/yaml.v3"
)

func init() {
	addBackendSwitch("agi.delete", "gcp", &a.opts.AGI.Delete.Gcp)
}

type agiCmd struct {
	List      agiListCmd      `command:"list" subcommands-optional:"true" description:"List AGI instances" webicon:"fas fa-list"`
	Create    agiCreateCmd    `command:"create" subcommands-optional:"true" description:"Create AGI instance" webicon:"fas fa-circle-plus" invwebforce:"true"`
	Start     agiStartCmd     `command:"start" subcommands-optional:"true" description:"Start AGI instance" webicon:"fas fa-play" invwebforce:"true"`
	Stop      agiStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop AGI instance" webicon:"fas fa-stop" invwebforce:"true"`
	Status    agiStatusCmd    `command:"status" subcommands-optional:"true" description:"Show status of an AGI instance" webicon:"fas fa-circle-question"`
	Details   agiDetailsCmd   `command:"details" subcommands-optional:"true" description:"Show details of an AGI instance" webicon:"fas fa-circle-info"`
	Destroy   agiDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy AGI instance" webicon:"fas fa-trash" invwebforce:"true"`
	Delete    agiDeleteCmd    `command:"delete" subcommands-optional:"true" description:"Destroy AGI instance and Delete AGI EFS volume of the same name" webicon:"fas fa-dumpster" invwebforce:"true" simplemode:"false"`
	Relabel   agiRelabelCmd   `command:"change-label" subcommands-optional:"true" description:"Change instance name label" webicon:"fas fa-tag"`
	Retrigger agiRetriggerCmd `command:"run-ingest" subcommands-optional:"true" description:"Retrigger log ingest again (will only do bits that have not been done before)" webicon:"fas fa-water"`
	Attach    agiAttachCmd    `command:"attach" subcommands-optional:"true" description:"Attach to an AGI Instance" webicon:"fas fa-terminal" simplemode:"false"`
	AddToken  agiAddTokenCmd  `command:"add-auth-token" subcommands-optional:"true" description:"Add an auth token to AGI Proxy - only valid if token auth type was selected" webicon:"fas fa-key"`
	Share     clusterShareCmd `command:"share" subcommands-optional:"true" description:"AWS/GCP: share the AGI node by importing a provided ssh public key file" webicon:"fas fa-share"`
	Exec      agiExecCmd      `command:"exec" hidden:"true" subcommands-optional:"true" description:"Run an AGI subsystem"`
	Monitor   agiMonitorCmd   `command:"monitor" subcommands-optional:"true" description:"AGI auto-sizing and spot->on-demand upgrading system monitor" webicon:"fas fa-equals" simplemode:"false"`
	Help      helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type agiListCmd struct {
	Owner      string   `long:"owner" description:"Only show resources tagged with this owner"`
	SortBy     []string `long:"sort-by" description:"sort by field name; must match exact header name; can be specified multiple times; format: asc:name dsc:name ascnum:name dscnum:name"`
	Json       bool     `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty bool     `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	Pager      bool     `long:"pager" description:"set to enable vertical and horizontal pager"`
	RenderType string   `long:"render" description:"different output rendering; supported: text,csv,tsv,html,markdown" default:"text"`
	Help       helpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if len(c.SortBy) == 0 {
		c.SortBy = []string{"dscnum:ExpiryTs", "dscnum:VolExpiryTs"}
	}
	a.opts.Inventory.List.Json = c.Json
	a.opts.Inventory.List.Owner = c.Owner
	a.opts.Inventory.List.Pager = c.Pager
	a.opts.Inventory.List.SortBy = c.SortBy
	a.opts.Inventory.List.JsonPretty = c.JsonPretty
	a.opts.Inventory.List.RenderType = c.RenderType
	return a.opts.Inventory.List.run(false, false, false, false, false, inventoryShowAGI|inventoryShowAGIStatus)
}

type agiStartCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiStartCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Cluster.Start.ClusterName = c.ClusterName
	a.opts.Cluster.Start.Nodes = "1"
	a.opts.Cluster.Start.NoStart = true
	return a.opts.Cluster.Start.doStart("agi")
}

type agiStopCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiStopCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Cluster.Stop.ClusterName = c.ClusterName
	a.opts.Cluster.Stop.Nodes = "1"
	return a.opts.Cluster.Stop.doStop("agi")
}

type agiAddTokenCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	TokenName   string          `short:"u" long:"token-name" description:"a unique token name; default:auto-generate"`
	TokenSize   int             `short:"s" long:"size" description:"size of the new token to be generated" default:"128"`
	Token       string          `short:"t" long:"token" description:"A 64+ character long token to use; if not specified, a random token will be generated"`
	GenURL      bool            `long:"url" description:"Generate an display a direct-access token URL; this isn't fully secure as proxies, if user uses them, can capture this"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiAddTokenCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.TokenSize < 64 {
		return fmt.Errorf("minimum token size is 64")
	}
	nodeUrl := ""
	if c.GenURL {
		inv, err := b.Inventory("", []int{InventoryItemClusters, InventoryItemAGI})
		if err != nil {
			return err
		}
		for vi, v := range inv.Clusters {
			if v.ClusterName != string(c.ClusterName) || v.Features&ClusterFeatureAGI == 0 {
				continue
			}
			nip := v.PublicIp
			if nip == "" {
				nip = v.PrivateIp
			}
			if nip == "" {
				return errors.New("AGI node IP is empty, is AGI down? See: aerolab agi list")
			}
			port := ""
			if a.opts.Config.Backend.Type == "docker" && inv.Clusters[vi].DockerExposePorts != "" {
				nip = "127.0.0.1"
				port = ":" + inv.Clusters[vi].DockerExposePorts
			}
			prot := "http://"
			if v.GcpLabels["aerolab4ssl"] == "true" || v.AwsTags["aerolab4ssl"] == "true" || v.DockerInternalPort == "443" {
				prot = "https://"
			}
			if a.opts.Config.Backend.Type == "aws" {
				if v.AwsTags["agiDomain"] != "" {
					nip = v.InstanceId + "." + a.opts.Config.Backend.Region + ".agi." + v.AwsTags["agiDomain"]
				}
			}
			nodeUrl = prot + nip + port + "/agi/menu?AGI_TOKEN="
		}
		if nodeUrl == "" {
			return errors.New("cluster IP could not be retrieved, cluster not found")
		}
		b.WorkOnServers()
	}
	loc := "/opt/agi/tokens"
	if c.TokenName == "" {
		c.TokenName = strconv.Itoa(int(time.Now().UnixNano()))
	}
	newToken := randToken(c.TokenSize, rand.NewSource(int64(time.Now().UnixNano())))
	loc = path.Join(loc, c.TokenName)
	err := b.CopyFilesToClusterReader(c.ClusterName.String(), []fileListReader{{
		filePath:     loc,
		fileContents: strings.NewReader(newToken),
		fileSize:     c.TokenSize,
	}}, []int{1})
	if err != nil {
		return err
	}
	b.RunCommands(c.ClusterName.String(), [][]string{{"bash", "-c", "kill -HUP $(systemctl show --property MainPID --value agi-proxy)"}}, []int{1})
	time.Sleep(time.Second)
	if !c.GenURL {
		fmt.Println(newToken)
	} else {
		fmt.Println(nodeUrl + newToken)
	}
	return nil
}

func randToken(n int, src rand.Source) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const (
		letterIdxBits = 6                    // 6 bits to represent a letter index
		letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
		letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
	)
	sb := strings.Builder{}
	sb.Grow(n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
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

type agiDestroyCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	Force       bool            `short:"f" long:"force" description:"force stop before destroy"`
	Parallel    bool            `short:"p" long:"parallel" description:"if destroying many AGI at once, set this to destroy in parallel"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiDestroyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Cluster.Destroy.ClusterName = c.ClusterName
	a.opts.Cluster.Destroy.Force = c.Force
	a.opts.Cluster.Destroy.Parallel = c.Parallel
	return a.opts.Cluster.Destroy.doDestroy("agi", args)
}

type agiDeleteCmd struct {
	agiDestroyCmd
	Gcp volumeDeleteGcpCmd `no-flag:"true"`
}

func (c *agiDeleteCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Cluster.Destroy.ClusterName = c.ClusterName
	a.opts.Cluster.Destroy.Force = c.Force
	a.opts.Cluster.Destroy.Parallel = c.Parallel
	err := a.opts.Cluster.Destroy.doDestroy("agi", args)
	if err != nil {
		log.Printf("Could not remove instance: %s", err)
	}
	a.opts.Volume.Delete.Name = c.ClusterName.String()
	a.opts.Volume.Delete.Gcp.Zone = c.Gcp.Zone
	err = a.opts.Volume.Delete.Execute(args)
	if err != nil {
		log.Printf("Could not remove volume: %s", err)
	}
	return nil
}

type agiRelabelCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	NewLabel    string          `short:"l" long:"label" description:"new label"`
	Gcpzone     string          `short:"z" long:"zone" description:"GCP only: zone where the instance is"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiRelabelCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	err := b.SetLabel(c.ClusterName.String(), "agiLabel", c.NewLabel, c.Gcpzone)
	if err != nil {
		return err
	}
	ips, err := b.GetNodeIpMap(c.ClusterName.String(), false)
	if err != nil {
		return err
	}
	if ip, ok := ips[1]; ok && ip != "" {
		err = b.CopyFilesToClusterReader(c.ClusterName.String(), []fileListReader{{
			filePath:     "/opt/agi/label",
			fileContents: strings.NewReader(c.NewLabel),
			fileSize:     len(c.NewLabel),
		}}, []int{1})
		if err != nil {
			return err
		}
	}
	return nil
}

type agiAttachCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	Detach      bool            `long:"detach" description:"detach the process stdin - will not kill process on CTRL+C"`
	Tail        []string        `description:"List containing command parameters to execute, ex: [\"ls\",\"/opt\"]" webrequired:"true"`
	Help        attachCmdHelp   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiAttachCmd) Execute(args []string) error {
	a.opts.Attach.Shell.Node = "1"
	a.opts.Attach.Shell.ClusterName = c.ClusterName
	a.opts.Attach.Shell.Detach = c.Detach
	a.opts.Attach.Shell.Tail = c.Tail
	return a.opts.Attach.Shell.run(args)
}

type agiRetriggerCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	LocalSource      *flags.Filename `long:"source-local" description:"get logs from a local directory"`
	SftpEnable       *bool           `long:"source-sftp-enable" description:"enable sftp source" simplemode:"false"`
	SftpThreads      *int            `long:"source-sftp-threads" description:"number of concurrent downloader threads" simplemode:"false"`
	SftpHost         *string         `long:"source-sftp-host" description:"sftp host" simplemode:"false"`
	SftpPort         *int            `long:"source-sftp-port" description:"sftp port" simplemode:"false"`
	SftpUser         *string         `long:"source-sftp-user" description:"sftp user" simplemode:"false"`
	SftpPass         *string         `long:"source-sftp-pass" description:"sftp password" webtype:"password" simplemode:"false"`
	SftpKey          *flags.Filename `long:"source-sftp-key" description:"key to use for sftp login for log download, alternative to password" simplemode:"false"`
	SftpPath         *string         `long:"source-sftp-path" description:"path on sftp to download logs from" simplemode:"false"`
	SftpRegex        *string         `long:"source-sftp-regex" description:"regex to apply for choosing what to download, the regex is applied on paths AFTER the sftp-path specification, not the whole path; start wih ^" simplemode:"false"`
	S3Enable         *bool           `long:"source-s3-enable" description:"enable s3 source" simplemode:"false"`
	S3Threads        *int            `long:"source-s3-threads" description:"number of concurrent downloader threads" simplemode:"false"`
	S3Region         *string         `long:"source-s3-region" description:"aws region where the s3 bucket is located" simplemode:"false"`
	S3Bucket         *string         `long:"source-s3-bucket" description:"s3 bucket name" simplemode:"false"`
	S3KeyID          *string         `long:"source-s3-key-id" description:"(optional) access key ID" simplemode:"false"`
	S3Secret         *string         `long:"source-s3-secret-key" description:"(optional) secret key" webtype:"password" simplemode:"false"`
	S3path           *string         `long:"source-s3-path" description:"path on s3 to download logs from" simplemode:"false"`
	S3Regex          *string         `long:"source-s3-regex" description:"regex to apply for choosing what to download, the regex is applied on paths AFTER the s3-path specification, not the whole path; start wih ^" simplemode:"false"`
	TimeRanges       *bool           `long:"ingest-timeranges-enable" description:"enable importing statistics only on a specified time range found in the logs" simplemode:"false"`
	TimeRangesFrom   *string         `long:"ingest-timeranges-from" description:"time range from, format: 2006-01-02T15:04:05Z07:00" simplemode:"false"`
	TimeRangesTo     *string         `long:"ingest-timeranges-to" description:"time range to, format: 2006-01-02T15:04:05Z07:00" simplemode:"false"`
	CustomSourceName *string         `long:"ingest-custom-source-name" description:"custom source name to disaplay in grafana" simplemode:"false"`
	PatternsFile     *flags.Filename `long:"ingest-patterns-file" description:"provide a custom patterns YAML file to the log ingest system" simplemode:"false"`
	IngestLogLevel   *int            `long:"ingest-log-level" description:"1-CRITICAL,2-ERROR,3-WARN,4-INFO,5-DEBUG,6-DETAIL" simplemode:"false"`
	IngestCpuProfile *bool           `long:"ingest-cpu-profiling" description:"enable log ingest cpu profiling" simplemode:"false"`
	Force            bool            `long:"force" description:"do not ask for confirmation, just continue" webdisable:"true" webset:"true"`
	Help             helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiRetriggerCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.SftpUser != nil && strings.HasPrefix(*c.SftpUser, "ENV::") {
		aa := os.Getenv(strings.Split(*c.SftpUser, "::")[1])
		c.SftpUser = &aa
	}
	if c.SftpPass != nil && strings.HasPrefix(*c.SftpPass, "ENV::") {
		aa := os.Getenv(strings.Split(*c.SftpPass, "::")[1])
		c.SftpPass = &aa
	}
	if c.S3KeyID != nil && strings.HasPrefix(*c.S3KeyID, "ENV::") {
		aa := os.Getenv(strings.Split(*c.S3KeyID, "::")[1])
		c.S3KeyID = &aa
	}
	if c.S3Secret != nil && strings.HasPrefix(*c.S3Secret, "ENV::") {
		aa := os.Getenv(strings.Split(*c.S3Secret, "::")[1])
		c.S3Secret = &aa
	}
	if c.S3Enable != nil && *c.S3Enable && c.S3path != nil && *c.S3path == "" {
		return errors.New("S3 path cannot be left empty")
	}
	// if sftp key, local source or patterns file are specified, ensure they exist
	for _, k := range []*string{(*string)(c.SftpKey), (*string)(c.PatternsFile), (*string)(c.LocalSource)} {
		if k != nil && *k != "" {
			if _, err := os.Stat(*k); err != nil {
				return fmt.Errorf("could not access %s: %s", *k, err)
			}
		}
	}

	// process time ranges from/to to time.Time
	var tfrom, tto time.Time
	var err error
	if c.TimeRangesFrom != nil && *c.TimeRangesFrom != "" {
		tfrom, err = time.Parse("2006-01-02T15:04:05Z07:00", *c.TimeRangesFrom)
		if err != nil {
			return fmt.Errorf("from time range invalid: %s", err)
		}
	}
	if c.TimeRangesTo != nil && *c.TimeRangesTo != "" {
		tto, err = time.Parse("2006-01-02T15:04:05Z07:00", *c.TimeRangesTo)
		if err != nil {
			return fmt.Errorf("to time range invalid: %s", err)
		}
	}

	// check if ingest is already running
	out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"/bin/bash", "-c", "cat /opt/agi/ingest.pid"}}, []int{1})
	if err == nil {
		_, err = b.RunCommands(c.ClusterName.String(), [][]string{{"/bin/bash", "-c", "ls /proc |egrep '^" + strings.Trim(string(out[0]), "\r\n\t ") + "$'"}}, []int{1})
		if err == nil {
			return errors.New("ingest already running")
		}
	}

	// read current config into the config struct
	out, err = b.RunCommands(c.ClusterName.String(), [][]string{{"cat", "/opt/agi/ingest.yaml"}}, []int{1})
	if len(out) == 0 {
		out = append(out, []byte(""))
	}
	if err != nil {
		return fmt.Errorf("could not get current config: %s: %s", err, string(out[0]))
	}
	oldConfig := string(out[0])
	conf, err := ingest.MakeConfigReader(true, strings.NewReader(oldConfig), true)
	if err != nil {
		return fmt.Errorf("could not unmarshal current config: %s", err)
	}

	// update any relevant parameters
	if c.SftpEnable != nil {
		conf.Downloader.SftpSource.Enabled = *c.SftpEnable
	}
	if c.SftpThreads != nil {
		conf.Downloader.SftpSource.Threads = *c.SftpThreads
	}
	if c.SftpHost != nil {
		conf.Downloader.SftpSource.Host = *c.SftpHost
	}
	if c.SftpPort != nil {
		conf.Downloader.SftpSource.Port = *c.SftpPort
	}
	if c.SftpUser != nil {
		conf.Downloader.SftpSource.Username = *c.SftpUser
	}
	if c.SftpPass != nil {
		conf.Downloader.SftpSource.Password = *c.SftpPass
	}
	if c.SftpKey != nil {
		conf.Downloader.SftpSource.KeyFile = "/opt/agi/sftp.key"
	}
	if c.SftpPath != nil {
		conf.Downloader.SftpSource.PathPrefix = *c.SftpPath
	}
	if c.SftpRegex != nil {
		conf.Downloader.SftpSource.SearchRegex = *c.SftpRegex
	}
	if c.S3Enable != nil {
		conf.Downloader.S3Source.Enabled = *c.S3Enable
	}
	if c.S3Threads != nil {
		conf.Downloader.S3Source.Threads = *c.S3Threads
	}
	if c.S3Region != nil {
		conf.Downloader.S3Source.Region = *c.S3Region
	}
	if c.S3Bucket != nil {
		conf.Downloader.S3Source.BucketName = *c.S3Bucket
	}
	if c.S3KeyID != nil {
		conf.Downloader.S3Source.KeyID = *c.S3KeyID
	}
	if c.S3Secret != nil {
		conf.Downloader.S3Source.SecretKey = *c.S3Secret
	}
	if c.S3path != nil {
		conf.Downloader.S3Source.PathPrefix = *c.S3path
	}
	if c.S3Regex != nil {
		conf.Downloader.S3Source.SearchRegex = *c.S3Regex
	}
	if c.TimeRanges != nil {
		conf.IngestTimeRanges.Enabled = *c.TimeRanges
	}
	if c.TimeRangesFrom != nil {
		conf.IngestTimeRanges.From = tfrom
	}
	if c.TimeRangesTo != nil {
		conf.IngestTimeRanges.To = tto
	}
	if c.CustomSourceName != nil {
		conf.CustomSourceName = *c.CustomSourceName
	}
	if c.PatternsFile != nil {
		conf.PatternsFile = "/opt/agi/patterns.yaml"
	}
	if c.IngestLogLevel != nil {
		conf.LogLevel = *c.IngestLogLevel
	}
	if c.IngestCpuProfile != nil {
		if *c.IngestCpuProfile {
			conf.CPUProfilingOutputFile = "/opt/agi/cpu.ingest.pprof"
		} else {
			conf.CPUProfilingOutputFile = ""
		}
	}

	// check if s3 is enabled but pass is "<redacted>"
	if conf.Downloader.S3Source.Enabled && conf.Downloader.S3Source.SecretKey == "<redacted>" {
		return errors.New("S3 source is enabled, but SecretKey has been redacted by the previous run; update the secret key value")
	}

	// check if sftp is enabled but no pass/key present, key does not exist, or pass=="<redacted>"
	if conf.Downloader.SftpSource.Enabled && (conf.Downloader.SftpSource.KeyFile == "<redacted>" || conf.Downloader.SftpSource.Password == "<redacted>") {
		return errors.New("sftp source is enabled, but the password ot keyFile is in <redacted> state, provide one and set the other to an empty string")
	}
	if conf.Downloader.SftpSource.Enabled && conf.Downloader.SftpSource.KeyFile == "" && conf.Downloader.SftpSource.Password == "" {
		return errors.New("sftp source is enabled, but no authentication method has been provided")
	}

	// marshal new config
	var encBuf bytes.Buffer
	var encBufPretty bytes.Buffer
	enc := yaml.NewEncoder(&encBuf)
	enc.SetIndent(2)
	encPretty := yaml.NewEncoder(&encBufPretty)
	encPretty.SetIndent(2)
	s3secret := conf.Downloader.S3Source.SecretKey
	sftpSecret := conf.Downloader.SftpSource.Password
	keySecret := conf.Downloader.SftpSource.KeyFile
	if conf.Downloader.S3Source.SecretKey != "" {
		conf.Downloader.S3Source.SecretKey = "<redacted>"
	}
	if conf.Downloader.SftpSource.Password != "" {
		conf.Downloader.SftpSource.Password = "<redacted>"
	}
	if conf.Downloader.SftpSource.KeyFile != "" {
		conf.Downloader.SftpSource.KeyFile = "<redacted>"
	}
	err = encPretty.Encode(conf)
	conf.Downloader.S3Source.SecretKey = s3secret
	conf.Downloader.SftpSource.Password = sftpSecret
	conf.Downloader.SftpSource.KeyFile = keySecret
	if err != nil {
		return fmt.Errorf("could not marshal new config to yaml: %s", err)
	}
	err = enc.Encode(conf)
	if err != nil {
		return fmt.Errorf("could not marshal new config to yaml: %s", err)
	}
	newConfigPretty := encBufPretty.Bytes()
	newConfig := encBuf.Bytes()

	// diff old and new config and show diff, ask if sure to continue
	fmt.Println(string(diff.Diff("old", []byte(oldConfig), "new", newConfigPretty)))
	if !c.Force {
		for {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Are you sure you want to continue (y/n)? ")

			yesno, err := reader.ReadString('\n')
			if err != nil {
				logExit(err)
			}

			yesno = strings.ToLower(strings.TrimSpace(yesno))

			if yesno == "y" || yesno == "yes" {
				break
			} else if yesno == "n" || yesno == "no" {
				fmt.Println("Aborting")
				return nil
			}
		}
	}

	// copy new config struct to cluster together with sftpkey if specified and patterns file if specified
	flist := []fileListReader{}
	if c.PatternsFile != nil && *c.PatternsFile != "" {
		stat, err := os.Stat(string(*c.PatternsFile))
		if err != nil {
			return fmt.Errorf("could not access patterns file: %s", err)
		}
		f, err := os.Open(string(*c.PatternsFile))
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
	if c.SftpKey != nil && *c.SftpKey != "" {
		stat, err := os.Stat(string(*c.SftpKey))
		if err != nil {
			return fmt.Errorf("could not access sftp key file: %s", err)
		}
		f, err := os.Open(string(*c.SftpKey))
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
	flist = append(flist, fileListReader{
		filePath:     "/opt/agi/ingest.yaml",
		fileContents: bytes.NewReader(newConfig),
		fileSize:     len(newConfig),
	})
	err = b.CopyFilesToClusterReader(c.ClusterName.String(), flist, []int{1})
	if err != nil {
		return fmt.Errorf("could not upload configuration to instance: %s", err)
	}

	// if local source is specified, upload logs to remote
	if c.LocalSource != nil && *c.LocalSource != "" {
		a.opts.Files.Upload.ClusterName = c.ClusterName
		a.opts.Files.Upload.Nodes = "1"
		a.opts.Files.Upload.IsClient = false
		a.opts.Files.Upload.Files.Source = *c.LocalSource
		a.opts.Files.Upload.Files.Destination = "/opt/agi/files/input/"
		err = a.opts.Files.Upload.runUpload(nil)
		if err != nil {
			return fmt.Errorf("failed to upload local source to remote: %s", err)
		}
	}
	// remove /opt/agi/ingest/steps.json on remote and restart agi-ingest
	if a.opts.Config.Backend.Type != "docker" {
		out, err = b.RunCommands(c.ClusterName.String(), [][]string{{"/bin/bash", "-c", "rm -f /opt/agi/ingest/steps.json; service agi-ingest start"}}, []int{1})
		if len(out) == 0 {
			out = append(out, []byte(""))
		}
		if err != nil {
			return fmt.Errorf("could not start ingest system: %s: %s", err, string(out[0]))
		}
	} else {
		out, err = b.RunCommands(c.ClusterName.String(), [][]string{{"/bin/bash", "-c", "rm -f /opt/agi/ingest/steps.json; /opt/autoload/ingest.sh"}}, []int{1})
		if len(out) == 0 {
			out = append(out, []byte(""))
		}
		if err != nil {
			return fmt.Errorf("could not start ingest system: %s: %s", err, string(out[0]))
		}
	}
	log.Println("Done")
	return nil
}

type agiStatusCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	Json        bool            `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty  bool            `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiStatusCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"aerolab", "agi", "exec", "ingest-status"}}, []int{1})
	if len(out) == 0 {
		out = append(out, []byte(""))
	}
	if err != nil {
		return fmt.Errorf("%s : %s", err, string(out[0]))
	}
	if c.Json && !c.JsonPretty {
		fmt.Println(string(out[0]))
		return nil
	}
	clusterStatus := &ingest.IngestStatusStruct{}
	err = json.Unmarshal(out[0], clusterStatus)
	if err != nil {
		return err
	}
	if c.JsonPretty {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "    ")
		return enc.Encode(clusterStatus)
	}
	fmt.Println("SERVICE STATUS:")
	fmt.Printf("* Aerospike       : %s\n", c.boolToUp(clusterStatus.AerospikeRunning, "UP", "DOWN"))
	fmt.Printf("* Plugin          : %s\n", c.boolToUp(clusterStatus.PluginRunning, "UP", "DOWN"))
	fmt.Printf("* Grafana-Helper  : %s\n", c.boolToUp(clusterStatus.GrafanaHelperRunning, "UP", "DOWN"))
	ingestStr := ""
	if clusterStatus.Ingest.CompleteSteps.CriticalError != "" {
		ingestStr = " CRITICAL_ERROR: " + clusterStatus.Ingest.CompleteSteps.CriticalError
	}
	fmt.Printf("* Ingest          : %s%s\n", c.boolToUp(clusterStatus.Ingest.Running, "UP", "DOWN"), ingestStr)

	downloadStr := ""
	processStr := ""
	if !clusterStatus.Ingest.CompleteSteps.Download {
		downloadStr = fmt.Sprintf(" (%s/%s complete %d%%)", convSize(clusterStatus.Ingest.DownloaderCompleteSize), convSize(clusterStatus.Ingest.DownloaderTotalSize), clusterStatus.Ingest.DownloaderCompletePct)
		if clusterStatus.Ingest.DownloaderCompletePct != 100 || clusterStatus.Ingest.DownloaderCompleteSize < clusterStatus.Ingest.DownloaderTotalSize {
			elapsed := time.Since(clusterStatus.Ingest.CompleteSteps.DownloadStartTime)
			bytesPerSec := int64(0)
			if int64(elapsed.Seconds()) > 0 {
				bytesPerSec = clusterStatus.Ingest.DownloaderCompleteSize / int64(elapsed.Seconds())
			}
			remainBytes := clusterStatus.Ingest.DownloaderTotalSize - clusterStatus.Ingest.DownloaderCompleteSize
			remain := time.Duration(0)
			if bytesPerSec > 0 {
				remain = time.Duration(remainBytes/bytesPerSec) * time.Second
			}
			endTime := time.Now().Add(remain)
			downloadStr = downloadStr + fmt.Sprintf(" (speed:%s/s) (elapsed:%s remaining:%s total:%s) (endTime:%s)", convSize(bytesPerSec), elapsed.String(), remain.String(), time.Duration(elapsed+remain).String(), endTime.Format("2006-01-02_15:04:05_MST"))
		}
	} else if clusterStatus.Ingest.CompleteSteps.PreProcess {
		processStr = fmt.Sprintf(" (%s/%s complete %d%%)", convSize(clusterStatus.Ingest.LogProcessorCompleteSize), convSize(clusterStatus.Ingest.LogProcessorTotalSize), clusterStatus.Ingest.LogProcessorCompletePct)
		if clusterStatus.Ingest.LogProcessorCompletePct != 100 || clusterStatus.Ingest.LogProcessorCompleteSize < clusterStatus.Ingest.LogProcessorTotalSize {
			elapsed := time.Since(clusterStatus.Ingest.CompleteSteps.ProcessLogsStartTime)
			bytesPerSec := int64(0)
			if int64(elapsed.Seconds()) > 0 {
				bytesPerSec = clusterStatus.Ingest.LogProcessorCompleteSize / int64(elapsed.Seconds())
			}
			remainBytes := clusterStatus.Ingest.LogProcessorTotalSize - clusterStatus.Ingest.LogProcessorCompleteSize
			remain := time.Duration(0)
			if bytesPerSec > 0 {
				remain = time.Duration(remainBytes/bytesPerSec) * time.Second
			}
			endTime := time.Now().Add(remain)
			processStr = processStr + fmt.Sprintf(" (speed:%s/s) (elapsed:%s remaining:%s total:%s) (endTime:%s)", convSize(bytesPerSec), elapsed.String(), remain.String(), time.Duration(elapsed+remain).String(), endTime.Format("2006-01-02_15:04:05_MST"))
		}
	}
	fmt.Println("\nINGEST STEPS:")
	fmt.Printf("* INIT         : %s\n", c.boolToProgress(clusterStatus.Ingest.CompleteSteps.Init, "DONE", "IN-PROGRESS", "IN-PROGRESS", true))
	fmt.Printf("* DOWNLOAD     : %s%s\n", c.boolToProgress(clusterStatus.Ingest.CompleteSteps.Download, "DONE", "PENDING", "IN-PROGRESS", clusterStatus.Ingest.CompleteSteps.Init), downloadStr)
	fmt.Printf("* UNPACK       : %s\n", c.boolToProgress(clusterStatus.Ingest.CompleteSteps.Unpack, "DONE", "PENDING", "IN-PROGRESS", clusterStatus.Ingest.CompleteSteps.Download))
	fmt.Printf("* PRE-PROCESS  : %s\n", c.boolToProgress(clusterStatus.Ingest.CompleteSteps.PreProcess, "DONE", "PENDING", "IN-PROGRESS", clusterStatus.Ingest.CompleteSteps.Unpack))
	fmt.Printf("* PROCESS-LOGS : %s%s\n", c.boolToProgress(clusterStatus.Ingest.CompleteSteps.ProcessLogs, "DONE", "PENDING", "IN-PROGRESS", clusterStatus.Ingest.CompleteSteps.PreProcess), processStr)
	fmt.Printf("* COLLECTINFO  : %s\n", c.boolToProgress(clusterStatus.Ingest.CompleteSteps.ProcessCollectInfo, "DONE", "PENDING", "IN-PROGRESS", clusterStatus.Ingest.CompleteSteps.PreProcess))

	if len(clusterStatus.Ingest.Errors) > 0 {
		fmt.Println("\nINGEST ERRORS:")
		for _, e := range clusterStatus.Ingest.Errors {
			fmt.Printf("* %s\n", strings.ReplaceAll(e, "\n", " \\n "))
		}
	}
	return nil
}

func (c *agiStatusCmd) boolToUp(a bool, t string, f string) string {
	if a {
		return t
	} else {
		return f
	}
}

func (c *agiStatusCmd) boolToProgress(a bool, t string, f1 string, f2 string, b bool) string {
	if a {
		return t
	} else if b {
		return f2
	} else {
		return f1
	}
}

type agiDetailsCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	DetailType  []string        `short:"t" long:"type" description:"downloader|unpacker|pre-processor|log-processor|cf-processor|steps ; can be specified multiple times, default: ALL"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiDetailsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if len(c.DetailType) == 0 {
		c.DetailType = []string{"downloader", "unpacker", "pre-processor", "log-processor", "cf-processor", "steps"}
	}
	cmdline := []string{"aerolab", "agi", "exec", "ingest-detail"}
	for _, detail := range c.DetailType {
		cmdline = append(cmdline, "-t", detail+".json")
	}
	out, err := b.RunCommands(c.ClusterName.String(), [][]string{cmdline}, []int{1})
	if len(out) == 0 {
		out = append(out, []byte(""))
	}
	if err != nil {
		return fmt.Errorf("%s\n%s", err, string(out[0]))
	}
	fmt.Println(string(out[0]))
	return nil
}

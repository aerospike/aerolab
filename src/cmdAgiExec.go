package main

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/grafanafix"
	"github.com/aerospike/aerolab/ingest"
	"github.com/aerospike/aerolab/notifier"
	"github.com/aerospike/aerolab/plugin"
	"github.com/bestmethod/inslice"
	"gopkg.in/yaml.v3"
)

type agiExecCmd struct {
	Plugin       agiExecPluginCmd       `command:"plugin" subcommands-optional:"true" description:"Aerospike-Grafana plugin"`
	GrafanaFix   agiExecGrafanaFixCmd   `command:"grafanafix" subcommands-optional:"true" description:"Deploy dashboards, configure grafana and load/save annotations"`
	Ingest       agiExecIngestCmd       `command:"ingest" subcommands-optional:"true" description:"Ingest logs into aerospike"`
	Proxy        agiExecProxyCmd        `command:"proxy" subcommands-optional:"true" description:"Proxy from aerolab to AGI services"`
	IngestStatus agiExecIngestStatusCmd `command:"ingest-status" subcommands-optional:"true" description:"Ingest logs into aerospike"`
	IngestDetail agiExecIngestDetailCmd `command:"ingest-detail" subcommands-optional:"true" description:"Ingest logs into aerospike"`
	Simulate     agiExecSimulateCmd     `command:"simulate" subcommands-optional:"true" description:"simulate a notification to the agi monitor"`
	Help         helpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type agiExecSimulateCmd struct {
	Path       string  `long:"path" description:"path to a json file to use for notification"`
	Make       bool    `long:"make" description:"set to make the notification file using resource manager code instead of sending it"`
	AGIName    string  `long:"make-agi-name" description:"set agiName when making the notificaiton json"`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	notify     notifier.HTTPSNotify
	deployJson string
}

func (c *agiExecSimulateCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	if c.Make {
		isDim := true
		if _, err := os.Stat("/opt/agi/nodim"); err == nil {
			isDim = false
		}
		notifyData, err := getAgiStatus(true, "/opt/agi/ingest/")
		if err != nil {
			return err
		}
		notifyItem := &ingest.NotifyEvent{
			IsDataInMemory:      isDim,
			IngestStatus:        notifyData,
			Event:               AgiEventResourceMonitor,
			AGIName:             c.AGIName,
			DeploymentJsonGzB64: c.deployJson,
		}
		data, err := json.MarshalIndent(notifyItem, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(c.Path, data, 0644)
	}
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return err
	}
	v := make(map[string]interface{})
	err = json.Unmarshal(data, &v)
	if err != nil {
		return err
	}
	data, err = json.Marshal(v)
	if err != nil {
		return err
	}
	nstring, err := os.ReadFile("/opt/agi/notifier.yaml")
	if err == nil {
		yaml.Unmarshal(nstring, &c.notify)
		c.notify.Init()
		defer c.notify.Close()
	}
	if c.notify.AGIMonitorUrl == "" && c.notify.Endpoint == "" {
		return errors.New("JSON notification is disabled")
	}
	deploymentjson, _ := os.ReadFile("/opt/agi/deployment.json.gz")
	c.deployJson = base64.StdEncoding.EncodeToString(deploymentjson)
	return c.notify.NotifyData(data)
}

type agiExecIngestStatusCmd struct {
	IngestPath string  `long:"ingest-stat-path" default:"/opt/agi/ingest/"`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecIngestStatusCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	resp, err := getAgiStatus(true, c.IngestPath)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(resp)
	return nil
}

type agiExecIngestDetailCmd struct {
	IngestPath string   `long:"ingest-stat-path" default:"/opt/agi/ingest/"`
	DetailType []string `short:"t" long:"detail-type" description:"file name of the progress detail"`
	Help       helpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecIngestDetailCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	files := []string{"downloader.json", "unpacker.json", "pre-processor.json", "log-processor.json", "cf-processor.json", "steps.json"}
	if len(c.DetailType) > 1 {
		fmt.Fprint(os.Stdout, "[\n")
	}
	for fi, fname := range c.DetailType {
		if !inslice.HasString(files, fname) {
			return errors.New("invalid detail type")
		}
		npath := path.Join(c.IngestPath, fname)
		if fname == "steps.json" {
			npath = "/opt/agi/ingest/steps.json"
		}
		gz := false
		if _, err := os.Stat(npath); err != nil {
			npath = npath + ".gz"
			if _, err := os.Stat(npath); err != nil {
				if len(c.DetailType) == 1 {
					return errors.New("file not found")
				} else {
					continue
				}
			}
			gz = true
		}
		f, err := os.Open(npath)
		if err != nil {
			return fmt.Errorf("could not open file: %s", err)
		}
		defer f.Close()
		var reader io.Reader
		reader = f
		if gz {
			fx, err := gzip.NewReader(f)
			if err != nil {
				return fmt.Errorf("could not open gz for reading: %s", err)
			}
			defer fx.Close()
			reader = fx
		}
		io.Copy(os.Stdout, reader)
		if len(c.DetailType) > 1 {
			if fi+1 == len(c.DetailType) {
				fmt.Fprint(os.Stdout, "\n]\n")
			} else {
				fmt.Fprint(os.Stdout, ",\n")
			}
		}
	}
	return nil
}

type agiExecPluginCmd struct {
	YamlFile string  `short:"y" long:"yaml" description:"Yaml config file"`
	Help     helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecPluginCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	os.Mkdir("/opt/agi", 0755)
	os.WriteFile("/opt/agi/plugin.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/plugin.pid")
	conf, err := plugin.MakeConfig(true, c.YamlFile, true)
	if err != nil {
		return err
	}
	p, err := plugin.Init(conf)
	if err != nil {
		return err
	}
	err = p.Listen()
	p.Close()
	return err
}

type agiExecGrafanaFixCmd struct {
	YamlFile string  `short:"y" long:"yaml" description:"Yaml config file"`
	Help     helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecGrafanaFixCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	os.Mkdir("/opt/agi", 0755)
	os.WriteFile("/opt/agi/grafanafix.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/grafanafix.pid")
	conf := new(grafanafix.GrafanaFix)
	if c.YamlFile != "" {
		f, err := os.Open(c.YamlFile)
		if err != nil {
			return err
		}
		conf, err = grafanafix.MakeConfig(true, f, true)
		f.Close()
		if err != nil {
			return err
		}
	}
	exec.Command("service", "grafana-server", "stop").CombinedOutput()
	err := grafanafix.EarlySetup("/etc/grafana/grafana.ini", "/etc/grafana/provisioning", "/var/lib/grafana/plugins", "", 0)
	if err != nil {
		return err
	}
	out, err := exec.Command("service", "grafana-server", "start").CombinedOutput()
	if err != nil {
		errstr := fmt.Sprintf("%s\n%s", string(out), err)
		var pid []byte
		retries := 0
		for {
			pid, _ = os.ReadFile("/var/run/grafana-server.pid")
			if len(pid) > 0 {
				break
			}
			if retries > 9 {
				return errors.New(errstr)
			}
			retries++
			time.Sleep(time.Second)
		}
		pidi, err := strconv.Atoi(string(pid))
		if err != nil {
			return fmt.Errorf("(%s): %s", err, errstr)
		}
		_, err = os.FindProcess(pidi)
		if err != nil {
			return fmt.Errorf("(%s): %s", err, errstr)
		}
	}
	grafanafix.Run(conf)
	return nil
}

type agiExecIngestCmd struct {
	AGIName    string `long:"agi-name"`
	YamlFile   string `short:"y" long:"yaml" description:"Yaml config file"`
	notify     notifier.HTTPSNotify
	notifyJSON bool
	deployJson string
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecIngestCmd) Execute(args []string) error {
	aerr := c.run(args)
	if aerr != nil {
		steps := new(ingest.IngestSteps)
		f, err := os.ReadFile("/opt/agi/ingest/steps.json")
		if err == nil {
			json.Unmarshal(f, steps)
		}
		steps.CriticalError = aerr.Error()
		f, err = json.Marshal(steps)
		if err == nil {
			err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
			if err == nil {
				os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
			}
		}
	}
	return aerr
}

const (
	AgiEventInitComplete       = "INGEST_STEP_INIT_COMPLETE"
	AgiEventDownloadComplete   = "INGEST_STEP_DOWNLOAD_COMPLETE"
	AgiEventUnpackComplete     = "INGEST_STEP_UNPACK_COMPLETE"
	AgiEventPreProcessComplete = "INGEST_STEP_PREPROCESS_COMPLETE"
	AgiEventProcessComplete    = "INGEST_STEP_PROCESS_COMPLETE"
	AgiEventIngestFinish       = "INGEST_FINISHED"
	AgiEventServiceDown        = "SERVICE_DOWN"
	AgiEventServiceUp          = "SERVICE_UP"
	AgiEventMaxAge             = "MAX_AGE_REACHED"
	AgiEventMaxInactive        = "MAX_INACTIVITY_REACHED"
	AgiEventSpotNoCapacity     = "SPOT_INSTANCE_CAPACITY_SHUTDOWN"
	AgiEventResourceMonitor    = "SYS_RESOURCE_USAGE_MONITOR" // run from AgiEventInitComplete until AgiEventIngestFinish; on timer, send to notifier http/monitor only, no slack
)

func (c *agiExecIngestCmd) resourceMonitor() {
	isDim := true
	if _, err := os.Stat("/opt/agi/nodim"); err == nil {
		isDim = false
	}
	for {
		time.Sleep(30 * time.Second)
		notifyData, err := getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err != nil {
			continue
		}
		notifyItem := &ingest.NotifyEvent{
			IsDataInMemory:      isDim,
			IngestStatus:        notifyData,
			Event:               AgiEventResourceMonitor,
			AGIName:             c.AGIName,
			DeploymentJsonGzB64: c.deployJson,
		}
		c.notify.NotifyJSON(notifyItem)
	}
}

func (c *agiExecIngestCmd) run(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	deploymentjson, _ := os.ReadFile("/opt/agi/deployment.json.gz")
	c.deployJson = base64.StdEncoding.EncodeToString(deploymentjson)
	os.Mkdir("/opt/agi", 0755)
	os.WriteFile("/opt/agi/ingest.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/ingest.pid")
	config, err := ingest.MakeConfig(true, c.YamlFile, true)
	if err != nil {
		return fmt.Errorf("MakeConfig: %s", err)
	}
	steps := new(ingest.IngestSteps)
	f, err := os.ReadFile("/opt/agi/ingest/steps.json")
	if err == nil {
		json.Unmarshal(f, steps)
	}
	steps.Init = false
	steps.CriticalError = ""
	f, err = json.Marshal(steps)
	if err == nil {
		err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
		if err == nil {
			os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
		}
	}
	// notifier load start
	isDim := true
	if _, err := os.Stat("/opt/agi/nodim"); err == nil {
		isDim = false
	}
	owner := ""
	ownerbyte, err := os.ReadFile("/opt/agi/owner")
	if err == nil {
		owner = strings.Trim(string(ownerbyte), "\r\n\t ")
	}
	nstring, err := os.ReadFile("/opt/agi/notifier.yaml")
	if err == nil {
		yaml.Unmarshal(nstring, &c.notify)
		c.notify.Init()
		defer c.notify.Close()
	}
	if c.notify.AGIMonitorUrl == "" && c.notify.Endpoint == "" {
		c.notifyJSON = false
	} else {
		c.notifyJSON = true
	}
	// notifier load end
	// slack notifier vars
	slacks3source := ""
	if config.Downloader.S3Source.Enabled {
		slacks3source = fmt.Sprintf("\n> *S3 Source*: %s:%s %s", config.Downloader.S3Source.BucketName, config.Downloader.S3Source.PathPrefix, config.Downloader.S3Source.SearchRegex)
	}
	slacksftpsource := ""
	if config.Downloader.SftpSource.Enabled {
		slacksftpsource = fmt.Sprintf("\n> *SFTP Source*: %s:%s %s", config.Downloader.SftpSource.Host, config.Downloader.SftpSource.PathPrefix, config.Downloader.SftpSource.SearchRegex)
	}
	slackcustomsource := ""
	if config.CustomSourceName != "" {
		slackcustomsource = fmt.Sprintf("\n> *Custom Source*: %s", config.CustomSourceName)
	}
	slackAccessDetails := fmt.Sprintf("Attach:\n  `aerolab agi attach -n %s`\nGet Web URL:\n  `aerolab agi list`\nGet Detailed Status:\n  `aerolab agi status -n %s`\nGet auth token:\n  `aerolab agi add-auth-token -n %s`\nChange Label:\n  `aerolab agi change-label -n %s -l \"new label\"`\nDestroy:\n  `aerolab agi destroy -f -n %s`\nDestroy and remove volume (AWS EFS only):\n  `aerolab agi delete -f -n %s`", c.AGIName, c.AGIName, c.AGIName, c.AGIName, c.AGIName, c.AGIName)
	// end slack notifier vars
	i, err := ingest.Init(config)
	if err != nil {
		return fmt.Errorf("Init: %s", err)
	}
	steps.Init = true
	if !steps.Download {
		steps.DownloadStartTime = time.Now().UTC()
	}
	f, err = json.Marshal(steps)
	if err == nil {
		err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
		if err == nil {
			os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
		}
	}
	notifyData, err := getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
	if err == nil {
		notifyItem := &ingest.NotifyEvent{
			IsDataInMemory:      isDim,
			IngestStatus:        notifyData,
			Event:               AgiEventInitComplete,
			AGIName:             c.AGIName,
			DeploymentJsonGzB64: c.deployJson,
		}
		err = c.notify.NotifyJSON(notifyItem)
		if err != nil {
			return fmt.Errorf("notify: %s", err)
		}
		slackagiLabel, _ := os.ReadFile("/opt/agi/label")
		c.notify.NotifySlack(AgiEventInitComplete, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", AgiEventInitComplete, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
	}
	if c.notifyJSON {
		go c.resourceMonitor()
	}
	if !steps.Download {
		err = i.Download()
		if err != nil {
			return fmt.Errorf("Download: %s", err)
		}
		steps.Download = true
		steps.DownloadEndTime = time.Now().UTC()
		f, err := json.Marshal(steps)
		if err == nil {
			err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
			if err == nil {
				os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
			}
		}
		notifyData, err := getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err == nil {
			notifyItem := &ingest.NotifyEvent{
				IsDataInMemory:      isDim,
				IngestStatus:        notifyData,
				Event:               AgiEventDownloadComplete,
				AGIName:             c.AGIName,
				DeploymentJsonGzB64: c.deployJson,
			}
			err = c.notify.NotifyJSON(notifyItem)
			if err != nil {
				return fmt.Errorf("notify: %s", err)
			}
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			c.notify.NotifySlack(AgiEventDownloadComplete, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", AgiEventDownloadComplete, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
		}
		if c.YamlFile != "" {
			// rewrite, redacting passwords for sources
			s3Pw := config.Downloader.S3Source.SecretKey
			sftpPw := config.Downloader.SftpSource.Password
			keyFile := config.Downloader.SftpSource.KeyFile
			if config.Downloader.S3Source.SecretKey != "" {
				config.Downloader.S3Source.SecretKey = "<redacted>"
			}
			if config.Downloader.SftpSource.Password != "" {
				config.Downloader.SftpSource.Password = "<redacted>"
			}
			if config.Downloader.SftpSource.KeyFile != "" {
				config.Downloader.SftpSource.KeyFile = "<redacted>"
			}
			f, err := os.OpenFile(c.YamlFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			enc := yaml.NewEncoder(f)
			enc.SetIndent(2)
			err = enc.Encode(config)
			f.Close()
			if err != nil {
				return err
			}
			config.Downloader.S3Source.SecretKey = s3Pw
			config.Downloader.SftpSource.Password = sftpPw
			config.Downloader.SftpSource.KeyFile = keyFile
		}
		os.Remove("/opt/agi/sftp.key")
	}
	if !steps.Unpack {
		err = i.Unpack()
		if err != nil {
			return fmt.Errorf("unpack: %s", err)
		}
		steps.Unpack = true
		f, err := json.Marshal(steps)
		if err == nil {
			err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
			if err == nil {
				os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
			}
		}
		notifyData, err := getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err == nil {
			notifyItem := &ingest.NotifyEvent{
				IsDataInMemory:      isDim,
				IngestStatus:        notifyData,
				Event:               AgiEventUnpackComplete,
				AGIName:             c.AGIName,
				DeploymentJsonGzB64: c.deployJson,
			}
			err = c.notify.NotifyJSON(notifyItem)
			if err != nil {
				return fmt.Errorf("notify: %s", err)
			}
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			c.notify.NotifySlack(AgiEventUnpackComplete, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", AgiEventUnpackComplete, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
		}
	}
	var foundLogs map[string]*ingest.LogFile
	var meta ingest.MetaEntries
	if !steps.PreProcess {
		err = i.PreProcess()
		if err != nil {
			return fmt.Errorf("PreProcess: %s", err)
		}
		steps.PreProcess = true
		if !steps.ProcessLogs {
			steps.ProcessLogsStartTime = time.Now().UTC()
		}
		f, err := json.Marshal(steps)
		if err == nil {
			err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
			if err == nil {
				os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
			}
		}
		foundLogs, meta, err = i.ProcessLogsPrep()
		if err != nil {
			return fmt.Errorf("ProcessLogsPrep: %s", err)
		}
		notifyData, err := getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err == nil {
			notifyItem := &ingest.NotifyEvent{
				IsDataInMemory:      isDim,
				IngestStatus:        notifyData,
				Event:               AgiEventPreProcessComplete,
				AGIName:             c.AGIName,
				DeploymentJsonGzB64: c.deployJson,
			}
			err = c.notify.NotifyJSON(notifyItem)
			if err != nil {
				return fmt.Errorf("notify: %s", err)
			}
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			c.notify.NotifySlack(AgiEventPreProcessComplete, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", AgiEventPreProcessComplete, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
		}
	}
	nerr := []error{}
	nerrLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	if !steps.ProcessLogs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := i.ProcessLogs(foundLogs, meta)
			steps.ProcessLogsEndTime = time.Now().UTC()
			if err != nil {
				nerrLock.Lock()
				nerr = append(nerr, fmt.Errorf("ProcessLogs: %s", err))
				nerrLock.Unlock()
			}
		}()
	}
	if !steps.ProcessCollectInfo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := i.ProcessCollectInfo()
			if err != nil {
				nerrLock.Lock()
				nerr = append(nerr, fmt.Errorf("ProcessCollectInfo: %s", err))
				nerrLock.Unlock()
			}
		}()
	}
	wg.Wait()
	i.Close()
	if !steps.ProcessLogs || !steps.ProcessCollectInfo {
		steps.ProcessCollectInfo = true
		steps.ProcessLogs = true
		f, err = json.Marshal(steps)
		if err == nil {
			err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
			if err == nil {
				os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
			}
		}
		notifyData, err := getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err == nil {
			notifyItem := &ingest.NotifyEvent{
				IsDataInMemory:      isDim,
				IngestStatus:        notifyData,
				Event:               AgiEventProcessComplete,
				AGIName:             c.AGIName,
				DeploymentJsonGzB64: c.deployJson,
			}
			err = c.notify.NotifyJSON(notifyItem)
			if err != nil {
				return fmt.Errorf("notify: %s", err)
			}
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			c.notify.NotifySlack(AgiEventProcessComplete, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", AgiEventProcessComplete, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
		}
	}
	if len(nerr) > 0 {
		errstr := ""
		for _, e := range nerr {
			if errstr != "" {
				errstr += "; "
			}
			errstr = errstr + e.Error()
		}
		return errors.New(errstr)
	}
	notifyData, err = getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
	if err == nil {
		notifyItem := &ingest.NotifyEvent{
			IsDataInMemory:      isDim,
			IngestStatus:        notifyData,
			Event:               AgiEventIngestFinish,
			AGIName:             c.AGIName,
			DeploymentJsonGzB64: c.deployJson,
		}
		err = c.notify.NotifyJSON(notifyItem)
		if err != nil {
			return fmt.Errorf("notify: %s", err)
		}
		slackagiLabel, _ := os.ReadFile("/opt/agi/label")
		c.notify.NotifySlack(AgiEventIngestFinish, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", AgiEventIngestFinish, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
	}
	return nil
}

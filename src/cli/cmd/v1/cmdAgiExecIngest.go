package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/agi"
	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"github.com/aerospike/aerolab/pkg/agi/notifier"
	"gopkg.in/yaml.v3"
)

// AgiExecIngestCmd runs the log ingest service.
// This is a hidden command that runs inside AGI instances, not called by users directly.
// It performs the complete log ingestion pipeline: download, unpack, preprocess, and process logs.
type AgiExecIngestCmd struct {
	AGIName    string               `long:"agi-name" description:"Name of this AGI instance"`
	Async      bool                 `long:"async" description:"If set, will asynchronously process logs and collectinfo"`
	YamlFile   string               `short:"y" long:"yaml" description:"Path to YAML config file" default:"/opt/agi/ingest.yaml"`
	notify     notifier.HTTPSNotify `no-default:"true"`
	notifyJSON bool
	deployJson string
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute runs the complete log ingestion pipeline.
// This function performs the following steps:
//  1. Load configuration from YAML file and environment variables
//  2. Initialize the ingestion system with database connections
//  3. Download logs from configured sources (S3, SFTP, local)
//  4. Unpack and decompress log files
//  5. Preprocess logs to identify clusters and nodes
//  6. Process logs and collectinfo files concurrently
//  7. Store processed data in Aerospike database
//
// At each step, it sends notifications to configured endpoints (Slack, webhook, AGI monitor).
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiExecIngestCmd) Execute(args []string) error {
	aerr := c.run(args)
	if aerr != nil {
		// Record critical error in steps.json
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

// resourceMonitor runs in the background and periodically sends resource usage notifications.
// This is used by the AGI monitor to track system status and make scaling decisions.
func (c *AgiExecIngestCmd) resourceMonitor(owner, slacks3source, slacksftpsource, slackcustomsource string) {
	isDim := true
	if _, err := os.Stat("/opt/agi/nodim"); err == nil {
		isDim = false
	}
	for {
		time.Sleep(30 * time.Second)
		notifyData, err := GetAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err != nil {
			continue
		}
		slackagiLabel, _ := os.ReadFile("/opt/agi/label")
		notifyItem := &ingest.NotifyEvent{
			Label:                      string(slackagiLabel),
			Owner:                      owner,
			S3Source:                   slacks3source,
			SftpSource:                 slacksftpsource,
			LocalSource:                slackcustomsource,
			IsDataInMemory:             isDim,
			IngestStatus:               notifyData,
			Event:                      agi.AgiEventResourceMonitor,
			AGIName:                    c.AGIName,
			DeploymentJsonGzB64:        c.deployJson,
			SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
		}
		c.notify.NotifyJSON(notifyItem)
	}
}

// run is the main implementation of the ingest pipeline
func (c *AgiExecIngestCmd) run(args []string) error {
	cmd := []string{"agi", "exec", "ingest"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	// Load AGIName from /opt/agi/name if not provided via command line
	// This file is created by agiCreate and contains the cluster name
	if c.AGIName == "" {
		nameBytes, err := os.ReadFile("/opt/agi/name")
		if err == nil {
			c.AGIName = strings.TrimSpace(string(nameBytes))
		}
	}

	// Validate AGIName is set - this is critical for monitor operations
	// If empty, the monitor could destroy ALL AGI instances instead of just this one
	if c.AGIName == "" {
		return errors.New("AGI name is required: either provide --agi-name or ensure /opt/agi/name exists")
	}

	// Load deployment JSON for monitor recovery
	deploymentjson, _ := os.ReadFile("/opt/agi/deployment.json.gz")
	c.deployJson = base64.StdEncoding.EncodeToString(deploymentjson)

	// Ensure directories exist
	os.MkdirAll("/opt/agi", 0755)
	os.MkdirAll("/opt/agi/ingest", 0755)

	// Write PID file for process management
	os.WriteFile("/opt/agi/ingest.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/ingest.pid")

	// Load ingest configuration
	yamlFile := c.YamlFile
	if _, err := os.Stat(yamlFile); os.IsNotExist(err) {
		yamlFile = "" // Use defaults if file doesn't exist
	}
	config, err := ingest.MakeConfig(true, yamlFile, true)
	if err != nil {
		return fmt.Errorf("MakeConfig: %s", err)
	}

	// Load/init steps tracking
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

	// Load notifier configuration
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

	// Build slack notification vars
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

	// Initialize ingest system
	system.Logger.Info("Initializing ingest system")
	if !steps.Init {
		steps.InitStartTime = time.Now().UTC()
	}
	i, err := ingest.Init(config)
	if err != nil {
		return fmt.Errorf("Init: %s", err)
	}
	steps.Init = true
	steps.InitEndTime = time.Now().UTC()
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

	// Send init complete notification
	notifyData, err := GetAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
	if err == nil {
		slackagiLabel, _ := os.ReadFile("/opt/agi/label")
		notifyItem := &ingest.NotifyEvent{
			Label:                      string(slackagiLabel),
			Owner:                      owner,
			S3Source:                   slacks3source,
			SftpSource:                 slacksftpsource,
			LocalSource:                slackcustomsource,
			IsDataInMemory:             isDim,
			IngestStatus:               notifyData,
			Event:                      agi.AgiEventInitComplete,
			AGIName:                    c.AGIName,
			DeploymentJsonGzB64:        c.deployJson,
			SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
		}
		err = c.notify.NotifyJSON(notifyItem)
		if err != nil {
			return fmt.Errorf("notify: %s", err)
		}
		c.notify.NotifySlack(agi.AgiEventInitComplete, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", agi.AgiEventInitComplete, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
	}

	// Start resource monitor if JSON notifications are enabled
	if c.notifyJSON {
		go c.resourceMonitor(owner, slacks3source, slacksftpsource, slackcustomsource)
	}

	// Step: Download
	if !steps.Download {
		if config.Downloader.S3Source.Enabled && config.Downloader.S3Source.PathPrefix == "" {
			return fmt.Errorf("Download: S3 enabled, but path is empty; refusing to AGI a whole bucket")
		}
		if config.Downloader.SftpSource.Enabled && config.Downloader.SftpSource.PathPrefix == "" {
			return fmt.Errorf("Download: Sftp enabled, but path is empty; refusing to AGI a whole sftp server")
		}
		system.Logger.Info("Downloading files from sources")
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
		notifyData, err := GetAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err == nil {
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			notifyItem := &ingest.NotifyEvent{
				Label:                      string(slackagiLabel),
				Owner:                      owner,
				S3Source:                   slacks3source,
				SftpSource:                 slacksftpsource,
				LocalSource:                slackcustomsource,
				IsDataInMemory:             isDim,
				IngestStatus:               notifyData,
				Event:                      agi.AgiEventDownloadComplete,
				AGIName:                    c.AGIName,
				DeploymentJsonGzB64:        c.deployJson,
				SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
			}
			err = c.notify.NotifyJSON(notifyItem)
			if err != nil {
				return fmt.Errorf("notify: %s", err)
			}
			c.notify.NotifySlack(agi.AgiEventDownloadComplete, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", agi.AgiEventDownloadComplete, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
		}
		// Rewrite config with redacted passwords after download
		if c.YamlFile != "" {
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

	// Step: Unpack
	if !steps.Unpack {
		system.Logger.Info("Unpacking files")
		steps.UnpackStartTime = time.Now().UTC()
		err = i.Unpack()
		if err != nil {
			return fmt.Errorf("unpack: %s", err)
		}
		steps.Unpack = true
		steps.UnpackEndTime = time.Now().UTC()
		f, err := json.Marshal(steps)
		if err == nil {
			err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
			if err == nil {
				os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
			}
		}
		notifyData, err := GetAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err == nil {
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			notifyItem := &ingest.NotifyEvent{
				Label:                      string(slackagiLabel),
				Owner:                      owner,
				S3Source:                   slacks3source,
				SftpSource:                 slacksftpsource,
				LocalSource:                slackcustomsource,
				IsDataInMemory:             isDim,
				IngestStatus:               notifyData,
				Event:                      agi.AgiEventUnpackComplete,
				AGIName:                    c.AGIName,
				DeploymentJsonGzB64:        c.deployJson,
				SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
			}
			err = c.notify.NotifyJSON(notifyItem)
			if err != nil {
				return fmt.Errorf("notify: %s", err)
			}
			c.notify.NotifySlack(agi.AgiEventUnpackComplete, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", agi.AgiEventUnpackComplete, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
		}
	}

	// Step: PreProcess
	var foundLogs map[string]*ingest.LogFile
	var meta ingest.MetaEntries
	if !steps.PreProcess {
		system.Logger.Info("Preprocessing files")
		steps.PreProcessStartTime = time.Now().UTC()
		err = i.PreProcess()
		if err != nil {
			return fmt.Errorf("PreProcess: %s", err)
		}
		steps.PreProcess = true
		steps.PreProcessEndTime = time.Now().UTC()
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
		notifyData, err := GetAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err == nil {
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			notifyItem := &ingest.NotifyEvent{
				Label:                      string(slackagiLabel),
				Owner:                      owner,
				S3Source:                   slacks3source,
				SftpSource:                 slacksftpsource,
				LocalSource:                slackcustomsource,
				IsDataInMemory:             isDim,
				IngestStatus:               notifyData,
				Event:                      agi.AgiEventPreProcessComplete,
				AGIName:                    c.AGIName,
				DeploymentJsonGzB64:        c.deployJson,
				SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
			}
			err = c.notify.NotifyJSON(notifyItem)
			if err != nil {
				return fmt.Errorf("notify: %s", err)
			}
			c.notify.NotifySlack(agi.AgiEventPreProcessComplete, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", agi.AgiEventPreProcessComplete, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
		}
	}

	// Step: Process logs and collectinfo
	nerr := []error{}
	nerrLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	var processLogsEndTime, processCollectInfoStartTime, processCollectInfoEndTime time.Time
	if !steps.ProcessLogs {
		steps.ProcessLogsStartTime = time.Now().UTC()
		wg.Add(1)
		go func() {
			defer wg.Done()
			system.Logger.Info("Processing logs")
			err := i.ProcessLogs(foundLogs, meta)
			processLogsEndTime = time.Now().UTC()
			if err != nil {
				nerrLock.Lock()
				nerr = append(nerr, fmt.Errorf("ProcessLogs: %s", err))
				nerrLock.Unlock()
			}
		}()
	}
	if !c.Async {
		wg.Wait()
	}
	if !steps.ProcessCollectInfo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			processCollectInfoStartTime = time.Now().UTC()
			system.Logger.Info("Processing collectinfo files")
			err := i.ProcessCollectInfo()
			processCollectInfoEndTime = time.Now().UTC()
			if err != nil {
				nerrLock.Lock()
				nerr = append(nerr, fmt.Errorf("ProcessCollectInfo: %s", err))
				nerrLock.Unlock()
			}
		}()
	}
	wg.Wait()
	// Copy timing data from goroutines
	if !processLogsEndTime.IsZero() {
		steps.ProcessLogsEndTime = processLogsEndTime
	}
	if !processCollectInfoStartTime.IsZero() {
		steps.ProcessCollectInfoStartTime = processCollectInfoStartTime
	}
	if !processCollectInfoEndTime.IsZero() {
		steps.ProcessCollectInfoEndTime = processCollectInfoEndTime
	}
	i.Close()

	// Update steps after processing
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
		notifyData, err := GetAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err == nil {
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			notifyItem := &ingest.NotifyEvent{
				Label:                      string(slackagiLabel),
				Owner:                      owner,
				S3Source:                   slacks3source,
				SftpSource:                 slacksftpsource,
				LocalSource:                slackcustomsource,
				IsDataInMemory:             isDim,
				IngestStatus:               notifyData,
				Event:                      agi.AgiEventProcessComplete,
				AGIName:                    c.AGIName,
				DeploymentJsonGzB64:        c.deployJson,
				SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
			}
			err = c.notify.NotifyJSON(notifyItem)
			if err != nil {
				return fmt.Errorf("notify: %s", err)
			}
			c.notify.NotifySlack(agi.AgiEventProcessComplete, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", agi.AgiEventProcessComplete, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
		}
	}

	// Check for processing errors
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

	// Send final notification
	notifyData, err = GetAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
	if err == nil {
		slackagiLabel, _ := os.ReadFile("/opt/agi/label")
		notifyItem := &ingest.NotifyEvent{
			Label:                      string(slackagiLabel),
			Owner:                      owner,
			S3Source:                   slacks3source,
			SftpSource:                 slacksftpsource,
			LocalSource:                slackcustomsource,
			IsDataInMemory:             isDim,
			IngestStatus:               notifyData,
			Event:                      agi.AgiEventIngestFinish,
			AGIName:                    c.AGIName,
			DeploymentJsonGzB64:        c.deployJson,
			SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
		}
		err = c.notify.NotifyJSON(notifyItem)
		if err != nil {
			return fmt.Errorf("notify: %s", err)
		}
		c.notify.NotifySlack(agi.AgiEventIngestFinish, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s", agi.AgiEventIngestFinish, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), owner, slacks3source, slacksftpsource, slackcustomsource), slackAccessDetails)
	}

	system.Logger.Info("Ingest completed successfully")
	return nil
}

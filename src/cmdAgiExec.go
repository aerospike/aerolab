package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/aerospike/aerolab/grafanafix"
	"github.com/aerospike/aerolab/ingest"
	"github.com/aerospike/aerolab/plugin"
	"gopkg.in/yaml.v2"
)

/* ./aerolab-linux-amd64 agi exec grafanafix -y /opt/agi/grafanafix.yaml
dashboards:
  fromDir: ""
  loadEmbedded: true
grafanaURL: "http://127.0.0.1:8850"
annotationFile: "/opt/agi/annotations.json"
*/

/* ./aerolab-linux-amd64 agi exec plugin -y /opt/agi/plugin.yaml
-- docker --
maxDataPointsReceived: 17280000
logLevel: 6
cpuProfilingOutputFile: "/opt/agi/cpu.plugin.pprof"
-- other --
maxDataPointsReceived: 34560000
logLevel: 6
cpuProfilingOutputFile: "/opt/agi/cpu.plugin.pprof"
*/

/* ./aerolab-linux-amd64 agi exec ingest -y /opt/agi/ingest.yaml
logLevel: 6
cpuProfilingOutputFile: "/opt/agi/cpu.ingest.pprof"
preProcessor:
  fileThreads: 6
  unpackerFileThreads: 4
processor:
  maxConcurrentLogFiles: 4
progressFile:
  disableWrite: false
  writeInterval: 10s
  compress: true
  outputFilePath: "/opt/agi/ingest"
progressPrint:
  enable: true
  updateInterval: 10s
  printOverallProgress: true
  printDetailProgress: true
patternsFile: ""
ingestTimeRanges:
  enabled: false
  from: ""
  to: ""
directories:
  collectInfo: "/opt/agi/files/collectinfo"
  logs: "/opt/agi/files/logs"
  dirtyTemp: "/opt/agi/files/input"
  noStatOut: "/opt/agi/files/no-stat"
  otherFiles: "/opt/agi/files/other"
customSourceName: ""
downloader:
  sftpSource:
    enabled: true
	threads: 4
	host: "asftp.aerospike.com"
	port: 22
	username: ""
	password: ""
	keyFile: ""
	pathPrefix: "/path/to/dir/"
	searchRegex: "^regexAfterPathPrefix"
  s3Source:
    enabled: true
	threads: 4
	region: "eu-west-1"
	bucketName: "logs-bucket"
	keyID: ""
	secretKey: ""
	pathPrefix: "/path/to/dir/"
	searchRegex: "^regexAfterPathPrefix"
*/

type agiExecCmd struct {
	Plugin     agiExecPluginCmd     `command:"plugin" subcommands-optional:"true" description:"Aerospike-Grafana plugin"`
	GrafanaFix agiExecGrafanaFixCmd `command:"grafanafix" subcommands-optional:"true" description:"Deploy dashboards, configure grafana and load/save annotations"`
	Ingest     agiExecIngestCmd     `command:"ingest" subcommands-optional:"true" description:"Ingest logs into aerospike"`
	Proxy      agiExecProxyCmd      `command:"proxy" subcommands-optional:"true" description:"Proxy from aerolab to AGI services"`
	Help       helpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
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
	return p.Listen()
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
	YamlFile string  `short:"y" long:"yaml" description:"Yaml config file"`
	Help     helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ingestSteps struct {
	Init               bool
	Download           bool
	Unpack             bool
	PreProcess         bool
	ProcessLogs        bool
	ProcessCollectInfo bool
	CriticalError      string
}

func (c *agiExecIngestCmd) Execute(args []string) error {
	aerr := c.run(args)
	if aerr != nil {
		steps := new(ingestSteps)
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

func (c *agiExecIngestCmd) run(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	os.Mkdir("/opt/agi", 0755)
	os.WriteFile("/opt/agi/ingest.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/ingest.pid")
	config, err := ingest.MakeConfig(true, c.YamlFile, true)
	if err != nil {
		return fmt.Errorf("MakeConfig: %s", err)
	}
	steps := new(ingestSteps)
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
	i, err := ingest.Init(config)
	if err != nil {
		return fmt.Errorf("Init: %s", err)
	}
	steps.Init = true
	f, err = json.Marshal(steps)
	if err == nil {
		err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
		if err == nil {
			os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
		}
	}
	if !steps.Download {
		err = i.Download()
		if err != nil {
			return fmt.Errorf("Download: %s", err)
		}
		steps.Download = true
		f, err := json.Marshal(steps)
		if err == nil {
			err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
			if err == nil {
				os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
			}
		}
		if c.YamlFile != "" {
			// rewrite, redacting passwords for sources
			s3Pw := config.Downloader.S3Source.SecretKey
			sftpPw := config.Downloader.SftpSource.Password
			if config.Downloader.S3Source.SecretKey != "" {
				config.Downloader.S3Source.SecretKey = "<redacted>"
			}
			if config.Downloader.SftpSource.Password != "" {
				config.Downloader.SftpSource.Password = "<redacted>"
			}
			f, err := os.OpenFile(c.YamlFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			err = yaml.NewEncoder(f).Encode(config)
			f.Close()
			if err != nil {
				return err
			}
			config.Downloader.S3Source.SecretKey = s3Pw
			config.Downloader.SftpSource.Password = sftpPw
		}
	}
	if !steps.Unpack {
		err = i.Unpack()
		if err != nil {
			return fmt.Errorf("Unpack: %s", err)
		}
		steps.Unpack = true
		f, err := json.Marshal(steps)
		if err == nil {
			err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
			if err == nil {
				os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
			}
		}
	}
	if !steps.PreProcess {
		err = i.PreProcess()
		if err != nil {
			return fmt.Errorf("PreProcess: %s", err)
		}
		steps.PreProcess = true
		f, err := json.Marshal(steps)
		if err == nil {
			err = os.WriteFile("/opt/agi/ingest/steps.json.new", f, 0644)
			if err == nil {
				os.Rename("/opt/agi/ingest/steps.json.new", "/opt/agi/ingest/steps.json")
			}
		}
	}
	nerr := []error{}
	nerrLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	if !steps.ProcessLogs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := i.ProcessLogs()
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
	return nil
}

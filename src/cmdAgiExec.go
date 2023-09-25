package main

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/aerospike/aerolab/grafanafix"
	"github.com/aerospike/aerolab/ingest"
	"github.com/aerospike/aerolab/plugin"
	"gopkg.in/yaml.v3"
)

// TODO: https listener
// TODO: proxy to filebrowser
// TODO: proxy to ttyd

/*
apt update && apt -y install wget adduser libfontconfig1 musl
wget https://dl.grafana.com/oss/release/grafana_10.1.2_amd64.deb
dpkg -i grafana_10.1.2_amd64.deb
## copy aerolab to instance
## aerolab config backend -t none
*/

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

func (c *agiExecIngestCmd) Execute(args []string) error {
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
	return ingest.RunWithConfig(config)
}

type agiExecProxyCmd struct {
	ListenPort      int           `short:"l" long:"listen-port" default:"80" description:"port to listen on"`
	MaxInactivity   time.Duration `short:"m" long:"max-inactivity" default:"2h" description:"Max user inactivity period after which the system will be shut down; 0=disable"`
	MaxUptime       time.Duration `short:"M" long:"max-uptime" default:"24h" description:"Max hard instance uptime; 0=disable"`
	ShutdownCommand string        `short:"c" long:"shutdown-command" default:"/sbin/poweroff" description:"Command to execute on max uptime or max inactivity being breached"`
	AuthType        string        `short:"a" long:"auth-type" default:"none" description:"Authentication type; supported: none|basic"`
	BasicAuthUser   string        `short:"u" long:"basic-auth-user" default:"admin" description:"Basic authentication username"`
	BasicAuthPass   string        `short:"p" long:"basic-auth-pass" default:"secure" description:"Basic authentication password"`
	Help            helpCmd       `command:"help" subcommands-optional:"true" description:"Print help"`
	isBasicAuth     bool
	lastActivity    *activity
	grafanaUrl      *url.URL
	grafanaProxy    *httputil.ReverseProxy
}

type activity struct {
	sync.Mutex
	lastActivity time.Time
}

func (a *activity) Set(t time.Time) {
	a.Lock()
	a.lastActivity = t
	a.Unlock()
}

func (a *activity) Get() (t time.Time) {
	a.Lock()
	t = a.lastActivity
	a.Unlock()
	return
}

func (c *agiExecProxyCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	c.lastActivity = new(activity)
	c.lastActivity.Set(time.Now())
	gurl, _ := url.Parse("http://127.0.0.1:8850/")
	gproxy := httputil.NewSingleHostReverseProxy(gurl)
	c.grafanaUrl = gurl
	c.grafanaProxy = gproxy
	if c.AuthType == "basic" {
		c.isBasicAuth = true
	}
	if c.MaxInactivity > 0 {
		go c.activityMonitor()
	}
	if c.MaxUptime > 0 {
		go c.maxUptime()
	}
	http.HandleFunc("/", c.proxyHandler)
	http.ListenAndServe("0.0.0.0:"+strconv.Itoa(c.ListenPort), nil)
	return nil
}

func (c *agiExecProxyCmd) maxUptime() {
	time.Sleep(c.MaxUptime)
	exec.Command(c.ShutdownCommand).CombinedOutput()
}

func (c *agiExecProxyCmd) activityMonitor() {
	for {
		time.Sleep(time.Minute)
		if _, err := os.Stat("/opt/agi/ingest.pid"); err == nil {
			c.lastActivity.Set(time.Now())
			continue
		}
		if time.Since(c.lastActivity.Get()) > c.MaxInactivity {
			exec.Command(c.ShutdownCommand).CombinedOutput()
		}
	}
}

func (c *agiExecProxyCmd) proxyHandler(w http.ResponseWriter, r *http.Request) {
	// be strict
	w.Header().Add("Strict-Transport-Security", "max-age=31536000")

	// auth check
	if c.isBasicAuth {
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		usermatch := subtle.ConstantTimeCompare([]byte(user), []byte(c.BasicAuthUser))
		passmatch := subtle.ConstantTimeCompare([]byte(pass), []byte(c.BasicAuthPass))
		if usermatch == 0 || passmatch == 0 {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// note down activity timestamp
	go c.lastActivity.Set(time.Now())

	// reverse proxy
	r.URL.Host = c.grafanaUrl.Host
	r.URL.Scheme = c.grafanaUrl.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = c.grafanaUrl.Host
	c.grafanaProxy.ServeHTTP(w, r)
}

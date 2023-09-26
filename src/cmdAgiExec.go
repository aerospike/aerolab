package main

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/grafanafix"
	"github.com/aerospike/aerolab/ingest"
	"github.com/aerospike/aerolab/plugin"
	"github.com/bestmethod/logger"
	"gopkg.in/yaml.v2"
)

/* TODO
agiExec:
// TODO: https listener
// TODO: cookie-based authentication
// TODO: oom-checker
// TODO: dynamic instance sizing(?)
// TODO: proxy should have a http endpoint for getting overall and detailed progress of ingest and getting instance logs(?)
// TODO: easily store and explose processing warnings and errors?

test: manually launch against a full set of logs and confirm everything works, get pprof and compare speed (agi exec * on aws)

write cmdAgi command-set to make all this work; agi is part of 'cluster' command set, but also has EFS volumes

aerolab agi from desktop create command will be responsible for installing aerospike (cluster create), deploying self on the instance, creating systemd and yaml files, and running all self-* services; that's all that should be required :) ... oh, and EFS mounts
... need to handle re-launching from where we finished if instance is shut down/rebooted
... need to handle spot instances and sizing
*/

// to restart ingest, rm -f /opt/agi/ingest/steps.json

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
	Download           bool
	Unpack             bool
	PreProcess         bool
	ProcessLogs        bool
	ProcessCollectInfo bool
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
	steps := new(ingestSteps)
	f, err := os.ReadFile("/opt/agi/ingest/steps.json")
	if err == nil {
		json.Unmarshal(f, steps)
	}
	i, err := ingest.Init(config)
	if err != nil {
		return fmt.Errorf("Init: %s", err)
	}
	if !steps.Download {
		err = i.Download()
		if err != nil {
			return fmt.Errorf("Download: %s", err)
		}
		steps.Download = true
		f, err := json.Marshal(steps)
		if err == nil {
			os.WriteFile("/opt/agi/ingest/steps.json", f, 0644)
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
			os.WriteFile("/opt/agi/ingest/steps.json", f, 0644)
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
			os.WriteFile("/opt/agi/ingest/steps.json", f, 0644)
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
			os.WriteFile("/opt/agi/ingest/steps.json", f, 0644)
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

type agiExecProxyCmd struct {
	ListenPort      int           `short:"l" long:"listen-port" default:"80" description:"port to listen on"`
	EntryDir        string        `short:"d" long:"entry-dir" default:"/opt/agi/files" description:"Entrypoint for ttyd and filebrowser"`
	MaxInactivity   time.Duration `short:"m" long:"max-inactivity" default:"1h" description:"Max user inactivity period after which the system will be shut down; 0=disable"`
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
	ttydUrl         *url.URL
	ttydProxy       *httputil.ReverseProxy
	fbUrl           *url.URL
	fbProxy         *httputil.ReverseProxy
	gottyConns      *counter
}

type counter struct {
	sync.Mutex
	c string
}

func (a *counter) Set(t string) {
	a.Lock()
	a.c = t
	a.Unlock()
}

func (a *counter) Get() (t string) {
	a.Lock()
	t = a.c
	a.Unlock()
	return
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
	os.MkdirAll(c.EntryDir, 0755)
	os.WriteFile("/opt/agi/proxy.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/proxy.pid")
	c.lastActivity = new(activity)
	c.gottyConns = new(counter)
	c.gottyConns.Set("0")
	c.lastActivity.Set(time.Now())
	gurl, _ := url.Parse("http://127.0.0.1:8850/")
	gproxy := httputil.NewSingleHostReverseProxy(gurl)
	c.grafanaUrl = gurl
	c.grafanaProxy = gproxy
	turl, _ := url.Parse("http://127.0.0.1:8852/")
	tproxy := httputil.NewSingleHostReverseProxy(turl)
	c.ttydUrl = turl
	c.ttydProxy = tproxy
	furl, _ := url.Parse("http://127.0.0.1:8853/")
	fproxy := httputil.NewSingleHostReverseProxy(furl)
	c.fbUrl = furl
	c.fbProxy = fproxy
	if c.AuthType == "basic" {
		c.isBasicAuth = true
	}
	go c.getDeps()
	if c.MaxInactivity > 0 {
		go c.activityMonitor()
	}
	if c.MaxUptime > 0 {
		go c.maxUptime()
	}
	http.HandleFunc("/", c.grafanaHandler)        // grafana
	http.HandleFunc("/ttyd", c.ttydHandler)       // web console tty
	http.HandleFunc("/ttyd/", c.ttydHandler)      // web console tty
	http.HandleFunc("/filebrowser", c.fbHandler)  // file browser
	http.HandleFunc("/filebrowser/", c.fbHandler) // file browser
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
		if c.gottyConns.Get() != "0" {
			c.lastActivity.Set(time.Now())
			continue
		}
		if time.Since(c.lastActivity.Get()) > c.MaxInactivity {
			exec.Command(c.ShutdownCommand).CombinedOutput()
		}
	}
}

func (c *agiExecProxyCmd) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Add("Strict-Transport-Security", "max-age=31536000")
	if c.isBasicAuth {
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return false
		}
		usermatch := subtle.ConstantTimeCompare([]byte(user), []byte(c.BasicAuthUser))
		passmatch := subtle.ConstantTimeCompare([]byte(pass), []byte(c.BasicAuthPass))
		if usermatch == 0 || passmatch == 0 {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return false
		}
	}
	// note down activity timestamp
	go c.lastActivity.Set(time.Now())
	return true
}

func (c *agiExecProxyCmd) grafanaHandler(w http.ResponseWriter, r *http.Request) {
	// auth check
	if !c.checkAuth(w, r) {
		return
	}
	// reverse proxy
	r.URL.Host = c.grafanaUrl.Host
	r.URL.Scheme = c.grafanaUrl.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = c.grafanaUrl.Host
	c.grafanaProxy.ServeHTTP(w, r)
}

func (c *agiExecProxyCmd) ttydHandler(w http.ResponseWriter, r *http.Request) {
	// auth check
	if !c.checkAuth(w, r) {
		return
	}
	// reverse proxy
	r.URL.Host = c.ttydUrl.Host
	r.URL.Scheme = c.ttydUrl.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = c.ttydUrl.Host
	c.ttydProxy.ServeHTTP(w, r)
}

func (c *agiExecProxyCmd) fbHandler(w http.ResponseWriter, r *http.Request) {
	// auth check
	if !c.checkAuth(w, r) {
		return
	}
	// reverse proxy
	r.URL.Host = c.fbUrl.Host
	r.URL.Scheme = c.fbUrl.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = c.fbUrl.Host
	c.fbProxy.ServeHTTP(w, r)
}

func (c *agiExecProxyCmd) getDeps() {
	go func() {
		logger.Info("Getting ttyd...")
		fd, err := os.OpenFile("/usr/local/bin/ttyd", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			logger.Error("ttyd-MAKEFILE: %s", err)
			return
		}
		arch := "x86_64" // .aarch64
		narch, _ := exec.Command("uname", "-m").CombinedOutput()
		if strings.Contains(string(narch), "arm") || strings.Contains(string(narch), "aarch") {
			arch = "aarch64"
		}
		resp, err := http.Get("https://github.com/tsl0922/ttyd/releases/download/1.7.3/ttyd." + arch)
		if err != nil {
			logger.Error("ttyd-GET: %s", err)
			fd.Close()
			return
		}
		_, err = io.Copy(fd, resp.Body)
		resp.Body.Close()
		fd.Close()
		if err != nil {
			logger.Error("ttyd-DOWNLOAD: %s", err)
			return
		}
		logger.Info("Running gotty!")
		com := exec.Command("/usr/local/bin/ttyd", "-p", "8852", "-i", "lo", "-P", "5", "-b", "/ttyd", "/bin/bash", "-c", "export TMOUT=3600 && echo '* aerospike-tools is installed' && echo '* less -S ...: enable horizontal scrolling in less using arrow keys' && echo '* showconf command: showconf collect_info.tgz' && echo '* showsysinfo command: showsysinfo collect_info.tgz' && echo '* showinterrupts command: showinterrupts collect_info.tgz' && /bin/bash")
		com.Dir = c.EntryDir
		sout, err := com.StdoutPipe()
		if err != nil {
			logger.Error("gotty cannot start: could not create stdout pipe: %s", err)
			return
		}
		serr, err2 := com.StderrPipe()
		if err2 != nil {
			logger.Error("gotty cannot start: could not create stderr pipe: %s", err2)
			return
		}
		err = com.Start()
		if err != nil {
			logger.Error("gotty cannot start: %s", err)
			return
		}
		go c.gottyWatcher(sout)
		go c.gottyWatcher(serr)
		err = com.Wait()
		if err != nil {
			logger.Error("gotty exited with error: %s", err)
			return
		}
	}()
	go func() {
		cur, err := filepath.Abs(os.Args[0])
		if err != nil {
			logger.Error("failed to get absolute path os self: %s", err)
			return
		}
		if _, err := os.Stat("/usr/local/bin/showconf"); err != nil {
			err = os.Symlink(cur, "/usr/local/bin/showconf")
			if err != nil {
				logger.Error("failed to symlink showconf: %s", err)
			}
		}
		if _, err := os.Stat("/usr/local/bin/showsysinfo"); err != nil {
			err = os.Symlink(cur, "/usr/local/bin/showsysinfo")
			if err != nil {
				logger.Error("failed to symlink showsysinfo: %s", err)
			}
		}
		if _, err := os.Stat("/usr/local/bin/showinterrupts"); err != nil {
			err = os.Symlink(cur, "/usr/local/bin/showinterrupts")
			if err != nil {
				logger.Error("failed to symlink showinterrupts: %s", err)
			}
		}
	}()
	go func() {
		logger.Info("Getting filebrowser...")
		fd, err := os.OpenFile("/opt/filebrowser.tgz", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			logger.Error("filebrowser-MAKEFILE: %s", err)
			return
		}
		arch := "amd64"
		narch, _ := exec.Command("uname", "-m").CombinedOutput()
		if strings.Contains(string(narch), "arm") || strings.Contains(string(narch), "aarch") {
			arch = "arm64"
		}
		resp, err := http.Get("https://github.com/filebrowser/filebrowser/releases/download/v2.25.0/linux-" + arch + "-filebrowser.tar.gz")
		if err != nil {
			logger.Error("filebrowser-GET: %s", err)
			fd.Close()
			return
		}
		_, err = io.Copy(fd, resp.Body)
		resp.Body.Close()
		fd.Close()
		if err != nil {
			logger.Error("filebrowser-DOWNLOAD: %s", err)
			return
		}
		logger.Info("Unpack filebrowser")
		out, err := exec.Command("tar", "-zxvf", "/opt/filebrowser.tgz", "-C", "/usr/local/bin/", "filebrowser").CombinedOutput()
		if err != nil {
			logger.Error("filebrowser-unpack: %s (%s)", string(out), err)
			return
		}
		logger.Info("Running filebrowser!")
		com := exec.Command("/usr/local/bin/filebrowser", "-p", "8853", "-r", c.EntryDir, "--noauth", "-d", "/opt/filebrowser.db", "-b", "/filebrowser/")
		com.Dir = c.EntryDir
		out, err = com.CombinedOutput()
		if err != nil {
			logger.Error("filebrowser: %s %s", err, string(out))
		}
	}()
}

func (c *agiExecProxyCmd) gottyWatcher(out io.Reader) {
	//r, _ := regexp.Compile(`connections: [0-9]+($|\n)`)
	r, _ := regexp.Compile(`clients: [0-9]+($|\n)`)
	r2, _ := regexp.Compile(`[0-9]+`)
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		line := scanner.Text()
		n := r.FindAllString(line, -1)
		if len(n) == 0 {
			continue
		}
		n1 := n[len(n)-1]
		connNew := r2.FindString(n1)
		if connNew == "" {
			continue
		}
		if connNew != c.gottyConns.Get() {
			logger.Info("GOTTY CONNS: %s", connNew)
			c.gottyConns.Set(connNew)
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Error("gottyWatcher scanner error: %s", err)
	}
	logger.Info("Exiting gottyWatcher")
}

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	_ "embed"

	"github.com/aerospike/aerolab/ingest"
	"github.com/aerospike/aerolab/notifier"
	"github.com/aerospike/aerolab/webui"
	"github.com/bestmethod/inslice"
	"github.com/bestmethod/logger"
	"github.com/fsnotify/fsnotify"
	ps "github.com/mitchellh/go-ps"
	"gopkg.in/yaml.v3"
)

//go:embed agiproxy.tgz
var proxyweb []byte

type agiExecProxyCmd struct {
	AGIName              string        `long:"agi-name"`
	InitialLabel         string        `short:"L" long:"label" description:"freeform label that will appear in the dashboards if set"`
	IngestProgressPath   string        `short:"i" long:"ingest-progress-path" default:"/opt/agi/ingest/" description:"path to where ingest stores it's json progress"`
	ListenPort           int           `short:"l" long:"listen-port" default:"80" description:"port to listen on"`
	HTTPS                bool          `short:"S" long:"https" description:"set to enable https listener"`
	CertFile             string        `short:"C" long:"cert-file" description:"required path to server cert file for tls"`
	KeyFile              string        `short:"K" long:"key-file" description:"required path to server key file for tls"`
	EntryDir             string        `short:"d" long:"entry-dir" default:"/opt/agi/files" description:"Entrypoint for ttyd and filebrowser"`
	MaxInactivity        time.Duration `short:"m" long:"max-inactivity" default:"1h" description:"Max user inactivity period after which the system will be shut down; 0=disable"`
	MaxUptime            time.Duration `short:"M" long:"max-uptime" default:"24h" description:"Max hard instance uptime; 0=disable"`
	ShutdownCommand      string        `short:"c" long:"shutdown-command" default:"/sbin/poweroff" description:"Command to execute on max uptime or max inactivity being breached"`
	AuthType             string        `short:"a" long:"auth-type" default:"none" description:"Authentication type; supported: none|basic|token"`
	BasicAuthUser        string        `short:"u" long:"basic-auth-user" default:"admin" description:"Basic authentication username"`
	BasicAuthPass        string        `short:"p" long:"basic-auth-pass" default:"secure" description:"Basic authentication password"`
	TokenAuthLocation    string        `short:"t" long:"token-path" default:"/opt/agi/tokens" description:"Directory where tokens are stored for access"`
	TokenName            string        `short:"T" long:"token-name" default:"AGI_TOKEN" description:"Name of the token variable and cookie to use"`
	DebugActivityMonitor bool          `short:"D" long:"debug-mode" description:"set to log activity monitor for debugging"`
	Help                 helpCmd       `command:"help" subcommands-optional:"true" description:"Print help"`
	isBasicAuth          bool
	isTokenAuth          bool
	lastActivity         *activity
	grafanaUrl           *url.URL
	grafanaProxy         *httputil.ReverseProxy
	ttydUrl              *url.URL
	ttydProxy            *httputil.ReverseProxy
	fbUrl                *url.URL
	fbProxy              *httputil.ReverseProxy
	gottyConns           *counter
	srv                  *http.Server
	tokens               *tokens
	notify               notifier.HTTPSNotify
	shuttingDown         bool
	shuttingDownMutex    *sync.Mutex
	slacks3source        string
	slacksftpsource      string
	slackcustomsource    string
	owner                string
	slackAccessDetails   string
	isDim                bool
	notifyJSON           bool
	deployJson           string
	wwwSimple            bool
}

type tokens struct {
	sync.RWMutex
	tokens []string
}

func (c *agiExecProxyCmd) loadTokensDo() {
	tokens := []string{}
	err := filepath.Walk(c.TokenAuthLocation, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			logger.Error("error on walk %s: %s", fpath, err)
			return nil
		}
		if info.IsDir() {
			return nil
		}
		token, err := os.ReadFile(fpath)
		if err != nil {
			logger.Error("could not read token file %s: %s", fpath, err)
			return nil
		}
		if len(token) < 64 {
			logger.Error("Token file %s contents too short, minimum token length is 64 characters", fpath)
			return nil
		}
		tokens = append(tokens, string(token))
		return nil
	})
	if err != nil {
		logger.Error("failed to read tokens: %s", err)
		return
	}
	c.tokens.Lock()
	c.tokens.tokens = tokens
	c.tokens.Unlock()
}

func (c *agiExecProxyCmd) loadTokensInterval() {
	for {
		c.loadTokensDo()
		time.Sleep(time.Minute)
	}
}

func (c *agiExecProxyCmd) loadTokens() {
	if c.AuthType != "token" {
		return
	}
	os.MkdirAll(c.TokenAuthLocation, 0755)
	go c.loadTokensInterval()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("fsnotify could not be started, tokens will not be dynamically monitored; switching to once-a-minute system: %s", err)
		return
	}
	defer watcher.Close()
	err = watcher.Add(c.TokenAuthLocation)
	if err != nil {
		logger.Error("fsnotify could not add token path, tokens will not be dynamically monitored; switching to once-a-minute system: %s", err)
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				logger.Error("fsnotify events error, tokens will not be dynamically monitored; switching to once-a-minute system")
				return
			}
			logger.Detail("fsnotify event:", event)
			c.loadTokensDo()
		case err, ok := <-watcher.Errors:
			logger.Error("fsnotify watcher error, tokens will not be dynamically monitored; switching to once-a-minute system (ok:%t err:%s)", ok, err)
			return
		}
	}
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
	if _, err := os.Stat("/tmp/aerolab.install.ssh"); err == nil {
		contents, err := os.ReadFile("/tmp/aerolab.install.ssh")
		if err == nil {
			putSSHAuthorizedKeys(string(contents))
		}
	}
	deploymentjson, _ := os.ReadFile("/opt/agi/deployment.json.gz")
	c.deployJson = base64.StdEncoding.EncodeToString(deploymentjson)
	os.MkdirAll(c.EntryDir, 0755)
	os.WriteFile("/opt/agi/proxy.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/proxy.pid")
	if _, err := os.Stat("/opt/agi/label"); err != nil {
		os.WriteFile("/opt/agi/label", []byte(c.InitialLabel), 0644)
	}
	ownerbyte, err := os.ReadFile("/opt/agi/owner")
	if err == nil {
		c.owner = string(ownerbyte)
	}
	c.slackAccessDetails = fmt.Sprintf("Attach:\n  `aerolab agi attach -n %s`\nGet Web URL:\n  `aerolab agi list`\nGet Detailed Status:\n  `aerolab agi status -n %s`\nGet auth token:\n  `aerolab agi add-auth-token -n %s`\nChange Label:\n  `aerolab agi change-label -n %s -l \"new label\"`\nDestroy:\n  `aerolab agi destroy -f -n %s`\nDestroy and remove volume (AWS EFS only):\n  `aerolab agi delete -f -n %s`", c.AGIName, c.AGIName, c.AGIName, c.AGIName, c.AGIName, c.AGIName)
	plist, err := ps.Processes()
	asdRunning := false
	if err == nil {
		for _, p := range plist {
			if strings.HasSuffix(p.Executable(), "asd") {
				asdRunning = true
				break
			}
		}
	}
	if !asdRunning {
		exec.Command("service", "aerospike", "start").CombinedOutput()
	}
	c.shuttingDownMutex = new(sync.Mutex)
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
	c.tokens = new(tokens)
	if c.AuthType == "basic" {
		c.isBasicAuth = true
	}
	if c.AuthType == "token" {
		c.isTokenAuth = true
	}
	go c.getDeps()
	// notifier load start
	nstring, err := os.ReadFile("/opt/agi/notifier.yaml")
	if err == nil {
		c.isDim = true
		if _, err := os.Stat("/opt/agi/nodim"); err == nil {
			c.isDim = false
		}

		yaml.Unmarshal(nstring, &c.notify)
		c.notify.Init()
		defer c.notify.Close()
		if c.notify.AGIMonitorUrl == "" && c.notify.Endpoint == "" {
			c.notifyJSON = false
		} else {
			c.notifyJSON = true
		}
		// for slack notifier
		if c.notify.SlackToken != "" {
			ingestConfig, err := ingest.MakeConfig(true, "/opt/agi/ingest.yaml", true)
			if err != nil {
				log.Printf("could not load ingest config for slack notifier: %s", err)
			} else {
				if ingestConfig.Downloader.S3Source.Enabled {
					c.slacks3source = fmt.Sprintf("\n> *S3 Source*: %s:%s %s", ingestConfig.Downloader.S3Source.BucketName, ingestConfig.Downloader.S3Source.PathPrefix, ingestConfig.Downloader.S3Source.SearchRegex)
				}
				if ingestConfig.Downloader.SftpSource.Enabled {
					c.slacksftpsource = fmt.Sprintf("\n> *SFTP Source*: %s:%s %s", ingestConfig.Downloader.SftpSource.Host, ingestConfig.Downloader.SftpSource.PathPrefix, ingestConfig.Downloader.SftpSource.SearchRegex)
				}
				if ingestConfig.CustomSourceName != "" {
					c.slackcustomsource = fmt.Sprintf("\n> *Custom Source*: %s", ingestConfig.CustomSourceName)
				}
			}
		}
		// end for slack
		go c.serviceMonitor()
		go c.spotMonitor()
	}
	// notifier load end
	if c.MaxInactivity > 0 {
		go c.activityMonitor()
	}
	if c.MaxUptime > 0 {
		go c.maxUptime()
	}
	go c.loadTokens()

	os.RemoveAll("/opt/agi/www")
	err = os.MkdirAll("/opt/agi/www", 0755)
	if err != nil {
		c.wwwSimple = true
		log.Printf("WARN: simple homepage, error: %s", err)
	} else {
		err = webui.InstallWebsite("/opt/agi/www", proxyweb)
		if err != nil {
			c.wwwSimple = true
			log.Printf("WARN: simple homepage, error: %s", err)
		}
	}

	http.HandleFunc("/agi/menu", c.handleList)               // URL list and status
	http.HandleFunc("/agi/dist/", c.wwwstatic)               // static files for URL list
	http.HandleFunc("/agi/api/status", c.handleStatus)       // menu web page API
	http.HandleFunc("/agi/api/logs", c.handleLogs)           // menu web page API
	http.HandleFunc("/agi/api/detail", c.handleIngestDetail) // menu web page API
	http.HandleFunc("/agi/monitor-challenge", c.secretValidate)
	http.HandleFunc("/agi/ttyd", c.ttydHandler)                 // web console tty
	http.HandleFunc("/agi/ttyd/", c.ttydHandler)                // web console tty
	http.HandleFunc("/agi/filebrowser", c.fbHandler)            // file browser
	http.HandleFunc("/agi/filebrowser/", c.fbHandler)           // file browser
	http.HandleFunc("/agi/shutdown", c.handleShutdown)          // gracefully shutdown the proxy
	http.HandleFunc("/agi/poweroff", c.handlePoweroff)          // poweroff the instance
	http.HandleFunc("/agi/status", c.handleStatus)              // high-level agi service status
	http.HandleFunc("/agi/inactivity", c.handleInactivity)      // print inactivity timers
	http.HandleFunc("/agi/ingest/detail", c.handleIngestDetail) // detailed logingest progress json; form: ?detail=[]string{"downloader.json", "unpacker.json", "pre-processor.json", "log-processor.json", "cf-processor.json"}
	http.HandleFunc("/", c.grafanaHandler)                      // grafana
	c.srv = &http.Server{Addr: "0.0.0.0:" + strconv.Itoa(c.ListenPort)}
	if c.HTTPS {
		tlsConfig := &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
		}
		c.srv.TLSConfig = tlsConfig
		if err := c.srv.ListenAndServeTLS(c.CertFile, c.KeyFile); err != http.ErrServerClosed {
			return err
		} else {
			return nil
		}
	}
	if err := c.srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	} else {
		return nil
	}
}

func (c *agiExecProxyCmd) secretValidate(w http.ResponseWriter, r *http.Request) {
	secret, err := os.ReadFile("/opt/agi/uuid")
	if err != nil {
		http.Error(w, "NO-SECRET", http.StatusInternalServerError)
		return
	}
	challenge := r.Header.Get("Agi-Monitor-Secret")
	if challenge != strings.Trim(string(secret), "\r\n\t ") {
		http.Error(w, "wrong", http.StatusTeapot)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (c *agiExecProxyCmd) handleListSimple(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	out := []byte(`<html><head><title>AGI URLs</title><meta http-equiv="Cache-Control" content="no-cache, no-store, must-revalidate" /><meta http-equiv="Pragma" content="no-cache" /><meta http-equiv="Expires" content="0" /></head><body><center>
<a href="/d/dashList/dashboard-list?from=now-7d&to=now&var-MaxIntervalSeconds=30&var-ProduceDelta&var-ClusterName=All&var-NodeIdent=All&var-Namespace=All&var-Histogram=NONE&var-HistogramDev=NONE&var-HistogramUs=NONE&var-HistogramCount=NONE&var-HistogramSize=NONE&var-XdrDcName=All&var-xdr5dc=All&var-warnC=All&var-warnCtx=All&var-errC=All&var-errCtx=All&orgId=1" target="_blank"><h1>Grafana</h1></a>
<a href="/agi/ttyd" target="_blank"><h1>Web Console (ttyd)</h1></a>
<a href="/agi/filebrowser" target="_blank"><h1>File Browser</h1></a>
</center></body></html>`)
	w.Write(out)
}

func (c *agiExecProxyCmd) handleLogs(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	type logs struct {
		AerospikeLogs  string
		ProxyLogs      string
		IngestLogs     string
		PluginLogs     string
		GrafanaFixLogs string
		Dmesg          string
	}
	l := new(logs)
	l.AerospikeLogs = c.getLog("/var/log/agi-aerospike.log", "")
	l.ProxyLogs = c.getLog("/var/log/agi-proxy.log", "agi-proxy")
	l.IngestLogs = c.getLog("/var/log/agi-ingest.log", "agi-ingest")
	l.GrafanaFixLogs = c.getLog("/var/log/agi-grafanafix.log", "agi-grafanafix")
	l.PluginLogs = c.getLog("/var/log/agi-plugin.log", "agi-plugin")
	dmesg, err := exec.Command("dmesg").CombinedOutput()
	if err != nil {
		dmesg = append(dmesg, []byte(err.Error())...)
	}
	l.Dmesg = string(dmesg)
	json.NewEncoder(w).Encode(l)
}

func (c *agiExecProxyCmd) getLog(path string, journalName string) string {
	s, err := os.Stat(path)
	if err == nil {
		f, err := os.Open(path)
		if err == nil {
			defer f.Close()
			if s.Size() > 20*1024 {
				f.Seek(-20*1024, 2)
				d, _ := io.ReadAll(f)
				idx := bytes.Index(d, []byte{'\n'})
				if idx == -1 {
					return string(d)
				} else {
					return string(d[idx+1:])
				}
			} else {
				d, _ := io.ReadAll(f)
				return string(d)
			}
		} else {
			return err.Error()
		}
	}
	if journalName == "" {
		return ""
	}
	l, err := exec.Command("journalctl", "-u", journalName, "-n", "200", "--no-pager").CombinedOutput()
	if err != nil {
		return string(append(l, []byte(err.Error())...))
	}
	return string(l)
}

func (c *agiExecProxyCmd) handleList(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Add("Pragma", "no-cache")
	w.Header().Add("Expires", "0")
	if c.wwwSimple {
		c.handleListSimple(w)
		return
	}
	www := os.DirFS("/opt/agi/www")
	t, err := template.ParseFS(www, "index.html")
	if err != nil {
		log.Print(err)
		c.handleListSimple(w)
		return
	}
	type np struct {
		Title       string
		Description string
	}
	nlabel, _ := os.ReadFile("/opt/agi/label")
	p := np{
		Title:       c.AGIName,
		Description: string(nlabel),
	}
	err = t.ExecuteTemplate(w, "index", p)
	if err != nil {
		log.Print(err)
		c.handleListSimple(w)
		return
	}
}

func (c *agiExecProxyCmd) wwwstatic(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, path.Join("/opt/agi/www", strings.TrimPrefix(strings.TrimLeft(r.URL.Path, "/"), "agi/")))
}

// form: ?detail=[]string{"downloader.json", "unpacker.json", "pre-processor.json", "log-processor.json", "cf-processor.json", "steps.json"}
func (c *agiExecProxyCmd) handleIngestDetail(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	fname := r.FormValue("detail")
	files := []string{"downloader.json", "unpacker.json", "pre-processor.json", "log-processor.json", "cf-processor.json", "steps.json"}
	if !inslice.HasString(files, fname) {
		http.Error(w, "invalid detail type", http.StatusBadRequest)
		return
	}
	npath := path.Join(c.IngestProgressPath, fname)
	if fname == "steps.json" {
		npath = "/opt/agi/ingest/steps.json"
	}
	gz := false
	if _, err := os.Stat(npath); err != nil {
		npath = npath + ".gz"
		if _, err := os.Stat(npath); err != nil {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		gz = true
	}
	f, err := os.Open(npath)
	if err != nil {
		http.Error(w, "could not open file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	var reader io.Reader
	reader = f
	if gz {
		fx, err := gzip.NewReader(f)
		if err != nil {
			http.Error(w, "could not open gz for reading: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer fx.Close()
		reader = fx
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

func (c *agiExecProxyCmd) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	r.ParseForm()
	logger.Info("Listener: status request from %s", r.RemoteAddr)
	resp, err := getAgiStatus(true, c.IngestProgressPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	if r.Form.Get("shorten") != "" {
		shorten, err := strconv.Atoi(r.Form.Get("shorten"))
		if err == nil {
			if len(resp.Ingest.Errors) > shorten {
				resp.Ingest.Errors = append(resp.Ingest.Errors[0:shorten], "...truncated entries: "+strconv.Itoa(len(resp.Ingest.Errors)-shorten))
			}
		} else {
			log.Print(err)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.Encode(resp)
}

func (c *agiExecProxyCmd) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	logger.Info("Listener: shutdown request from %s", r.RemoteAddr)
	c.shuttingDownMutex.Lock()
	c.shuttingDown = true
	c.shuttingDownMutex.Unlock()
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Shutting down..."))
	go func() {
		timeout := 60 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := c.srv.Shutdown(ctx); err != nil {
			logger.Debug("Graceful Server Shutdown Failed, Forcing shutdown: %s", err)
			c.srv.Close()
		}
	}()
}

func (c *agiExecProxyCmd) handlePoweroff(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	logger.Info("Listener: shutdown request from %s", r.RemoteAddr)
	c.shuttingDownMutex.Lock()
	c.shuttingDown = true
	c.shuttingDownMutex.Unlock()
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Poweroff..."))
	go func() {
		shcomm := strings.Split(c.ShutdownCommand, " ")
		shparams := []string{}
		if len(shcomm) > 1 {
			shparams = shcomm[1:]
		}
		out, err := exec.Command(shcomm[0], shparams...).CombinedOutput()
		if err != nil {
			log.Printf("ERROR: INACTIVITY MONITOR: could not poweroff the instance: %s : %s", err, string(out))
		} else {
			log.Printf("ACTIVITY MONITOR: poweroff command issued: %s, result: %s", c.ShutdownCommand, string(out))
		}
	}()
}

func (c *agiExecProxyCmd) maxUptime() {
	logger.Info("MAX UPTIME: hard shutdown time: %s", time.Now().Add(c.MaxUptime).String())
	time.Sleep(c.MaxUptime - time.Minute)
	c.shuttingDownMutex.Lock()
	c.shuttingDown = true
	c.shuttingDownMutex.Unlock()
	go func() {
		notifyData, err := getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err == nil {
			notifyItem := &ingest.NotifyEvent{
				IsDataInMemory:             c.isDim,
				IngestStatus:               notifyData,
				Event:                      AgiEventMaxAge,
				AGIName:                    c.AGIName,
				DeploymentJsonGzB64:        c.deployJson,
				SSHAuthorizedKeysFileGzB64: getSSHAuthorizedKeysGzB64(),
			}
			c.notify.NotifyJSON(notifyItem)
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			c.notify.NotifySlack(AgiEventMaxAge, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *Max age reached, shutting down*", AgiEventMaxAge, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
		}
	}()
	time.Sleep(time.Minute)
	shcomm := strings.Split(c.ShutdownCommand, " ")
	shparams := []string{}
	if len(shcomm) > 1 {
		shparams = shcomm[1:]
	}
	out, err := exec.Command(shcomm[0], shparams...).CombinedOutput()
	if err != nil {
		log.Printf("ERROR: INACTIVITY MONITOR: could not poweroff the instance: %s : %s", err, string(out))
	} else {
		log.Printf("ACTIVITY MONITOR: poweroff command issued: %s, result: %s", c.ShutdownCommand, string(out))
	}
}

func spotGetInstanceActionGcp() (shuttingDown bool) {
	req, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/computeMetadata/v1/instance/preempted?wait_for_change=true", nil)
	if err != nil {
		return false
	}
	req.Header.Add("Metadata-Flavor", "Google")
	tr := &http.Transport{
		DisableKeepAlives: true,
		IdleConnTimeout:   30 * time.Second,
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	if string(body) == "TRUE" {
		return true
	}
	return false
}

func (c *agiExecProxyCmd) spotMonitorGcp() {
	for {
		time.Sleep(10 * time.Second)
		if !spotGetInstanceActionGcp() {
			continue
		}
		/*
			stat, err := getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
			if err != nil {
				logger.Warn("spot-monitor: could not get process status")
				continue
			}
		*/
		stat, _ := getAgiStatus(false, "/opt/agi/ingest/")
		body := "GCP-SPOT-PREEMPTED-NO-CAPACITY"
		c.shuttingDownMutex.Lock()
		c.shuttingDown = true
		c.shuttingDownMutex.Unlock()
		notifyItem := &ingest.NotifyEvent{
			IsDataInMemory:             c.isDim,
			IngestStatus:               stat,
			Event:                      AgiEventSpotNoCapacity,
			AGIName:                    c.AGIName,
			EventDetail:                string(body),
			DeploymentJsonGzB64:        c.deployJson,
			SSHAuthorizedKeysFileGzB64: getSSHAuthorizedKeysGzB64(),
		}
		c.notify.NotifyJSON(notifyItem)
		slackagiLabel, _ := os.ReadFile("/opt/agi/label")
		c.notify.NotifySlack(AgiEventSpotNoCapacity, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *AWS Shutting spot instance down due to capacity restrictions*", AgiEventSpotNoCapacity, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
		time.Sleep(2 * time.Minute)
		c.shuttingDownMutex.Lock()
		c.shuttingDown = false
		c.shuttingDownMutex.Unlock()
	}
}

func spotGetInstanceActionAws() (data []byte, retCode int, err error) {
	req, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data/spot/instance-action", nil)
	if err != nil {
		return nil, 0, err
	}

	tr := &http.Transport{
		DisableKeepAlives: true,
		IdleConnTimeout:   30 * time.Second,
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		body = append(body, []byte("<ERROR:BODY READ ERROR>")...)
	}
	return body, resp.StatusCode, nil
}

func (c *agiExecProxyCmd) spotMonitor() {
	for {
		func() {
			req, err := http.NewRequest(http.MethodGet, "http://169.254.169.254", nil)
			if err != nil {
				return
			}

			tr := &http.Transport{
				DisableKeepAlives: true,
				IdleConnTimeout:   30 * time.Second,
			}
			client := &http.Client{
				Timeout:   30 * time.Second,
				Transport: tr,
			}
			defer client.CloseIdleConnections()
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return
			}
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				return
			}
			if strings.Contains(string(body), "computeMetadata") {
				log.Println("Discovered instance provider GCP")
				c.spotMonitorGcp()
			} else if strings.Contains(string(body), "latest") {
				log.Println("Discovered instance provider AWS")
				c.spotMonitorAws()
			} else {
				return
			}
		}()
		time.Sleep(5 * time.Second)
	}
}

func (c *agiExecProxyCmd) spotMonitorAws() {
	for {
		time.Sleep(30 * time.Second)
		body, code, err := spotGetInstanceActionAws()
		if err != nil || code < 200 || code > 299 {
			continue
		}
		/*
			stat, err := getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
			if err != nil {
				logger.Warn("spot-monitor: could not get process status")
				continue
			}
		*/
		stat, _ := getAgiStatus(false, "/opt/agi/ingest/")
		c.shuttingDownMutex.Lock()
		c.shuttingDown = true
		c.shuttingDownMutex.Unlock()
		notifyItem := &ingest.NotifyEvent{
			IsDataInMemory:             c.isDim,
			IngestStatus:               stat,
			Event:                      AgiEventSpotNoCapacity,
			AGIName:                    c.AGIName,
			EventDetail:                string(body),
			DeploymentJsonGzB64:        c.deployJson,
			SSHAuthorizedKeysFileGzB64: getSSHAuthorizedKeysGzB64(),
		}
		c.notify.NotifyJSON(notifyItem)
		slackagiLabel, _ := os.ReadFile("/opt/agi/label")
		c.notify.NotifySlack(AgiEventSpotNoCapacity, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *AWS Shutting spot instance down due to capacity restrictions*", AgiEventSpotNoCapacity, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
		time.Sleep(2 * time.Minute)
		c.shuttingDownMutex.Lock()
		c.shuttingDown = false
		c.shuttingDownMutex.Unlock()
	}
}

func (c *agiExecProxyCmd) serviceMonitor() {
	servicesRunning := []bool{true, true, true, true}
	for {
		time.Sleep(time.Minute)
		c.shuttingDownMutex.Lock()
		if c.shuttingDown {
			c.shuttingDownMutex.Unlock()
			continue
		}
		c.shuttingDownMutex.Unlock()
		stat, err := getAgiStatus(true, "/opt/agi/ingest/")
		if err != nil {
			logger.Warn("service-monitor: could not get process status")
			continue
		}
		notifyDown := false
		notifyUp := false
		for i, isStopped := range []bool{!stat.AerospikeRunning, !stat.GrafanaHelperRunning, !stat.PluginRunning, !stat.Ingest.Running && (!stat.Ingest.CompleteSteps.ProcessLogs || !stat.Ingest.CompleteSteps.ProcessCollectInfo)} {
			if isStopped && servicesRunning[i] {
				notifyDown = true
			} else if !isStopped && !servicesRunning[i] {
				notifyUp = true
			}
			servicesRunning[i] = !isStopped
		}
		if notifyDown {
			notifyItem := &ingest.NotifyEvent{
				IsDataInMemory:             c.isDim,
				IngestStatus:               stat,
				Event:                      AgiEventServiceDown,
				AGIName:                    c.AGIName,
				DeploymentJsonGzB64:        c.deployJson,
				SSHAuthorizedKeysFileGzB64: getSSHAuthorizedKeysGzB64(),
			}
			c.notify.NotifyJSON(notifyItem)
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			c.notify.NotifySlack(AgiEventServiceDown, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *A required service has quit unexpectedly, check: aerolab agi status*", AgiEventServiceDown, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
		} else if notifyUp {
			notifyItem := &ingest.NotifyEvent{
				IsDataInMemory:             c.isDim,
				IngestStatus:               stat,
				Event:                      AgiEventServiceUp,
				AGIName:                    c.AGIName,
				DeploymentJsonGzB64:        c.deployJson,
				SSHAuthorizedKeysFileGzB64: getSSHAuthorizedKeysGzB64(),
			}
			c.notify.NotifyJSON(notifyItem)
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			c.notify.NotifySlack(AgiEventServiceUp, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *A required service has started back up, check: aerolab agi status*", AgiEventServiceUp, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
		}
	}
}

func (c *agiExecProxyCmd) activityMonitor() {
	var lastActivity time.Time
	for {
		time.Sleep(time.Minute)
		if _, err := os.Stat("/opt/agi/ingest.pid"); err == nil {
			c.lastActivity.Set(time.Now())
			if c.DebugActivityMonitor {
				log.Printf("ingest.pid found at %s", c.lastActivity.Get())
			}
			continue
		}
		if c.gottyConns.Get() != "0" {
			c.lastActivity.Set(time.Now())
			if c.DebugActivityMonitor {
				log.Printf("gottyConns '%s != 0' found at %s", c.gottyConns.Get(), c.lastActivity.Get())
			}
			continue
		}
		pids, err := ps.Processes()
		if err == nil {
			for _, pid := range pids {
				if pid.Pid() == 1 {
					continue
				}
				if pid.Executable() == "bash" {
					c.lastActivity.Set(time.Now())
					if c.DebugActivityMonitor {
						log.Printf("bash (pid=%d ppid=%d) found at %s", pid.Pid(), pid.PPid(), c.lastActivity.Get())
					}
					break
				}
			}
		}
		newActivity := c.lastActivity.Get()
		if c.DebugActivityMonitor {
			log.Printf("lastActivity at %s newActivity at %s maxInactivity %s currentInactivity %s", lastActivity, newActivity, c.MaxInactivity, time.Since(newActivity))
		}
		if time.Since(newActivity) > c.MaxInactivity {
			go func() {
				notifyData, err := getAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
				if err == nil {
					notifyItem := &ingest.NotifyEvent{
						IsDataInMemory:             c.isDim,
						IngestStatus:               notifyData,
						Event:                      AgiEventMaxInactive,
						AGIName:                    c.AGIName,
						DeploymentJsonGzB64:        c.deployJson,
						SSHAuthorizedKeysFileGzB64: getSSHAuthorizedKeysGzB64(),
					}
					c.notify.NotifyJSON(notifyItem)
					slackagiLabel, _ := os.ReadFile("/opt/agi/label")
					c.notify.NotifySlack(AgiEventMaxInactive, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *Max inactivity reached, shutting instance down*", AgiEventMaxInactive, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
				}
			}()
			time.Sleep(time.Minute)
			c.shuttingDownMutex.Lock()
			c.shuttingDown = true
			c.shuttingDownMutex.Unlock()
			shcomm := strings.Split(c.ShutdownCommand, " ")
			shparams := []string{}
			if len(shcomm) > 1 {
				shparams = shcomm[1:]
			}
			out, err := exec.Command(shcomm[0], shparams...).CombinedOutput()
			if err != nil {
				log.Printf("ERROR: INACTIVITY MONITOR: could not poweroff the instance: %s : %s", err, string(out))
			} else {
				log.Printf("ACTIVITY MONITOR: poweroff command issued: %s, result: %s", c.ShutdownCommand, string(out))
			}
		}
		if lastActivity.IsZero() || !lastActivity.Equal(newActivity) {
			lastActivity = newActivity
			logger.Debug("INACTIVITY SHUTDOWN UPDATE: shutdown at %s", lastActivity.Add(c.MaxInactivity))
		}
	}
}

func (c *agiExecProxyCmd) handleInactivity(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	logger.Info("Listener: inactivity status request from %s", r.RemoteAddr)
	lastActivity := c.lastActivity.Get()
	w.Write([]byte(fmt.Sprintf("lastActivity:%s maxInactivity:%s currentInactivity:%s", lastActivity.Format(time.RFC3339), c.MaxInactivity, time.Since(lastActivity))))
}

func (c *agiExecProxyCmd) checkAuthOnly(w http.ResponseWriter, r *http.Request) bool {
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
	// note: cookies are state, while everything else is valid for this request only
	// if user is trying to auth, first step is to set a cookie and refresh (redirect to self)
	// then auth actually happens based on cookie value
	if c.isTokenAuth {
		// if token is provided as form value, set cookie with token value and redirect to self
		r.ParseForm()
		t := r.FormValue(c.TokenName)
		if t != "" {
			http.SetCookie(w, &http.Cookie{
				Name:   c.TokenName,
				Value:  t,
				MaxAge: 0,
				Path:   "/",
			})
			http.Redirect(w, r, r.URL.Path, http.StatusFound)
			return false
		}
		// get token cookie value
		tc, err := r.Cookie(c.TokenName)
		if err == nil {
			t = tc.Value
		}
		// no token cookie, show auth form
		if t == "" {
			c.displayAuthTokenRequest(w, r)
			return false
		}
		// actually try to authenticate
		c.tokens.RLock()
		if !inslice.HasString(c.tokens.tokens, t) {
			c.tokens.RUnlock()
			c.displayAuthTokenRequest(w, r)
			return false
		}
		c.tokens.RUnlock()
	}
	return true
}

func (c *agiExecProxyCmd) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	ret := c.checkAuthOnly(w, r)
	if ret {
		go c.lastActivity.Set(time.Now())
	}
	return ret
}

func (c *agiExecProxyCmd) displayAuthTokenRequest(w http.ResponseWriter, r *http.Request) {
	tc, err := r.Cookie("X-AGI-CALLER")
	if err == nil {
		if tc.Value == "webui" {
			http.Error(w, "token invalid", http.StatusUnauthorized)
			return
		}
	}
	w.Write([]byte(`<html><head><title>authenticate</title></head><body><form>Authentication Token: <input type=text name="` + c.TokenName + `"><input type=Submit name="Login" value="Login"></form></body></html>`))
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
		com := exec.Command("/usr/local/bin/ttyd", "-p", "8852", "-i", "lo", "-P", "5", "-b", "/agi/ttyd", "/bin/bash", "-c", "export TMOUT=3600 && echo '* lnav tool is installed for log analysis' && echo '* aerospike-tools is installed' && echo '* less -S ...: enable horizontal scrolling in less using arrow keys' && echo '* showconf command: showconf collect_info.tgz' && echo '* showsysinfo command: showsysinfo collect_info.tgz' && echo '* showinterrupts command: showinterrupts collect_info.tgz' && /bin/bash")
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
		com := exec.Command("/usr/local/bin/filebrowser", "-p", "8853", "-r", c.EntryDir, "--noauth", "-d", "/opt/filebrowser.db", "-b", "/agi/filebrowser/")
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

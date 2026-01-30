package cmd

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
	"html"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/aerospike/aerolab/pkg/agi"
	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"github.com/aerospike/aerolab/pkg/agi/notifier"
	"github.com/aerospike/aerolab/pkg/webui"
	"github.com/bestmethod/inslice"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	ps "github.com/mitchellh/go-ps"
	"gopkg.in/yaml.v3"
)

// AgiExecProxyCmd runs the AGI web proxy service.
// This is the main entry point for web access to AGI instances, providing:
// - Reverse proxy to Grafana (port 3000 -> /)
// - Reverse proxy to ttyd web terminal (/agi/ttyd)
// - Reverse proxy to filebrowser (/agi/filebrowser)
// - Authentication (none, basic, or token-based)
// - Activity monitoring for auto-shutdown
// - Service health monitoring
// - Spot instance termination monitoring (AWS/GCP)
type AgiExecProxyCmd struct {
	YamlFile             string        `short:"y" long:"yaml" description:"Path to YAML config file"`
	AGIName              string        `long:"agi-name" description:"AGI instance name" yaml:"agiName"`
	InitialLabel         string        `short:"L" long:"label" description:"Freeform label that will appear in the dashboards if set" yaml:"label"`
	IngestProgressPath   string        `short:"i" long:"ingest-progress-path" default:"/opt/agi/ingest/" description:"Path to where ingest stores its JSON progress" yaml:"ingestProgressPath"`
	ListenPort           int           `short:"l" long:"listen-port" default:"80" description:"Port to listen on" yaml:"listenPort"`
	HTTPS                bool          `short:"S" long:"https" description:"Set to enable HTTPS listener" yaml:"https"`
	CertFile             string        `short:"C" long:"cert-file" description:"Required path to server cert file for TLS" yaml:"certFile"`
	KeyFile              string        `short:"K" long:"key-file" description:"Required path to server key file for TLS" yaml:"keyFile"`
	EntryDir             string        `short:"d" long:"entry-dir" default:"/opt/agi/files" description:"Entrypoint for ttyd and filebrowser" yaml:"entryDir"`
	MaxInactivity        time.Duration `short:"m" long:"max-inactivity" default:"1h" description:"Max user inactivity period after which the system will be shut down; 0=disable" yaml:"maxInactivity"`
	MaxUptime            time.Duration `short:"M" long:"max-uptime" default:"24h" description:"Max hard instance uptime; 0=disable" yaml:"maxUptime"`
	ShutdownCommand      string        `short:"c" long:"shutdown-command" default:"/usr/bin/systemctl stop aerospike; /usr/bin/sync; /sbin/poweroff -p || /sbin/poweroff" description:"Command to execute on max uptime or max inactivity being breached" yaml:"shutdownCommand"`
	AuthType             string        `short:"a" long:"auth-type" default:"none" description:"Authentication type; supported: none|basic|token" yaml:"authType"`
	BasicAuthUser        string        `short:"u" long:"basic-auth-user" default:"admin" description:"Basic authentication username" yaml:"basicAuthUser"`
	BasicAuthPass        string        `short:"p" long:"basic-auth-pass" default:"secure" description:"Basic authentication password" yaml:"basicAuthPass"`
	TokenAuthLocation    string        `short:"t" long:"token-path" default:"/opt/agi/tokens" description:"Directory where tokens are stored for access" yaml:"tokenPath"`
	TokenName            string        `short:"T" long:"token-name" default:"AGI_TOKEN" description:"Name of the token variable and cookie to use" yaml:"tokenName"`
	DebugActivityMonitor bool          `short:"D" long:"debug-mode" description:"Set to log activity monitor for debugging" yaml:"debugMode"`
	Help                 HelpCmd       `command:"help" subcommands-optional:"true" description:"Print help"`

	// Internal state fields
	isBasicAuth        bool
	isTokenAuth        bool
	lastActivity       *activity              `no-default:"true"`
	grafanaUrl         *url.URL               `no-default:"true"`
	grafanaProxy       *httputil.ReverseProxy `no-default:"true"`
	ttydUrl            *url.URL               `no-default:"true"`
	ttydProxy          *httputil.ReverseProxy `no-default:"true"`
	fbUrl              *url.URL               `no-default:"true"`
	fbProxy            *httputil.ReverseProxy `no-default:"true"`
	gottyConns         *counter               `no-default:"true"`
	srv                *http.Server           `no-default:"true"`
	tokens             *tokens                `no-default:"true"`
	notify             notifier.HTTPSNotify   `no-default:"true"`
	shuttingDown       bool
	shuttingDownMutex  *sync.Mutex `no-default:"true"`
	slacks3source      string
	slacksftpsource    string
	slackcustomsource  string
	owner              string
	slackAccessDetails string
	isDim              bool
	notifyJSON         bool
	deployJson         string
	wwwSimple          bool
	prettySource       string
}

// tokens is a thread-safe container for authentication tokens
type tokens struct {
	sync.RWMutex
	tokens []string
}

// counter is a thread-safe string counter for tracking connections
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

// activity is a thread-safe last activity timestamp tracker
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

// Execute runs the AGI proxy service.
// This is the main entry point that starts all proxy components:
// - HTTP/HTTPS server
// - Reverse proxies
// - Token management
// - Activity monitoring
// - Service monitoring
// - Spot instance monitoring
func (c *AgiExecProxyCmd) Execute(args []string) error {
	// Load YAML config file if specified
	if c.YamlFile != "" {
		yamlData, err := os.ReadFile(c.YamlFile)
		if err != nil {
			return fmt.Errorf("could not read yaml config file: %w", err)
		}
		if err := yaml.Unmarshal(yamlData, c); err != nil {
			return fmt.Errorf("could not parse yaml config file: %w", err)
		}
		log.Printf("INFO: Loaded proxy config from %s", c.YamlFile)
	}

	// Handle SSH key restoration from temporary file
	if _, err := os.Stat("/tmp/aerolab.install.ssh"); err == nil {
		contents, err := os.ReadFile("/tmp/aerolab.install.ssh")
		if err == nil {
			PutSSHAuthorizedKeys(string(contents))
		}
	}

	// Ensure UUID exists for monitor authentication
	// Each AGI instance needs a unique secret; generate if missing
	if _, err := os.Stat("/opt/agi/uuid"); os.IsNotExist(err) {
		secret := uuid.New().String()
		if err := os.WriteFile("/opt/agi/uuid", []byte(secret), 0644); err != nil {
			log.Printf("Warning: could not create /opt/agi/uuid: %s", err)
		} else {
			log.Printf("INFO: Generated new UUID for monitor authentication")
		}
	}

	// Load deployment JSON for monitor recovery
	deploymentjson, err := os.ReadFile("/opt/agi/deployment.json.gz")
	if err != nil {
		log.Printf("Warning: could not load deployment.json.gz for monitor recovery: %s", err)
	} else {
		c.deployJson = base64.StdEncoding.EncodeToString(deploymentjson)
	}

	// Ensure entry directory exists
	os.MkdirAll(c.EntryDir, 0755)

	// Write PID file
	os.WriteFile("/opt/agi/proxy.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/proxy.pid")

	// Initialize label if not present
	if _, err := os.Stat("/opt/agi/label"); err != nil {
		os.WriteFile("/opt/agi/label", []byte(c.InitialLabel), 0644)
	}

	// Load owner info
	ownerbyte, err := os.ReadFile("/opt/agi/owner")
	if err == nil {
		c.owner = string(ownerbyte)
	}

	// Build access details for Slack notifications
	c.slackAccessDetails = fmt.Sprintf("Attach:\n  `aerolab agi attach -n %s`\nGet Web URL:\n  `aerolab agi list`\nGet Detailed Status:\n  `aerolab agi status -n %s`\nGet auth token:\n  `aerolab agi add-auth-token -n %s`\nChange Label:\n  `aerolab agi change-label -n %s -l \"new label\"`\nDestroy:\n  `aerolab agi destroy -f -n %s`\nDestroy and remove volume (AWS EFS only):\n  `aerolab agi delete -f -n %s`", c.AGIName, c.AGIName, c.AGIName, c.AGIName, c.AGIName, c.AGIName)

	// Check if aerospike is running, start if not
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

	// Initialize internal state
	c.shuttingDownMutex = new(sync.Mutex)
	c.lastActivity = new(activity)
	c.gottyConns = new(counter)
	c.gottyConns.Set("0")
	c.lastActivity.Set(time.Now())

	// Setup reverse proxies
	// Note: These URLs are hardcoded local addresses and should never fail to parse.
	// We handle errors defensively but they indicate a programming bug if triggered.
	gurl, err := url.Parse("http://127.0.0.1:8850/")
	if err != nil {
		return fmt.Errorf("failed to parse grafana proxy URL: %w", err)
	}
	gproxy := httputil.NewSingleHostReverseProxy(gurl)
	c.grafanaUrl = gurl
	c.grafanaProxy = gproxy

	turl, err := url.Parse("http://127.0.0.1:8852/")
	if err != nil {
		return fmt.Errorf("failed to parse ttyd proxy URL: %w", err)
	}
	tproxy := httputil.NewSingleHostReverseProxy(turl)
	c.ttydUrl = turl
	c.ttydProxy = tproxy

	furl, err := url.Parse("http://127.0.0.1:8853/")
	if err != nil {
		return fmt.Errorf("failed to parse filebrowser proxy URL: %w", err)
	}
	fproxy := httputil.NewSingleHostReverseProxy(furl)
	c.fbUrl = furl
	c.fbProxy = fproxy

	// Setup authentication
	c.tokens = new(tokens)
	if c.AuthType == "basic" {
		c.isBasicAuth = true
	}
	if c.AuthType == "token" {
		c.isTokenAuth = true
	}

	// Start dependency downloads
	go c.getDeps()

	// Load ingest config for source information
	ingestConfig, err := ingest.MakeConfig(true, "/opt/agi/ingest.yaml", true)
	if err != nil {
		log.Printf("could not load ingest config for slack notifier: %s", err)
	} else {
		if ingestConfig.Downloader.S3Source.Enabled {
			c.prettySource = fmt.Sprintf("S3 Source: %s:%s %s", ingestConfig.Downloader.S3Source.BucketName, ingestConfig.Downloader.S3Source.PathPrefix, ingestConfig.Downloader.S3Source.SearchRegex)
		}
		if ingestConfig.Downloader.SftpSource.Enabled {
			if c.prettySource != "" {
				c.prettySource = c.prettySource + "<br>"
			}
			c.prettySource = c.prettySource + fmt.Sprintf("SFTP Source: %s:%s %s", ingestConfig.Downloader.SftpSource.Host, ingestConfig.Downloader.SftpSource.PathPrefix, ingestConfig.Downloader.SftpSource.SearchRegex)
		}
		if ingestConfig.CustomSourceName != "" {
			if c.prettySource != "" {
				c.prettySource = c.prettySource + "<br>"
			}
			c.prettySource = c.prettySource + fmt.Sprintf("Custom Source: %s", ingestConfig.CustomSourceName)
		}
	}

	// Load notifier configuration
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

		// Build Slack source strings
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

		// Start background monitors
		go c.serviceMonitor()
		go c.spotMonitor()
	}

	// Start activity monitor
	if c.MaxInactivity > 0 {
		go c.activityMonitor()
	}

	// Start max uptime monitor
	if c.MaxUptime > 0 {
		go c.maxUptime()
	}

	// Start token loader
	go c.loadTokens()

	// Extract web UI
	os.RemoveAll("/opt/agi/www")
	err = os.MkdirAll("/opt/agi/www", 0755)
	if err != nil {
		c.wwwSimple = true
		log.Printf("WARN: simple homepage, error: %s", err)
	} else {
		err = webui.InstallWebsite("/opt/agi/www", agi.AgiProxyWeb)
		if err != nil {
			c.wwwSimple = true
			log.Printf("WARN: simple homepage, error: %s", err)
		}
	}

	// Register HTTP handlers
	http.HandleFunc("/agi/ok", c.handleTokenTest)            // test token, returns ok or lack of success
	http.HandleFunc("/agi/menu", c.handleList)               // URL list and status
	http.HandleFunc("/agi/dist/", c.wwwstatic)               // static files for URL list
	http.HandleFunc("/agi/api/status", c.handleStatus)       // menu web page API
	http.HandleFunc("/agi/api/logs", c.handleLogs)           // menu web page API
	http.HandleFunc("/agi/api/detail", c.handleIngestDetail) // menu web page API
	http.HandleFunc("/agi/monitor-challenge", c.secretValidate)
	http.HandleFunc("/agi/monitor-resize-fs", c.resizeFilesystem) // resize filesystem after volume resize
	http.HandleFunc("/agi/ttyd", c.ttydHandler)                   // web console tty
	http.HandleFunc("/agi/ttyd/", c.ttydHandler)                  // web console tty
	http.HandleFunc("/agi/filebrowser", c.fbHandler)              // file browser
	http.HandleFunc("/agi/filebrowser/", c.fbHandler)             // file browser
	http.HandleFunc("/agi/shutdown", c.handleShutdown)            // gracefully shutdown the proxy
	http.HandleFunc("/agi/poweroff", c.handlePoweroff)            // poweroff the instance
	http.HandleFunc("/agi/status", c.handleStatus)                // high-level agi service status
	http.HandleFunc("/agi/inactivity", c.handleInactivity)        // print inactivity timers
	http.HandleFunc("/agi/ingest/detail", c.handleIngestDetail)   // detailed logingest progress json
	http.HandleFunc("/", c.grafanaHandler)                        // grafana

	// Start HTTP server
	c.srv = &http.Server{Addr: "0.0.0.0:" + strconv.Itoa(c.ListenPort)}
	if c.HTTPS {
		tlsConfig := &tls.Config{
			MinVersion:       tls.VersionTLS12,
			CurvePreferences: []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
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

// secretValidate handles challenge-response authentication from the monitor
func (c *AgiExecProxyCmd) secretValidate(w http.ResponseWriter, r *http.Request) {
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

// resizeFilesystem handles requests from the monitor to resize the filesystem
// after a volume resize. It finds the device mounted at /opt/agi and runs resize2fs.
func (c *AgiExecProxyCmd) resizeFilesystem(w http.ResponseWriter, r *http.Request) {
	// Authenticate using the same secret as secretValidate
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

	// Find the device mounted at /opt/agi using findmnt
	cmd := exec.Command("findmnt", "-n", "-o", "SOURCE", "/opt/agi")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("ERROR: resizeFilesystem: failed to find mount source for /opt/agi: %s", err)
		http.Error(w, fmt.Sprintf("failed to find mount source: %s", err), http.StatusInternalServerError)
		return
	}

	device := strings.TrimSpace(string(output))
	if device == "" {
		log.Printf("ERROR: resizeFilesystem: no device found mounted at /opt/agi")
		http.Error(w, "no device mounted at /opt/agi", http.StatusInternalServerError)
		return
	}

	log.Printf("INFO: resizeFilesystem: resizing filesystem on device %s", device)

	// Run resize2fs to extend the filesystem
	resizeCmd := exec.Command("resize2fs", device)
	resizeOutput, err := resizeCmd.CombinedOutput()
	if err != nil {
		log.Printf("ERROR: resizeFilesystem: resize2fs failed on %s: %s (output: %s)", device, err, string(resizeOutput))
		http.Error(w, fmt.Sprintf("resize2fs failed: %s - %s", err, string(resizeOutput)), http.StatusInternalServerError)
		return
	}

	log.Printf("INFO: resizeFilesystem: successfully resized filesystem on %s: %s", device, string(resizeOutput))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("resized %s: %s", device, string(resizeOutput))))
}

// loadTokensDo reads all token files from the token directory
func (c *AgiExecProxyCmd) loadTokensDo(lockEarly bool) {
	if lockEarly {
		c.tokens.Lock()
		defer c.tokens.Unlock()
	}
	tokens := []string{}
	err := filepath.Walk(c.TokenAuthLocation, func(fpath string, info fs.FileInfo, err error) error {
		if err != nil {
			log.Printf("ERROR: error on walk %s: %s", fpath, err)
			return nil
		}
		if info.IsDir() {
			return nil
		}
		token, err := os.ReadFile(fpath)
		if err != nil {
			log.Printf("ERROR: could not read token file %s: %s", fpath, err)
			return nil
		}
		if len(token) < 64 {
			log.Printf("ERROR: Token file %s contents too short, minimum token length is 64 characters", fpath)
			return nil
		}
		tokens = append(tokens, string(token))
		return nil
	})
	if err != nil {
		log.Printf("ERROR: failed to read tokens: %s", err)
		return
	}
	if !lockEarly {
		c.tokens.Lock()
	}
	c.tokens.tokens = tokens
	if !lockEarly {
		c.tokens.Unlock()
	}
}

// loadTokensInterval loads tokens on a 1-minute interval
func (c *AgiExecProxyCmd) loadTokensInterval() {
	for {
		c.loadTokensDo(false)
		time.Sleep(time.Minute)
	}
}

// tokenViaHUP reloads tokens on SIGHUP signal
func (c *AgiExecProxyCmd) tokenViaHUP() {
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGHUP)

	go func() {
		for sig := range s {
			log.Printf("Hot %s signal, reloading tokens", sig)
			c.loadTokensDo(true)
		}
	}()
}

// loadTokens manages token loading with fsnotify and fallback
func (c *AgiExecProxyCmd) loadTokens() {
	if c.AuthType != "token" {
		return
	}
	os.MkdirAll(c.TokenAuthLocation, 0755)
	go c.loadTokensInterval()
	go c.tokenViaHUP()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("ERROR: fsnotify could not be started, tokens will not be dynamically monitored; switching to once-a-minute system: %s", err)
		return
	}
	defer watcher.Close()
	err = watcher.Add(c.TokenAuthLocation)
	if err != nil {
		log.Printf("ERROR: fsnotify could not add token path, tokens will not be dynamically monitored; switching to once-a-minute system: %s", err)
		return
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				log.Printf("ERROR: fsnotify events error, tokens will not be dynamically monitored; switching to once-a-minute system")
				return
			}
			log.Printf("DEBUG: fsnotify event: %v", event)
			c.loadTokensDo(false)
		case err, ok := <-watcher.Errors:
			log.Printf("ERROR: fsnotify watcher error, tokens will not be dynamically monitored; switching to once-a-minute system (ok:%t err:%s)", ok, err)
			return
		}
	}
}

// handleListSimple renders a simple HTML menu without the template
func (c *AgiExecProxyCmd) handleListSimple(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	out := []byte(`<html><head><title>AGI URLs</title><meta http-equiv="Cache-Control" content="no-cache, no-store, must-revalidate" /><meta http-equiv="Pragma" content="no-cache" /><meta http-equiv="Expires" content="0" /></head><body><center>
<a href="/d/dashList/dashboard-list?from=now-7d&to=now&var-MaxIntervalSeconds=30&var-ProduceDelta&var-ClusterName=All&var-NodeIdent=All&var-Namespace=All&var-Histogram=NONE&var-HistogramDev=NONE&var-HistogramUs=NONE&var-HistogramCount=NONE&var-HistogramSize=NONE&var-XdrDcName=All&var-xdr5dc=All&var-warnC=All&var-warnCtx=All&var-errC=All&var-errCtx=All&orgId=1" target="_blank"><h1>Grafana</h1></a>
<a href="/agi/ttyd" target="_blank"><h1>Web Console (ttyd)</h1></a>
<a href="/agi/filebrowser" target="_blank"><h1>File Browser</h1></a>
</center></body></html>`)
	w.Write(out)
}

// handleLogs returns service log contents
func (c *AgiExecProxyCmd) handleLogs(w http.ResponseWriter, r *http.Request) {
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

	// Check if running in Docker (uses /var/log/services/) or cloud (uses /var/log/agi-*.log with systemd)
	if _, err := os.Stat("/var/log/services"); err == nil {
		// Docker mode - read from /var/log/services/ (except aerospike which uses same path as cloud)
		l.AerospikeLogs = c.getLogFile("/var/log/agi-aerospike.log")
		l.ProxyLogs = c.getLogFile("/var/log/services/agi-proxy.log")
		l.IngestLogs = c.getLogFile("/var/log/services/agi-ingest.log")
		l.GrafanaFixLogs = c.getLogFile("/var/log/services/agi-grafanafix.log")
		l.PluginLogs = c.getLogFile("/var/log/services/agi-plugin.log")
	} else {
		// Cloud mode (AWS/GCP) - use systemd with journalctl fallback
		l.AerospikeLogs = c.getLog("/var/log/agi-aerospike.log", "")
		l.ProxyLogs = c.getLog("/var/log/agi-proxy.log", "agi-proxy")
		l.IngestLogs = c.getLog("/var/log/agi-ingest.log", "agi-ingest")
		l.GrafanaFixLogs = c.getLog("/var/log/agi-grafanafix.log", "agi-grafanafix")
		l.PluginLogs = c.getLog("/var/log/agi-plugin.log", "agi-plugin")
	}

	dmesg, err := exec.Command("dmesg").CombinedOutput()
	if err != nil {
		dmesg = append(dmesg, []byte(err.Error())...)
	}
	l.Dmesg = string(dmesg)
	json.NewEncoder(w).Encode(l)
}

// getLogFile reads the last 20KB of a log file (simple version for Docker)
func (c *AgiExecProxyCmd) getLogFile(path string) string {
	s, err := os.Stat(path)
	if err != nil {
		return fmt.Sprintf("Log file not found: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Sprintf("Error opening log file: %s", err)
	}
	defer f.Close()
	if s.Size() > 20*1024 {
		f.Seek(-20*1024, 2)
		d, _ := io.ReadAll(f)
		idx := bytes.Index(d, []byte{'\n'})
		if idx == -1 {
			return string(d)
		}
		return string(d[idx+1:])
	}
	d, _ := io.ReadAll(f)
	return string(d)
}

// getLog reads the last 20KB of a log file or falls back to journalctl
func (c *AgiExecProxyCmd) getLog(path string, journalName string) string {
	var result strings.Builder

	// Get service status if journalName is provided
	if journalName != "" {
		status, err := exec.Command("systemctl", "is-active", journalName+".service").CombinedOutput()
		statusStr := strings.TrimSpace(string(status))
		if err != nil || strings.Contains(statusStr, "Unknown command") {
			// is-active not supported (Docker) - try systemctl status and parse
			allStatus, statusErr := exec.Command("systemctl", "status").CombinedOutput()
			if statusErr == nil {
				lines := strings.Split(string(allStatus), "\n")
				found := false
				for _, line := range lines {
					if strings.HasSuffix(line, ": "+journalName) || strings.HasSuffix(line, ": "+journalName+".service") {
						if strings.Contains(line, "Running") {
							result.WriteString(fmt.Sprintf("=== Service %s status: active ===\n", journalName))
						} else if strings.Contains(line, "Stopped") {
							result.WriteString(fmt.Sprintf("=== Service %s status: inactive ===\n", journalName))
						} else {
							result.WriteString(fmt.Sprintf("=== Service %s status: %s ===\n", journalName, line))
						}
						found = true
						break
					}
				}
				if !found {
					result.WriteString(fmt.Sprintf("=== Service %s status: not found ===\n", journalName))
				}
			} else {
				result.WriteString(fmt.Sprintf("=== Service %s status: unknown (systemctl error) ===\n", journalName))
			}
		} else {
			result.WriteString(fmt.Sprintf("=== Service %s status: %s ===\n", journalName, statusStr))
		}
	}

	s, err := os.Stat(path)
	if err == nil {
		f, err := os.Open(path)
		if err == nil {
			defer f.Close()
			result.WriteString(fmt.Sprintf("=== Log file: %s (size: %d bytes) ===\n", path, s.Size()))
			if s.Size() > 20*1024 {
				f.Seek(-20*1024, 2)
				d, _ := io.ReadAll(f)
				idx := bytes.Index(d, []byte{'\n'})
				if idx == -1 {
					result.WriteString(string(d))
				} else {
					result.WriteString(string(d[idx+1:]))
				}
			} else {
				d, _ := io.ReadAll(f)
				result.WriteString(string(d))
			}
			return result.String()
		} else {
			result.WriteString(fmt.Sprintf("=== Error opening log file %s: %s ===\n", path, err))
		}
	} else {
		result.WriteString(fmt.Sprintf("=== Log file %s not found: %s ===\n", path, err))
	}

	if journalName == "" {
		return result.String()
	}

	result.WriteString(fmt.Sprintf("=== Journalctl for %s ===\n", journalName))
	l, err := exec.Command("journalctl", "-u", journalName, "-n", "200", "--no-pager").CombinedOutput()
	if err != nil {
		result.WriteString(string(l))
		result.WriteString(err.Error())
	} else {
		result.WriteString(string(l))
	}
	return result.String()
}

// handleList renders the main menu page
func (c *AgiExecProxyCmd) handleList(w http.ResponseWriter, r *http.Request) {
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
		HTTPTitle   string
		Title       string
		Description string
	}
	nlabel := []byte{}
	for _, labelFile := range []string{"/opt/agi/label", "/opt/agi/name"} {
		nlabela, _ := os.ReadFile(labelFile)
		if string(nlabela) == "" {
			continue
		}
		if len(nlabel) == 0 {
			nlabel = nlabela
		} else {
			nlabel = append(nlabel, []byte(" - ")...)
			nlabel = append(nlabel, nlabela...)
		}
	}
	for i := range nlabel {
		if nlabel[i] == 32 || nlabel[i] == 45 || nlabel[i] == 46 || nlabel[i] == 61 || nlabel[i] == 95 {
			continue
		}
		if nlabel[i] >= 48 && nlabel[i] <= 58 {
			continue
		}
		if nlabel[i] >= 65 && nlabel[i] <= 90 {
			continue
		}
		if nlabel[i] >= 97 && nlabel[i] <= 122 {
			continue
		}
		nlabel[i] = ' '
	}
	p := np{
		HTTPTitle:   html.EscapeString(string(nlabel)),
		Title:       html.EscapeString(c.prettySource),
		Description: html.EscapeString(string(nlabel)),
	}
	err = t.ExecuteTemplate(w, "index", p)
	if err != nil {
		log.Print(err)
		c.handleListSimple(w)
		return
	}
}

// wwwstatic serves static files from /opt/agi/www
func (c *AgiExecProxyCmd) wwwstatic(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, path.Join("/opt/agi/www", strings.TrimPrefix(strings.TrimLeft(r.URL.Path, "/"), "agi/")))
}

// handleIngestDetail returns detailed ingest progress
func (c *AgiExecProxyCmd) handleIngestDetail(w http.ResponseWriter, r *http.Request) {
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

// handleTokenTest returns OK if authentication succeeds
func (c *AgiExecProxyCmd) handleTokenTest(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

// handleStatus returns the AGI service status
func (c *AgiExecProxyCmd) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	r.ParseForm()
	log.Printf("INFO: Listener: status request from %s", r.RemoteAddr)
	resp, err := GetAgiStatus(true, c.IngestProgressPath)
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

// handleShutdown gracefully shuts down the proxy
func (c *AgiExecProxyCmd) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	log.Printf("INFO: Listener: shutdown request from %s", r.RemoteAddr)
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
			log.Printf("DEBUG: Graceful Server Shutdown Failed, Forcing shutdown: %s", err)
			c.srv.Close()
		}
	}()
}

// handlePoweroff powers off the instance
func (c *AgiExecProxyCmd) handlePoweroff(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	log.Printf("INFO: Listener: poweroff request from %s", r.RemoteAddr)
	c.shuttingDownMutex.Lock()
	c.shuttingDown = true
	c.shuttingDownMutex.Unlock()
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Poweroff..."))
	go func() {
		out, err := exec.Command("/bin/bash", "-c", c.ShutdownCommand).CombinedOutput()
		if err != nil {
			log.Printf("ERROR: POWEROFF: could not poweroff the instance: %s : %s", err, string(out))
		} else {
			log.Printf("POWEROFF: poweroff command issued: %s, result: %s", c.ShutdownCommand, string(out))
		}
	}()
}

// maxUptime monitors for max uptime and triggers shutdown
func (c *AgiExecProxyCmd) maxUptime() {
	log.Printf("INFO: MAX UPTIME: hard shutdown time: %s", time.Now().Add(c.MaxUptime).String())
	time.Sleep(c.MaxUptime - time.Minute)
	c.shuttingDownMutex.Lock()
	c.shuttingDown = true
	c.shuttingDownMutex.Unlock()
	go func() {
		notifyData, err := GetAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
		if err == nil {
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			notifyItem := &ingest.NotifyEvent{
				Label:                      string(slackagiLabel),
				Owner:                      c.owner,
				S3Source:                   c.slacks3source,
				SftpSource:                 c.slacksftpsource,
				LocalSource:                c.slackcustomsource,
				IsDataInMemory:             c.isDim,
				IngestStatus:               notifyData,
				Event:                      agi.AgiEventMaxAge,
				AGIName:                    c.AGIName,
				DeploymentJsonGzB64:        c.deployJson,
				SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
			}
			c.notify.NotifyJSON(notifyItem)
			c.notify.NotifySlack(agi.AgiEventMaxAge, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *Max age reached, shutting down*", agi.AgiEventMaxAge, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
		}
	}()
	time.Sleep(time.Minute)
	out, err := exec.Command("/bin/bash", "-c", c.ShutdownCommand).CombinedOutput()
	if err != nil {
		log.Printf("ERROR: MAX UPTIME: could not poweroff the instance: %s : %s", err, string(out))
	} else {
		log.Printf("MAX UPTIME: poweroff command issued: %s, result: %s", c.ShutdownCommand, string(out))
	}
}

// spotGetInstanceActionGcp checks GCP metadata for preemption
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

// spotMonitorGcp monitors GCP spot instance preemption
func (c *AgiExecProxyCmd) spotMonitorGcp() {
	for {
		time.Sleep(10 * time.Second)
		if !spotGetInstanceActionGcp() {
			continue
		}
		stat, _ := GetAgiStatus(false, "/opt/agi/ingest/")
		body := "GCP-SPOT-PREEMPTED-NO-CAPACITY"
		c.shuttingDownMutex.Lock()
		c.shuttingDown = true
		c.shuttingDownMutex.Unlock()
		slackagiLabel, _ := os.ReadFile("/opt/agi/label")
		notifyItem := &ingest.NotifyEvent{
			Label:                      string(slackagiLabel),
			Owner:                      c.owner,
			S3Source:                   c.slacks3source,
			SftpSource:                 c.slacksftpsource,
			LocalSource:                c.slackcustomsource,
			IsDataInMemory:             c.isDim,
			IngestStatus:               stat,
			Event:                      agi.AgiEventSpotNoCapacity,
			AGIName:                    c.AGIName,
			EventDetail:                string(body),
			DeploymentJsonGzB64:        c.deployJson,
			SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
		}
		c.notify.NotifyJSON(notifyItem)
		c.notify.NotifySlack(agi.AgiEventSpotNoCapacity, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *GCP Shutting spot instance down due to capacity restrictions*", agi.AgiEventSpotNoCapacity, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
		time.Sleep(2 * time.Minute)
		c.shuttingDownMutex.Lock()
		c.shuttingDown = false
		c.shuttingDownMutex.Unlock()
	}
}

// spotGetInstanceActionAws checks AWS metadata for spot instance action
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

// spotMonitor detects cloud provider and starts appropriate monitoring
func (c *AgiExecProxyCmd) spotMonitor() {
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

// spotMonitorAws monitors AWS spot instance termination
func (c *AgiExecProxyCmd) spotMonitorAws() {
	for {
		time.Sleep(30 * time.Second)
		body, code, err := spotGetInstanceActionAws()
		if err != nil || code < 200 || code > 299 {
			continue
		}
		stat, _ := GetAgiStatus(false, "/opt/agi/ingest/")
		c.shuttingDownMutex.Lock()
		c.shuttingDown = true
		c.shuttingDownMutex.Unlock()
		slackagiLabel, _ := os.ReadFile("/opt/agi/label")
		notifyItem := &ingest.NotifyEvent{
			Label:                      string(slackagiLabel),
			Owner:                      c.owner,
			S3Source:                   c.slacks3source,
			SftpSource:                 c.slacksftpsource,
			LocalSource:                c.slackcustomsource,
			IsDataInMemory:             c.isDim,
			IngestStatus:               stat,
			Event:                      agi.AgiEventSpotNoCapacity,
			AGIName:                    c.AGIName,
			EventDetail:                string(body),
			DeploymentJsonGzB64:        c.deployJson,
			SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
		}
		c.notify.NotifyJSON(notifyItem)
		c.notify.NotifySlack(agi.AgiEventSpotNoCapacity, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *AWS Shutting spot instance down due to capacity restrictions*", agi.AgiEventSpotNoCapacity, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
		time.Sleep(2 * time.Minute)
		c.shuttingDownMutex.Lock()
		c.shuttingDown = false
		c.shuttingDownMutex.Unlock()
	}
}

// serviceMonitor monitors AGI service health and sends notifications
func (c *AgiExecProxyCmd) serviceMonitor() {
	servicesRunning := []bool{true, true, true, true}
	for {
		time.Sleep(time.Minute)
		c.shuttingDownMutex.Lock()
		if c.shuttingDown {
			c.shuttingDownMutex.Unlock()
			continue
		}
		c.shuttingDownMutex.Unlock()
		stat, err := GetAgiStatus(true, "/opt/agi/ingest/")
		if err != nil {
			log.Printf("WARN: service-monitor: could not get process status")
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
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			notifyItem := &ingest.NotifyEvent{
				Label:                      string(slackagiLabel),
				Owner:                      c.owner,
				S3Source:                   c.slacks3source,
				SftpSource:                 c.slacksftpsource,
				LocalSource:                c.slackcustomsource,
				IsDataInMemory:             c.isDim,
				IngestStatus:               stat,
				Event:                      agi.AgiEventServiceDown,
				AGIName:                    c.AGIName,
				DeploymentJsonGzB64:        c.deployJson,
				SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
			}
			c.notify.NotifyJSON(notifyItem)
			c.notify.NotifySlack(agi.AgiEventServiceDown, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *A required service has quit unexpectedly, check: aerolab agi status*", agi.AgiEventServiceDown, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
		} else if notifyUp {
			slackagiLabel, _ := os.ReadFile("/opt/agi/label")
			notifyItem := &ingest.NotifyEvent{
				Label:                      string(slackagiLabel),
				Owner:                      c.owner,
				S3Source:                   c.slacks3source,
				SftpSource:                 c.slacksftpsource,
				LocalSource:                c.slackcustomsource,
				IsDataInMemory:             c.isDim,
				IngestStatus:               stat,
				Event:                      agi.AgiEventServiceUp,
				AGIName:                    c.AGIName,
				DeploymentJsonGzB64:        c.deployJson,
				SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
			}
			c.notify.NotifyJSON(notifyItem)
			c.notify.NotifySlack(agi.AgiEventServiceUp, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *A required service has started back up, check: aerolab agi status*", agi.AgiEventServiceUp, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
		}
	}
}

// activityMonitor monitors user activity and triggers shutdown on inactivity
func (c *AgiExecProxyCmd) activityMonitor() {
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
				notifyData, err := GetAgiStatus(c.notifyJSON, "/opt/agi/ingest/")
				if err == nil {
					slackagiLabel, _ := os.ReadFile("/opt/agi/label")
					notifyItem := &ingest.NotifyEvent{
						Label:                      string(slackagiLabel),
						Owner:                      c.owner,
						S3Source:                   c.slacks3source,
						SftpSource:                 c.slacksftpsource,
						LocalSource:                c.slackcustomsource,
						IsDataInMemory:             c.isDim,
						IngestStatus:               notifyData,
						Event:                      agi.AgiEventMaxInactive,
						AGIName:                    c.AGIName,
						DeploymentJsonGzB64:        c.deployJson,
						SSHAuthorizedKeysFileGzB64: GetSSHAuthorizedKeysGzB64(),
					}
					c.notify.NotifyJSON(notifyItem)
					c.notify.NotifySlack(agi.AgiEventMaxInactive, fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *Max inactivity reached, shutting instance down*", agi.AgiEventMaxInactive, time.Now().Format(time.RFC822), c.AGIName, string(slackagiLabel), c.owner, c.slacks3source, c.slacksftpsource, c.slackcustomsource), c.slackAccessDetails)
				}
			}()
			time.Sleep(time.Minute)
			c.shuttingDownMutex.Lock()
			c.shuttingDown = true
			c.shuttingDownMutex.Unlock()
			out, err := exec.Command("/bin/bash", "-c", c.ShutdownCommand).CombinedOutput()
			if err != nil {
				log.Printf("ERROR: INACTIVITY MONITOR: could not poweroff the instance: %s : %s", err, string(out))
			} else {
				log.Printf("ACTIVITY MONITOR: poweroff command issued: %s, result: %s", c.ShutdownCommand, string(out))
			}
		}
		if lastActivity.IsZero() || !lastActivity.Equal(newActivity) {
			lastActivity = newActivity
			log.Printf("DEBUG: INACTIVITY SHUTDOWN UPDATE: shutdown at %s", lastActivity.Add(c.MaxInactivity))
		}
	}
}

// handleInactivity returns current inactivity timing information
func (c *AgiExecProxyCmd) handleInactivity(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuthOnly(w, r) {
		return
	}
	log.Printf("INFO: Listener: inactivity status request from %s", r.RemoteAddr)
	lastActivity := c.lastActivity.Get()
	w.Write([]byte(fmt.Sprintf("lastActivity:%s maxInactivity:%s currentInactivity:%s", lastActivity.Format(time.RFC3339), c.MaxInactivity, time.Since(lastActivity))))
}

// checkAuthOnly checks authentication without updating activity timestamp
func (c *AgiExecProxyCmd) checkAuthOnly(w http.ResponseWriter, r *http.Request) bool {
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

// checkAuth checks authentication and updates activity timestamp
func (c *AgiExecProxyCmd) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	ret := c.checkAuthOnly(w, r)
	if ret {
		go c.lastActivity.Set(time.Now())
	}
	return ret
}

// displayAuthTokenRequest shows the token authentication form
func (c *AgiExecProxyCmd) displayAuthTokenRequest(w http.ResponseWriter, r *http.Request) {
	tc, err := r.Cookie("X-AGI-CALLER")
	if err == nil {
		if tc.Value == "webui" {
			http.Error(w, "token invalid", http.StatusUnauthorized)
			return
		}
	}
	w.Write([]byte(`<html><head><title>authenticate</title></head><body><form>Authentication Token: <input type=text name="` + c.TokenName + `"><input type=Submit name="Login" value="Login"></form></body></html>`))
}

// grafanaHandler proxies requests to Grafana
func (c *AgiExecProxyCmd) grafanaHandler(w http.ResponseWriter, r *http.Request) {
	// auth check
	if !c.checkAuth(w, r) {
		return
	}
	// reverse proxy
	r.URL.Host = c.grafanaUrl.Host
	r.URL.Scheme = c.grafanaUrl.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = c.grafanaUrl.Host
	r.Header.Del("Origin")
	c.grafanaProxy.ServeHTTP(w, r)
}

// ttydHandler proxies requests to ttyd web terminal
func (c *AgiExecProxyCmd) ttydHandler(w http.ResponseWriter, r *http.Request) {
	// auth check
	if !c.checkAuth(w, r) {
		return
	}
	// reverse proxy
	r.URL.Host = c.ttydUrl.Host
	r.URL.Scheme = c.ttydUrl.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = c.ttydUrl.Host
	r.Header.Del("Origin")
	c.ttydProxy.ServeHTTP(w, r)
}

// fbHandler proxies requests to filebrowser
func (c *AgiExecProxyCmd) fbHandler(w http.ResponseWriter, r *http.Request) {
	// auth check
	if !c.checkAuth(w, r) {
		return
	}
	// reverse proxy
	r.URL.Host = c.fbUrl.Host
	r.URL.Scheme = c.fbUrl.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = c.fbUrl.Host
	r.Header.Del("Origin")
	c.fbProxy.ServeHTTP(w, r)
}

// getTtyd downloads and runs ttyd web terminal
func (c *AgiExecProxyCmd) getTtyd() error {
	// Skip download if ttyd already exists - do NOT overwrite existing binary
	if _, err := os.Stat("/usr/local/bin/ttyd"); err != nil {
		log.Printf("INFO: Getting ttyd...")
		fd, err := os.OpenFile("/usr/local/bin/ttyd", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if err != nil {
			// If file already exists (race condition) or text file busy, skip download
			if os.IsExist(err) || strings.Contains(err.Error(), "text file busy") {
				log.Printf("DEBUG: ttyd binary already exists, skipping download")
			} else {
				return fmt.Errorf("ttyd-MAKEFILE: %s", err)
			}
		} else {
			arch := "x86_64"
			narch, _ := exec.Command("uname", "-m").CombinedOutput()
			if strings.Contains(string(narch), "arm") || strings.Contains(string(narch), "aarch") {
				arch = "aarch64"
			}
			client := &http.Client{}
			client.Timeout = time.Minute
			defer client.CloseIdleConnections()
			req, err := http.NewRequest("GET", "https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd."+arch, nil)
			if err != nil {
				fd.Close()
				os.Remove("/usr/local/bin/ttyd")
				return err
			}
			response, err := client.Do(req)
			if err != nil {
				fd.Close()
				os.Remove("/usr/local/bin/ttyd")
				return err
			}
			_, err = io.Copy(fd, response.Body)
			response.Body.Close()
			fd.Close()
			if err != nil {
				os.Remove("/usr/local/bin/ttyd")
				return fmt.Errorf("ttyd-DOWNLOAD: %s", err)
			}
		}
	}
	log.Printf("INFO: Running ttyd!")
	nlabel := []byte{}
	for _, labelFile := range []string{"/opt/agi/label", "/opt/agi/name"} {
		nlabela, _ := os.ReadFile(labelFile)
		if string(nlabela) == "" {
			continue
		}
		if len(nlabel) == 0 {
			nlabel = nlabela
		} else {
			nlabel = append(nlabel, []byte(" - ")...)
			nlabel = append(nlabel, nlabela...)
		}
	}
	for i := range nlabel {
		if nlabel[i] == 32 || nlabel[i] == 45 || nlabel[i] == 46 || nlabel[i] == 61 || nlabel[i] == 95 {
			continue
		}
		if nlabel[i] >= 48 && nlabel[i] <= 58 {
			continue
		}
		if nlabel[i] >= 65 && nlabel[i] <= 90 {
			continue
		}
		if nlabel[i] >= 97 && nlabel[i] <= 122 {
			continue
		}
		nlabel[i] = ' '
	}
	com := exec.Command("/usr/local/bin/ttyd", "-t", "titleFixed="+string(nlabel), "-t", "scrollback=10000", "-T", "vt220", "-W", "-p", "8852", "-i", "lo", "-P", "5", "-b", "/agi/ttyd", "/bin/bash", "-c", "export TMOUT=3600 && echo '* lnav tool is installed for log analysis' && echo '* aerospike-tools is installed' && echo '* less -S ...: enable horizontal scrolling in less using arrow keys' && echo '* showconf command: showconf collect_info.tgz' && echo '* showsysinfo command: showsysinfo collect_info.tgz' && echo '* showinterrupts command: showinterrupts collect_info.tgz' && /bin/bash")
	com.Dir = c.EntryDir
	sout, err := com.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ttyd cannot start: could not create stdout pipe: %s", err)
	}
	serr, err2 := com.StderrPipe()
	if err2 != nil {
		return fmt.Errorf("ttyd cannot start: could not create stderr pipe: %s", err2)
	}
	err = com.Start()
	if err != nil {
		return fmt.Errorf("ttyd cannot start: %s", err)
	}
	go c.gottyWatcher(sout)
	go c.gottyWatcher(serr)
	err = com.Wait()
	if err != nil {
		return fmt.Errorf("ttyd exited with error: %s", err)
	}
	return nil
}

// getFilebrowser downloads and runs filebrowser
func (c *AgiExecProxyCmd) getFilebrowser() error {
	// Skip download if filebrowser already exists - do NOT overwrite existing binary
	if _, err := os.Stat("/usr/local/bin/filebrowser"); err != nil {
		log.Printf("INFO: Getting filebrowser...")
		fd, err := os.OpenFile("/opt/filebrowser.tgz", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if err != nil {
			// If file already exists (race condition), check if filebrowser binary exists now
			if os.IsExist(err) {
				if _, statErr := os.Stat("/usr/local/bin/filebrowser"); statErr == nil {
					log.Printf("DEBUG: filebrowser binary already exists, skipping download")
				}
			} else {
				return err
			}
		} else {
			arch := "amd64"
			narch, _ := exec.Command("uname", "-m").CombinedOutput()
			if strings.Contains(string(narch), "arm") || strings.Contains(string(narch), "aarch") {
				arch = "arm64"
			}
			client := &http.Client{}
			client.Timeout = time.Minute
			defer client.CloseIdleConnections()
			req, err := http.NewRequest("GET", "https://github.com/filebrowser/filebrowser/releases/download/v2.32.0/linux-"+arch+"-filebrowser.tar.gz", nil)
			if err != nil {
				fd.Close()
				os.Remove("/opt/filebrowser.tgz")
				return err
			}
			response, err := client.Do(req)
			if err != nil {
				fd.Close()
				os.Remove("/opt/filebrowser.tgz")
				return err
			}
			_, err = io.Copy(fd, response.Body)
			response.Body.Close()
			fd.Close()
			if err != nil {
				os.Remove("/opt/filebrowser.tgz")
				return err
			}
			log.Printf("INFO: Unpack filebrowser")
			out, err := exec.Command("tar", "-zxvf", "/opt/filebrowser.tgz", "-C", "/usr/local/bin/", "filebrowser").CombinedOutput()
			if err != nil {
				return fmt.Errorf("filebrowser: %s %s", err, string(out))
			}
		}
	}
	log.Printf("INFO: Running filebrowser!")
	com := exec.Command("/usr/local/bin/filebrowser", "-p", "8853", "-r", c.EntryDir, "--noauth", "-d", "/opt/filebrowser.db", "-b", "/agi/filebrowser/")
	com.Dir = c.EntryDir
	out, err := com.CombinedOutput()
	if err != nil {
		return fmt.Errorf("filebrowser: %s %s", err, string(out))
	}
	return nil
}

// getDeps starts background goroutines to download and run dependencies
func (c *AgiExecProxyCmd) getDeps() {
	go func() {
		for {
			err := c.getTtyd()
			if err != nil {
				log.Printf("ERROR: TTYD: %s", err)
			}
			time.Sleep(30 * time.Second)
		}
	}()
	go func() {
		for {
			err := c.getFilebrowser()
			if err != nil {
				log.Printf("ERROR: FILEBROWSER: %s", err)
			}
			time.Sleep(30 * time.Second)
		}
	}()
	go func() {
		cur, err := filepath.Abs(os.Args[0])
		if err != nil {
			log.Printf("ERROR: failed to get absolute path of self: %s", err)
			return
		}
		if _, err := os.Stat("/usr/local/bin/showconf"); err != nil {
			err = os.Symlink(cur, "/usr/local/bin/showconf")
			if err != nil {
				log.Printf("ERROR: failed to symlink showconf: %s", err)
			}
		}
		if _, err := os.Stat("/usr/local/bin/showsysinfo"); err != nil {
			err = os.Symlink(cur, "/usr/local/bin/showsysinfo")
			if err != nil {
				log.Printf("ERROR: failed to symlink showsysinfo: %s", err)
			}
		}
		if _, err := os.Stat("/usr/local/bin/showinterrupts"); err != nil {
			err = os.Symlink(cur, "/usr/local/bin/showinterrupts")
			if err != nil {
				log.Printf("ERROR: failed to symlink showinterrupts: %s", err)
			}
		}
	}()
}

// gottyWatcher monitors ttyd output for connection count
func (c *AgiExecProxyCmd) gottyWatcher(out io.Reader) {
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
			log.Printf("INFO: TTYD CONNS: %s", connNew)
			c.gottyConns.Set(connNew)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("ERROR: gottyWatcher scanner error: %s", err)
	}
	log.Printf("INFO: Exiting gottyWatcher")
}

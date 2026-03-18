package cmd

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/utils/openbrowser"
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
	"github.com/aerospike/aerolab/pkg/webui"
)

// WebUICmd runs the AeroLab REST API server.
// This provides a REST interface to all aerolab commands, enabling:
// - Command exploration (list commands, parameters, metadata)
// - Asynchronous command execution via JSON payloads with job tracking
// - File upload/download streaming
// - Job status monitoring and log streaming
type WebUICmd struct {
	ListenAddr    string `short:"l" long:"listen" default:"127.0.0.1:3333" description:"Address to listen on (host:port)"`
	HTTPS         bool   `long:"https" description:"Enable HTTPS listener"`
	CertFile      string `long:"cert" description:"Path to TLS certificate file (required if --https)"`
	KeyFile       string `long:"key" description:"Path to TLS key file (required if --https)"`
	AuthType      string `long:"auth" default:"none" description:"Authentication type: none|basic|token"`
	BasicAuthUser string `long:"basic-user" default:"admin" description:"Basic auth username"`
	BasicAuthPass string `long:"basic-pass" default:"" description:"Basic auth password"`
	TokenAuthPath string `long:"token-path" default:"" description:"Path to file containing valid tokens (one per line)"`
	CORSOrigins   string `long:"cors-origins" default:"*" description:"Comma-separated list of allowed CORS origins"`
	ReadTimeout   int    `long:"read-timeout" default:"300" description:"HTTP read timeout in seconds"`
	WriteTimeout  int    `long:"write-timeout" default:"300" description:"HTTP write timeout in seconds"`
	UserHeader    string `long:"user-header" default:"" description:"Header name to extract user from (e.g., X-User, X-Forwarded-User). If not set or header missing, uses system user."`
	WebRoot       string `long:"webroot" default:"/" description:"set the web root that should be served, useful if proxying from eg /aerolab on a webserver"`

	// File browser / upload settings
	BlockServerLs      bool   `long:"block-server-ls" description:"Block file exploration on the server altogether"`
	AllowLsEverywhere  bool   `long:"always-server-ls" description:"By default server file browser only works on localhost; enable to allow from everywhere"`
	MaxUploadSizeBytes int    `long:"max-upload-size-bytes" description:"Max file upload size in bytes when server browsing is blocked (0=disable uploads)" default:"209715200"`
	UploadTempDir      string `long:"upload-temp-dir" description:"Temporary directory for file uploads (default: {aerolabRoot}/web.tmp)"`

	// Inventory refresh
	MinimumInterval string `long:"minimum-interval" default:"10s" description:"minimum interval between inventory refreshes (avoid API limit exhaustion)"`

	// Job limits
	MaxConcurrentJob int `long:"max-concurrent-job" default:"5" description:"Max number of jobs to run concurrently"`
	MaxQueuedJob     int `long:"max-queued-job" default:"10" description:"Max number of jobs to queue for execution"`
	ShowMaxHistory   int `long:"show-max-history" default:"100" description:"show only this amount of completed historical items max"`

	// Firewall
	UniqueFirewalls bool `long:"unique-firewalls" description:"for multi-user hosted mode: enable per-username firewalls"`

	// AGI
	AgiStrictTls bool `long:"agi-strict-tls" description:"when performing inventory lookup, expect valid AGI certificates"`

	// WebSocket proxy
	WsProxyOrigin string `long:"ws-proxy-origin" description:"when using proxies, set this to host (or host:port) URI that Origin header should also be accepted for"`

	// Simple mode
	ForceSimpleMode bool `long:"force-simple-mode" description:"force use of simple mode, limiting the number of features and switches that show up"`

	// Page title
	PageTitle string `long:"page-title" default:"AeroLab Web UI" description:"change the title of the webpages"`

	// Inventory polling
	RefreshInterval string `long:"refresh-interval" default:"30s" description:"change interval at which the inventory is refreshed in the background"`

	// Browser options
	NoBrowser bool `long:"nobrowser" description:"Do not automatically open the browser on startup"`

	// Config file for parameter overrides
	ConfigFile string `long:"config" description:"Path to YAML config file for WebUI parameter overrides (choices, defaults, visibility)"`

	// Job lifecycle options
	HistoryExpires  string `long:"history-expires" default:"72h" description:"time to keep job from their start in history"`
	MaxJobRuntime   string `long:"max-job-runtime" default:"60m" description:"Maximum time a job can run before being killed (e.g., 1h, 30m). Set to 0 for no limit."`
	CleanupInterval string `long:"cleanup-interval" default:"1h" description:"How often to run cleanup of old jobs"`

	// AGI Monitor (embedded)
	AgiMonitorEnable bool                `long:"agi-monitor-enable" description:"Enable built-in AGI monitor for auto-sizing and spot rotation"`
	AgiMonitor       AgiMonitorConfigCmd `group:"AGI Monitor" namespace:"agi-monitor" description:"AGI Monitor configuration (requires --agi-monitor-enable)"`

	// Subcommands
	Exec WebUIExecCmd `command:"exec" subcommands-optional:"true" description:"Execute command (internal use)" hidden:"true"`
	Help HelpCmd      `command:"help" subcommands-optional:"true" description:"Print help"`

	// Internal state
	system             *System
	commandTree        *CommandInfo
	srv                *http.Server
	tokens             []string
	tokensMutex        sync.RWMutex
	isBasicAuth        bool
	isTokenAuth        bool
	jobManager         *JobManager
	historyExpiresDur  time.Duration
	maxJobRuntimeDur   time.Duration
	cleanupIntervalDur time.Duration
	refreshIntervalDur time.Duration
	minimumIntervalDur time.Duration
	jobSemaphore       chan struct{}
	pendingJobs        atomic.Int32
	shutdownChan       chan struct{}
	rootPath           string // normalized root path (e.g., "/boblab" or "")
	spaHandler         *webui.SPAHandler

	// AGI token cache – stores auth tokens per AGI instance for reuse
	agiTokens agiTokenCache

	// AGI status cache – stores ingest status display strings per AGI instance
	agiStatus agiStatusCache

	// AGI Monitor (embedded) – internal state
	agiMonitorInstance *agiMonitor       // nil when disabled
	agiSizingState     agiSizingStateMap // tracks which AGI instances are being sized

	// Expiry-aware refresh scheduler
	expiryScheduler *expiryRefreshScheduler

	// Inventory refresh rate-limiting
	lastInventoryRefreshTime time.Time
	lastInventoryRefreshMu   sync.Mutex

	// Graceful shutdown
	shuttingDown atomic.Bool
	jobWg        sync.WaitGroup

	// Simple mode configuration
	simpleModeConfig *SimpleModeConfig
}

func parseDurationWithDays(s string) (time.Duration, error) {
	return ParseExtendedDuration(s)
}

func (c *WebUICmd) Execute(args []string) error {
	cmd := []string{"webui"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	// Set non-interactive mode for all command executions
	os.Setenv("AEROLAB_NONINTERACTIVE", "1")
	if c.UniqueFirewalls {
		os.Setenv("AEROLAB_UNIQUE_FIREWALLS", "1")
	}

	c.system = system

	// Validate HTTPS config
	if c.HTTPS {
		if c.CertFile == "" || c.KeyFile == "" {
			return Error(fmt.Errorf("--cert and --key are required when --https is enabled"), system, cmd, c, args)
		}
	}

	// Setup authentication
	switch c.AuthType {
	case "none":
		// No auth
	case "basic":
		if c.BasicAuthPass == "" {
			return Error(fmt.Errorf("--basic-pass is required when --auth=basic"), system, cmd, c, args)
		}
		c.isBasicAuth = true
	case "token":
		if c.TokenAuthPath == "" {
			return Error(fmt.Errorf("--token-path is required when --auth=token"), system, cmd, c, args)
		}
		c.isTokenAuth = true
		if err := c.loadTokens(); err != nil {
			return Error(fmt.Errorf("failed to load tokens: %w", err), system, cmd, c, args)
		}
	default:
		return Error(fmt.Errorf("invalid auth type: %s (must be none|basic|token)", c.AuthType), system, cmd, c, args)
	}

	// Build command tree via reflection
	system.Logger.Info("Building command tree...")
	c.commandTree = BuildCommandTree(system.Opts)
	system.Logger.Info("Command tree built with %d top-level commands", len(c.commandTree.Children))

	// Filter out parameters for non-active backends (mirrors CLI's ShowHideBackend behavior)
	backendType := ""
	if system.Opts != nil && system.Opts.Config.Backend.Type != "" {
		backendType = system.Opts.Config.Backend.Type
	}
	if backendType != "" && backendType != "none" {
		filterBackendParameters(c.commandTree, backendType)
		system.Logger.Info("Filtered command parameters for backend: %s", backendType)
	}

	// Apply simple mode overrides from config file
	c.simpleModeConfig = system.SimpleModeConfig
	if c.simpleModeConfig != nil {
		c.simpleModeConfig.ApplyToCommandTree(c.commandTree)
		system.Logger.Info("Applied simple mode overrides from config (force=%t)", c.simpleModeConfig.ForceEnabled)
	}

	// Apply --force-simple-mode: set env var for subprocesses but do NOT block
	// the webui process itself (avoid chicken-and-egg if webui is hidden in simple mode config)
	if c.ForceSimpleMode {
		os.Setenv("AEROLAB_FORCE_SIMPLE_MODE", "true")
		if c.simpleModeConfig == nil {
			c.simpleModeConfig = &SimpleModeConfig{ForceEnabled: true}
		} else {
			c.simpleModeConfig.ForceEnabled = true
		}
		c.simpleModeConfig.ApplyToCommandTree(c.commandTree)
	}

	// Apply parameter overrides from external config file
	if c.ConfigFile != "" {
		if err := applyConfigOverrides(c.commandTree, c.ConfigFile, system); err != nil {
			return Error(fmt.Errorf("failed to apply config overrides: %w", err), system, cmd, c, args)
		}
	}

	// Parse duration options
	c.historyExpiresDur, err = parseDurationWithDays(c.HistoryExpires)
	if err != nil {
		return Error(fmt.Errorf("invalid --history-expires duration: %w", err), system, cmd, c, args)
	}
	c.maxJobRuntimeDur, err = parseDurationWithDays(c.MaxJobRuntime)
	if err != nil {
		return Error(fmt.Errorf("invalid --max-job-runtime duration: %w", err), system, cmd, c, args)
	}
	c.cleanupIntervalDur, err = parseDurationWithDays(c.CleanupInterval)
	if err != nil {
		return Error(fmt.Errorf("invalid --cleanup-interval duration: %w", err), system, cmd, c, args)
	}
	c.refreshIntervalDur, err = parseDurationWithDays(c.RefreshInterval)
	if err != nil {
		return Error(fmt.Errorf("invalid --refresh-interval duration: %w", err), system, cmd, c, args)
	}
	c.minimumIntervalDur, err = parseDurationWithDays(c.MinimumInterval)
	if err != nil {
		return Error(fmt.Errorf("invalid --minimum-interval duration: %w", err), system, cmd, c, args)
	}

	// Re-initialize backend with inventory polling enabled (if interval > 0)
	if c.refreshIntervalDur > 0 {
		system.InitOptions.Backend = nil // force fresh InitBackend with poll settings
		system.InitOptions.Backend = &InitBackend{
			PollInventoryHourly: true,
			PollInterval:        c.refreshIntervalDur,
			UseCache:            system.Opts.Config.Backend.InventoryCache,
			GCPAuthMethod:       clouds.GCPAuthMethod(system.Opts.Config.Backend.GCPAuthMethod),
			GCPBrowser:          !system.Opts.Config.Backend.GCPNoBrowser,
			GCPClientID:         system.Opts.Config.Backend.GCPClientID,
			GCPClientSecret:     system.Opts.Config.Backend.GCPClientSecret,
		}
		if err := system.GetBackend(true); err != nil {
			return Error(fmt.Errorf("failed to reinitialize backend with polling: %w", err), system, cmd, c, args)
		}
		system.Logger.Info("Backend inventory polling enabled (interval: %s)", c.refreshIntervalDur)
	}

	// Initialize job manager
	system.Logger.Info("Initializing job manager...")
	jobMgr, err := NewJobManager()
	if err != nil {
		return Error(fmt.Errorf("failed to initialize job manager: %w", err), system, cmd, c, args)
	}
	c.jobManager = jobMgr
	system.Logger.Info("Job manager initialized")

	// Initialize job concurrency semaphore
	if c.MaxConcurrentJob > 0 {
		c.jobSemaphore = make(chan struct{}, c.MaxConcurrentJob)
	}

	// Initialize shutdown channel
	c.shutdownChan = make(chan struct{})

	// Normalize root path
	c.rootPath = ""
	if c.WebRoot != "" && c.WebRoot != "/" {
		c.rootPath = "/" + strings.Trim(c.WebRoot, "/")
	}

	// Initialize upload temp dir if not set
	if c.UploadTempDir == "" {
		if cfgDir, err := ConfigFileName(); err == nil {
			c.UploadTempDir = filepath.Join(filepath.Dir(cfgDir), "web.tmp")
		} else {
			c.UploadTempDir = filepath.Join(os.TempDir(), "aerolab-web-tmp")
		}
	}

	// Initialize SPA handler for serving the web UI
	spaHandler, err := webui.NewSPAHandler()
	if err != nil {
		system.Logger.Warn("Failed to initialize web UI handler: %s (web UI will be disabled)", err)
	} else {
		c.spaHandler = spaHandler
		system.Logger.Info("Web UI initialized")
	}

	// Setup HTTP handlers
	mux := http.NewServeMux()
	prefix := c.rootPath

	// Exploration endpoints
	mux.HandleFunc(prefix+"/api/commands", c.handleExplore)
	mux.HandleFunc(prefix+"/api/commands/", c.handleExplore)

	// OpenAPI specification
	mux.HandleFunc(prefix+"/api/openapi", c.handleOpenAPI)
	mux.HandleFunc(prefix+"/api/openapi.json", c.handleOpenAPI)

	// Health check
	mux.HandleFunc(prefix+"/api/health", c.handleHealth)

	// Job management endpoints
	mux.HandleFunc(prefix+"/api/jobs", c.handleJobsList)
	mux.HandleFunc(prefix+"/api/jobs/", c.handleJobsRoute)
	mux.HandleFunc(prefix+"/api/generate-cli", c.handleGenerateCLI)

	// Inventory endpoints (more specific routes first)
	mux.HandleFunc(prefix+"/api/inventory/schema", c.handleInventorySchema)
	mux.HandleFunc(prefix+"/api/inventory/action", c.handleInventoryAction)
	mux.HandleFunc(prefix+"/api/inventory/connect/", c.handleInventoryConnect)
	mux.HandleFunc(prefix+"/api/inventory/", c.handleInventoryData)

	// Terminal WebSocket endpoint
	mux.HandleFunc(prefix+"/api/terminal/ws", c.handleTerminalWS)

	// File browser endpoints
	mux.HandleFunc(prefix+"/api/fs/homedir", c.handleFSHomedir)
	mux.HandleFunc(prefix+"/api/fs/ls", c.handleFSLs)

	// Embedded AGI Monitor endpoints (optional)
	if c.AgiMonitorEnable {
		c.initEmbeddedAgiMonitor()
		monitorPrefix := prefix + "/api/agi-monitor"
		mux.HandleFunc(monitorPrefix+"/health", c.agiMonitorInstance.handleHealth)
		mux.HandleFunc(monitorPrefix+"/", c.agiMonitorInstance.handle)
		system.Logger.Info("AGI Monitor enabled at %s/", monitorPrefix)
	}

	// Execution and static file endpoints (catch-all)
	mux.HandleFunc(prefix+"/", c.handleRequest)

	// Create server with timeouts
	c.srv = &http.Server{
		Addr:         c.ListenAddr,
		Handler:      c.corsMiddleware(mux),
		ReadTimeout:  time.Duration(c.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(c.WriteTimeout) * time.Second,
	}

	// Start cleanup goroutine if enabled
	if c.historyExpiresDur > 0 && c.cleanupIntervalDur > 0 {
		system.Logger.Info("Starting cleanup loop (history-expires: %s, interval: %s)", c.HistoryExpires, c.CleanupInterval)
		go c.runCleanupLoop()
	}

	// Start scriptlog cleanup goroutine (always enabled, 24h interval, 30d retention)
	system.Logger.Info("Starting scriptlog cleanup loop (30-day retention, 24h interval)")
	go c.runScriptlogCleanupLoop()

	// Start AGI token cache cleanup goroutine
	c.agiTokens.init()
	go c.runAgiTokenCleanupLoop()

	// Start AGI status polling goroutine (fetches /agi/status every 5 minutes)
	c.agiStatus.init()
	go c.runAgiStatusLoop()

	// Start embedded AGI Monitor ban tracker cleanup (if enabled)
	if c.AgiMonitorEnable && c.agiMonitorInstance != nil {
		go c.runAgiMonitorBanCleanup()
	}

	// Start expiry-aware refresh scheduler
	c.expiryScheduler = &expiryRefreshScheduler{
		system: system,
		webui:  c,
	}
	// Perform initial schedule after a brief delay to let inventory populate
	go func() {
		time.Sleep(15 * time.Second)
		c.expiryScheduler.reschedule()
	}()

	// Setup graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop // first signal
		c.shuttingDown.Store(true)

		runningCount := c.jobManager.CountRunningJobs()
		if runningCount > 0 {
			system.Logger.Info("Shutting down, waiting for %d running job(s) to complete...", runningCount)
			system.Logger.Info("Press Ctrl+C again to force shutdown and kill all jobs")
		} else {
			system.Logger.Info("Shutting down server...")
		}

		close(c.shutdownChan)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		//nolint:errcheck
		c.srv.Shutdown(ctx)

		// Wait for second signal OR job completion
		done := make(chan struct{})
		go func() {
			c.jobWg.Wait()
			close(done)
		}()

		select {
		case <-stop:
			system.Logger.Info("Force shutdown requested, killing all running jobs...")
			c.jobManager.KillAllRunningJobs()
			os.Exit(1)
		case <-done:
			system.Logger.Info("All jobs completed, server stopped")
		}
	}()

	// Start server
	system.Logger.Info("Starting REST API server on %s (HTTPS: %t, Auth: %s)", c.ListenAddr, c.HTTPS, c.AuthType)
	if c.rootPath != "" {
		system.Logger.Info("Root path: %s", c.rootPath)
	}
	if c.maxJobRuntimeDur > 0 {
		system.Logger.Info("Max job runtime: %s", c.MaxJobRuntime)
	} else {
		system.Logger.Info("Max job runtime: unlimited")
	}

	// Build the URL for the browser
	if !c.NoBrowser {
		scheme := "http"
		if c.HTTPS {
			scheme = "https"
		}
		host := c.ListenAddr
		if strings.HasPrefix(host, ":") {
			host = "localhost" + host
		}
		browserURL := fmt.Sprintf("%s://%s%s", scheme, host, c.rootPath)
		go func() {
			dialAddr := c.ListenAddr
			if strings.HasPrefix(dialAddr, ":") {
				dialAddr = "localhost" + dialAddr
			}
			for range 50 {
				conn, err := net.DialTimeout("tcp", dialAddr, 100*time.Millisecond)
				if err == nil {
					conn.Close()
					system.Logger.Info("Opening browser at %s", browserURL)
					if err := openbrowser.Open(browserURL); err != nil {
						system.Logger.Warn("Failed to open browser: %s", err)
					}
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			system.Logger.Warn("Server did not become ready in time, skipping browser open")
		}()
	}

	if c.HTTPS {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			CurvePreferences: []tls.CurveID{
				tls.CurveP521, tls.CurveP384, tls.CurveP256,
			},
		}
		c.srv.TLSConfig = tlsConfig
		if err := c.srv.ListenAndServeTLS(c.CertFile, c.KeyFile); err != http.ErrServerClosed {
			return Error(err, system, cmd, c, args)
		}
	} else {
		if err := c.srv.ListenAndServe(); err != http.ErrServerClosed {
			return Error(err, system, cmd, c, args)
		}
	}

	system.Logger.Info("Server stopped")
	return Error(nil, system, cmd, c, args)
}

// reinitializeBackend re-reads the config file, reinitializes the backend, and rebuilds
// the command tree. Call this after config/backend completes to pick up the new backend.
func (c *WebUICmd) reinitializeBackend() error {
	if c.system == nil {
		return fmt.Errorf("system not initialized")
	}
	system := c.system
	system.Logger.Info("Reinitializing backend after config/backend")

	cfgFile, err := ConfigFileName()
	if err != nil {
		log.Printf("reinitializeBackend: failed to get config file path: %s", err)
		return fmt.Errorf("config file path: %w", err)
	}

	// Reset the backend config to zero values before re-parsing. The INI writer
	// uses IniCommentDefaults, so fields that match the struct-tag default (e.g.
	// Region="" for docker) are written as comments. The INI parser skips
	// commented-out lines and leaves existing field values untouched. Without
	// this reset, stale values from the previous backend (e.g. Region="us-east-1"
	// from AWS) would survive into the new backend and cause errors like
	// "region us-east-1 not found" when switching to docker.
	system.Opts.Config.Backend = ConfigBackendCmd{}

	// Re-parse the config file to pick up the new backend settings
	if err := system.IniParser.ParseFile(cfgFile); err != nil {
		log.Printf("reinitializeBackend: failed to parse config file: %s", err)
		return fmt.Errorf("parse config: %w", err)
	}

	// Skip backend init for "none" type
	backendType := system.Opts.Config.Backend.Type
	if backendType == "" || backendType == "none" {
		system.Logger.Info("Backend type is %q, skipping backend initialization", backendType)
		system.Backend = nil
		c.commandTree = BuildCommandTree(system.Opts)
		if backendType != "" {
			filterBackendParameters(c.commandTree, backendType)
		}
		if c.simpleModeConfig != nil {
			c.simpleModeConfig.ApplyToCommandTree(c.commandTree)
		}
		if c.ConfigFile != "" {
			if err := applyConfigOverrides(c.commandTree, c.ConfigFile, system); err != nil {
				log.Printf("reinitializeBackend: failed to apply config overrides: %s", err)
			}
		}
		return nil
	}

	// Set InitOptions.Backend so GetBackend uses the correct config
	pollEnabled := c.refreshIntervalDur > 0
	system.InitOptions.Backend = nil
	if pollEnabled {
		system.InitOptions.Backend = &InitBackend{
			PollInventoryHourly: true,
			PollInterval:        c.refreshIntervalDur,
			UseCache:            system.Opts.Config.Backend.InventoryCache,
			GCPAuthMethod:       clouds.GCPAuthMethod(system.Opts.Config.Backend.GCPAuthMethod),
			GCPBrowser:          !system.Opts.Config.Backend.GCPNoBrowser,
			GCPClientID:         system.Opts.Config.Backend.GCPClientID,
			GCPClientSecret:     system.Opts.Config.Backend.GCPClientSecret,
		}
	}
	if err := system.GetBackend(pollEnabled); err != nil {
		log.Printf("reinitializeBackend: failed to get backend: %s", err)
		return fmt.Errorf("get backend: %w", err)
	}

	// Rebuild command tree and apply filters (mirror Execute lines 145-172)
	c.commandTree = BuildCommandTree(system.Opts)
	filterBackendParameters(c.commandTree, backendType)
	system.Logger.Info("Filtered command parameters for backend: %s", backendType)

	if c.simpleModeConfig != nil {
		c.simpleModeConfig.ApplyToCommandTree(c.commandTree)
	}

	if c.ConfigFile != "" {
		if err := applyConfigOverrides(c.commandTree, c.ConfigFile, system); err != nil {
			log.Printf("reinitializeBackend: failed to apply config overrides: %s", err)
		}
	}

	system.Logger.Info("Backend reinitialized successfully")
	return nil
}

// loadTokens reads authentication tokens from the configured file
func (c *WebUICmd) loadTokens() error {
	data, err := os.ReadFile(c.TokenAuthPath)
	if err != nil {
		return err
	}
	c.tokensMutex.Lock()
	defer c.tokensMutex.Unlock()
	c.tokens = []string{}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			c.tokens = append(c.tokens, line)
		}
	}
	return nil
}

// allowServerBrowse checks whether the current request is allowed to browse
// the server's local filesystem. The logic mirrors the old webCmd.allowls:
//   - BlockServerLs disables browsing everywhere
//   - AllowLsEverywhere enables it for all origins
//   - Otherwise, only localhost requests without proxy headers are allowed
func (c *WebUICmd) allowServerBrowse(r *http.Request) bool {
	if c.BlockServerLs {
		return false
	}
	if c.AllowLsEverywhere {
		return true
	}
	// Check for proxy headers – their presence means the request was forwarded
	proxyHeaders := []string{"X-Real-Ip", "X-Forwarded-For", "X-Forwarded-Host"}
	for _, h := range proxyHeaders {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	// Extract host (without port)
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	// Strip brackets from IPv6
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// corsMiddleware adds CORS headers to responses
func (c *WebUICmd) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowedOrigin := c.getAllowedOrigin(origin)

		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Auth-Token")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Add Vary header when not using wildcard to ensure proper caching
		if allowedOrigin != "*" {
			w.Header().Set("Vary", "Origin")
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// getAllowedOrigin returns the appropriate CORS origin header value
func (c *WebUICmd) getAllowedOrigin(requestOrigin string) string {
	origins := c.CORSOrigins
	if origins == "" || origins == "*" {
		return "*"
	}

	// If only one origin is configured, return it directly
	if !strings.Contains(origins, ",") {
		return origins
	}

	// Multiple origins configured - check if request origin is allowed
	allowedOrigins := strings.Split(origins, ",")
	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == requestOrigin {
			return requestOrigin
		}
	}

	// Request origin not in allowed list - return first allowed origin
	// (browser will reject if it doesn't match)
	return strings.TrimSpace(allowedOrigins[0])
}

// checkAuth verifies authentication based on configured auth type
func (c *WebUICmd) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	if c.isBasicAuth {
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="AeroLab REST API"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return false
		}
		usermatch := subtle.ConstantTimeCompare([]byte(user), []byte(c.BasicAuthUser))
		passmatch := subtle.ConstantTimeCompare([]byte(pass), []byte(c.BasicAuthPass))
		if usermatch == 0 || passmatch == 0 {
			w.Header().Set("WWW-Authenticate", `Basic realm="AeroLab REST API"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return false
		}
	}
	if c.isTokenAuth {
		token := r.Header.Get("X-Auth-Token")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token == "" {
			http.Error(w, "Unauthorized: missing X-Auth-Token header", http.StatusUnauthorized)
			return false
		}
		c.tokensMutex.RLock()
		found := false
		for _, t := range c.tokens {
			if subtle.ConstantTimeCompare([]byte(token), []byte(t)) == 1 {
				found = true
				break
			}
		}
		c.tokensMutex.RUnlock()
		if !found {
			http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
			return false
		}
	}
	return true
}

// handleHealth returns server health status
func (c *WebUICmd) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	forceSimple := c.simpleModeConfig != nil && c.simpleModeConfig.ForceEnabled
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"status":            "ok",
		"version":           getVersion(),
		"forceSimpleMode":   forceSimple,
		"pageTitle":         c.PageTitle,
		"allowServerBrowse": c.allowServerBrowse(r),
	})
}

func getVersion() string {
	_, _, _, ver := GetAerolabVersion()
	return ver
}

// handleRequest routes requests to appropriate handlers
func (c *WebUICmd) handleRequest(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}

	// Strip root path prefix
	path := r.URL.Path
	if c.rootPath != "" {
		path = strings.TrimPrefix(path, c.rootPath)
	}
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	// Skip API paths - they're handled separately
	if strings.HasPrefix(path, "api/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case "GET":
		// Check if this looks like a static file request or web UI route
		if c.spaHandler != nil && c.isWebUIRequest(path) {
			c.serveWebUI(w, r, path)
			return
		}
		// GET on command path returns command info (API style)
		c.handleCommandInfo(w, r, path)
	case "PUT", "POST":
		// PUT/POST executes the command
		c.handleExecute(w, r, path)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// isWebUIRequest determines if a request should be served by the web UI
func (c *WebUICmd) isWebUIRequest(path string) bool {
	// Empty path or paths that look like web UI routes
	if path == "" || path == "index.html" {
		return true
	}
	// Static assets
	if strings.HasPrefix(path, "assets/") ||
		strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".css") ||
		strings.HasSuffix(path, ".ico") ||
		strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".svg") ||
		strings.HasSuffix(path, ".woff") ||
		strings.HasSuffix(path, ".woff2") ||
		strings.HasSuffix(path, ".ttf") {
		return true
	}
	// Web UI routes (commands, jobs, inventory)
	if strings.HasPrefix(path, "commands") || strings.HasPrefix(path, "jobs") || strings.HasPrefix(path, "inventory") {
		return true
	}
	return false
}

// serveWebUI serves the web UI, injecting configuration for index.html
func (c *WebUICmd) serveWebUI(w http.ResponseWriter, r *http.Request, path string) {
	// For index.html or SPA routes, inject config
	if path == "" || path == "index.html" || strings.HasPrefix(path, "commands") || strings.HasPrefix(path, "jobs") || strings.HasPrefix(path, "inventory") {
		c.serveIndexWithConfig(w, r)
		return
	}

	// Serve static assets directly
	// Adjust the request path for the SPA handler
	if c.rootPath != "" {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, c.rootPath)
	}
	c.spaHandler.ServeHTTP(w, r)
}

// serveIndexWithConfig serves index.html with injected configuration
func (c *WebUICmd) serveIndexWithConfig(w http.ResponseWriter, r *http.Request) {
	// Read index.html from embedded FS
	content, err := webui.ReadIndexHTML()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Inject runtime config before </head>
	forceSimple := c.simpleModeConfig != nil && c.simpleModeConfig.ForceEnabled
	config := fmt.Sprintf(`<script>window.__AEROLAB_CONFIG__={rootPath:"%s",version:"%s",forceSimpleMode:%t,pageTitle:"%s"}</script>`,
		c.rootPath, getVersion(), forceSimple, c.PageTitle)

	html := strings.Replace(string(content), "</head>", config+"</head>", 1)
	html = strings.Replace(html, "<title>AeroLab</title>", fmt.Sprintf("<title>%s</title>", c.PageTitle), 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(html)) //nolint:errcheck
}

// handleExplore handles GET /api/commands and GET /api/commands/{path}
func (c *WebUICmd) handleExplore(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Strip root path prefix first, then /api/commands
	urlPath := r.URL.Path
	if c.rootPath != "" {
		urlPath = strings.TrimPrefix(urlPath, c.rootPath)
	}
	path := strings.TrimPrefix(urlPath, "/api/commands")
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	var result any
	if path == "" {
		// Return full command tree
		result = c.commandTree
	} else {
		// Find specific command
		cmd := c.commandTree.FindByPath(path)
		if cmd == nil {
			http.Error(w, fmt.Sprintf("Command not found: %s", path), http.StatusNotFound)
			return
		}
		// Resolve dynamic choices (e.g. instance types, zones) before returning
		c.resolveDynamicChoices(cmd)
		result = cmd
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(result) //nolint:errcheck
}

// handleCommandInfo returns command information for a path
func (c *WebUICmd) handleCommandInfo(w http.ResponseWriter, r *http.Request, path string) {
	if path == "" {
		// Return root command info
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(c.commandTree) //nolint:errcheck
		return
	}

	cmd := c.commandTree.FindByPath(path)
	if cmd == nil {
		http.Error(w, fmt.Sprintf("Command not found: %s", path), http.StatusNotFound)
		return
	}

	// Resolve dynamic choices if system is available
	c.resolveDynamicChoices(cmd)

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(cmd) //nolint:errcheck
}

// handleExecute executes a command based on the path and JSON body
// If the dryRun query parameter is set to "true", returns the CLI command without executing.
func (c *WebUICmd) handleExecute(w http.ResponseWriter, r *http.Request, path string) {
	if path == "" {
		http.Error(w, "Command path required", http.StatusBadRequest)
		return
	}

	// Enforce simple mode restrictions server-side
	if c.simpleModeConfig != nil && c.simpleModeConfig.ForceEnabled {
		dotPath := SimpleModePathFromSlash(path)
		if err := c.simpleModeConfig.CheckCommandAllowed(dotPath); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}

	// Find command info
	cmdInfo := c.commandTree.FindByPath(path)
	if cmdInfo == nil {
		http.Error(w, fmt.Sprintf("Command not found: %s", path), http.StatusNotFound)
		return
	}

	if cmdInfo.HasChildren && len(cmdInfo.Parameters) == 0 {
		http.Error(w, fmt.Sprintf("Cannot execute command group '%s', specify a subcommand", path), http.StatusBadRequest)
		return
	}

	// Check for dry-run mode
	if r.URL.Query().Get("dryRun") == "true" {
		c.handleDryRun(w, r, path)
		return
	}

	// Check for file upload/download special handling
	hasUpload := false
	hasDownload := false
	for _, p := range cmdInfo.Parameters {
		if p.WebType == "upload" {
			hasUpload = true
		}
		if p.WebType == "download" {
			hasDownload = true
		}
	}

	if hasDownload && r.Method == "GET" {
		c.handleFileDownload(w, r, path, cmdInfo)
		return
	}

	if hasUpload {
		c.handleFileUpload(w, r, path, cmdInfo)
		return
	}

	// Execute command with JSON body
	c.executeCommand(w, r, path, cmdInfo)
}

// handleDryRun returns the CLI command without executing it
func (c *WebUICmd) handleDryRun(w http.ResponseWriter, r *http.Request, path string) {
	preferShort := r.URL.Query().Get("preferShort") == "true"

	// Parse JSON body into map
	var params map[string]any
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %s", err), http.StatusBadRequest)
			return
		}
	}

	// Try reflection-based CLI generation first (more accurate with defaults)
	cliCmd, err := c.generateCLIWithReflection(path, params, preferShort, false)
	if err != nil {
		// Fall back to simple map-based generation
		log.Printf("Warning: reflection-based CLI generation failed for %s: %s; falling back to map-based generation", path, err)
		cliCmd = generateCLICommand(path, params)
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{ //nolint:errcheck
		"dryRun":      true,
		"commandPath": path,
		"cli":         cliCmd,
		"parameters":  params,
	})
}

// executeCommand submits a command as an async job and returns the job ID
func (c *WebUICmd) executeCommand(w http.ResponseWriter, r *http.Request, path string, cmdInfo *CommandInfo) {
	if c.shuttingDown.Load() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Server is shutting down, no new commands accepted"}) //nolint:errcheck
		return
	}

	// Parse request body - support both JSON and multipart/form-data
	var params map[string]any
	var tempDir string
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Multipart form: uploaded files + JSON params blob
		if c.MaxUploadSizeBytes > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, int64(c.MaxUploadSizeBytes)+1<<20)
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse multipart form: %s", err), http.StatusBadRequest)
			return
		}

		// Extract params from _params JSON field
		if paramsJSON := r.FormValue("_params"); paramsJSON != "" {
			if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
				http.Error(w, fmt.Sprintf("Invalid _params JSON: %s", err), http.StatusBadRequest)
				return
			}
		} else {
			params = make(map[string]any)
		}

		// Process uploaded files
		if r.MultipartForm != nil && r.MultipartForm.File != nil {
			// Create temp directory for this request
			requestID := fmt.Sprintf("%d", time.Now().UnixNano())
			tempDir = filepath.Join(c.UploadTempDir, requestID)
			if err := os.MkdirAll(tempDir, 0755); err != nil {
				http.Error(w, fmt.Sprintf("Failed to create temp directory: %s", err), http.StatusInternalServerError)
				return
			}

			for fieldName, fileHeaders := range r.MultipartForm.File {
				if len(fileHeaders) == 0 {
					continue
				}
				fh := fileHeaders[0]
				src, err := fh.Open()
				if err != nil {
					os.RemoveAll(tempDir)
					http.Error(w, fmt.Sprintf("Failed to open uploaded file %s: %s", fieldName, err), http.StatusInternalServerError)
					return
				}

				// Save to temp file using original filename
				dstPath := filepath.Join(tempDir, fh.Filename)
				dst, err := os.Create(dstPath)
				if err != nil {
					src.Close()
					os.RemoveAll(tempDir)
					http.Error(w, fmt.Sprintf("Failed to create temp file: %s", err), http.StatusInternalServerError)
					return
				}
				_, err = io.Copy(dst, src)
				src.Close()
				dst.Close()
				if err != nil {
					os.RemoveAll(tempDir)
					http.Error(w, fmt.Sprintf("Failed to save uploaded file: %s", err), http.StatusInternalServerError)
					return
				}

				// Replace the param value with the temp file path
				params[fieldName] = dstPath
			}
		}
	} else if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %s", err), http.StatusBadRequest)
			return
		}
	}

	// Get user from request
	user := c.getUserFromRequest(r)

	// Generate a clean CLI command that omits default values
	cliCmd, cliErr := c.generateCLIWithReflection(path, params, false, false)
	if cliErr != nil {
		cliCmd = "" // fall back to map-based generation inside CreateJob
	}

	// Create job
	job, err := c.jobManager.CreateJob(user, path, params, cmdInfo.InvWebForce, cliCmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create job: %s", err), http.StatusInternalServerError)
		return
	}

	// Store temp directory path for cleanup after job completes
	if tempDir != "" {
		job.TempDir = tempDir
	}

	// Check queue limit
	if c.MaxQueuedJob > 0 {
		pending := int(c.pendingJobs.Load())
		if pending >= c.MaxConcurrentJob+c.MaxQueuedJob {
			// Too many jobs queued
			_ = c.jobManager.UpdateJobStatus(job.ID, JobStatusFailed, "Too many jobs queued. Please wait for some to complete.")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"error": "Too many jobs queued",
			"jobId": job.ID,
		})
			return
		}
	}

	// Track pending jobs
	c.pendingJobs.Add(1)

	// Start async execution with concurrency limiting
	go func() {
		defer c.pendingJobs.Add(-1)
		// Acquire semaphore (blocks if at max concurrent)
		if c.jobSemaphore != nil {
			c.jobSemaphore <- struct{}{}
			defer func() { <-c.jobSemaphore }()
		}
		c.executeJobAsync(job)
	}()

	// Build base URL for response
	scheme := "http"
	if c.HTTPS {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	// Return job submission response
	response := &JobSubmitResponse{
		JobID:         job.ID,
		User:          job.User,
		CommandPath:   job.CommandPath,
		CLICommand:    job.CLICommand,
		Status:        job.Status,
		CreatedAt:     job.CreatedAt,
		StatusURL:     fmt.Sprintf("%s/api/jobs/%s", baseURL, job.ID),
		LogsURL:       fmt.Sprintf("%s/api/jobs/%s/logs", baseURL, job.ID),
		LogsStreamURL: fmt.Sprintf("%s/api/jobs/%s/logs/stream", baseURL, job.ID),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 Accepted for async operations
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(response) //nolint:errcheck
}

// executeJobAsync runs the command as a subprocess for complete log capture and killability
func (c *WebUICmd) executeJobAsync(job *Job) {
	c.jobWg.Add(1)
	defer c.jobWg.Done()

	// Clean up temp directory (uploaded files) when job finishes
	if job.TempDir != "" {
		defer os.RemoveAll(job.TempDir)
	}

	// Update status to running
	if err := c.jobManager.UpdateJobStatus(job.ID, JobStatusRunning, ""); err != nil {
		log.Printf("Failed to update job status: %s", err)
	}

	// Prepare input JSON for subprocess
	input := webUIExecInput{
		CommandPath: job.CommandPath,
		Parameters:  job.Parameters,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		//nolint:errcheck
		c.jobManager.UpdateJobStatus(job.ID, JobStatusError, fmt.Sprintf("Failed to marshal input: %s", err))
		return
	}

	// Snapshot the parent's in-memory inventory to a temp file so the subprocess
	// can load it via ExistingInventory instead of hitting cloud APIs on startup.
	var invFile string
	if c.system != nil && c.system.Backend != nil {
		if inv := c.system.Backend.GetInventory(); inv != nil {
			invJSON, jerr := json.Marshal(inv)
			if jerr == nil {
				f, ferr := os.CreateTemp("", "aerolab-inv-*.json")
				if ferr == nil {
					_, werr := f.Write(invJSON)
					f.Close()
					if werr == nil {
						invFile = f.Name()
					} else {
						os.Remove(f.Name())
						log.Printf("Job %s: failed to write inventory temp file: %s", job.ID, werr)
					}
				} else {
					log.Printf("Job %s: failed to create inventory temp file: %s", job.ID, ferr)
				}
			} else {
				log.Printf("Job %s: failed to marshal inventory: %s", job.ID, jerr)
			}
		}
	}
	if invFile != "" {
		defer os.Remove(invFile)
	}

	// Create context with timeout (if configured)
	var ctx context.Context
	var cancel context.CancelFunc
	if c.maxJobRuntimeDur > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), c.maxJobRuntimeDur)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	// Create subprocess command using the current binary
	cmd := exec.CommandContext(ctx, os.Args[0], "webui", "exec")
	cmd.Env = append(os.Environ(), "AEROLAB_NONINTERACTIVE=1")
	if invFile != "" {
		cmd.Env = append(cmd.Env, "AEROLAB_INVENTORY_FILE="+invFile)
	}

	// Setup stdin pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		//nolint:errcheck
		c.jobManager.UpdateJobStatus(job.ID, JobStatusError, fmt.Sprintf("Failed to create stdin pipe: %s", err))
		return
	}

	// Open log file for capturing output
	logFile, err := c.jobManager.OpenLogFile(job)
	if err != nil {
		//nolint:errcheck
		c.jobManager.UpdateJobStatus(job.ID, JobStatusError, fmt.Sprintf("Failed to open log file: %s", err))
		return
	}
	defer logFile.Close()

	// Redirect stdout and stderr to log file
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Snapshot the config timestamp file BEFORE the subprocess starts so we
	// can detect whether config/backend actually wrote a new config.
	var configTsBefore string
	if job.CommandPath == "config/backend" {
		if cfgFile, err := ConfigFileName(); err == nil {
			if data, err := os.ReadFile(cfgFile + ".ts"); err == nil {
				configTsBefore = string(data)
			}
		}
	}

	// Start the subprocess
	if err := cmd.Start(); err != nil {
		c.jobManager.UpdateJobStatus(job.ID, JobStatusError, fmt.Sprintf("Failed to start subprocess: %s", err))
		return
	}

	// Save PID for cancellation support
	job.PID = cmd.Process.Pid
	if err := c.jobManager.SaveJob(job); err != nil {
		log.Printf("Failed to save job PID: %s", err)
	}

	// Write input JSON to stdin and close
	_, err = stdin.Write(inputJSON)
	if err != nil {
		log.Printf("Failed to write to stdin: %s", err)
	}
	stdin.Close()

	// Wait for subprocess to complete
	err = cmd.Wait()

	// Determine exit code
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	job.ExitCode = &exitCode

	// Reload job from disk to pick up any changes (e.g., Cancelled flag set by handleCancelJob)
	if reloadedJob, reloadErr := c.jobManager.GetJob(job.ID); reloadErr == nil {
		job.Cancelled = reloadedJob.Cancelled
	}

	// Refresh the server's in-memory inventory BEFORE setting final job status.
	// Commands run as subprocesses which have their own backend instance, so the
	// parent web UI server's inventory becomes stale after create/destroy/start/stop
	// operations. The refresh must happen before the status update because the
	// frontend receives the job completion event via SSE and immediately refetches
	// inventory — if we refresh after setting status, the frontend races and gets
	// stale data.
	if job.RefreshInventory && c.system != nil && c.system.Backend != nil {
		log.Printf("Job %s (%s): refreshing server inventory", job.ID, job.CommandPath)
		const maxRefreshRetries = 3
		var refreshErr error
		for attempt := 1; attempt <= maxRefreshRetries; attempt++ {
			var didRefresh bool
			didRefresh, refreshErr = c.forceRefreshInventoryIfAllowed()
			if refreshErr == nil {
				if didRefresh {
					log.Printf("Job %s (%s): inventory refreshed successfully", job.ID, job.CommandPath)
				}
				break
			}
			if attempt < maxRefreshRetries {
				log.Printf("Job %s (%s): WARNING failed to refresh inventory (attempt %d/%d): %s; retrying in %ds...", job.ID, job.CommandPath, attempt, maxRefreshRetries, refreshErr, attempt)
				time.Sleep(time.Duration(attempt) * time.Second)
			} else {
				log.Printf("Job %s (%s): WARNING failed to refresh inventory after %d attempts: %s", job.ID, job.CommandPath, maxRefreshRetries, refreshErr)
			}
		}
		// Reschedule expiry timer after inventory refresh
		if c.expiryScheduler != nil {
			c.expiryScheduler.reschedule()
		}
	}

	// Determine final status based on how the process ended
	if ctx.Err() == context.DeadlineExceeded {
		job.TimedOut = true
		//nolint:errcheck
		c.jobManager.UpdateJobStatusWithMeta(job.ID, JobStatusFailed, "Job timed out after "+c.MaxJobRuntime, job)
	} else if job.Cancelled {
		// Job was cancelled by user via handleCancelJob
		//nolint:errcheck
		c.jobManager.UpdateJobStatusWithMeta(job.ID, JobStatusFailed, "Cancelled by user", job)
	} else if ctx.Err() == context.Canceled {
		// Context was cancelled (likely due to server shutdown)
		//nolint:errcheck
		c.jobManager.UpdateJobStatusWithMeta(job.ID, JobStatusFailed, "Job was cancelled", job)
	} else if err != nil {
		c.jobManager.UpdateJobStatusWithMeta(job.ID, JobStatusFailed, err.Error(), job)
	} else {
		// config/backend requires frontend reload to pick up new command tree, inventory, etc.
		// Only reload when the config was actually changed (ExecTypeSet writes the config file).
		// When the user just views the current config (no --type flag), the file is not written
		// and we should skip the reload.
		if job.CommandPath == "config/backend" {
			configChanged := false
			if cfgFile, err := ConfigFileName(); err == nil {
				if data, err := os.ReadFile(cfgFile + ".ts"); err == nil {
					configChanged = string(data) != configTsBefore
				} else if configTsBefore != "" {
					// .ts file disappeared — treat as changed
					configChanged = true
				}
			}
			if configChanged {
				job.ReloadRequired = true
				if err := c.reinitializeBackend(); err != nil {
					log.Printf("Job %s (%s): backend reinit failed: %s", job.ID, job.CommandPath, err)
					// Still mark job completed - the config was written by the subprocess
				}
			}
		}
		c.jobManager.UpdateJobStatusWithMeta(job.ID, JobStatusCompleted, "", job)
	}
}

// resolveDynamicChoices resolves webchoice:"method::Name" for parameters
func (c *WebUICmd) resolveDynamicChoices(cmd *CommandInfo) {
	for i := range cmd.Parameters {
		p := &cmd.Parameters[i]
		if p.ChoicesMethod != "" {
			// Try to resolve dynamic choices, passing namespace and fieldName
			// so ResolveDynamicChoices can search into nested group structs
			choices, labels, err := ResolveDynamicChoices(c.system, cmd.Path, p.Name, p.ChoicesMethod, p.Namespace, p.FieldName)
			if err != nil {
				log.Printf("Warning: failed to resolve dynamic choices for %s.%s: %s", cmd.Path, p.Name, err)
			} else {
				p.Choices = choices
				p.ChoiceLabels = labels
			}
		}
	}
}

// ExecuteResponse is the JSON response for command execution
type ExecuteResponse struct {
	Success          bool     `json:"success"`
	Path             string   `json:"path"`
	Error            string   `json:"error,omitempty"`
	Result           any      `json:"result,omitempty"`
	Logs             []string `json:"logs,omitempty"`
	RefreshInventory bool     `json:"refreshInventory,omitempty"`
}

// ExecuteResult holds the result of command execution
type ExecuteResult struct {
	Result any
	Logs   []string
}

// getInventory returns the current inventory
func (c *WebUICmd) getInventory() *backends.Inventory {
	if c.system != nil && c.system.Backend != nil {
		return c.system.Backend.GetInventory()
	}
	return nil
}

// forceRefreshInventoryIfAllowed calls ForceRefreshInventory only if at least
// minimumIntervalDur has elapsed since the last refresh. Returns (false, nil)
// when skipped (too soon), (true, nil) on success, (false, err) on error.
func (c *WebUICmd) forceRefreshInventoryIfAllowed() (didRefresh bool, err error) {
	c.lastInventoryRefreshMu.Lock()
	defer c.lastInventoryRefreshMu.Unlock()
	if c.minimumIntervalDur > 0 && !c.lastInventoryRefreshTime.IsZero() {
		if time.Since(c.lastInventoryRefreshTime) < c.minimumIntervalDur {
			return false, nil // skip, too soon
		}
	}
	if c.system == nil || c.system.Backend == nil {
		return false, nil
	}
	err = c.system.Backend.ForceRefreshInventory()
	if err != nil {
		return false, err
	}
	c.lastInventoryRefreshTime = time.Now()
	return true, nil
}

// getUserFromRequest extracts the user from request based on configuration
func (c *WebUICmd) getUserFromRequest(r *http.Request) string {
	// Check custom header first if configured
	if c.UserHeader != "" {
		if user := r.Header.Get(c.UserHeader); user != "" {
			return sanitizePathComponent(user)
		}
	}

	// Fall back to basic auth username if using basic auth
	if c.isBasicAuth {
		if user, _, ok := r.BasicAuth(); ok && user != "" {
			return sanitizePathComponent(user)
		}
	}

	// Fall back to system owner (standard aerolab user discovery)
	return GetCurrentOwnerUser()
}

// handleJobsList handles GET /api/jobs - lists jobs with optional filters
func (c *WebUICmd) handleJobsList(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := c.getUserFromRequest(r)
	statusFilter := r.URL.Query().Get("status")
	allUsers := r.URL.Query().Get("all") == "true"

	jobs, err := c.jobManager.ListJobs(user, statusFilter, allUsers, c.ShowMaxHistory)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list jobs: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(&JobListResponse{ //nolint:errcheck
		Jobs:  jobs,
		Count: len(jobs),
	})
}

// handleJobsRoute routes /api/jobs/{jobId}/* requests
func (c *WebUICmd) handleJobsRoute(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}

	// Strip root path prefix first, then /api/jobs/
	urlPath := r.URL.Path
	if c.rootPath != "" {
		urlPath = strings.TrimPrefix(urlPath, c.rootPath)
	}
	path := strings.TrimPrefix(urlPath, "/api/jobs/")
	path = strings.TrimSuffix(path, "/")

	if path == "" {
		c.handleJobsList(w, r)
		return
	}

	parts := strings.SplitN(path, "/", 2)
	jobID := parts[0]

	if len(parts) == 1 {
		// GET /api/jobs/{jobId} - get job details
		c.handleJobDetails(w, r, jobID)
		return
	}

	subPath := parts[1]
	switch subPath {
	case "logs":
		c.handleJobLogs(w, r, jobID)
	case "logs/stream":
		c.handleJobLogsStream(w, r, jobID)
	default:
		http.NotFound(w, r)
	}
}

// handleJobDetails handles GET /api/jobs/{jobId} - get job details, DELETE - cancel job
func (c *WebUICmd) handleJobDetails(w http.ResponseWriter, r *http.Request, jobID string) {
	switch r.Method {
	case "GET":
		user := c.getUserFromRequest(r)
		allUsers := r.URL.Query().Get("all") == "true"

		job, err := c.jobManager.GetJobForUser(jobID, user, allUsers)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(job) //nolint:errcheck

	case "DELETE":
		c.handleCancelJob(w, r, jobID)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleJobLogs handles GET /api/jobs/{jobId}/logs - get job logs (one-shot)
func (c *WebUICmd) handleJobLogs(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := c.getUserFromRequest(r)
	allUsers := r.URL.Query().Get("all") == "true"

	job, err := c.jobManager.GetJobForUser(jobID, user, allUsers)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	logs, err := c.jobManager.ReadLogs(job)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read logs: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{ //nolint:errcheck
		"jobId":  jobID,
		"status": job.Status,
		"logs":   logs,
	})
}

// handleJobLogsStream handles GET /api/jobs/{jobId}/logs/stream - stream logs via SSE
func (c *WebUICmd) handleJobLogsStream(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := c.getUserFromRequest(r)
	allUsers := r.URL.Query().Get("all") == "true"

	job, err := c.jobManager.GetJobForUser(jobID, user, allUsers)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	var offset int64 = 0
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Send initial status
	fmt.Fprintf(w, "event: status\ndata: %s\n\n", job.Status) //nolint:errcheck
	flusher.Flush()

	// Helper to send any remaining logs
	sendRemainingLogs := func() {
		content, _, err := c.jobManager.ReadLogsFromOffset(job, offset)
		if err == nil && content != "" {
			// Each line becomes a "data:" field; per the SSE spec, multiple
			// data fields within one event are joined with \n, so preserving
			// the trailing empty element from Split keeps the trailing newline
			// intact and prevents the last line of one chunk from being
			// concatenated with the first line of the next chunk.
			// \r is stripped because the SSE parser treats it as a line
			// terminator, which would corrupt the data field.
			for line := range strings.SplitSeq(content, "\n") {
				fmt.Fprintf(w, "data: %s\n", strings.TrimRight(line, "\r")) //nolint:errcheck
			}
			fmt.Fprintf(w, "\n") //nolint:errcheck
			flusher.Flush()
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Read new log content
			content, newOffset, err := c.jobManager.ReadLogsFromOffset(job, offset)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error()) //nolint:errcheck
				flusher.Flush()
				return
			}

			if content != "" {
				// Send log content as SSE event.
				// Preserve all elements from Split (including trailing empty
				// string) so the SSE event data retains trailing \n.  Strip
				// \r to avoid SSE parser treating it as a line terminator.
				for line := range strings.SplitSeq(content, "\n") {
					fmt.Fprintf(w, "data: %s\n", strings.TrimRight(line, "\r")) //nolint:errcheck
				}
				fmt.Fprintf(w, "\n") //nolint:errcheck
				flusher.Flush()
				offset = newOffset
			}

			// Refresh job status
			job, err = c.jobManager.GetJob(jobID)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: job not found\n\n") //nolint:errcheck
				flusher.Flush()
				return
			}

			// Check if job is complete
			if job.Status == JobStatusCompleted || job.Status == JobStatusFailed || job.Status == JobStatusError {
				// Read any final logs that may have been written between last read and status change
				sendRemainingLogs()
				// Send final status (include reloadRequired for config/backend)
				completePayload := map[string]any{
					"status": job.Status,
					"error":  job.Error,
				}
				if job.ReloadRequired {
					completePayload["reloadRequired"] = true
				}
				completeJSON, _ := json.Marshal(completePayload)
				fmt.Fprintf(w, "event: complete\ndata: %s\n\n", completeJSON) //nolint:errcheck
				flusher.Flush()
				return
			}
		}
	}
}

// handleGenerateCLI handles POST /api/generate-cli - generates CLI command without executing
func (c *WebUICmd) handleGenerateCLI(w http.ResponseWriter, r *http.Request) {
	if !c.checkAuth(w, r) {
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CommandPath     string         `json:"commandPath"`
		Parameters      map[string]any `json:"parameters"`
		PreferShort     bool           `json:"preferShort"`     // Use short flags (-n) instead of long (--name)
		IncludeDefaults bool           `json:"includeDefaults"` // Include flags even when they match defaults
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %s", err), http.StatusBadRequest)
		return
	}

	if req.CommandPath == "" {
		http.Error(w, "commandPath is required", http.StatusBadRequest)
		return
	}

	// Verify command exists
	cmdInfo := c.commandTree.FindByPath(req.CommandPath)
	if cmdInfo == nil {
		http.Error(w, fmt.Sprintf("Command not found: %s", req.CommandPath), http.StatusNotFound)
		return
	}

	// Try reflection-based CLI generation first (more accurate with defaults)
	cliCmd, err := c.generateCLIWithReflection(req.CommandPath, req.Parameters, req.PreferShort, req.IncludeDefaults)
	if err != nil {
		// Fall back to simple map-based generation
		log.Printf("Warning: reflection-based CLI generation failed for %s: %s; falling back to map-based generation", req.CommandPath, err)
		cliCmd = generateCLICommand(req.CommandPath, req.Parameters)
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]string{ //nolint:errcheck
		"cli": cliCmd,
	})
}

// generateCLIWithReflection generates a CLI command using reflection to properly handle
// struct defaults, nested groups, and proper shell escaping.
func (c *WebUICmd) generateCLIWithReflection(cmdPath string, params map[string]any, preferShort bool, includeDefaults bool) (string, error) {
	// Find the command struct type by path
	cmdVal, err := getCommandValueByPath(c.system.Opts, cmdPath)
	if err != nil {
		return "", err
	}

	// Create a new instance seeded with struct tag defaults (NOT saved config).
	// This ensures fields the user didn't touch match their declared defaults
	// and are excluded from the generated CLI. The old approach copied saved
	// config values, making the CLI show flags the user never set.
	cmdType := cmdVal.Type()
	newCmd := reflect.New(cmdType).Elem()
	applyTagDefaults(newCmd)

	// Apply parameters from the request
	if params != nil {
		if err := applyParameters(newCmd, params); err != nil {
			return "", fmt.Errorf("failed to apply parameters: %w", err)
		}
	}

	// Determine active backend type for filtering non-active backend parameters
	activeBackend := ""
	if c.system != nil && c.system.Opts != nil && c.system.Opts.Config.Backend.Type != "" {
		activeBackend = c.system.Opts.Config.Backend.Type
	}

	// Use ReconstructCommandLineForBackend to generate the CLI command,
	// filtering out parameters belonging to non-active backends.
	cmdParts := strings.Split(cmdPath, "/")
	return ReconstructCommandLineForBackend(cmdParts, newCmd.Addr().Interface(), preferShort, includeDefaults, activeBackend), nil
}

// runCleanupLoop periodically cleans up old completed/failed jobs
func (c *WebUICmd) runCleanupLoop() {
	if c.historyExpiresDur == 0 {
		return // Cleanup disabled
	}

	ticker := time.NewTicker(c.cleanupIntervalDur)
	defer ticker.Stop()

	for {
		select {
		case <-c.shutdownChan:
			return
		case <-ticker.C:
			count, err := c.jobManager.CleanupOldJobs(c.historyExpiresDur)
			if err != nil {
				log.Printf("Warning: cleanup failed: %s", err)
			} else if count > 0 {
				log.Printf("Cleaned up %d old jobs", count)
			}
		}
	}
}

// runScriptlogCleanupLoop periodically cleans up old scriptlog entries (30 days)
func (c *WebUICmd) runScriptlogCleanupLoop() {
	// Immediate cleanup on startup
	if err := scriptlog.CleanupOldFailures(30 * 24 * time.Hour); err != nil {
		log.Printf("Warning: initial scriptlog cleanup failed: %s", err)
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-c.shutdownChan:
			return
		case <-ticker.C:
			if err := scriptlog.CleanupOldFailures(30 * 24 * time.Hour); err != nil {
				log.Printf("Warning: scriptlog cleanup failed: %s", err)
			}
		}
	}
}

// handleCancelJob handles DELETE /api/jobs/{jobId} - cancel a running job
func (c *WebUICmd) handleCancelJob(w http.ResponseWriter, r *http.Request, jobID string) {
	user := c.getUserFromRequest(r)
	allUsers := r.URL.Query().Get("all") == "true"
	force := r.URL.Query().Get("force") == "true"

	job, err := c.jobManager.GetJobForUser(jobID, user, allUsers)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Check if job is running
	if job.Status != JobStatusRunning {
		http.Error(w, "Job is not running", http.StatusBadRequest)
		return
	}

	// Check if we have a PID to kill
	if job.PID == 0 {
		http.Error(w, "Job has no PID (may be using legacy in-process execution)", http.StatusBadRequest)
		return
	}

	// Find the process
	process, err := os.FindProcess(job.PID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Process not found: %s", err), http.StatusNotFound)
		return
	}

	// Send signal
	var sig syscall.Signal
	if force {
		sig = syscall.SIGKILL
	} else {
		sig = syscall.SIGTERM
	}

	if err := process.Signal(sig); err != nil {
		// Check if process already exited
		if err == os.ErrProcessDone {
			http.Error(w, "Process already completed", http.StatusGone)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to send signal: %s", err), http.StatusInternalServerError)
		return
	}

	// Mark job as cancelled - only set the flag, let executeJobAsync handle the final status update
	// This avoids a race condition where both this handler and executeJobAsync try to update status
	job.Cancelled = true
	if err := c.jobManager.SaveJob(job); err != nil {
		log.Printf("Warning: failed to save cancelled flag for job %s: %s", jobID, err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"status":  "cancelling",
		"signal":  sig.String(),
		"force":   force,
		"jobId":   jobID,
		"message": fmt.Sprintf("Sent %s to process %d", sig.String(), job.PID),
	})
}

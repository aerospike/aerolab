package cmd

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/agi"
	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"github.com/aerospike/aerolab/pkg/agi/notifier"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/shutdown"
	"github.com/bestmethod/inslice"
	"github.com/lithammer/shortuuid"
	"golang.org/x/crypto/acme/autocert"
	"gopkg.in/yaml.v3"
)

// agiMonitorNotify represents a notification payload sent by the monitor.
type agiMonitorNotify struct {
	Name   string
	Action string
	Stage  string
	Error  *string
	Disk   *agiMonitorNotifyDisk
	RAM    *agiMonitorNotifyRAM
	Event  *ingest.NotifyEvent
}

// agiMonitorNotifyDisk contains disk sizing information.
type agiMonitorNotifyDisk struct {
	InitialSizeGB int
	FinalSizeGB   int
}

// agiMonitorNotifyRAM contains RAM/instance sizing information.
type agiMonitorNotifyRAM struct {
	InitialInstanceType string
	FinalInstanceType   string
	DisableDIM          bool
}

// inventoryCache holds cached inventory data for the monitor.
type inventoryCache struct {
	inventory *backends.Inventory
	expiry    time.Time
}

// ipBanTracker tracks failed auth attempts and banned IPs
type ipBanTracker struct {
	sync.RWMutex
	failures map[string][]time.Time // IP -> list of failure timestamps
	bans     map[string]time.Time   // IP -> ban expiry time
}

// newIPBanTracker creates a new IP ban tracker
func newIPBanTracker() *ipBanTracker {
	return &ipBanTracker{
		failures: make(map[string][]time.Time),
		bans:     make(map[string]time.Time),
	}
}

// isBanned checks if an IP is currently banned, and resets the ban timer if so
func (t *ipBanTracker) isBanned(ip string) bool {
	t.Lock()
	defer t.Unlock()
	if expiry, ok := t.bans[ip]; ok {
		if time.Now().Before(expiry) {
			// Reset ban timer on each connection attempt
			t.bans[ip] = time.Now().Add(1 * time.Hour)
			return true
		}
		// Ban expired, remove it
		delete(t.bans, ip)
	}
	return false
}

// recordFailure records a failed auth attempt and returns true if the IP should be banned
func (t *ipBanTracker) recordFailure(ip string) bool {
	t.Lock()
	defer t.Unlock()

	now := time.Now()
	cutoff := now.Add(-5 * time.Minute)

	// Clean old failures for this IP
	var recentFailures []time.Time
	for _, ts := range t.failures[ip] {
		if ts.After(cutoff) {
			recentFailures = append(recentFailures, ts)
		}
	}
	recentFailures = append(recentFailures, now)
	t.failures[ip] = recentFailures

	// Check if we should ban
	if len(recentFailures) >= 5 {
		return true
	}
	return false
}

// ban bans an IP for 1 hour
func (t *ipBanTracker) ban(ip string) {
	t.Lock()
	defer t.Unlock()
	t.bans[ip] = time.Now().Add(1 * time.Hour)
	delete(t.failures, ip) // Clear failures since we're banning
}

// cleanup removes expired bans and old failure records
func (t *ipBanTracker) cleanup() {
	t.Lock()
	defer t.Unlock()

	now := time.Now()
	cutoff := now.Add(-5 * time.Minute)

	// Remove expired bans
	for ip, expiry := range t.bans {
		if now.After(expiry) {
			delete(t.bans, ip)
		}
	}

	// Remove old failures
	for ip, failures := range t.failures {
		var recent []time.Time
		for _, ts := range failures {
			if ts.After(cutoff) {
				recent = append(recent, ts)
			}
		}
		if len(recent) == 0 {
			delete(t.failures, ip)
		} else {
			t.failures[ip] = recent
		}
	}
}

// Execute implements the command execution for agi monitor listen.
//
// The monitor listener receives events from AGI instances and handles:
//   - Spot instance capacity rotation to on-demand
//   - RAM/instance sizing decisions
//   - Disk sizing (GCP only)
//   - Notification forwarding to Slack and custom endpoints
//
// Authentication is performed via challenge-response:
//  1. AGI instance sends notification with secret
//  2. Monitor calls back to AGI /agi/monitor-challenge endpoint
//  3. Validates secret matches before processing event
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiMonitorListenCmd) Execute(args []string) error {
	cmd := []string{"agi", "monitor", "listen"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	// Initialize locks if not set
	if c.invLock == nil {
		c.invLock = new(sync.Mutex)
	}
	if c.execLock == nil {
		c.execLock = new(sync.Mutex)
	}

	// Log warning if DisablePricingAPI is enabled since it's not supported in new backend
	if c.DisablePricingAPI {
		system.Logger.Warn("DisablePricingAPI flag is set but not supported in the current backend implementation; pricing API will remain enabled")
	}

	// Create working directory
	err = os.MkdirAll("/var/lib/agimonitor", 0755)
	if err != nil {
		return Error(fmt.Errorf("failed to create working directory: %w", err), system, cmd, c, args)
	}
	err = os.Chdir("/var/lib/agimonitor")
	if err != nil {
		return Error(fmt.Errorf("failed to change to working directory: %w", err), system, cmd, c, args)
	}

	// Load configuration from file if exists
	if _, err := os.Stat("/etc/agimonitor.yaml"); err == nil {
		data, err := os.ReadFile("/etc/agimonitor.yaml")
		if err != nil {
			return Error(fmt.Errorf("failed to read config file: %w", err), system, cmd, c, args)
		}
		err = yaml.Unmarshal(data, c)
		if err != nil {
			return Error(fmt.Errorf("failed to parse config file: %w", err), system, cmd, c, args)
		}
	}

	// Log configuration
	system.Logger.Info("Configuration:")
	configYaml, _ := yaml.Marshal(c)
	system.Logger.Info("%s", string(configYaml))

	// Validate autocert configuration
	if len(c.AutoCertDomains) > 0 && c.AutoCertEmail == "" {
		return Error(errors.New("if autocert domains is in use, a valid email must be provided for letsencrypt registration"), system, cmd, c, args)
	}

	// Initialize notifier
	c.notifier = &notifier.HTTPSNotify{
		Endpoint:     c.NotifyURL,
		Headers:      []string{c.NotifyHeader},
		SlackToken:   c.SlackToken,
		SlackChannel: c.SlackChannel,
		SlackEvents:  "INSTANCE_SIZING_DISK_RAM,INSTANCE_SIZING_DISK,INSTANCE_SIZING_RAM,INSTANCE_SPOT_CAPACITY",
	}
	c.notifier.Init()

	// Create monitor instance with system reference
	monitor := &agiMonitor{
		cmd:        c,
		system:     system,
		cache:      &inventoryCache{},
		banTracker: newIPBanTracker(),
	}

	// Start ban tracker cleanup goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			<-ticker.C
			if shutdown.IsShuttingDown() {
				return
			}
			// Check for expired bans and log removals
			monitor.banTracker.Lock()
			now := time.Now()
			for ip, expiry := range monitor.banTracker.bans {
				if now.After(expiry) {
					system.Logger.Info("IP ban expired, removing: %s", ip)
					delete(monitor.banTracker.bans, ip)
				}
			}
			monitor.banTracker.Unlock()
			monitor.banTracker.cleanup()
		}
	}()

	// Setup HTTP handler
	http.HandleFunc("/", monitor.handle)
	http.HandleFunc("/agi/health", monitor.handleHealth)

	// Start server
	err = c.startServer(system, monitor)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	return Error(nil, system, cmd, c, args)
}

// agiMonitor holds the monitor state and references.
type agiMonitor struct {
	cmd        *AgiMonitorListenCmd
	system     *System
	cache      *inventoryCache
	banTracker *ipBanTracker
}

// startServer starts the HTTP/HTTPS server based on configuration.
func (c *AgiMonitorListenCmd) startServer(system *System, monitor *agiMonitor) error {
	// Create custom error log writer to capture TLS handshake errors for banning
	errorLog := log.New(&tlsErrorLogWriter{monitor: monitor}, "", 0)

	if c.NoTLS {
		system.Logger.Info("Listening on http://%s", c.ListenAddress)
		srv := &http.Server{
			Addr:     c.ListenAddress,
			ErrorLog: errorLog,
		}
		return srv.ListenAndServe()
	}

	// Create autocert cache directory if needed
	if _, err := os.Stat("autocert-cache"); err != nil {
		err = os.Mkdir("autocert-cache", 0755)
		if err != nil {
			return fmt.Errorf("failed to create autocert cache directory: %w", err)
		}
	}

	// Use autocert if domains are specified
	if len(c.AutoCertDomains) > 0 {
		m := &autocert.Manager{
			Cache:      autocert.DirCache("autocert-cache"),
			Prompt:     autocert.AcceptTOS,
			Email:      c.AutoCertEmail,
			HostPolicy: autocert.HostWhitelist(c.AutoCertDomains...),
		}

		// Load or generate fallback certificate for connections without SNI (e.g., direct IP)
		fallbackCertFile := "/etc/ssl/certs/ssl-cert-snakeoil.pem"
		fallbackKeyFile := "/etc/ssl/private/ssl-cert-snakeoil.key"
		if !isFile(fallbackCertFile) || !isFile(fallbackKeyFile) {
			if err := generateSnakeoilCert(); err != nil {
				return fmt.Errorf("failed to generate fallback certificate: %w", err)
			}
		}
		fallbackCert, err := tls.LoadX509KeyPair(fallbackCertFile, fallbackKeyFile)
		if err != nil {
			return fmt.Errorf("failed to load fallback certificate: %w", err)
		}

		// Start HTTP handler for ACME challenge
		go func() {
			srv := &http.Server{
				Addr:    ":80",
				Handler: m.HTTPHandler(nil),
			}
			system.Logger.Info("AutoCert: Listening on 0.0.0.0:80")
			err := srv.ListenAndServe()
			system.Logger.Error("AutoCert HTTP server error: %s", err)
		}()

		// Start HTTPS server with autocert and fallback for missing/unknown SNI
		tlsConfig := m.TLSConfig()
		originalGetCertificate := tlsConfig.GetCertificate
		tlsConfig.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			// If no server name provided (direct IP connection), use fallback cert
			if hello.ServerName == "" {
				system.Logger.Debug("TLS connection without SNI, using fallback certificate")
				return &fallbackCert, nil
			}
			// If server name not in autocert domains, use fallback cert
			if !inslice.HasString(c.AutoCertDomains, hello.ServerName) {
				system.Logger.Debug("TLS connection with unknown SNI '%s', using fallback certificate", hello.ServerName)
				return &fallbackCert, nil
			}
			return originalGetCertificate(hello)
		}
		s := &http.Server{
			Addr:      c.ListenAddress,
			TLSConfig: tlsConfig,
			ErrorLog:  errorLog,
		}
		s.TLSConfig.MinVersion = tls.VersionTLS12
		s.TLSConfig.CurvePreferences = []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256}
		s.TLSConfig.CipherSuites = []uint16{
			tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		}
		system.Logger.Info("Listening on https://%s", c.ListenAddress)
		return s.ListenAndServeTLS("", "")
	}

	// Use custom certificates or generate self-signed
	certFile := c.CertFile
	keyFile := c.KeyFile
	if certFile == "" && keyFile == "" {
		certFile = "/etc/ssl/certs/ssl-cert-snakeoil.pem"
		keyFile = "/etc/ssl/private/ssl-cert-snakeoil.key"
		if !isFile(certFile) || !isFile(keyFile) {
			err := generateSnakeoilCert()
			if err != nil {
				return fmt.Errorf("failed to generate self-signed certificate: %w", err)
			}
		}
	}

	system.Logger.Info("Listening on https://%s", c.ListenAddress)
	srv := &http.Server{
		Addr:     c.ListenAddress,
		ErrorLog: errorLog,
	}
	srv.TLSConfig = &tls.Config{
		MinVersion:       tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
	}
	return srv.ListenAndServeTLS(certFile, keyFile)
}

// isFile checks if a file exists.
func isFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// generateSnakeoilCert generates a self-signed certificate.
func generateSnakeoilCert() error {
	snakeScript := `which apt
ISAPT=$?
set -e
if [ $ISAPT -eq 0 ]
then
    apt update && apt -y install ssl-cert
else
    yum install -y wget mod_ssl
    mkdir -p /etc/ssl/certs /etc/ssl/private
    openssl req -new -x509 -nodes -out /etc/ssl/certs/ssl-cert-snakeoil.pem -keyout /etc/ssl/private/ssl-cert-snakeoil.key -days 3650 -subj '/CN=www.example.com'
fi
`
	err := os.WriteFile("/tmp/snakeoil.sh", []byte(snakeScript), 0755)
	if err != nil {
		return err
	}
	out, err := exec.Command("/bin/bash", "/tmp/snakeoil.sh").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

// handleHealth handles the /agi/health endpoint.
func (m *agiMonitor) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handle is the main request handler for the monitor.
func (m *agiMonitor) handle(w http.ResponseWriter, r *http.Request) {
	uuid := shortuuid.New()

	// Extract IP for ban checking
	reqIP := strings.Split(r.RemoteAddr, ":")[0]

	// Check if IP is banned
	if m.banTracker.isBanned(reqIP) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("banned"))
		return
	}

	// Check for auth header
	authHeader := r.Header.Get("Agi-Monitor-Auth")
	if authHeader == "" {
		if m.banTracker.recordFailure(reqIP) {
			m.banTracker.ban(reqIP)
			m.system.Logger.Warn("IP banned for 1 hour due to repeated auth failures: %s", reqIP)
		}
		m.respond(w, r, uuid, 401, "auth header missing", "auth header missing")
		return
	}

	// Decode auth header
	authObj, err := notifier.DecodeAuthJson(authHeader)
	if err != nil {
		if m.banTracker.recordFailure(reqIP) {
			m.banTracker.ban(reqIP)
			m.system.Logger.Warn("IP banned for 1 hour due to repeated auth failures: %s", reqIP)
		}
		m.respond(w, r, uuid, 401, "auth header invalid json", "auth header invalid json: "+err.Error())
		m.log(uuid, "handle", authHeader)
		return
	}

	// Get inventory and find matching instance
	inv := m.getInventory(false)
	instance := m.findInstance(inv, authObj.InstanceId)

	// If not found, refresh inventory and try again
	if instance == nil {
		inv = m.getInventory(true)
		instance = m.findInstance(inv, authObj.InstanceId)
	}

	// Build log data
	logJson := struct {
		Instance *backends.Instance
		AuthObj  *notifier.AgiMonitorAuth
	}{
		Instance: instance,
		AuthObj:  authObj,
	}
	logData, _ := json.Marshal(logJson)

	if instance == nil {
		m.respond(w, r, uuid, 401, "auth: instance not found", "auth: instance not found: "+string(logData))
		return
	}

	// Verify instance details
	if err := m.verifyInstance(authObj, instance, r); err != nil {
		m.respond(w, r, uuid, 401, "auth: incorrect", err.Error()+": "+string(logData))
		return
	}

	// Challenge-response callback
	secretChallenge := r.Header.Get("Agi-Monitor-Secret")
	reqDomain := reqIP

	// Check for DNS domain
	// First check for full DNS name (set by configureAGIDNS for EFS/shortuuid prefixes)
	if agiDNSName, ok := instance.Tags["agiDNSName"]; ok && agiDNSName != "" {
		reqDomain = agiDNSName
	} else if agiDomain, ok := instance.Tags["agiDomain"]; ok && agiDomain != "" {
		// Fallback to instance ID based domain for backwards compatibility
		reqDomain = fmt.Sprintf("%s.%s.agi.%s", instance.InstanceID, m.system.Opts.Config.Backend.Region, agiDomain)
	}
	// Verify DNS IP matches for security (applies to both agiDNSName and agiDomain)
	// If DNS lookup fails, fall back to using IP for callback (DNS may not be configured)
	if reqDomain != reqIP {
		ips, err := net.LookupIP(reqDomain)
		if err != nil {
			m.log(uuid, "auth", fmt.Sprintf("DNS lookup failed for %s, falling back to IP %s: %s", reqDomain, reqIP, err))
			reqDomain = reqIP
		} else {
			domainFound := false
			for _, ip := range ips {
				if inslice.HasString([]string{instance.IP.Private, instance.IP.Public}, ip.String()) {
					domainFound = true
				}
			}
			if !domainFound {
				m.log(uuid, "auth", fmt.Sprintf("DNS IP mismatch for %s (instance:[%s,%s] req:%s), falling back to IP", reqDomain, instance.IP.Private, instance.IP.Public, reqIP))
				reqDomain = reqIP
			}
		}
	}

	var callbackFailure error
	if accepted, err := m.challengeCallback(reqDomain, secretChallenge); err != nil {
		callbackFailure = err
	} else if !accepted {
		m.respond(w, r, uuid, 401, "auth: incorrect", "auth:7 incorrect: challenge callback not accepted")
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		m.respond(w, r, uuid, 400, "message body read error", "io.ReadAll(r.Body): "+err.Error())
		return
	}

	// Parse event
	event := &ingest.NotifyEvent{}
	err = json.Unmarshal(body, event)
	if err != nil {
		m.respond(w, r, uuid, 400, "message json malformed", "json.Unmarshal(body): "+err.Error())
		return
	}

	// Debug logging
	if m.cmd.DebugEvents {
		debugEvent, _ := json.MarshalIndent(event, "", "  ")
		m.log(uuid, "debug", string(debugEvent))
	}

	// Handle event
	evt, _ := json.Marshal(event)
	switch event.Event {
	case agi.AgiEventSpotNoCapacity:
		if m.cmd.DisableCapacity {
			m.respond(w, r, uuid, 200, "ignoring: capacity handling disabled", "ignoring: capacity handling disabled")
			return
		}
		testJson := &AgiCreateCmd{}
		if !m.getDeploymentJSON(uuid, event, testJson) {
			m.respond(w, r, uuid, 400, "capacity: invalid deployment json", "Capacity: abort on invalid deployment json")
			return
		}
		m.respond(w, r, uuid, 418, "capacity: rotating to on-demand", "Capacity: start on-demand rotation: "+string(evt))
		go m.handleCapacity(uuid, event)

	case agi.AgiEventInitComplete, agi.AgiEventDownloadComplete, agi.AgiEventUnpackComplete,
		agi.AgiEventPreProcessComplete, agi.AgiEventResourceMonitor, agi.AgiEventServiceDown:
		if m.cmd.DisableSizing {
			m.respond(w, r, uuid, 200, "ignoring: sizing disabled", "ignoring: sizing disabled")
			return
		}
		if callbackFailure != nil {
			m.respond(w, r, uuid, 401, "auth: incorrect", "auth:7 incorrect: callback failed: "+callbackFailure.Error())
			return
		}
		m.handleCheckSizing(w, r, uuid, event, authObj.InstanceType, instance.ZoneName, reqDomain, secretChallenge)

	default:
		m.respond(w, r, uuid, 200, "event received", "event received: "+event.Event)
	}
}

// findInstance finds an AGI instance by ID in the inventory.
// Only returns instances of type "agi" to prevent the monitor from processing
// events for non-AGI instances (including itself, which is type "agimonitor").
func (m *agiMonitor) findInstance(inv *backends.Inventory, instanceID string) *backends.Instance {
	if inv == nil {
		return nil
	}
	// Only look at AGI instances, not other types like "agimonitor"
	agiInstances := inv.Instances.WithType("agi")
	for _, inst := range agiInstances.Describe() {
		if inst.InstanceID == instanceID {
			return inst
		}
	}
	return nil
}

// getInventory returns the cached inventory or fetches a new one.
func (m *agiMonitor) getInventory(forceRefresh bool) *backends.Inventory {
	m.cmd.invLock.Lock()
	defer m.cmd.invLock.Unlock()

	if forceRefresh || m.cache.expiry.Before(time.Now()) {
		// Force the backend to refresh from AWS/GCP, not just return its cached inventory
		if err := m.system.Backend.ForceRefreshInventory(); err != nil {
			m.system.Logger.Warn("Failed to force refresh inventory: %s", err)
		}
		m.cache.inventory = m.system.Backend.GetInventory()
		m.cache.expiry = time.Now().Add(10 * time.Second)
	}
	return m.cache.inventory
}

// verifyInstance verifies the auth object matches the instance.
func (m *agiMonitor) verifyInstance(auth *notifier.AgiMonitorAuth, inst *backends.Instance, r *http.Request) error {
	backendType := m.system.Opts.Config.Backend.Type

	// Verify image ID
	if backendType == "aws" && auth.ImageId != inst.ImageID {
		return fmt.Errorf("auth:1 incorrect: image ID mismatch")
	} else if backendType == "gcp" && !strings.HasSuffix(inst.ImageID, "/"+auth.ImageId) {
		return fmt.Errorf("auth:1 incorrect: image ID mismatch")
	}

	// Verify private IP
	if auth.PrivateIp != inst.IP.Private {
		return fmt.Errorf("auth:2 incorrect: private IP mismatch")
	}

	// Verify availability zone
	if !strings.HasPrefix(auth.AvailabilityZoneName, inst.ZoneName) {
		return fmt.Errorf("auth:3 incorrect: zone mismatch")
	}

	// Verify security groups
	// For AWS, auth.SecurityGroups contains names, but inst.Firewalls contains IDs
	// We need to compare using the names from BackendSpecific
	if backendType == "aws" {
		if awsDetail, ok := inst.BackendSpecific.(*baws.InstanceDetail); ok {
			for _, sg := range awsDetail.SecurityGroups {
				sgName := ""
				if sg.GroupName != nil {
					sgName = *sg.GroupName
				}
				if !inslice.HasString(auth.SecurityGroups, sgName) {
					return fmt.Errorf("auth:4 incorrect: security group mismatch")
				}
			}
		} else {
			return fmt.Errorf("auth:4 incorrect: could not get AWS instance details")
		}
	} else {
		// For GCP, use the Firewalls field directly (contains tag names)
		for _, sg := range inst.Firewalls {
			if !inslice.HasString(auth.SecurityGroups, sg) {
				return fmt.Errorf("auth:4 incorrect: security group mismatch")
			}
		}
	}

	// Verify instance type
	instanceType := inst.InstanceType
	if backendType == "gcp" {
		parts := strings.Split(instanceType, "/")
		instanceType = parts[len(parts)-1]
	}
	if auth.InstanceType != instanceType {
		return fmt.Errorf("auth:5 incorrect: instance type mismatch")
	}

	// Verify request IP matches instance IP
	reqIP := strings.Split(r.RemoteAddr, ":")[0]
	if !inslice.HasString([]string{inst.IP.Private, inst.IP.Public}, reqIP) {
		return fmt.Errorf("auth:6 incorrect: request IP does not match node IP (instance:[%s,%s] req:%s)", inst.IP.Private, inst.IP.Public, reqIP)
	}

	return nil
}

// challengeCallback performs the challenge-response callback to verify the AGI instance.
func (m *agiMonitor) challengeCallback(ip string, secret string) (bool, error) {
	ret, err := m.challengeCallbackDo("https", ip, secret)
	if err != nil {
		ret, err = m.challengeCallbackDo("http", ip, secret)
	}
	return ret, err
}

// challengeCallbackDo performs the actual callback request.
func (m *agiMonitor) challengeCallbackDo(prot string, ip string, secret string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, prot+"://"+ip+"/agi/monitor-challenge", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Agi-Monitor-Secret", secret)
	tr := &http.Transport{
		DisableKeepAlives: true,
		IdleConnTimeout:   10 * time.Second,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: !m.cmd.StrictAGITLS},
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: tr,
	}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTeapot {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return false, fmt.Errorf("wrong error code: %d", resp.StatusCode)
	}
	return true, nil
}

// getDeploymentJSON extracts deployment JSON from the event.
func (m *agiMonitor) getDeploymentJSON(uuid string, event *ingest.NotifyEvent, dst interface{}) bool {
	deployDetail, err := base64.StdEncoding.DecodeString(event.DeploymentJsonGzB64)
	if err != nil {
		m.log(uuid, "getDeploymentJson", "base64.StdEncoding.DecodeString: "+err.Error())
		return false
	}
	un, err := gzip.NewReader(bytes.NewReader(deployDetail))
	if err != nil {
		m.log(uuid, "getDeploymentJson", "gzip.NewReader: "+err.Error())
		return false
	}
	deployDetail, err = io.ReadAll(un)
	un.Close()
	if err != nil {
		m.log(uuid, "getDeploymentJson", "io.Read(gz): "+err.Error())
		return false
	}
	err = json.Unmarshal(deployDetail, dst)
	if err != nil {
		m.log(uuid, "getDeploymentJson", "json.Unmarshal: "+err.Error())
		return false
	}
	return true
}

// handleCapacity handles spot instance capacity rotation to on-demand.
// It destroys the spot instance and uses agiStart to reattach the volume as on-demand.
func (m *agiMonitor) handleCapacity(uuid string, event *ingest.NotifyEvent) {
	m.cmd.execLock.Lock()
	defer m.cmd.execLock.Unlock()

	shutdown.AddJob()
	defer shutdown.DoneJob()

	// CRITICAL: Validate AGIName is not empty before proceeding
	// If empty, the destroy filter would match ALL AGI instances instead of just one
	if event.AGIName == "" {
		m.log(uuid, "capacity", "CRITICAL: event.AGIName is empty, refusing to proceed to prevent destroying all AGI instances")
		return
	}

	nnotify := &agiMonitorNotify{
		Name:   event.AGIName,
		Action: agiMonitorNotifyActionSpotCapacity,
		Stage:  agiMonitorNotifyStageStart,
		Event:  event,
	}
	m.sendNotify(nnotify)

	// Add tag to indicate monitor is working on this instance
	inv := m.getInventory(true)
	instances := inv.Instances.WithName(event.AGIName)
	if instances.Count() > 0 {
		if err := instances.AddTags(map[string]string{"monitorState": "sizing-capacity"}); err != nil {
			m.log(uuid, "capacity", fmt.Sprintf("Warning: failed to add monitorState tag: %s", err))
		}
	}

	// Destroy the spot instance
	destroyCmd := &InstancesDestroyCmd{
		Force: true,
		Filters: InstancesListFilter{
			ClusterName: event.AGIName,
			Type:        "agi",
		},
	}
	_, err := destroyCmd.DestroyInstances(m.system, inv, nil)
	if err != nil {
		nnotify.Stage = agiMonitorNotifyStageError
		nnotify.Error = errStr(err)
		m.sendNotify(nnotify)
		m.log(uuid, "capacity", fmt.Sprintf("Error destroying instance, attempting to continue (%s)", err))
		return
	}

	// Use agiStart to reattach volume as on-demand (spot=false)
	// agiStart reads settings from EFS/volume tags and applies our overrides
	spotFalse := false
	startCmd := &AgiStartCmd{
		Name: TypeAgiClusterName(event.AGIName),
		Reattach: Reattach{
			SpotOverride:  &spotFalse,
			OwnerOverride: event.Owner,
		},
	}

	// Refresh inventory after destroy
	inv = m.getInventory(true)
	newInstances, err := startCmd.StartAGI(m.system, inv, m.system.Logger, nil)
	if err != nil {
		nnotify.Stage = agiMonitorNotifyStageError
		nnotify.Error = errStr(err)
		m.sendNotify(nnotify)
		m.log(uuid, "capacity", fmt.Sprintf("Error creating new instance (%s)", err))
		return
	}

	// Restore SSH authorized keys after instance creation
	if event.SSHAuthorizedKeysFileGzB64 != "" && newInstances.Count() > 0 {
		err = m.restoreSSHAuthorizedKeys(newInstances[0], event.SSHAuthorizedKeysFileGzB64)
		if err != nil {
			m.log(uuid, "capacity", fmt.Sprintf("Warning: failed to restore SSH keys: %s", err))
		}
	}

	nnotify.Stage = agiMonitorNotifyStageDone
	m.sendNotify(nnotify)
	m.log(uuid, "capacity", "rotated to on-demand instance")
}

// restoreSSHAuthorizedKeys restores SSH authorized keys to an instance.
func (m *agiMonitor) restoreSSHAuthorizedKeys(instance *backends.Instance, keysGzB64 string) error {
	if keysGzB64 == "" {
		return nil
	}

	// Decode and decompress
	data, err := base64.StdEncoding.DecodeString(keysGzB64)
	if err != nil {
		return fmt.Errorf("failed to decode SSH keys: %w", err)
	}

	un, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to decompress SSH keys: %w", err)
	}
	defer un.Close()

	keys, err := io.ReadAll(un)
	if err != nil {
		return fmt.Errorf("failed to read SSH keys: %w", err)
	}

	// Upload via SFTP
	conf, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	cli, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer cli.Close()

	// Ensure .ssh directory exists
	_ = cli.RawClient().MkdirAll("/root/.ssh")

	// Write authorized_keys
	err = cli.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/root/.ssh/authorized_keys",
		Source:      bytes.NewReader(keys),
		Permissions: 0600,
	})
	if err != nil {
		return fmt.Errorf("failed to write authorized_keys: %w", err)
	}

	return nil
}

// handleCheckSizing checks if sizing is required and triggers it.
func (m *agiMonitor) handleCheckSizing(w http.ResponseWriter, r *http.Request, uuid string, event *ingest.NotifyEvent, currentType string, zone string, instanceIP string, secret string) {
	nnotify := &agiMonitorNotify{
		Name:  event.AGIName,
		Event: event,
	}

	// Check for required disk sizing on GCP
	diskNewSize := uint64(0)
	if m.system.Opts.Config.Backend.Type == "gcp" {
		if event.IngestStatus.System.DiskTotalBytes/1024/1024/1024 < uint64(m.cmd.SizingMaxDiskGB) {
			if event.IngestStatus.System.DiskFreeBytes > 0 && event.IngestStatus.System.DiskTotalBytes > 0 {
				usedPct := 1 - (float64(event.IngestStatus.System.DiskFreeBytes) / float64(event.IngestStatus.System.DiskTotalBytes))
				if event.IngestStatus.Ingest.LogProcessorCompletePct < 100 && usedPct > float64(m.cmd.GCPDiskThresholdPct)/100 {
					diskNewSize = (event.IngestStatus.System.DiskTotalBytes / 1024 / 1024 / 1024) + uint64(m.cmd.GCPDiskIncreaseGB)
					if diskNewSize > uint64(m.cmd.SizingMaxDiskGB) {
						diskNewSize = uint64(m.cmd.SizingMaxDiskGB)
					}
					nnotify.Disk = &agiMonitorNotifyDisk{
						InitialSizeGB: int(event.IngestStatus.System.DiskTotalBytes / 1024 / 1024 / 1024),
						FinalSizeGB:   int(diskNewSize),
					}
				}
			}
		}
	}

	// Check if RAM is running out and size accordingly
	newType := ""
	disableDim := false
	performRamSizing := m.checkRAMSizing(w, r, uuid, event, currentType, zone, &newType, &disableDim)

	// Set notification data
	if !performRamSizing {
		newType = ""
		disableDim = false
	} else {
		nnotify.RAM = &agiMonitorNotifyRAM{
			InitialInstanceType: currentType,
			FinalInstanceType:   newType,
			DisableDIM:          disableDim,
		}
	}
	if newType == "" && disableDim {
		newType = currentType
	}

	// Test deployment JSON before proceeding
	testJson := &AgiCreateCmd{}

	// Perform sizing based on requirements
	if diskNewSize > 0 && newType == "" && !disableDim {
		if !m.getDeploymentJSON(uuid, event, testJson) {
			m.respond(w, r, uuid, 400, "sizing: invalid deployment json", "Sizing: abort on invalid deployment json")
			return
		}
		m.respond(w, r, uuid, 200, "sizing: adding disk capacity", fmt.Sprintf("Sizing: changing disk capacity from %d GiB to %d GiB", event.IngestStatus.System.DiskTotalBytes/1024/1024/1024, diskNewSize))
		go m.handleSizingDisk(uuid, event, int64(diskNewSize), nnotify, instanceIP, secret)
	} else if diskNewSize == 0 && (newType != "" || disableDim) {
		if !m.getDeploymentJSON(uuid, event, testJson) {
			m.respond(w, r, uuid, 400, "sizing: invalid deployment json", "Sizing: abort on invalid deployment json")
			return
		}
		m.respond(w, r, uuid, 418, "sizing: instance-ram", fmt.Sprintf("Sizing: instance-ram currentType=%s newType=%s disableDim=%t", currentType, newType, disableDim))
		go m.handleSizingRAM(uuid, event, newType, disableDim, nnotify)
	} else if diskNewSize > 0 && (newType != "" || disableDim) {
		if !m.getDeploymentJSON(uuid, event, testJson) {
			m.respond(w, r, uuid, 400, "sizing: invalid deployment json", "Sizing: abort on invalid deployment json")
			return
		}
		m.respond(w, r, uuid, 418, "sizing: instance-disk-and-ram", fmt.Sprintf("Sizing: instance-disk-and-ram old-disk=%dGiB new-disk=%dGiB currentType=%s newType=%s disableDim=%t", event.IngestStatus.System.DiskTotalBytes/1024/1024/1024, diskNewSize, currentType, newType, disableDim))
		go m.handleSizingDiskAndRAM(uuid, event, int64(diskNewSize), newType, disableDim, nnotify, instanceIP, secret)
	} else {
		m.respond(w, r, uuid, 200, "sizing: not required", "sizing: not required")
	}
}

// checkRAMSizing determines if RAM sizing is needed.
func (m *agiMonitor) checkRAMSizing(w http.ResponseWriter, r *http.Request, uuid string, event *ingest.NotifyEvent, currentType string, zone string, newType *string, disableDim *bool) bool {
	switch event.Event {
	case agi.AgiEventServiceDown:
		if event.IngestStatus.AerospikeRunning && event.IngestStatus.PluginRunning {
			// Ingest or grafana helper is down, do NOT size; only Plugin and Aerospike crashes should be sized
			return false
		}
		if event.IsDataInMemory && m.cmd.SizingNoDIMFirst {
			*disableDim = true
		} else {
			instanceTypes, itype, err := m.getInstanceTypes(zone, currentType)
			if err != nil {
				m.respond(w, r, uuid, 500, "sizing: get instance types failure", fmt.Sprintf("Sizing: getInstanceTypes: %s", err))
				return false
			}
			*newType, err = m.sizeInstanceType(instanceTypes, currentType, int(itype.MemoryGiB+1))
			if err != nil {
				if !event.IsDataInMemory {
					m.respond(w, r, uuid, 400, "sizing: "+err.Error(), fmt.Sprintf("Sizing: %s", err))
					return false
				}
				*disableDim = true
			}
		}

	case agi.AgiEventPreProcessComplete:
		requiredRam := 0
		dimRequiredMemory := int(float64(event.IngestStatus.Ingest.LogProcessorTotalSize)*m.cmd.DimMultiplier/1024/1024/1024) + m.cmd.RAMThresMinFreeGB + 2
		noDimRequiredMemory := int(float64(event.IngestStatus.Ingest.LogProcessorTotalSize)*m.cmd.NoDimMultiplier/1024/1024/1024) + m.cmd.RAMThresMinFreeGB + 2
		if event.IsDataInMemory {
			requiredRam = dimRequiredMemory
		} else {
			requiredRam = noDimRequiredMemory
		}

		instanceTypes, itype, err := m.getInstanceTypes(zone, currentType)
		if err != nil {
			m.respond(w, r, uuid, 500, "sizing: get instance types failure", fmt.Sprintf("Sizing: getInstanceTypes: %s", err))
			return false
		}

		if itype.MemoryGiB >= float64(requiredRam) {
			return false
		}

		if event.IsDataInMemory && m.cmd.SizingNoDIMFirst {
			*disableDim = true
			if itype.MemoryGiB < float64(noDimRequiredMemory) {
				*newType, err = m.sizeInstanceType(instanceTypes, currentType, noDimRequiredMemory)
				if err != nil {
					m.log(uuid, "sizing", "WARNING: reached max sizing and will not have enough RAM anyways: "+err.Error())
				}
			}
		} else if event.IsDataInMemory {
			*newType, err = m.sizeInstanceType(instanceTypes, currentType, requiredRam)
			if err != nil {
				*disableDim = true
			}
		} else {
			*newType, err = m.sizeInstanceType(instanceTypes, currentType, requiredRam)
			if err != nil {
				if *newType == currentType {
					m.respond(w, r, uuid, 500, "sizing: max reached and may still run out of memory", fmt.Sprintf("Sizing: reached max and will probably still run out of RAM: %s", err))
					return false
				}
				m.log(uuid, "sizing", "WARNING: reached max sizing and will not have enough RAM anyways: "+err.Error())
			}
		}

	default:
		// Use float64 division to preserve precision for threshold comparisons.
		// For example, 7.5GB free should not be rounded down to 7GB.
		memFreeGB := float64(event.IngestStatus.System.MemoryFreeBytes) / 1024 / 1024 / 1024
		memUsedPct := float64(0)
		if event.IngestStatus.System.MemoryTotalBytes > 0 && event.IngestStatus.System.MemoryFreeBytes > 0 {
			memUsedPct = 1 - (float64(event.IngestStatus.System.MemoryFreeBytes) / float64(event.IngestStatus.System.MemoryTotalBytes))
		}

		if memFreeGB < float64(m.cmd.RAMThresMinFreeGB) || memUsedPct > float64(m.cmd.RAMThresUsedPct)/100 {
			if event.IsDataInMemory && m.cmd.SizingNoDIMFirst {
				*disableDim = true
			} else {
				instanceTypes, itype, err := m.getInstanceTypes(zone, currentType)
				if err != nil {
					m.respond(w, r, uuid, 500, "sizing: get instance types failure", fmt.Sprintf("Sizing: getInstanceTypes: %s", err))
					return false
				}
				*newType, err = m.sizeInstanceType(instanceTypes, currentType, int(itype.MemoryGiB+1))
				if err != nil {
					if !event.IsDataInMemory {
						m.respond(w, r, uuid, 400, "sizing: "+err.Error(), fmt.Sprintf("Sizing: %s", err))
						return false
					}
					*disableDim = true
				}
			}
		} else {
			return false
		}
	}

	return true
}

// getInstanceTypes gets available instance types for the zone.
func (m *agiMonitor) getInstanceTypes(zone string, currentType string) (backends.InstanceTypeList, *backends.InstanceType, error) {
	backendType := backends.BackendType(m.system.Opts.Config.Backend.Type)
	instanceTypes, err := m.system.Backend.GetInstanceTypes(backendType)
	if err != nil {
		return nil, nil, err
	}

	// Find current type
	var itype *backends.InstanceType
	for _, t := range instanceTypes {
		if t.Name == currentType {
			itype = t
			break
		}
	}
	if itype == nil {
		return instanceTypes, nil, fmt.Errorf("instance type '%s' not found", currentType)
	}
	return instanceTypes, itype, nil
}

// sizeInstanceType finds an appropriate instance type with required memory.
func (m *agiMonitor) sizeInstanceType(instanceTypes backends.InstanceTypeList, currentType string, requiredMemory int) (string, error) {
	family := ""
	backendType := m.system.Opts.Config.Backend.Type

	if backendType == "aws" {
		if !strings.Contains(currentType, ".") {
			return currentType, errors.New("family not found")
		}
		family = strings.Split(currentType, ".")[0] + "."
	} else {
		fsplit := strings.Split(currentType, "-")
		if len(fsplit) != 3 {
			return currentType, errors.New("family type cannot be sized")
		}
		family = fsplit[0] + "-" + fsplit[1] + "-"
	}

	type ntype struct {
		name string
		ram  float64
	}

	ntypes := []ntype{}
	for _, t := range instanceTypes {
		if t.MemoryGiB > float64(m.cmd.SizingMaxRamGB) {
			continue
		}
		if strings.HasPrefix(t.Name, family) {
			ntypes = append(ntypes, ntype{
				name: t.Name,
				ram:  t.MemoryGiB,
			})
		}
	}
	if len(ntypes) == 0 {
		return currentType, errors.New("family not in list or list exhausted")
	}
	sort.Slice(ntypes, func(i, j int) bool {
		return ntypes[i].ram < ntypes[j].ram
	})

	newType := ""
	for _, n := range ntypes {
		if n.ram >= float64(requiredMemory) {
			return n.name, nil
		}
		newType = n.name
	}
	return newType, errors.New("sizing exhausted")
}

// handleSizingDisk handles disk sizing only.
func (m *agiMonitor) handleSizingDisk(uuid string, event *ingest.NotifyEvent, newSize int64, nnotify *agiMonitorNotify, instanceIP string, secret string) {
	m.cmd.execLock.Lock()
	defer m.cmd.execLock.Unlock()

	shutdown.AddJob()
	defer shutdown.DoneJob()

	createCmd := &AgiCreateCmd{}
	if !m.getDeploymentJSON(uuid, event, createCmd) {
		return
	}

	nnotify.Action = agiMonitorNotifyActionDisk
	nnotify.Stage = agiMonitorNotifyStageStart
	m.sendNotify(nnotify)

	err := m.handleSizingDiskDo(uuid, event, newSize, createCmd, instanceIP, secret)
	if err != nil {
		nnotify.Stage = agiMonitorNotifyStageError
		nnotify.Error = errStr(err)
		m.sendNotify(nnotify)
		return
	}

	nnotify.Stage = agiMonitorNotifyStageDone
	m.sendNotify(nnotify)
}

// handleSizingRAM handles RAM sizing only.
func (m *agiMonitor) handleSizingRAM(uuid string, event *ingest.NotifyEvent, newType string, disableDim bool, nnotify *agiMonitorNotify) {
	m.cmd.execLock.Lock()
	defer m.cmd.execLock.Unlock()

	shutdown.AddJob()
	defer shutdown.DoneJob()

	createCmd := &AgiCreateCmd{}
	if !m.getDeploymentJSON(uuid, event, createCmd) {
		return
	}

	nnotify.Action = agiMonitorNotifyActionRAM
	nnotify.Stage = agiMonitorNotifyStageStart
	m.sendNotify(nnotify)

	err := m.handleSizingRAMDo(uuid, event, newType, disableDim, createCmd)
	if err != nil {
		nnotify.Stage = agiMonitorNotifyStageError
		nnotify.Error = errStr(err)
		m.sendNotify(nnotify)
		return
	}

	nnotify.Stage = agiMonitorNotifyStageDone
	m.sendNotify(nnotify)
}

// handleSizingDiskAndRAM handles both disk and RAM sizing.
func (m *agiMonitor) handleSizingDiskAndRAM(uuid string, event *ingest.NotifyEvent, newSize int64, newType string, disableDim bool, nnotify *agiMonitorNotify, instanceIP string, secret string) {
	m.cmd.execLock.Lock()
	defer m.cmd.execLock.Unlock()

	shutdown.AddJob()
	defer shutdown.DoneJob()

	createCmd := &AgiCreateCmd{}
	if !m.getDeploymentJSON(uuid, event, createCmd) {
		return
	}

	nnotify.Action = agiMonitorNotifyActionDiskRAM
	nnotify.Stage = agiMonitorNotifyStageStart
	m.sendNotify(nnotify)

	errA := m.handleSizingDiskDo(uuid, event, newSize, createCmd, instanceIP, secret)
	errB := m.handleSizingRAMDo(uuid, event, newType, disableDim, createCmd)

	if errA != nil || errB != nil {
		var ea, eb string
		if errA != nil {
			ea = errA.Error()
		}
		if errB != nil {
			eb = errB.Error()
		}
		nnotify.Stage = agiMonitorNotifyStageError
		nnotify.Error = errStr(fmt.Errorf("%s ; %s", ea, eb))
		m.sendNotify(nnotify)
		return
	}

	nnotify.Stage = agiMonitorNotifyStageDone
	m.sendNotify(nnotify)
}

// handleSizingDiskDo performs the actual disk resize.
func (m *agiMonitor) handleSizingDiskDo(uuid string, event *ingest.NotifyEvent, newSize int64, createCmd *AgiCreateCmd, instanceIP string, secret string) error {
	// Get inventory and find volume by name
	inv := m.getInventory(true)
	volumes := inv.Volumes.WithName(string(createCmd.ClusterName))
	if volumes.Count() == 0 {
		return fmt.Errorf("volume not found: %s", createCmd.ClusterName)
	}

	// Resize volume via cloud API
	err := volumes.Resize(backends.StorageSize(newSize), 10*time.Minute)
	if err != nil {
		m.log(uuid, "volume", fmt.Sprintf("Error resizing volume (%s)", err))
		return err
	}

	// Call the AGI proxy to resize the filesystem
	// This is needed because the cloud API only resizes the block device,
	// not the filesystem on it
	m.log(uuid, "volume", fmt.Sprintf("Volume resized, now resizing filesystem on instance %s", instanceIP))
	err = m.callResizeFilesystem(instanceIP, secret)
	if err != nil {
		m.log(uuid, "volume", fmt.Sprintf("Error resizing filesystem (%s)", err))
		return fmt.Errorf("volume resized but filesystem resize failed: %w", err)
	}

	m.log(uuid, "volume", "Filesystem resize complete")
	return nil
}

// callResizeFilesystem calls the AGI proxy to resize the filesystem after a volume resize.
func (m *agiMonitor) callResizeFilesystem(instanceIP string, secret string) error {
	// Try HTTPS first, then HTTP
	var lastErr error
	for _, prot := range []string{"https", "http"} {
		req, err := http.NewRequest(http.MethodPost, prot+"://"+instanceIP+"/agi/monitor-resize-fs", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Agi-Monitor-Secret", secret)
		tr := &http.Transport{
			DisableKeepAlives: true,
			IdleConnTimeout:   10 * time.Second,
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: !m.cmd.StrictAGITLS},
		}
		client := &http.Client{
			Timeout:   5 * time.Minute, // resize2fs can take a while on large volumes
			Transport: tr,
		}
		defer client.CloseIdleConnections()
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if prot == "https" {
				continue // try HTTP
			}
			return fmt.Errorf("failed to connect to instance %s: %w", instanceIP, lastErr)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusTeapot {
			return fmt.Errorf("authentication failed")
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("resize-fs failed with status %d: %s", resp.StatusCode, string(body))
		}
		return nil
	}
	return fmt.Errorf("failed to connect to instance %s: %w", instanceIP, lastErr)
}

// handleSizingRAMDo performs the actual RAM/instance resize.
// It destroys the old instance and uses agiStart to reattach the volume with the new instance type.
func (m *agiMonitor) handleSizingRAMDo(uuid string, event *ingest.NotifyEvent, newType string, disableDim bool, _ *AgiCreateCmd) error {
	// CRITICAL: Validate AGIName is not empty before proceeding
	// If empty, the destroy filter would match ALL AGI instances instead of just one
	if event.AGIName == "" {
		return fmt.Errorf("CRITICAL: event.AGIName is empty, refusing to proceed to prevent destroying all AGI instances")
	}

	// Add tag to indicate monitor is working on this instance
	inv := m.getInventory(true)
	instances := inv.Instances.WithName(event.AGIName)
	if instances.Count() > 0 {
		if err := instances.AddTags(map[string]string{"monitorState": "sizing-instance"}); err != nil {
			m.log(uuid, "sizing", fmt.Sprintf("Warning: failed to add monitorState tag: %s", err))
		}
	}

	// Destroy the old instance
	destroyCmd := &InstancesDestroyCmd{
		Force: true,
		Filters: InstancesListFilter{
			ClusterName: event.AGIName,
			Type:        "agi",
		},
	}
	_, err := destroyCmd.DestroyInstances(m.system, inv, nil)
	if err != nil {
		m.log(uuid, "sizing", fmt.Sprintf("Error destroying instance, attempting to continue (%s)", err))
		return err
	}

	// Use agiStart to reattach volume with new instance type
	// agiStart reads settings from EFS/volume tags and applies our overrides
	var noDIMOverride *bool
	if disableDim {
		noDIMOverride = &disableDim
	}
	startCmd := &AgiStartCmd{
		Name: TypeAgiClusterName(event.AGIName),
		Reattach: Reattach{
			InstanceTypeOverride: newType,
			NoDIMOverride:        noDIMOverride,
			OwnerOverride:        event.Owner,
		},
	}

	// Refresh inventory after destroy
	inv = m.getInventory(true)
	newInstances, err := startCmd.StartAGI(m.system, inv, m.system.Logger, nil)
	if err != nil {
		m.log(uuid, "sizing", fmt.Sprintf("Error creating new instance (%s)", err))
		return err
	}

	// Restore SSH authorized keys after instance creation
	if event.SSHAuthorizedKeysFileGzB64 != "" && newInstances.Count() > 0 {
		errRestore := m.restoreSSHAuthorizedKeys(newInstances[0], event.SSHAuthorizedKeysFileGzB64)
		if errRestore != nil {
			m.log(uuid, "sizing", fmt.Sprintf("Warning: failed to restore SSH keys: %s", errRestore))
		}
	}

	if disableDim {
		m.log(uuid, "sizing", "disabled data-in-memory, rotated to instance type: "+newType)
	} else {
		m.log(uuid, "sizing", "rotated to instance type: "+newType)
	}
	return nil
}

// sendNotify sends a notification via the notifier.
func (m *agiMonitor) sendNotify(nnotify *agiMonitorNotify) {
	m.cmd.notifier.NotifyJSON(nnotify)

	stageMsg := ""
	switch nnotify.Stage {
	case agiMonitorNotifyStageStart:
		stageMsg = "*Stage*: Job Start"
	case agiMonitorNotifyStageDone:
		stageMsg = "*Stage*: Job Done"
	case agiMonitorNotifyStageError:
		errMsg := ""
		if nnotify.Error != nil {
			errMsg = *nnotify.Error
		}
		stageMsg = fmt.Sprintf("*Stage*: Error (%s)", errMsg)
	default:
		m.log("", "notifier", fmt.Sprintf("Unrecognized stage: %v", nnotify))
	}

	// Build source string
	s3Source := ""
	sftpSource := ""
	localSource := ""
	if nnotify.Event != nil {
		if nnotify.Event.S3Source != "" {
			s3Source = "\n> *S3*: " + nnotify.Event.S3Source
		}
		if nnotify.Event.SftpSource != "" {
			sftpSource = "\n> *SFTP*: " + nnotify.Event.SftpSource
		}
		if nnotify.Event.LocalSource != "" {
			localSource = "\n> *Local*: " + nnotify.Event.LocalSource
		}
	}

	switch nnotify.Action {
	case agiMonitorNotifyActionDisk:
		m.cmd.notifier.NotifySlack("INSTANCE_SIZING_DISK", fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *AGI Monitor increasing disk size*\n> %s",
			"MONITOR_INSTANCE_SIZING_DISK", time.Now().Format(time.RFC822), nnotify.Name,
			m.getLabel(nnotify), m.getOwner(nnotify), s3Source, sftpSource, localSource, stageMsg), "")
	case agiMonitorNotifyActionRAM:
		m.cmd.notifier.NotifySlack("INSTANCE_SIZING_RAM", fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *AGI Monitor increasing instance size*\n> %s",
			"MONITOR_INSTANCE_SIZING_RAM", time.Now().Format(time.RFC822), nnotify.Name,
			m.getLabel(nnotify), m.getOwner(nnotify), s3Source, sftpSource, localSource, stageMsg), "")
	case agiMonitorNotifyActionDiskRAM:
		m.cmd.notifier.NotifySlack("INSTANCE_SIZING_DISK_RAM", fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *AGI Monitor increasing instance and disk size*\n> %s",
			"MONITOR_INSTANCE_SIZING_DISK_RAM", time.Now().Format(time.RFC822), nnotify.Name,
			m.getLabel(nnotify), m.getOwner(nnotify), s3Source, sftpSource, localSource, stageMsg), "")
	case agiMonitorNotifyActionSpotCapacity:
		m.cmd.notifier.NotifySlack("INSTANCE_SPOT_CAPACITY", fmt.Sprintf("*%s* _@ %s_\n> *AGI Name*: %s\n> *AGI Label*: %s\n> *Owner*: %s%s%s%s\n> *AGI Monitor rotating instance from SPOT to ON_DEMAND*\n> %s",
			"MONITOR_INSTANCE_SPOT_CAPACITY", time.Now().Format(time.RFC822), nnotify.Name,
			m.getLabel(nnotify), m.getOwner(nnotify), s3Source, sftpSource, localSource, stageMsg), "")
	default:
		m.log("", "notifier", fmt.Sprintf("Unrecognized action: %v", nnotify))
	}
}

// getLabel returns the label from the notification event.
func (m *agiMonitor) getLabel(nnotify *agiMonitorNotify) string {
	if nnotify.Event != nil {
		return nnotify.Event.Label
	}
	return ""
}

// getOwner returns the owner from the notification event.
func (m *agiMonitor) getOwner(nnotify *agiMonitorNotify) string {
	if nnotify.Event != nil {
		return nnotify.Event.Owner
	}
	return ""
}

// log logs a message with UUID and action context.
func (m *agiMonitor) log(uuid string, action string, line string) {
	m.system.Logger.Info("tid:%s action:%s log:%s", uuid, action, line)
}

// respond sends an HTTP response and logs it.
func (m *agiMonitor) respond(w http.ResponseWriter, r *http.Request, uuid string, code int, value string, logmsg string) {
	m.system.Logger.Info("tid:%s remoteAddr:%s requestUri:%s method:%s returnCode:%d log:%s", uuid, r.RemoteAddr, r.RequestURI, r.Method, code, logmsg)
	if code > 299 || code < 200 {
		http.Error(w, value, code)
	} else {
		w.WriteHeader(code)
		w.Write([]byte(value))
	}
}

// errStr converts an error to a string pointer, or nil if error is nil.
func errStr(e error) *string {
	if e == nil {
		return nil
	}
	n := e.Error()
	return &n
}

// tlsErrorLogWriter is a custom io.Writer that captures TLS handshake errors
// and records them as failures for IP banning purposes.
type tlsErrorLogWriter struct {
	monitor *agiMonitor
}

// Write implements io.Writer. It parses log messages for TLS handshake errors
// and records failures for the source IP to trigger banning after repeated attempts.
func (w *tlsErrorLogWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	w.monitor.system.Logger.Debug("http server error: %s", strings.TrimSpace(msg))

	// Check for TLS handshake errors
	// Format: "http: TLS handshake error from 1.2.3.4:12345: ..."
	if strings.Contains(msg, "TLS handshake error from") {
		// Extract IP from the message
		parts := strings.SplitN(msg, "TLS handshake error from ", 2)
		if len(parts) == 2 {
			addrPart := strings.SplitN(parts[1], ":", 2)
			if len(addrPart) >= 1 {
				ip := strings.TrimSpace(addrPart[0])
				if ip != "" {
					if w.monitor.banTracker.recordFailure(ip) {
						w.monitor.banTracker.ban(ip)
						w.monitor.system.Logger.Warn("IP banned for 1 hour due to repeated TLS handshake failures: %s", ip)
					}
				}
			}
		}
	}
	return len(p), nil
}

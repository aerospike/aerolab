//go:build !nowebui

package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aerospike-community/aerolab/pkg/sshexec"
)

// agiTokenCache provides a thread-safe, per-instance cache of AGI auth tokens.
// Tokens are generated on first connect and reused for subsequent connections.
// A background goroutine periodically prunes tokens for destroyed instances.
type agiTokenCache struct {
	mu       sync.Mutex
	entries  map[string]string // instance name -> token
	creating sync.Map          // instance name -> *sync.Mutex (per-instance creation lock)
}

func (c *agiTokenCache) init() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]string)
	}
}

func (c *agiTokenCache) get(name string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.entries[name]
	return t, ok
}

func (c *agiTokenCache) set(name string, token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[name] = token
}

func (c *agiTokenCache) remove(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, name)
}

// cleanup removes cached tokens for instances that no longer exist.
func (c *agiTokenCache) cleanup(activeNames map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name := range c.entries {
		if !activeNames[name] {
			delete(c.entries, name)
		}
	}
}

// getCreateLock returns a per-instance mutex to serialize token creation.
func (c *agiTokenCache) getCreateLock(name string) *sync.Mutex {
	val, _ := c.creating.LoadOrStore(name, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// getOrCreateToken returns a cached token or generates a new one for the instance.
// Uses per-instance locking so concurrent requests for the same instance don't
// create duplicate tokens.
func (w *WebUICmd) getOrCreateAgiToken(name string, inst *backends.Instance) (string, error) {
	// Fast path: token already cached
	if token, ok := w.agiTokens.get(name); ok {
		return token, nil
	}

	// Acquire per-instance lock to prevent duplicate token creation
	lock := w.agiTokens.getCreateLock(name)
	lock.Lock()
	defer lock.Unlock()

	// Double-check after acquiring lock (another goroutine may have created it)
	if token, ok := w.agiTokens.get(name); ok {
		return token, nil
	}

	// Generate token. The file name must not encode the creation time, since
	// a known mint time narrows the search space for guessing the token.
	token, err := generateRandomToken(128)
	if err != nil {
		return "", fmt.Errorf("could not generate token: %w", err)
	}
	tokenID, err := generateRandomID(8)
	if err != nil {
		return "", fmt.Errorf("could not generate token name: %w", err)
	}
	tokenName := "webui-" + tokenID

	// Write token to the AGI instance via SFTP
	confs, err := backends.InstanceList{inst}.GetSftpConfig("root")
	if err != nil {
		return "", fmt.Errorf("could not get SFTP config: %w", err)
	}
	for _, conf := range confs {
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return "", fmt.Errorf("could not create SFTP client: %w", err)
		}
		defer cli.Close()

		_ = cli.RawClient().MkdirAll("/opt/agi/tokens")
		tokenPath := fmt.Sprintf("/opt/agi/tokens/%s", tokenName)
		err = cli.WriteFile(true, &sshexec.FileWriter{
			DestPath:    tokenPath,
			Source:      bytes.NewReader([]byte(token)),
			Permissions: 0600,
		})
		if err != nil {
			return "", fmt.Errorf("failed to write token file: %w", err)
		}
	}

	// Signal proxy to reload tokens via SIGHUP
	backends.InstanceList{inst}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "kill -HUP $(systemctl show --property MainPID --value agi-proxy) 2>/dev/null || true"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	// Brief pause for the proxy to pick up the new token
	time.Sleep(time.Second)

	// Cache the token
	w.agiTokens.set(name, token)
	return token, nil
}

// buildAgiBaseURL constructs the protocol://host:port base URL for an AGI instance.
// Handles Docker port mapping and AWS DNS names, mirroring the logic in
// AgiAddTokenCmd.buildAccessURL.
func (w *WebUICmd) buildAgiBaseURL(inst *backends.Instance) (string, error) {
	ip := inst.IP.Public
	if ip == "" {
		ip = inst.IP.Private
	}
	if ip == "" {
		return "", fmt.Errorf("AGI node IP is empty, is AGI down?")
	}

	protocol := "https://"
	containerPort := "443"
	if inst.Tags != nil {
		if ssl, ok := inst.Tags["aerolab4ssl"]; ok && ssl == "false" {
			protocol = "http://"
			containerPort = "80"
		}
	}

	port := ""
	if w.system != nil && w.system.Opts.Config.Backend.Type == "docker" {
		ip = "127.0.0.1"
		for _, fw := range inst.Firewalls {
			parts := strings.Split(fw, ",")
			if len(parts) != 2 {
				continue
			}
			var hp, cp string
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if after, ok := strings.CutPrefix(part, "host="); ok {
					hostPart := after
					if colonIdx := strings.LastIndex(hostPart, ":"); colonIdx >= 0 {
						hp = hostPart[colonIdx+1:]
					}
				} else if after, ok := strings.CutPrefix(part, "container="); ok {
					cp = after
				}
			}
			if cp == containerPort && hp != "" {
				port = ":" + hp
				break
			}
		}
	}

	if w.system != nil && w.system.Opts.Config.Backend.Type == "aws" && inst.Tags != nil {
		if dnsName, ok := inst.Tags["agiDNSName"]; ok && dnsName != "" {
			ip = dnsName
		} else if domain, ok := inst.Tags["agiDomain"]; ok && domain != "" {
			ip = inst.InstanceID + "." + w.system.Opts.Config.Backend.Region + ".agi." + domain
		}
	}

	return protocol + ip + port, nil
}

// buildAgiTokenURLBase constructs the URL prefix for token-based AGI access.
// The token is carried in the URL fragment rather than the query string:
// browsers never send fragments to the server, so the token cannot leak into
// access logs or Referer headers.
func (w *WebUICmd) buildAgiTokenURLBase(inst *backends.Instance) (string, error) {
	base, err := w.buildAgiBaseURL(inst)
	if err != nil {
		return "", err
	}
	return base + "/agi/menu#AGI_TOKEN=", nil
}

// runAgiTokenCleanupLoop periodically removes cached tokens for AGI instances
// that no longer exist in the inventory.
func (c *WebUICmd) runAgiTokenCleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.shutdownChan:
			return
		case <-ticker.C:
			c.cleanupAgiTokens()
		}
	}
}

func (c *WebUICmd) cleanupAgiTokens() {
	inventory := c.getInventory()
	if inventory == nil {
		return
	}

	agiInstances := inventory.Instances.WithTags(map[string]string{"aerolab.type": "agi"})
	activeNames := make(map[string]bool)
	for _, inst := range agiInstances.Describe() {
		activeNames[inst.ClusterName] = true
	}

	c.agiTokens.cleanup(activeNames)
}

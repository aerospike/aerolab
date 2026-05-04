//go:build !noagi && !nowebui

package cmd

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"github.com/aerospike/aerolab/pkg/backend/backends"
)

// agiStatusCache is a thread-safe cache of AGI ingest status display strings,
// keyed by AGI instance name (clusterName).
type agiStatusCache struct {
	mu      sync.RWMutex
	entries map[string]string // instance name -> status display string (e.g. "READY", "(2/6) DOWNLOAD 45%")
}

func (c *agiStatusCache) init() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]string)
	}
}

func (c *agiStatusCache) get(name string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries[name]
}

func (c *agiStatusCache) set(name string, status string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[name] = status
}

// cleanup removes cached statuses for instances that no longer exist.
func (c *agiStatusCache) cleanup(activeNames map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name := range c.entries {
		if !activeNames[name] {
			delete(c.entries, name)
		}
	}
}

// runAgiStatusLoop periodically fetches AGI ingest status for all running
// AGI instances and caches the results. Runs every 5 minutes.
func (c *WebUICmd) runAgiStatusLoop() {
	// Fetch once on startup after a brief delay to let inventory populate
	time.Sleep(10 * time.Second)
	c.fetchAllAgiStatuses()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.shutdownChan:
			return
		case <-ticker.C:
			c.fetchAllAgiStatuses()
		}
	}
}

// fetchAllAgiStatuses gets the current inventory, finds all running AGI
// instances, and fetches their ingest status in parallel (up to 20 at a time).
// If any previously-running instances are unreachable, it triggers ForceRefreshInventory.
func (c *WebUICmd) fetchAllAgiStatuses() {
	inventory := c.getInventory()
	if inventory == nil {
		return
	}

	agiInstances := inventory.Instances.
		WithTags(map[string]string{"aerolab.type": "agi"}).
		WithState(backends.LifeCycleStateRunning).
		Describe()

	if len(agiInstances) == 0 {
		return
	}

	// Track active names for cleanup
	activeNames := make(map[string]bool)
	for _, inst := range agiInstances {
		activeNames[inst.ClusterName] = true
	}
	c.agiStatus.cleanup(activeNames)

	// Fetch statuses in parallel, capped at 20 concurrent goroutines
	sem := make(chan struct{}, 20)
	wg := &sync.WaitGroup{}
	var unreachableCount int64

	for _, inst := range agiInstances {
		sem <- struct{}{}
		wg.Add(1)
		go func(inst *backends.Instance) {
			defer func() {
				<-sem
				wg.Done()
			}()
			if !c.fetchSingleAgiStatus(inst) {
				atomic.AddInt64(&unreachableCount, 1)
			}
		}(inst)
	}

	wg.Wait()

	if unreachableCount > 0 && c.system != nil && c.system.Backend != nil {
		c.system.Logger.Info("Detected %d unreachable AGI instance(s), refreshing inventory", unreachableCount)
		if _, err := c.forceRefreshInventoryIfAllowed(); err != nil {
			c.system.Logger.Warn("Failed to refresh inventory after AGI unreachable: %s", err)
		}
		// Reschedule expiry timer after inventory refresh
		if c.expiryScheduler != nil {
			c.expiryScheduler.reschedule()
		}
	}
}

// fetchSingleAgiStatus fetches the ingest status for a single AGI instance,
// using the cached auth token. On 401, it invalidates the token and retries once.
// Returns true if the instance was reachable, false if unreachable.
func (c *WebUICmd) fetchSingleAgiStatus(inst *backends.Instance) bool {
	name := inst.ClusterName

	baseURL, err := c.buildAgiBaseURL(inst)
	if err != nil {
		c.logError("AGI status: cannot build URL for %s: %s", name, err)
		c.agiStatus.set(name, "unreachable")
		return false
	}

	token, err := c.getOrCreateAgiToken(name, inst)
	if err != nil {
		c.logError("AGI status: cannot get token for %s: %s", name, err)
		c.agiStatus.set(name, "unreachable")
		return false
	}

	statusMsg, done := c.doAgiStatusRequest(baseURL, token, name)
	if done {
		c.agiStatus.set(name, statusMsg)
		return statusMsg != "unreachable"
	}

	// Got 401 — invalidate token and retry once
	c.agiTokens.remove(name)
	token, err = c.getOrCreateAgiToken(name, inst)
	if err != nil {
		c.logError("AGI status: cannot get new token for %s after 401: %s", name, err)
		c.agiStatus.set(name, "unreachable")
		return false
	}

	statusMsg, done = c.doAgiStatusRequest(baseURL, token, name)
	if done {
		c.agiStatus.set(name, statusMsg)
		return statusMsg != "unreachable"
	}
	c.agiStatus.set(name, "unreachable")
	return false
}

// doAgiStatusRequest makes a single HTTP GET to /agi/status with the given token.
// Returns (statusMsg, true) on success or non-retryable error.
// Returns ("", false) on 401 to signal the caller to retry with a new token.
func (c *WebUICmd) doAgiStatusRequest(baseURL, token, name string) (string, bool) {
	tr := &http.Transport{
		DisableKeepAlives: true,
		IdleConnTimeout:   10 * time.Second,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: !c.AgiStrictTls},
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: tr,
	}
	defer client.CloseIdleConnections()

	req, err := http.NewRequest("GET", baseURL+"/agi/status", nil)
	if err != nil {
		c.logError("AGI status: request build error for %s: %s", name, err)
		return "unreachable", true
	}
	req.AddCookie(&http.Cookie{Name: "AGI_TOKEN", Value: token})
	req.AddCookie(&http.Cookie{Name: "X-AGI-CALLER", Value: "webui"})

	response, err := client.Do(req)
	if err != nil {
		c.logError("AGI status: HTTP error for %s at %s: %s", name, baseURL, err)
		return "unreachable", true
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		return "", false // signal caller to retry with new token
	}

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		c.logError("AGI status: HTTP %d for %s: %s", response.StatusCode, name, string(body))
		return "unreachable", true
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "unknown", true
	}

	statusMsg := mapIngestStatusFromJSON(body)
	return statusMsg, true
}

// mapIngestStatusFromJSON unmarshals the JSON body into IngestStatusStruct
// and maps it to a human-readable status display string.
func mapIngestStatusFromJSON(body []byte) string {
	s := &ingest.IngestStatusStruct{}
	if err := json.Unmarshal(body, s); err != nil {
		return "unknown"
	}
	return mapIngestStatus(s)
}

// mapIngestStatus converts an IngestStatusStruct to a short display string.
// This mirrors the v780 logic from cmdWebInventory.go asyncGetAGIStatus.
func mapIngestStatus(s *ingest.IngestStatusStruct) string {
	hasErrors := len(s.Ingest.Errors) > 0

	var statusMsg string
	switch {
	case !s.GrafanaHelperRunning:
		statusMsg = "ERR: GRAFANAFIX DOWN"
	case !s.PluginRunning:
		statusMsg = "ERR: PLUGIN DOWN"
	case s.Ingest.CompleteSteps == nil || !s.Ingest.CompleteSteps.Init:
		statusMsg = "(1/6) INIT"
	case !s.Ingest.CompleteSteps.Download:
		statusMsg = fmt.Sprintf("(2/6) DOWNLOAD %d%%", s.Ingest.DownloaderCompletePct)
	case !s.Ingest.CompleteSteps.Unpack:
		statusMsg = "(3/6) UNPACK"
	case !s.Ingest.CompleteSteps.PreProcess:
		statusMsg = "(4/6) PRE-PROCESS"
	case !s.Ingest.CompleteSteps.ProcessLogs:
		statusMsg = fmt.Sprintf("(5/6) PROCESS %d%%", s.Ingest.LogProcessorCompletePct)
	case !s.Ingest.CompleteSteps.ProcessCollectInfo:
		statusMsg = "(6/6) COLLECTINFO"
	default:
		statusMsg = "READY"
		if hasErrors {
			statusMsg = "READY, HasErrors"
		}
	}

	if !strings.HasPrefix(statusMsg, "READY") && !s.Ingest.Running {
		statusMsg = "ERR: INGEST DOWN"
	}

	return statusMsg
}

//go:build noagi && !nowebui

package cmd

import (
	"net/http"
	"sync"
)

type agiStatusCache struct {
	mu      sync.RWMutex
	entries map[string]string
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

func (c *agiStatusCache) set(name, status string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[name] = status
}

func (c *agiStatusCache) cleanup(activeNames map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name := range c.entries {
		if !activeNames[name] {
			delete(c.entries, name)
		}
	}
}

type agiMonitor struct{}

func (m *agiMonitor) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
}

func (m *agiMonitor) handle(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
}

func (c *WebUICmd) runAgiStatusLoop() {
	<-c.shutdownChan
}

func (c *WebUICmd) initEmbeddedAgiMonitor() {
	c.agiMonitorInstance = &agiMonitor{}
}

func (c *WebUICmd) runAgiMonitorBanCleanup() {
	<-c.shutdownChan
}

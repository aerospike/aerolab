package cmd

import (
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/notifier"
)

// agiSizingStateMap tracks which AGI instances are currently being sized by the
// embedded monitor. The Status column in the AGI inventory shows "SIZING" for
// any instance whose name appears in this map.
type agiSizingStateMap struct {
	mu      sync.RWMutex
	entries map[string]string // AGI name -> sizing action (e.g. "sizing-capacity", "sizing-ram", "sizing-disk")
}

func (s *agiSizingStateMap) init() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]string)
	}
}

func (s *agiSizingStateMap) set(name, action string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[name] = action
}

func (s *agiSizingStateMap) clear(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, name)
}

func (s *agiSizingStateMap) get(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entries[name]
}

// initEmbeddedAgiMonitor initializes the embedded AGI monitor when
// --agi-monitor-enable is set. It creates the agiMonitor instance that shares
// the WebUI's *System (and therefore its backend / inventory). Call this after
// the mux has been created but before the server starts listening.
func (c *WebUICmd) initEmbeddedAgiMonitor() {
	// Initialize locks on the AgiMonitorConfigCmd if not already set
	if c.AgiMonitor.invLock == nil {
		c.AgiMonitor.invLock = new(sync.Mutex)
	}
	if c.AgiMonitor.execLock == nil {
		c.AgiMonitor.execLock = new(sync.Mutex)
	}

	// Initialize notifier
	c.AgiMonitor.notifier = &notifier.HTTPSNotify{
		Endpoint:     c.AgiMonitor.NotifyURL,
		Headers:      []string{c.AgiMonitor.NotifyHeader},
		SlackToken:   c.AgiMonitor.SlackToken,
		SlackChannel: c.AgiMonitor.SlackChannel,
		SlackEvents:  "INSTANCE_SIZING_DISK_RAM,INSTANCE_SIZING_DISK,INSTANCE_SIZING_RAM,INSTANCE_SPOT_CAPACITY",
	}
	c.AgiMonitor.notifier.Init()

	// Initialize sizing state tracker
	c.agiSizingState.init()

	// Create monitor instance sharing the WebUI's system
	c.agiMonitorInstance = &agiMonitor{
		cmd:         &c.AgiMonitor,
		system:      c.system,
		cache:       &inventoryCache{},
		banTracker:  newIPBanTracker(),
		sizingState: &c.agiSizingState,
		onRefreshCallback: func() {
			// Reschedule expiry timer after monitor forces an inventory refresh
			if c.expiryScheduler != nil {
				c.expiryScheduler.reschedule()
			}
		},
	}
}

// runAgiMonitorBanCleanup runs the ban tracker cleanup loop for the embedded
// AGI monitor. It mirrors the cleanup goroutine from the standalone monitor's
// Execute but respects the WebUI shutdownChan instead of the shutdown package.
func (c *WebUICmd) runAgiMonitorBanCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-c.shutdownChan:
			return
		case <-ticker.C:
			if c.agiMonitorInstance == nil {
				return
			}
			// Remove expired bans and log removals
			c.agiMonitorInstance.banTracker.Lock()
			now := time.Now()
			for ip, expiry := range c.agiMonitorInstance.banTracker.bans {
				if now.After(expiry) {
					c.system.Logger.Info("AGI Monitor: IP ban expired, removing: %s", ip)
					delete(c.agiMonitorInstance.banTracker.bans, ip)
				}
			}
			c.agiMonitorInstance.banTracker.Unlock()
			c.agiMonitorInstance.banTracker.cleanup()
		}
	}
}

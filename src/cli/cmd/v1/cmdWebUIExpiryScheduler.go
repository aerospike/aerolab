//go:build !nowebui

package cmd

import (
	"log"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

// expiryRefreshScheduler schedules a ForceRefreshInventory at the next
// instance/volume expiry time. After every ForceRefreshInventory (from any
// source: poll timer, AGI unreachable detection, monitor action, job
// completion, or the scheduler itself), call reschedule() to scan the
// inventory for the nearest future expiry and arm a one-shot timer.
type expiryRefreshScheduler struct {
	mu     sync.Mutex
	timer  *time.Timer
	system *System
	webui  *WebUICmd
}

// reschedule scans the current inventory for the nearest future expiry time
// and arms a one-shot timer that will call ForceRefreshInventory at that time.
// Any previously armed timer is cancelled first.
func (s *expiryRefreshScheduler) reschedule() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Cancel any pending timer
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}

	if s.system == nil || s.system.Backend == nil {
		return
	}

	// Scan inventory for nearest expiry
	inventory := s.system.Backend.GetInventory()
	if inventory == nil {
		return
	}

	nearest := s.findNearestExpiry(inventory)
	if nearest.IsZero() {
		return // nothing expires
	}

	delay := time.Until(nearest)
	if delay <= 0 {
		// Already expired — refresh immediately (in a goroutine to avoid blocking)
		go s.doRefreshAndReschedule()
		return
	}

	// Add a small buffer (5 seconds) to ensure the expiry action has completed
	delay += 5 * time.Second

	s.timer = time.AfterFunc(delay, func() {
		s.doRefreshAndReschedule()
	})

	log.Printf("Expiry scheduler: next refresh in %s (at %s)", delay.Round(time.Second), nearest.Format(time.RFC3339))
}

// doRefreshAndReschedule forces a backend inventory refresh and then
// reschedules for the next expiry time. This is the callback used by the
// one-shot timer. Respects minimumIntervalDur rate-limiting.
func (s *expiryRefreshScheduler) doRefreshAndReschedule() {
	if s.webui == nil {
		return
	}
	if didRefresh, err := s.webui.forceRefreshInventoryIfAllowed(); err != nil {
		log.Printf("Expiry scheduler: ForceRefreshInventory failed: %s", err)
	} else if didRefresh {
		log.Printf("Expiry scheduler: inventory refreshed at scheduled expiry time")
	}
	s.reschedule() // scan new inventory, schedule next
}

// findNearestExpiry scans all instances and volumes for the earliest future
// expiry time. Returns zero time if nothing has a future expiry.
func (s *expiryRefreshScheduler) findNearestExpiry(inv *backends.Inventory) time.Time {
	var nearest time.Time
	now := time.Now()

	// Scan instances
	for _, inst := range inv.Instances.Describe() {
		if !inst.Expires.IsZero() && inst.Expires.After(now) {
			if nearest.IsZero() || inst.Expires.Before(nearest) {
				nearest = inst.Expires
			}
		}
	}

	// Scan volumes
	for _, vol := range inv.Volumes.Describe() {
		if !vol.Expires.IsZero() && vol.Expires.After(now) {
			if nearest.IsZero() || vol.Expires.Before(nearest) {
				nearest = vol.Expires
			}
		}
	}

	return nearest
}

package cmd

import "sync"

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

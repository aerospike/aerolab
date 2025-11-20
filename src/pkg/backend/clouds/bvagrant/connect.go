package bvagrant

import (
	"fmt"
	"sync"
)

// vagrantStateCache caches Vagrant VM state information to reduce
// the number of `vagrant status` calls which can be expensive
type vagrantStateCache struct {
	cache map[string]*vagrantVMState // key: vmID
	mu    sync.RWMutex
}

type vagrantVMState struct {
	Name      string
	State     string // running, poweroff, saved, aborted, not_created, etc.
	Provider  string
	Directory string
}

func newVagrantStateCache() *vagrantStateCache {
	return &vagrantStateCache{
		cache: make(map[string]*vagrantVMState),
	}
}

func (c *vagrantStateCache) set(vmID string, state *vagrantVMState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[vmID] = state
}

func (c *vagrantStateCache) get(vmID string) (*vagrantVMState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	state, ok := c.cache[vmID]
	return state, ok
}

func (c *vagrantStateCache) delete(vmID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, vmID)
}

func (c *vagrantStateCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*vagrantVMState)
}

// getVagrantWorkDir returns the working directory for a Vagrant region.
// If the region is not defined or workDir is empty, it returns the default work directory.
//
// Parameters:
//   - region: the region name
//
// Returns:
//   - string: the working directory path
//   - error: nil on success, or an error if region is not found
func (s *b) getVagrantWorkDir(region string) (string, error) {
	if region == "" || region == "default" {
		return s.workDir, nil
	}

	if s.credentials == nil {
		return "", fmt.Errorf("credentials not configured")
	}

	regionDefinition, ok := s.credentials.Regions[region]
	if !ok {
		return "", fmt.Errorf("region %s not found", region)
	}

	if regionDefinition.WorkDir != "" {
		return regionDefinition.WorkDir, nil
	}

	return s.workDir, nil
}

// getVagrantProvider returns the Vagrant provider to use (virtualbox, vmware_desktop, libvirt, etc).
//
// Returns:
//   - string: the provider name
func (s *b) getVagrantProvider() string {
	if s.credentials != nil && s.credentials.Provider != "" {
		return s.credentials.Provider
	}
	return "virtualbox" // default
}

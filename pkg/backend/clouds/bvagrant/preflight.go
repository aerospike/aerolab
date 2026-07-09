package bvagrant

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/file"
	"github.com/aerospike/aerolab/pkg/utils/versions"
)

// minVagrantVersion is the minimum vagrant version this backend supports.
const minVagrantVersion = "2.4.0"

const preflightCacheFileName = "preflight.json"

// preflightResult records the outcome of an environment check for the vagrant backend:
// binary presence/version, installed plugins, detected providers, and any issues found.
type preflightResult struct {
	CheckedAt      time.Time `json:"checkedAt"`
	VagrantVersion string    `json:"vagrantVersion"`
	Plugins        []string  `json:"plugins"`
	Providers      []string  `json:"providers"`
	OK             bool      `json:"ok"`
	Issues         []string  `json:"issues"`
}

func (s *b) preflightCacheFile() string {
	return filepath.Join(s.configDir, preflightCacheFileName)
}

// Preflight checks that vagrant is installed, meets the minimum version, and that at
// least the configured default provider (if any) is available. Results are cached to
// <configDir>/preflight.json; only a fully OK result is cached, so a failing environment
// is always re-checked on the next non-forced call rather than sticking around stale.
// force=true always bypasses the cache and re-runs (and rewrites) the check.
func (s *b) Preflight(force bool) (*preflightResult, error) {
	if s.lookPath == nil {
		s.lookPath = exec.LookPath
	}

	if !force {
		cached, ok, err := s.loadPreflightCache()
		if err != nil {
			return nil, err
		}
		if ok {
			return cached, nil
		}
	}

	result := &preflightResult{CheckedAt: time.Now()}

	binaryPath := "vagrant"
	if s.credentials != nil && s.credentials.BinaryPath != "" {
		binaryPath = s.credentials.BinaryPath
	}

	if _, err := s.lookPath(binaryPath); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf(
			"vagrant binary %q not found: install it from https://developer.hashicorp.com/vagrant/install", binaryPath))
	} else if s.runner != nil {
		ver, err := s.runner.Version()
		if err != nil {
			result.Issues = append(result.Issues, fmt.Sprintf("failed to determine vagrant version: %v", err))
		} else {
			result.VagrantVersion = ver
			if versions.Compare(ver, minVagrantVersion) < 0 {
				result.Issues = append(result.Issues, fmt.Sprintf(
					"vagrant version %s found, but %s or newer is required", ver, minVagrantVersion))
			}
		}
	}

	var plugins []string
	if s.runner != nil {
		var err error
		plugins, err = s.runner.PluginList()
		if err != nil {
			result.Issues = append(result.Issues, fmt.Sprintf("failed to list vagrant plugins: %v", err))
		}
	}
	result.Plugins = plugins

	providers := s.detectProviders(plugins)
	result.Providers = providers

	if s.credentials != nil && s.credentials.DefaultProvider != "" {
		if !containsStr(providers, s.credentials.DefaultProvider) {
			result.Issues = append(result.Issues, fmt.Sprintf(
				"configured default provider %q was not detected (detected providers: %v)",
				s.credentials.DefaultProvider, providers))
		}
	}

	result.OK = len(result.Issues) == 0

	if result.OK {
		if err := s.savePreflightCache(result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// detectProviders reports which vagrant providers appear usable on this system.
// virtualbox needs VBoxManage on PATH; libvirt needs both virsh on PATH and the
// vagrant-libvirt plugin installed; vmware_desktop needs the vagrant-vmware-desktop
// plugin; hyperv is only ever reported on Windows.
func (s *b) detectProviders(plugins []string) []string {
	var providers []string

	if _, err := s.lookPath("VBoxManage"); err == nil {
		providers = append(providers, "virtualbox")
	}
	if _, err := s.lookPath("virsh"); err == nil && containsStr(plugins, "vagrant-libvirt") {
		providers = append(providers, "libvirt")
	}
	if containsStr(plugins, "vagrant-vmware-desktop") {
		providers = append(providers, "vmware_desktop")
	}
	if runtime.GOOS == "windows" {
		providers = append(providers, "hyperv")
	}

	return providers
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// loadPreflightCache reads a cached preflight result. A missing file, or a cached
// result whose OK is false, is reported as "no usable cache" (ok=false, err=nil) rather
// than an error — callers should fall through to running a fresh check. A cache file
// that exists but cannot be parsed is a real error.
func (s *b) loadPreflightCache() (*preflightResult, bool, error) {
	raw, err := os.ReadFile(s.preflightCacheFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	result := &preflightResult{}
	if err := json.Unmarshal(raw, result); err != nil {
		return nil, false, fmt.Errorf("corrupt preflight cache: %w", err)
	}
	if !result.OK {
		return nil, false, nil
	}
	return result, true, nil
}

func (s *b) savePreflightCache(result *preflightResult) error {
	if err := os.MkdirAll(s.configDir, 0755); err != nil {
		return err
	}
	return file.StoreJSON(s.preflightCacheFile(), ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, result)
}

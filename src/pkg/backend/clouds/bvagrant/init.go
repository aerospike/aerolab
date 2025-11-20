package bvagrant

import (
	"encoding/json"
	"os"
	"path"
	"slices"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/utils/counters"
	"github.com/aerospike/aerolab/pkg/utils/file"
	"github.com/rglonek/logger"
)

type b struct {
	configDir           string
	credentials         *clouds.VAGRANT
	project             string
	sshKeysDir          string
	log                 *logger.Logger
	aerolabVersion      string
	networks            backends.NetworkList
	firewalls           backends.FirewallList
	instances           backends.InstanceList
	volumes             backends.VolumeList
	images              backends.ImageList
	workDir             string
	invalidateCacheFunc func(names ...string) error
	listAllProjects     bool
	createInstanceCount *counters.Int
	regions             []string // used as vagrant region definitions
	vagrantCache        *vagrantStateCache
}

func init() {
	backends.RegisterBackend(backends.BackendTypeVagrant, &b{})
}

// SetInventory sets the current inventory from the backend cache.
// This is called by the backend manager after inventory refresh.
func (s *b) SetInventory(networks backends.NetworkList, firewalls backends.FirewallList, instances backends.InstanceList, volumes backends.VolumeList, images backends.ImageList) {
	s.networks = networks
	s.firewalls = firewalls
	s.instances = instances
	s.volumes = volumes
	s.images = images
}

// SetConfig initializes the Vagrant backend configuration.
//
// Parameters:
//   - dir: configuration directory for storing region and state data
//   - credentials: cloud credentials containing Vagrant provider settings
//   - project: project name for resource tagging
//   - sshKeyDir: directory for SSH key storage
//   - log: logger instance
//   - aerolabVersion: current aerolab version
//   - workDir: working directory for Vagrant operations
//   - invalidateCacheFunc: callback to invalidate backend cache
//   - listAllProjects: if true, list resources from all projects
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (s *b) SetConfig(dir string, credentials *clouds.Credentials, project string, sshKeyDir string, log *logger.Logger, aerolabVersion string, workDir string, invalidateCacheFunc func(names ...string) error, listAllProjects bool) error {
	s.configDir = dir
	if credentials != nil {
		s.credentials = &credentials.VAGRANT
	}
	s.project = project
	s.sshKeysDir = sshKeyDir
	s.log = log
	s.aerolabVersion = aerolabVersion
	s.workDir = workDir
	s.invalidateCacheFunc = invalidateCacheFunc
	s.listAllProjects = listAllProjects
	s.createInstanceCount = counters.NewInt(0)
	s.vagrantCache = newVagrantStateCache()

	// read regions
	err := s.setConfigRegions()
	if err != nil {
		return err
	}

	return nil
}

func (s *b) setConfigRegions() error {
	regionsFile := path.Join(s.configDir, "regions.json")
	s.log.Detail("setConfigRegions: looking for %s", regionsFile)
	_, err := os.Stat(regionsFile)
	if err != nil && !os.IsNotExist(err) {
		// error reading
		return err
	}
	if err != nil {
		// file does not exist
		s.log.Detail("setConfigRegions: %s does not exist, not parsing", regionsFile)
		return nil
	}
	// read
	f, err := os.Open(regionsFile)
	if err != nil {
		return err
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&s.regions)
	if err != nil {
		return err
	}
	s.log.Detail("setConfigRegions: result=%v", s.regions)
	return nil
}

// ListEnabledZones returns the list of enabled Vagrant regions.
//
// Returns:
//   - []string: list of region names
//   - error: nil on success, or an error describing what failed
func (s *b) ListEnabledZones() ([]string, error) {
	return s.regions, nil
}

// EnableZones enables additional Vagrant regions.
//
// Parameters:
//   - names: region names to enable
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (s *b) EnableZones(names ...string) error {
	regions, err := s.ListEnabledZones()
	if err != nil {
		return err
	}
	for _, r := range names {
		if slices.Contains(regions, r) {
			continue
		}
		regions = append(regions, r)
	}
	s.regions = regions
	return file.StoreJSON(path.Join(s.configDir, "regions.json"), ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, regions)
}

// DisableZones disables specified Vagrant regions.
//
// Parameters:
//   - names: region names to disable
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (s *b) DisableZones(names ...string) error {
	currentRegions, err := s.ListEnabledZones()
	if err != nil {
		return err
	}
	regions := []string{}
	for _, r := range currentRegions {
		if slices.Contains(names, r) {
			continue
		}
		regions = append(regions, r)
	}
	s.regions = regions
	return file.StoreJSON(path.Join(s.configDir, "regions.json"), ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, regions)
}

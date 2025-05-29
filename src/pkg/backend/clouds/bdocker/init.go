package bdocker

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/counters"
	"github.com/aerospike/aerolab/pkg/file"
	"github.com/rglonek/logger"
)

type b struct {
	configDir           string
	credentials         *clouds.DOCKER
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
	regions             []string // used as docker host definitions
	builders            map[string]map[string]*dockerBuilder
	builderMutex        sync.Mutex
	usedPorts           *usedPorts
	isPodman            map[string]bool // region -> true if podman is used
}

func init() {
	backends.RegisterBackend(backends.BackendTypeDocker, &b{})
}

func (s *b) SetInventory(networks backends.NetworkList, firewalls backends.FirewallList, instances backends.InstanceList, volumes backends.VolumeList, images backends.ImageList) {
	s.networks = networks
	s.firewalls = firewalls
	s.instances = instances
	s.volumes = volumes
	s.images = images
	s.usedPorts.reset(s.instances)
}

func (s *b) SetConfig(dir string, credentials *clouds.Credentials, project string, sshKeyDir string, log *logger.Logger, aerolabVersion string, workDir string, invalidateCacheFunc func(names ...string) error, listAllProjects bool) error {
	s.configDir = dir
	if credentials != nil {
		s.credentials = &credentials.DOCKER
	}
	s.project = project
	s.sshKeysDir = sshKeyDir
	s.log = log
	s.aerolabVersion = aerolabVersion
	s.workDir = workDir
	s.invalidateCacheFunc = invalidateCacheFunc
	s.listAllProjects = listAllProjects
	s.createInstanceCount = counters.NewInt(0)
	s.builders = make(map[string]map[string]*dockerBuilder)
	s.usedPorts = &usedPorts{}
	// read regions
	err := s.setConfigRegions()
	if err != nil {
		return err
	}
	s.isPodman = make(map[string]bool)
	for _, region := range s.regions {
		s.isPodman[region] = s.testPodman(region)
	}
	return nil
}

func (s *b) testPodman(region string) bool {
	cli, err := s.getDockerClient(region)
	if err != nil {
		return false
	}
	version, err := cli.ServerVersion(context.Background())
	if err != nil {
		s.log.Warn("DOCKER: testing whether podman or docker is used for region=%s, error=%v", region, err)
		return false
	}
	for _, c := range version.Components {
		if strings.Contains(strings.ToUpper(c.Name), "PODMAN") {
			return true
		}
	}
	return false
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

func (s *b) ListEnabledZones() ([]string, error) {
	return s.regions, nil
}

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

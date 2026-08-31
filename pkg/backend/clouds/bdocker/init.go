package bdocker

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/aerospike-community/aerolab/pkg/backend/backends"
	"github.com/aerospike-community/aerolab/pkg/backend/clouds"
	"github.com/aerospike-community/aerolab/pkg/sshexec"
	"github.com/aerospike-community/aerolab/pkg/utils/counters"
	"github.com/aerospike-community/aerolab/pkg/utils/file"
	"github.com/moby/moby/client"
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
	hostKeys            *sshexec.HostKeyStore
	hostKeysStrict      bool
}

func init() {
	backends.RegisterBackend(backends.BackendTypeDocker, &b{})
}

func (s *b) SetHostKeyPolicy(store *sshexec.HostKeyStore, strict bool) {
	s.hostKeys = store
	s.hostKeysStrict = strict
}

// SetIdentity is a no-op: docker containers are reached over the local docker
// socket, so there is no cloud firewall to lock to a caller address.
func (s *b) SetIdentity(identity *backends.Identity) {}

// EnsureCallerAccess is a no-op for the same reason as SetIdentity.
func (s *b) EnsureCallerAccess(instances backends.InstanceList) {}

// applyHostKeyPolicy points an SSH client config at the host key store so the
// connection is verified against the key previously learned for this instance.
func (s *b) applyHostKeyPolicy(clientConf *sshexec.ClientConf, i *backends.Instance) {
	if s.hostKeys == nil {
		return
	}
	id := i.HostKeyID()
	if id == "" {
		return
	}
	clientConf.HostKeyStore = s.hostKeys
	clientConf.HostKeyID = id
	clientConf.HostKeyStrict = s.hostKeysStrict
	if s.log != nil {
		clientConf.HostKeyLogf = s.log.Warn
	}
}

// forgetHostKeys drops remembered host keys for the given instances. Called
// when instances are created or terminated, so a node number that comes back
// on a fresh container is learned again instead of tripping a mismatch.
func (s *b) forgetHostKeys(ids ...string) {
	if s.hostKeys == nil || len(ids) == 0 {
		return
	}
	if err := s.hostKeys.Forget(ids...); err != nil && s.log != nil {
		s.log.Warn("Could not update the SSH host key store: %s", err)
	}
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
	version, err := cli.ServerVersion(context.Background(), client.ServerVersionOptions{})
	if err != nil {
		// The Moby client refuses to negotiate below API 1.44 (Docker Engine
		// 25+). Podman only advertises that from 5.8 onwards, so an older
		// Podman fails here and would silently be treated as Docker.
		if strings.Contains(err.Error(), "is not supported by this client") {
			s.log.Warn("DOCKER: region=%s rejected API version negotiation (%v); Docker Engine 25+ or a Podman that advertises Docker API 1.44+ is required", region, err)
			return false
		}
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
		// No file means nothing is enabled. This backend is a process-global
		// singleton, so leaving s.regions as-is would carry regions over from a
		// previously configured root dir. EnableZones writes the file
		// synchronously, so the file is the source of truth.
		s.log.Detail("setConfigRegions: %s does not exist, clearing enabled regions", regionsFile)
		s.regions = nil
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

func (s *b) ListAvailableZones() ([]string, error) {
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

package bvagrant

import (
	"errors"
	"os/exec"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/rglonek/logger"
)

const defaultSubnet = "192.168.56.0/24"

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
	runner              runner
	clusterLocks        map[string]*sync.Mutex
	clusterLocksMu      sync.Mutex
	lookPath            func(string) (string, error)
	// sshReadyPoll, if set, replaces defaultSSHReadyPoll (used by CreateInstances to wait
	// for freshly created nodes to become ssh-reachable). Tests override this to avoid
	// depending on real SSH/network; production leaves it nil to get the real behavior.
	sshReadyPoll func(instances backends.InstanceList, waitDur time.Duration) error
}

func init() {
	backends.RegisterBackend(backends.BackendTypeVagrant, &b{})
}

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
	s.clusterLocks = make(map[string]*sync.Mutex)
	s.lookPath = exec.LookPath

	// Default subnet if empty
	if s.credentials != nil && s.credentials.Subnet == "" {
		s.credentials.Subnet = defaultSubnet
	}

	if s.credentials != nil {
		s.runner = &realRunner{binaryPath: s.credentials.BinaryPath, log: log}
	}

	return nil
}

func (s *b) SetInventory(networks backends.NetworkList, firewalls backends.FirewallList, instances backends.InstanceList, volumes backends.VolumeList, images backends.ImageList) {
	s.networks = networks
	s.firewalls = firewalls
	s.instances = instances
	s.volumes = volumes
	s.images = images
}

func (s *b) ListEnabledZones() ([]string, error) {
	return []string{"local"}, nil
}

func (s *b) ListAvailableZones() ([]string, error) {
	return []string{"local"}, nil
}

func (s *b) EnableZones(names ...string) error {
	return errors.New("vagrant backend has a single fixed zone \"local\"")
}

func (s *b) DisableZones(names ...string) error {
	return errors.New("vagrant backend has a single fixed zone \"local\"")
}

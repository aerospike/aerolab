package backends

import (
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/lithammer/shortuuid"
	"github.com/rglonek/logger"

	"github.com/aerospike/aerolab/pkg/backend/cache"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
)

type Config struct {
	RootDir          string              `yaml:"RootDir" json:"RootDir"`
	Cache            bool                `yaml:"Cache" json:"Cache"`
	Credentials      *clouds.Credentials `yaml:"Credentials" json:"Credentials"`
	LogLevel         logger.LogLevel     `yaml:"logLevel" json:"logLevel"`
	LogMillisecond   bool                `yaml:"logMillisecond" json:"logMillisecond"`
	AerolabVersion   string              `yaml:"aerolabVersion" json:"aerolabVersion"`
	ListAllProjects  bool                `yaml:"listAllProjects" json:"listAllProjects"`
	CustomSSHKeyPath string              `yaml:"customSSHKeyPath" json:"customSSHKeyPath"`
}

type cacheMetadata struct {
	CacheUpdateTimestamp time.Time `json:"cacheUpdateTimestamp"`
}

var cloudList = make(map[BackendType]Cloud)

func RegisterBackend(name BackendType, c Cloud) {
	cloudList[name] = c
}

func (b *backend) AddRegion(backendType BackendType, names ...string) error {
	if _, ok := cloudList[backendType]; !ok {
		return fmt.Errorf("backend type %s not found", backendType)
	}
	err := cloudList[backendType].EnableZones(names...)
	if err != nil {
		return err
	}
	if !b.cache.Enabled {
		return nil
	}
	b.pollLock.Lock()
	defer b.pollLock.Unlock()
	b.invalidatedLock.Lock()
	defer b.invalidatedLock.Unlock()
	b.invalidated = CacheInvalidateAll
	b.cache.Delete()
	return nil
}

func (b *backend) RemoveRegion(backendType BackendType, names ...string) error {
	if _, ok := cloudList[backendType]; !ok {
		return fmt.Errorf("backend type %s not found", backendType)
	}
	err := cloudList[backendType].DisableZones(names...)
	if err != nil {
		return err
	}
	if !b.cache.Enabled {
		return nil
	}
	b.pollLock.Lock()
	defer b.pollLock.Unlock()
	b.invalidatedLock.Lock()
	defer b.invalidatedLock.Unlock()
	b.invalidated = CacheInvalidateAll
	b.cache.Delete()
	return nil
}

func (b *backend) ListEnabledRegions(backendType BackendType) (name []string, err error) {
	if _, ok := cloudList[backendType]; !ok {
		return nil, fmt.Errorf("backend type %s not found", backendType)
	}
	return cloudList[backendType].ListEnabledZones()
}

func (b *backend) ListAvailableZones(backendType BackendType) (zones []string, err error) {
	if _, ok := cloudList[backendType]; !ok {
		return nil, fmt.Errorf("backend type %s not found", backendType)
	}
	return cloudList[backendType].ListAvailableZones()
}

func (b *backend) pollTimer() {
	log := b.log.WithPrefix("pollTimer")
	for {
		log.Debug("Sleeping for 1 hour")
		sleepEnd := time.Now().Add(time.Hour)
		for {
			time.Sleep(time.Second)
			if time.Now().After(sleepEnd) {
				break
			}
			if b.closed {
				return
			}
		}
		log.Debug("Waking up")
		b.pollLock.Lock()
		log.Debug("Lock obtained, inventory refresh started")
		errs := b.poll(nil)
		b.pollLock.Unlock()
		for _, err := range errs {
			b.log.Error("Inventory refresh failure: %s", err)
		}
		if len(errs) == 0 {
			log.Debug("Inventory refresh completed successfully")
		}
	}
}
func (b *backend) ForceRefreshInventory() error {
	log := b.log.WithPrefix("ForceRefreshInventory job=" + shortuuid.New() + " ")
	log.Debug("Starting inventory refresh")
	b.pollLock.Lock()
	defer b.pollLock.Unlock()
	log.Debug("Lock obtained, inventory refresh started")
	errs := b.poll(nil)
	if errs == nil {
		return nil
	}
	var errstring string
	for _, e := range errs {
		if errstring != "" {
			errstring = errstring + " ;; "
		}
		errstring = errstring + e.Error()
	}
	return errors.New(errstring)
}

func InternalNew(project string, c *Config, pollInventoryHourly bool, enabledBackends []BackendType, setInventory *Inventory) (Backend, error) {
	if project == "" {
		return nil, errors.New("project name cannot be empty")
	}
	b := getBackendObject(project, c)
	b.log = logger.NewLogger()
	b.log.SetLogLevel(c.LogLevel)
	b.log.SetPrefix("BACKEND ")
	b.log.MillisecondLogging(c.LogMillisecond)
	b.enabledBackends = make(map[BackendType]Cloud)
	for _, cname := range enabledBackends {
		cloud, ok := cloudList[cname]
		if !ok {
			return nil, fmt.Errorf("backend type %s not found", cname)
		}
		b.enabledBackends[cname] = cloud
		sshKeyPath := path.Join(c.RootDir, project, "ssh-keys", string(cname))
		if c.CustomSSHKeyPath != "" {
			sshKeyPath = path.Join(c.CustomSSHKeyPath, project, "ssh-keys", string(cname))
		}
		err := cloud.SetConfig(path.Join(c.RootDir, project, "config", string(cname)), c.Credentials, project, sshKeyPath, b.log.WithPrefix(string(cname)+" "), c.AerolabVersion, path.Join(c.RootDir, project, "workdir", string(cname)), b.invalidate, c.ListAllProjects)
		if err != nil {
			return nil, err
		}
	}
	b.cache = &cache.Cache{
		Enabled: b.config.Cache,
		Dir:     path.Join(c.RootDir, project, "cache"),
	}
	// if we are told to set the inventory to a specific value
	if setInventory != nil {
		b.setInventory(setInventory)
		for cname, cloud := range b.enabledBackends {
			cloud.SetInventory(b.networks[cname], b.firewalls[cname], b.instances[cname], b.volumes[cname], b.images[cname])
		}
		if pollInventoryHourly {
			go b.pollTimer()
		}
		return b, nil
	}
	// if we have disk caching enabled, load the cache
	if b.config.Cache {
		err := b.loadCache()
		if err == nil {
			for cname, cloud := range b.enabledBackends {
				cloud.SetInventory(b.networks[cname], b.firewalls[cname], b.instances[cname], b.volumes[cname], b.images[cname])
			}
			if pollInventoryHourly {
				go b.pollTimer()
			}
			return b, nil
		}
		if err != cache.ErrNoCacheFile {
			b.log.Warn("Could not load cache files: %s", err)
		}
	}
	// force refresh the inventory as we do not have one yet
	err := b.ForceRefreshInventory()
	if err != nil {
		return b, err
	}
	if pollInventoryHourly {
		go b.pollTimer()
	}
	return b, nil
}

func (b *backend) Close() error {
	b.closed = true
	return nil
}

func (b *backend) MigrateV7Resources(backendType BackendType, input *MigrateV7Input) (*MigrationResult, error) {
	if _, ok := cloudList[backendType]; !ok {
		return nil, fmt.Errorf("backend type %s not found", backendType)
	}
	return cloudList[backendType].MigrateV7Resources(input)
}

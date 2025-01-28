package backend

import (
	"errors"
	"path"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/cache"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/rglonek/logger"
)

type Config struct {
	RootDir     string              `yaml:"RootDir" json:"RootDir"`
	Cache       bool                `yaml:"Cache" json:"Cache"`
	Credentials *clouds.Credentials `yaml:"Credentials" json:"Credentials"`
	LogLevel    logger.LogLevel     `yaml:"logLevel" json:"logLevel"`
}

type cacheMetadata struct {
	CacheUpdateTimestamp time.Time `json:"cacheUpdateTimestamp"`
}

var cloudList = make(map[BackendType]Cloud)

func RegisterBackend(name BackendType, c Cloud) {
	cloudList[name] = c
}

func (b *backend) AddRegion(backendType BackendType, names ...string) error {
	err := cloudList[backendType].EnableZones(names...)
	if err != nil {
		return err
	}
	if !b.cache.Enabled {
		return nil
	}
	b.cache.Invalidate()
	return b.ForceRefreshInventory()
}

func (b *backend) RemoveRegion(backendType BackendType, names ...string) error {
	err := cloudList[backendType].DisableZones(names...)
	if err != nil {
		return err
	}
	if !b.cache.Enabled {
		return nil
	}
	b.cache.Invalidate()
	return b.ForceRefreshInventory()
}

func (b *backend) ListEnabledRegions(backendType BackendType) (name []string, err error) {
	return cloudList[backendType].ListEnabledZones()
}
func (b *backend) pollTimer() {
	for {
		time.Sleep(time.Hour)
		errs := b.poll()
		for _, err := range errs {
			b.log.Error("Inventory refresh failure: %s", err)
		}
	}
}
func (b *backend) ForceRefreshInventory() error {
	errs := b.poll()
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

func Init(project string, c *Config, pollInventoryHourly bool) (Backend, error) {
	if project == "" {
		return nil, errors.New("project name cannot be empty")
	}
	b := getBackendObject(project, c)
	for cname, cloud := range cloudList {
		err := cloud.SetConfig(path.Join(c.RootDir, project, "config", string(cname)), c.Credentials, project, path.Join(c.RootDir, project, "ssh-keys", string(cname)))
		if err != nil {
			return nil, err
		}
	}
	b.log = logger.NewLogger()
	b.log.SetLogLevel(c.LogLevel)
	b.log.SetPrefix("BACKEND ")
	b.cache = &cache.Cache{
		Enabled: b.config.Cache,
		Dir:     path.Join(c.RootDir, project, "cache"),
	}
	if b.config.Cache {
		err := b.loadCache()
		if err == nil {
			if pollInventoryHourly {
				go b.pollTimer()
			}
			return b, nil
		}
		if err != cache.ErrNoCacheFile {
			b.log.Warn("Could not load cache files: %s", err)
		}
	}
	err := b.ForceRefreshInventory()
	if err != nil {
		return b, err
	}
	if pollInventoryHourly {
		go b.pollTimer()
	}
	return b, nil
}

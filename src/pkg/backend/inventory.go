package backend

import (
	"errors"
	"log"
	"sync"
	"time"
)

type Inventory struct {
	Volumes   Volumes   // permanent volumes which do not go away (not tied to instance lifetime), be it EFS, or EBS/pd-ssd
	Instances Instances // all instances, clusters, clients, whatever
	//TODO Images    Images    // images - used for templates; always prefilled with supported OS template images found in the backends, and then with customer image templates on top
	//TODO Firewalls Firewalls // AWS security groups, GCP firewalls
	//TODO Networks  Networks  // VPCs, Subnets
	//TODO Expiries  Expiries  // Expiry system: to be made obsolete in the future by using tiny instances with aerolab on them for expiries instead - removing complexity
}

type StorageSize int64

const (
	StorageKiB StorageSize = 1024
	StorageMiB StorageSize = StorageKiB * 1024
	StorageGiB StorageSize = StorageMiB * 1024
	StorageTiB StorageSize = StorageGiB * 1024
	StorageKB  StorageSize = 1000
	StorageMB  StorageSize = StorageKB * 1000
	StorageGB  StorageSize = StorageMB * 1000
	StorageTB  StorageSize = StorageGB * 1000
)

func (b *backend) GetInventory() (*Inventory, error) {
	volumes := VolumeList{}
	for _, v := range b.volumes {
		volumes = append(volumes, v...)
	}
	instances := InstanceList{}
	for _, v := range b.instances {
		instances = append(instances, v...)
	}
	return &Inventory{
		Volumes:   volumes,
		Instances: instances,
	}, nil
}

type Backend interface {
	GetInventory() (*Inventory, error)
	AddRegion(names ...string) error
	RemoveRegion(names ...string) error
	ListEnabledRegions() (name []string, err error)
	ForceRefreshInventory() error
}

type backend struct {
	volumes   map[BackendType]VolumeList
	instances map[BackendType]InstanceList
	pollLock  *sync.Mutex
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

func (b *backend) pollTimer() {
	for {
		time.Sleep(time.Hour)
		errs := b.poll()
		for _, err := range errs {
			log.Printf("Inventory refresh failure: %s", err)
		}
	}
}

func (b *backend) AddRegion(names ...string) error {
	return nil
}

func (b *backend) RemoveRegion(names ...string) error {
	return nil
}

func (b *backend) ListEnabledRegions() (name []string, err error) {
	return nil, nil
}

// Initialize backends
// TODO: configuration on init - configure backends and their regions/zones
// TODO: we need backend functions to enable,disable,add,remove backends and their regions/zones
// TODO: if caching enabled, load cache instead of running ForceRefreshInventory
func Init(project string, c *Credentials) (Backend, error) {
	b := &backend{
		volumes:   make(map[BackendType]VolumeList),
		instances: make(map[BackendType]InstanceList),
		pollLock:  new(sync.Mutex),
	}
	err := b.ForceRefreshInventory()
	if err != nil {
		return b, err
	}
	go b.pollTimer()
	return b, nil
}

type Credentials struct {
	// TODO: credentials details go here
}

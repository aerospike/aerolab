package backend

import (
	"errors"
	"path"
	"slices"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/cache"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

// TODO: expiry system in it's own pkg/

type InstanceTypeList []*InstanceType

type InstanceType struct {
	Name             string
	Region           string
	CPUs             int
	MemoryGiB        float64
	NvmeCount        int
	NvmeTotalSizeGiB int
	Arch             []Architecture
	PricePerHour     InstanceTypePrice
}

type InstanceTypePrice struct {
	OnDemand float64
	Spot     float64
	Currency string
}

type VolumePriceList []*VolumePrice

type VolumePrice struct {
	Type           string
	PricePerGBHour float64
	Region         string
	Currency       string
}

const (
	CacheInvalidateVolume   = "volumes"
	CacheInvalidateInstance = "instances"
	CacheInvalidateImage    = "images"
	CacheInvalidateNetwork  = "networks"
	CacheInvalidateFirewall = "firewalls"
)

var CacheInvalidateAll = []string{CacheInvalidateVolume, CacheInvalidateInstance, CacheInvalidateImage, CacheInvalidateNetwork, CacheInvalidateFirewall}

type Cloud interface {
	// basics
	SetConfig(configDir string, credentials *clouds.Credentials, project string, sshKeyDir string, log *logger.Logger, aerolabVersion string, workDir string, invalidateCacheFunc func(names ...string) error) error
	SetInventory(networks NetworkList, firewalls FirewallList, instances InstanceList, volumes VolumeList, images ImageList)
	ListEnabledZones() ([]string, error)
	EnableZones(names ...string) error
	DisableZones(names ...string) error
	// pricing
	GetVolumePrices() (VolumePriceList, error)
	GetInstanceTypes() (InstanceTypeList, error)
	// inventory
	GetVolumes() (VolumeList, error)
	GetInstances(VolumeList, NetworkList, FirewallList) (InstanceList, error)
	GetImages() (ImageList, error)
	GetNetworks() (NetworkList, error)
	GetFirewalls(NetworkList) (FirewallList, error)
	// create actions
	CreateFirewall(input *CreateFirewallInput, waitDur time.Duration) (*CreateFirewallOutput, error)
	CreateVolume(input *CreateVolumeInput) (*CreateVolumeOutput, error)
	CreateVolumeGetPrice(input *CreateVolumeInput) (costGB float64, err error)
	CreateImage(input *CreateImageInput, waitDur time.Duration) (*CreateImageOutput, error)
	CreateInstances(input *CreateInstanceInput, waitDur time.Duration) (*CreateInstanceOutput, error)
	CreateInstancesGetPrice(input *CreateInstanceInput) (costPPH, costGB float64, err error)
	// actions on multiple instances
	InstancesAddTags(instances InstanceList, tags map[string]string) error
	InstancesRemoveTags(instances InstanceList, tagKeys []string) error
	InstancesTerminate(instances InstanceList, waitDur time.Duration) error
	InstancesStop(instances InstanceList, force bool, waitDur time.Duration) error
	InstancesStart(instances InstanceList, waitDur time.Duration) error
	InstancesExec(instances InstanceList, e *ExecInput) []*ExecOutput
	InstancesGetSftpConfig(instances InstanceList, username string) ([]*sshexec.ClientConf, error)
	InstancesAssignFirewalls(instances InstanceList, fw FirewallList) error
	InstancesRemoveFirewalls(instances InstanceList, fw FirewallList) error
	// actions on multiple volumes
	VolumesAddTags(volumes VolumeList, tags map[string]string, waitDur time.Duration) error
	VolumesRemoveTags(volumes VolumeList, tagKeys []string, waitDur time.Duration) error
	DeleteVolumes(volumes VolumeList, fw FirewallList, waitDur time.Duration) error
	AttachVolumes(volumes VolumeList, instance *Instance, sharedMountData *VolumeAttachShared, waitDur time.Duration) error
	DetachVolumes(volumes VolumeList, instance *Instance, waitDur time.Duration) error
	ResizeVolumes(volumes VolumeList, newSizeGiB StorageSize) error
	// actions on images
	ImagesDelete(images ImageList, waitDur time.Duration) error
	ImagesAddTags(images ImageList, tags map[string]string) error
	ImagesRemoveTags(images ImageList, tagKeys []string) error
	// actions on networks
	NetworksDelete(networks NetworkList, waitDur time.Duration) error
	NetworksDeleteSubnets(subnets SubnetList, waitDur time.Duration) error
	NetworksAddTags(networks NetworkList, tags map[string]string) error
	NetworksRemoveTags(networks NetworkList, tagKeys []string) error
	// firewall actions
	FirewallsUpdate(fw FirewallList, ports PortsIn, waitDur time.Duration) error
	FirewallsDelete(fw FirewallList, waitDur time.Duration) error
	FirewallsAddTags(fw FirewallList, tags map[string]string, waitDur time.Duration) error
	FirewallsRemoveTags(fw FirewallList, tagKeys []string, waitDur time.Duration) error
}

type Backend interface {
	GetInventory() (*Inventory, error)
	AddRegion(backendType BackendType, names ...string) error
	RemoveRegion(backendType BackendType, names ...string) error
	ListEnabledRegions(backendType BackendType) (name []string, err error)
	ForceRefreshInventory() error   // force refresh all inventory items from backends
	RefreshChangedInventory() error // refresh inventory items which have been marked as changed by actions
	CreateFirewall(input *CreateFirewallInput, waitDur time.Duration) (*CreateFirewallOutput, error)
	CreateVolume(input *CreateVolumeInput) (*CreateVolumeOutput, error)
	CreateVolumeGetPrice(input *CreateVolumeInput) (costGB float64, err error)
	CreateImage(input *CreateImageInput, waitDur time.Duration) (*CreateImageOutput, error)
	CreateInstances(input *CreateInstanceInput, waitDur time.Duration) (*CreateInstanceOutput, error)
	CreateInstancesGetPrice(input *CreateInstanceInput) (costPPH, costGB float64, err error)
}

type backend struct {
	project         string
	config          *Config
	cache           *cache.Cache
	volumes         map[BackendType]VolumeList
	instances       map[BackendType]InstanceList
	images          map[BackendType]ImageList
	networks        map[BackendType]NetworkList
	firewalls       map[BackendType]FirewallList
	pollLock        *sync.Mutex
	log             *logger.Logger
	invalidated     []string
	invalidatedLock *sync.Mutex
}

// networks->firewalls(networks)->volumes(networks, firewalls)-->instances(volumes, networks, firewalls)
// images - no dependencies
type Inventory struct {
	Networks  Networks  // VPCs, Subnets
	Firewalls Firewalls // AWS security groups, GCP firewalls
	Volumes   Volumes   // permanent volumes which do not go away (not tied to instance lifetime), be it EFS, or EBS/pd-ssd
	Instances Instances // all instances, clusters, clients, whatever
	Images    Images    // images - used for templates; always prefilled with supported OS template images found in the backends, and then with customer image templates on top
}

func getBackendObject(project string, c *Config) *backend {
	return &backend{
		project:   project,
		config:    c,
		volumes:   make(map[BackendType]VolumeList),
		instances: make(map[BackendType]InstanceList),
		images:    make(map[BackendType]ImageList),
		networks:  make(map[BackendType]NetworkList),
		firewalls: make(map[BackendType]FirewallList),
		pollLock:  new(sync.Mutex),
	}
}

func (b *backend) loadCache() error {
	m := &cacheMetadata{}
	err := b.cache.Get(path.Join(b.project, "metadata"), m)
	if err != nil {
		return err
	}
	if m.CacheUpdateTimestamp.Add(time.Hour).Before(time.Now()) {
		return cache.ErrNoCacheFile
	}
	err = b.cache.Get(path.Join(b.project, "networks"), &b.networks)
	if err != nil {
		return err
	}
	err = b.cache.Get(path.Join(b.project, "firewalls"), &b.firewalls)
	if err != nil {
		return err
	}
	err = b.cache.Get(path.Join(b.project, "volumes"), &b.volumes)
	if err != nil {
		return err
	}
	err = b.cache.Get(path.Join(b.project, "instances"), &b.instances)
	if err != nil {
		return err
	}
	err = b.cache.Get(path.Join(b.project, "images"), &b.images)
	if err != nil {
		return err
	}
	// check if cache should be invalidated as some volumes or instances expired
	for _, v := range b.volumes {
		for _, vol := range v {
			if vol.Expires.Before(time.Now()) {
				return cache.ErrNoCacheFile
			}
		}
	}
	for _, v := range b.instances {
		for _, inst := range v {
			if inst.Expires.Before(time.Now()) {
				return cache.ErrNoCacheFile
			}
		}
	}
	return nil
}

func (b *backend) poll(items []string) []error {
	var errs []error

	log := b.log.WithPrefix("PollInventory ")

	if len(items) == 0 || slices.Contains(items, CacheInvalidateVolume) {
		log.Debug("Getting networks")
		for n, v := range cloudList {
			d, err := v.GetNetworks()
			if err != nil {
				errs = append(errs, err)
			} else {
				b.networks[n] = d
			}
		}
		err := b.cache.Store(path.Join(b.project, "networks"), b.networks)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(items) == 0 || slices.Contains(items, CacheInvalidateFirewall) {
		log.Debug("Getting firewalls")
		for n, v := range cloudList {
			d, err := v.GetFirewalls(b.networks[n])
			if err != nil {
				errs = append(errs, err)
			} else {
				b.firewalls[n] = d
			}
		}
		err := b.cache.Store(path.Join(b.project, "firewalls"), b.firewalls)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(items) == 0 || slices.Contains(items, CacheInvalidateVolume) {
		log.Debug("Getting volumes")
		for n, v := range cloudList {
			d, err := v.GetVolumes()
			if err != nil {
				errs = append(errs, err)
			} else {
				b.volumes[n] = d
			}
		}
		err := b.cache.Store(path.Join(b.project, "volumes"), b.volumes)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(items) == 0 || slices.Contains(items, CacheInvalidateInstance) {
		log.Debug("Getting instances")
		for n, v := range cloudList {
			d, err := v.GetInstances(b.volumes[n], b.networks[n], b.firewalls[n])
			if err != nil {
				errs = append(errs, err)
			} else {
				b.instances[n] = d
			}
		}
		err := b.cache.Store(path.Join(b.project, "instances"), b.instances)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(items) == 0 || slices.Contains(items, CacheInvalidateImage) {
		log.Debug("Getting images")
		for n, v := range cloudList {
			d, err := v.GetImages()
			if err != nil {
				errs = append(errs, err)
			} else {
				b.images[n] = d
			}
		}
		err := b.cache.Store(path.Join(b.project, "images"), b.images)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) == 0 && len(items) == 0 {
		log.Debug("Storing metadata")
		err := b.cache.Store(path.Join(b.project, "metadata"), cacheMetadata{
			CacheUpdateTimestamp: time.Now(),
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	for cname, cloud := range cloudList {
		cloud.SetInventory(b.networks[cname], b.firewalls[cname], b.instances[cname], b.volumes[cname], b.images[cname])
	}
	log.Debug("Done")
	return errs
}

func (b *backend) GetInventory() (*Inventory, error) {
	networks := NetworkList{}
	for _, v := range b.networks {
		networks = append(networks, v...)
	}
	firewalls := FirewallList{}
	for _, v := range b.firewalls {
		firewalls = append(firewalls, v...)
	}
	volumes := VolumeList{}
	for _, v := range b.volumes {
		volumes = append(volumes, v...)
	}
	instances := InstanceList{}
	for _, v := range b.instances {
		instances = append(instances, v...)
	}
	images := ImageList{}
	for _, v := range b.images {
		images = append(images, v...)
	}
	return &Inventory{
		Volumes:   volumes,
		Instances: instances,
		Images:    images,
		Networks:  networks,
		Firewalls: firewalls,
	}, nil
}

func (b *backend) RefreshChangedInventory() error {
	log := b.log.WithPrefix("RefreshChangedInventory")
	log.Debug("Starting inventory refresh")
	b.pollLock.Lock()
	defer b.pollLock.Unlock()
	log.Debug("Poll Lock obtained, obtaining invalidated items lock")
	b.invalidatedLock.Lock()
	defer b.invalidatedLock.Unlock()
	log.Debug("Invalidated Lock obtained, inventory refresh started")
	errs := b.poll(b.invalidated)
	if len(errs) != 0 {
		var errstring error
		for _, e := range errs {
			errstring = errors.Join(errstring, e)
		}
		return errstring
	}
	b.invalidated = []string{}
	err := b.cache.Store(path.Join(b.project, "invalidated"), b.invalidated)
	if err != nil {
		return err
	}
	return nil
}

func (b *backend) invalidate(items ...string) error {
	log := b.log.WithPrefix("invalidateInventoryCache")
	log.Debug("Invalidating items: %v", items)
	b.invalidatedLock.Lock()
	defer b.invalidatedLock.Unlock()
	log.Debug("Invalidated Lock obtained, invalidating items")
	b.invalidated = append(b.invalidated, items...)
	err := b.cache.Store(path.Join(b.project, "invalidated"), b.invalidated)
	if err != nil {
		return err
	}
	log.Debug("Invalidated, returning")
	return nil
}

package backend

import (
	"path"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/cache"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

type Cloud interface {
	SetConfig(configDir string, credentials *clouds.Credentials, project string, sshKeyDir string, log *logger.Logger) error
	ListEnabledZones() ([]string, error)
	EnableZones(names ...string) error
	DisableZones(names ...string) error
	GetVolumes() (VolumeList, error)
	GetInstances(VolumeList) (InstanceList, error)
	GetImages() (ImageList, error)
	// TODO: cloud must implement CreateVolumes, CreateInstances, CreateImages
	// actions on multiple instances
	InstancesAddTags(instances InstanceList, tags map[string]string) error
	InstancesRemoveTags(instances InstanceList, tagKeys []string) error
	InstancesTerminate(instances InstanceList, waitDur time.Duration) error
	InstancesStop(instances InstanceList, force bool, waitDur time.Duration) error
	InstancesStart(instances InstanceList, waitDur time.Duration) error
	InstancesExec(instances InstanceList, e *ExecInput) []*ExecOutput
	InstancesGetSftpConfig(instances InstanceList, username string) ([]*sshexec.ClientConf, error)
	// actions on multiple volumes
	VolumesAddTags(volumes VolumeList, tags map[string]string, waitDur time.Duration) error
	VolumesRemoveTags(volumes VolumeList, tagKeys []string, waitDur time.Duration) error
	DeleteVolumes(volumes VolumeList, waitDur time.Duration) error
	AttachVolumes(volumes VolumeList, instance *Instance, mountTargetDirectory *string) error
	DetachVolumes(volumes VolumeList, instance *Instance) error
	ResizeVolumes(volumes VolumeList, newSizeGiB StorageSize) error
	// actions on images
	ImagesDelete(images ImageList, waitDur time.Duration) error
}

type Backend interface {
	// TODO: backend must implement CreateVolumes, CreateInstances, CreateImages
	GetInventory() (*Inventory, error)
	AddRegion(backendType BackendType, names ...string) error
	RemoveRegion(backendType BackendType, names ...string) error
	ListEnabledRegions(backendType BackendType) (name []string, err error)
	ForceRefreshInventory() error
}

type backend struct {
	project   string
	config    *Config
	cache     *cache.Cache
	volumes   map[BackendType]VolumeList
	instances map[BackendType]InstanceList
	images    map[BackendType]ImageList
	pollLock  *sync.Mutex
	log       *logger.Logger
}

// networks->firewalls(networks)->volumes(networks,firewalls)->instances(volumes)
// images - no dependencies
// expiries - no dependencies
type Inventory struct {
	//TODO Networks  Networks  // VPCs, Subnets
	//TODO Firewalls Firewalls // AWS security groups, GCP firewalls
	Volumes   Volumes   // permanent volumes which do not go away (not tied to instance lifetime), be it EFS, or EBS/pd-ssd
	Instances Instances // all instances, clusters, clients, whatever
	Images    Images    // images - used for templates; always prefilled with supported OS template images found in the backends, and then with customer image templates on top
	//TODO Expiries  Expiries  // Expiry system: to be made obsolete in the future by using tiny instances with aerolab on them for expiries instead - removing complexity
}

func getBackendObject(project string, c *Config) *backend {
	return &backend{
		project:   project,
		config:    c,
		volumes:   make(map[BackendType]VolumeList),
		instances: make(map[BackendType]InstanceList),
		images:    make(map[BackendType]ImageList),
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
	return nil
}

func (b *backend) poll() []error {
	b.pollLock.Lock()
	defer b.pollLock.Unlock()
	var errs []error

	log := b.log.WithPrefix("PollInventory ")
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

	log.Debug("Getting instances")
	for n, v := range cloudList {
		d, err := v.GetInstances(b.volumes[n])
		if err != nil {
			errs = append(errs, err)
		} else {
			b.instances[n] = d
		}
	}
	err = b.cache.Store(path.Join(b.project, "instances"), b.instances)
	if err != nil {
		errs = append(errs, err)
	}

	log.Debug("Getting images")
	for n, v := range cloudList {
		d, err := v.GetImages()
		if err != nil {
			errs = append(errs, err)
		} else {
			b.images[n] = d
		}
	}
	err = b.cache.Store(path.Join(b.project, "images"), b.images)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		log.Debug("Storing metadata")
		err = b.cache.Store(path.Join(b.project, "metadata"), cacheMetadata{
			CacheUpdateTimestamp: time.Now(),
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	log.Debug("Done")
	return errs
}

func (b *backend) GetInventory() (*Inventory, error) {
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
	}, nil
}

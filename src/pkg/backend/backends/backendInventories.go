package backends

import (
	"encoding/json"
	"errors"
	"path"
	"slices"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/cache"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
	"gopkg.in/yaml.v3"
)

type InstanceTypeList []*InstanceType

type InstanceType struct {
	Name             string
	Region           string
	CPUs             int
	GPUs             int
	MemoryGiB        float64
	NvmeCount        int
	NvmeTotalSizeGiB int
	Arch             Architectures
	PricePerHour     InstanceTypePrice
	BackendSpecific  interface{}
}

type Architectures []Architecture

func (a Architectures) String() []string {
	ret := []string{}
	for _, arch := range a {
		ret = append(ret, arch.String())
	}
	return ret
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
	SetConfig(configDir string, credentials *clouds.Credentials, project string, sshKeyDir string, log *logger.Logger, aerolabVersion string, workDir string, invalidateCacheFunc func(names ...string) error, listAllProjects bool) error
	SetInventory(networks NetworkList, firewalls FirewallList, instances InstanceList, volumes VolumeList, images ImageList)
	ListEnabledZones() ([]string, error)
	EnableZones(names ...string) error
	DisableZones(names ...string) error
	// expiry
	ExpiryInstall(intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error // if force is false, it will only install if previous installation was failed or version is different
	ExpiryRemove(zones ...string) error
	ExpiryChangeConfiguration(logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error
	ExpiryList() ([]*ExpirySystem, error)
	ExpiryChangeFrequency(intervalMinutes int, zones ...string) error
	VolumesChangeExpiry(volumes VolumeList, expiry time.Time) error
	InstancesChangeExpiry(instances InstanceList, expiry time.Time) error
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
	InstancesUpdateHostsFile(instances InstanceList, hostsEntries []string, parallelSSHThreads int) error
	// actions on multiple volumes
	VolumesAddTags(volumes VolumeList, tags map[string]string, waitDur time.Duration) error
	VolumesRemoveTags(volumes VolumeList, tagKeys []string, waitDur time.Duration) error
	DeleteVolumes(volumes VolumeList, fw FirewallList, waitDur time.Duration) error
	AttachVolumes(volumes VolumeList, instance *Instance, sharedMountData *VolumeAttachShared, waitDur time.Duration) error
	DetachVolumes(volumes VolumeList, instance *Instance, waitDur time.Duration) error
	ResizeVolumes(volumes VolumeList, newSizeGiB StorageSize, waitDur time.Duration) error
	// actions on images
	ImagesDelete(images ImageList, waitDur time.Duration) error
	ImagesAddTags(images ImageList, tags map[string]string) error
	ImagesRemoveTags(images ImageList, tagKeys []string) error
	// actions on networks
	//NetworksDelete(networks NetworkList, waitDur time.Duration) error
	//NetworksDeleteSubnets(subnets SubnetList, waitDur time.Duration) error
	//NetworksAddTags(networks NetworkList, tags map[string]string) error
	//NetworksRemoveTags(networks NetworkList, tagKeys []string) error
	// firewall actions
	FirewallsUpdate(fw FirewallList, ports PortsIn, waitDur time.Duration) error
	FirewallsDelete(fw FirewallList, waitDur time.Duration) error
	FirewallsAddTags(fw FirewallList, tags map[string]string, waitDur time.Duration) error
	FirewallsRemoveTags(fw FirewallList, tagKeys []string, waitDur time.Duration) error
	CleanupDNS() error // cleanup stale DNS records, if spot instances are being used, this is normally run by the expiry handler
	// docker-only commands
	DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error // create a new docker network
	DockerDeleteNetwork(region string, name string) error                                           // delete a docker network
	DockerPruneNetworks(region string) error
	// resolve network placement
	ResolveNetworkPlacement(placement string) (vpc *Network, subnet *Subnet, zone string, err error)
}

type Backend interface {
	// inventory handling
	GetInventory() *Inventory                   // get currently cached inventory
	ForceRefreshInventory() error               // force refresh all inventory items from backends
	RefreshChangedInventory() error             // refresh inventory items which have been marked as changed by actions
	GetRefreshedInventory() (*Inventory, error) // refresh changed inventory and get the new inventory
	// region handling
	AddRegion(backendType BackendType, names ...string) error
	RemoveRegion(backendType BackendType, names ...string) error
	ListEnabledRegions(backendType BackendType) (name []string, err error)
	// create actions
	CreateFirewall(input *CreateFirewallInput, waitDur time.Duration) (*CreateFirewallOutput, error)
	CreateVolume(input *CreateVolumeInput) (*CreateVolumeOutput, error)
	CreateVolumeGetPrice(input *CreateVolumeInput) (costGB float64, err error)
	CreateImage(input *CreateImageInput, waitDur time.Duration) (*CreateImageOutput, error)
	CreateInstances(input *CreateInstanceInput, waitDur time.Duration) (*CreateInstanceOutput, error)
	CreateInstancesGetPrice(input *CreateInstanceInput) (costPPH, costGB float64, err error)
	// cleanup
	CleanupDNS() error                                    // cleanup stale DNS records, if spot instances are being used, this is normally run by the expiry handler
	DeleteProjectResources(backendType BackendType) error // delete all resources in the project in the given backend type, this does NOT remove the ExpirySystem which is project-agnostic
	// expiry
	ExpiryInstall(backendType BackendType, intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error // if force is false, it will only install if previous installation was failed or version is different
	ExpiryRemove(backendType BackendType, zones ...string) error
	ExpiryChangeFrequency(backendType BackendType, intervalMinutes int, zones ...string) error
	ExpiryList() (*ExpiryList, error)
	ExpiryChangeConfiguration(backendType BackendType, logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error
	// close the backend object
	Close() error
	// special docker-only commands
	DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error // create a new docker network
	DockerDeleteNetwork(region string, name string) error                                           // delete a docker network
	DockerPruneNetworks(region string) error
	// instance types and pricing
	GetVolumePrices(backendType BackendType) (VolumePriceList, error)
	GetInstanceTypes(backendType BackendType) (InstanceTypeList, error)
	// resolve network placement
	ResolveNetworkPlacement(backendType BackendType, placement string) (vpc *Network, subnet *Subnet, zone string, err error)
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
	enabledBackends map[BackendType]Cloud
	closed          bool
}

func getBackendObject(project string, c *Config) *backend {
	return &backend{
		project:         project,
		config:          c,
		volumes:         make(map[BackendType]VolumeList),
		instances:       make(map[BackendType]InstanceList),
		images:          make(map[BackendType]ImageList),
		networks:        make(map[BackendType]NetworkList),
		firewalls:       make(map[BackendType]FirewallList),
		pollLock:        new(sync.Mutex),
		invalidatedLock: new(sync.Mutex),
	}
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

func (i *Inventory) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"networks":  i.Networks.Describe(),
		"firewalls": i.Firewalls.Describe(),
		"volumes":   i.Volumes.Describe(),
		"instances": i.Instances.Describe(),
		"images":    i.Images.Describe(),
	}
}

func (i *Inventory) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Networks  NetworkList  `json:"networks"`
		Firewalls FirewallList `json:"firewalls"`
		Volumes   VolumeList   `json:"volumes"`
		Instances InstanceList `json:"instances"`
		Images    ImageList    `json:"images"`
	}{
		Networks:  i.Networks.Describe(),
		Firewalls: i.Firewalls.Describe(),
		Volumes:   i.Volumes.Describe(),
		Instances: i.Instances.Describe(),
		Images:    i.Images.Describe(),
	})
}

func (i *Inventory) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(struct {
		Networks  NetworkList  `yaml:"networks"`
		Firewalls FirewallList `yaml:"firewalls"`
		Volumes   VolumeList   `yaml:"volumes"`
		Instances InstanceList `yaml:"instances"`
		Images    ImageList    `yaml:"images"`
	}{
		Networks:  i.Networks.Describe(),
		Firewalls: i.Firewalls.Describe(),
		Volumes:   i.Volumes.Describe(),
		Instances: i.Instances.Describe(),
		Images:    i.Images.Describe(),
	})
}

func (i *Inventory) UnmarshalJSON(data []byte) error {
	b := struct {
		Networks  NetworkList  `yaml:"networks"`
		Firewalls FirewallList `yaml:"firewalls"`
		Volumes   VolumeList   `yaml:"volumes"`
		Instances InstanceList `yaml:"instances"`
		Images    ImageList    `yaml:"images"`
	}{}
	err := json.Unmarshal(data, &b)
	if err != nil {
		return err
	}
	i.Networks = b.Networks
	i.Firewalls = b.Firewalls
	i.Volumes = b.Volumes
	i.Instances = b.Instances
	i.Images = b.Images
	return nil
}

func (i *Inventory) UnmarshalYAML(node *yaml.Node) error {
	b := struct {
		Networks  NetworkList  `yaml:"networks"`
		Firewalls FirewallList `yaml:"firewalls"`
		Volumes   VolumeList   `yaml:"volumes"`
		Instances InstanceList `yaml:"instances"`
		Images    ImageList    `yaml:"images"`
	}{}
	err := node.Decode(&b)
	if err != nil {
		return err
	}
	i.Networks = b.Networks
	i.Firewalls = b.Firewalls
	i.Volumes = b.Volumes
	i.Instances = b.Instances
	i.Images = b.Images
	return nil
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
	err = b.cache.Get(path.Join(b.project, "invalidated"), &b.invalidated)
	if err != nil {
		return err
	}
	return nil
}

func (b *backend) poll(items []string) []error {
	start := time.Now()
	var errs []error

	log := b.log.WithPrefix("PollInventory ")

	slices.Sort(items)
	items = slices.Compact(items)

	netWg := new(sync.WaitGroup)
	fwWg := new(sync.WaitGroup)
	volWg := new(sync.WaitGroup)
	instWg := new(sync.WaitGroup)
	imgWg := new(sync.WaitGroup)

	// images can run immediately
	imgWg.Add(1)
	go func() {
		defer imgWg.Done()
		if len(items) == 0 || slices.Contains(items, CacheInvalidateImage) {
			log.Debug("Getting images")
			for n, v := range b.enabledBackends {
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
	}()

	// volumes can run immediately
	volWg.Add(1)
	go func() {
		defer volWg.Done()
		if len(items) == 0 || slices.Contains(items, CacheInvalidateVolume) {
			log.Debug("Getting volumes")
			for n, v := range b.enabledBackends {
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
	}()

	netWg.Add(1)
	go func() {
		defer netWg.Done()
		if len(items) == 0 || slices.Contains(items, CacheInvalidateNetwork) {
			log.Debug("Getting networks")
			for n, v := range b.enabledBackends {
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
	}()

	netWg.Wait() // must complete networks before we can do firewalls
	fwWg.Add(1)
	go func() {
		defer fwWg.Done()
		if len(items) == 0 || slices.Contains(items, CacheInvalidateFirewall) {
			log.Debug("Getting firewalls")
			for n, v := range b.enabledBackends {
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
	}()

	fwWg.Wait()  // must complete firewalls before we can do instances; this ensures networks completed too
	volWg.Wait() // must complete volumes before we can do instances
	instWg.Add(1)
	go func() {
		defer instWg.Done()
		if len(items) == 0 || slices.Contains(items, CacheInvalidateInstance) {
			log.Debug("Getting instances")
			for n, v := range b.enabledBackends {
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
	}()

	// wait for all the above to complete
	instWg.Wait()
	imgWg.Wait()

	if len(errs) == 0 && len(items) == 0 {
		log.Debug("Storing metadata")
		err := b.cache.Store(path.Join(b.project, "metadata"), cacheMetadata{
			CacheUpdateTimestamp: time.Now(),
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	for cname, cloud := range b.enabledBackends {
		cloud.SetInventory(b.networks[cname], b.firewalls[cname], b.instances[cname], b.volumes[cname], b.images[cname])
	}
	log.Debug("Done")
	log.Detail("Inventory poll of %v took %s", items, time.Since(start))
	return errs
}

func (b *backend) GetRefreshedInventory() (*Inventory, error) {
	err := b.RefreshChangedInventory()
	return b.GetInventory(), err
}

func (b *backend) setInventory(inventory *Inventory) {
	for _, v := range inventory.Networks.Describe() {
		if _, ok := b.networks[v.BackendType]; !ok {
			b.networks[v.BackendType] = NetworkList{}
		}
		b.networks[v.BackendType] = append(b.networks[v.BackendType], v)
	}
	for _, v := range inventory.Firewalls.Describe() {
		if _, ok := b.firewalls[v.BackendType]; !ok {
			b.firewalls[v.BackendType] = FirewallList{}
		}
		b.firewalls[v.BackendType] = append(b.firewalls[v.BackendType], v)
	}
	for _, v := range inventory.Volumes.Describe() {
		if _, ok := b.volumes[v.BackendType]; !ok {
			b.volumes[v.BackendType] = VolumeList{}
		}
		b.volumes[v.BackendType] = append(b.volumes[v.BackendType], v)
	}
	for _, v := range inventory.Instances.Describe() {
		if _, ok := b.instances[v.BackendType]; !ok {
			b.instances[v.BackendType] = InstanceList{}
		}
		b.instances[v.BackendType] = append(b.instances[v.BackendType], v)
	}
	for _, v := range inventory.Images.Describe() {
		if _, ok := b.images[v.BackendType]; !ok {
			b.images[v.BackendType] = ImageList{}
		}
		b.images[v.BackendType] = append(b.images[v.BackendType], v)
	}
}

func (b *backend) GetInventory() *Inventory {
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
	}
}

func (b *backend) RefreshChangedInventory() error {
	log := b.log.WithPrefix("RefreshChangedInventory ")
	log.Debug("Starting inventory refresh")
	b.pollLock.Lock()
	defer b.pollLock.Unlock()
	log.Debug("Poll Lock obtained, obtaining invalidated items lock")
	b.invalidatedLock.Lock()
	defer b.invalidatedLock.Unlock()
	log.Debug("Invalidated Lock obtained, inventory refresh started")

	// first check if any volumes or instances expired
VOLUMES_EXPIRED_LOOP:
	for _, v := range b.volumes {
		for _, vol := range v {
			if vol.Expires.Before(time.Now()) {
				b.invalidated = append(b.invalidated, CacheInvalidateVolume)
				break VOLUMES_EXPIRED_LOOP
			}
		}
	}
INSTANCES_EXPIRED_LOOP:
	for _, v := range b.instances {
		for _, inst := range v {
			if inst.Expires.Before(time.Now()) {
				b.invalidated = append(b.invalidated, CacheInvalidateInstance)
				break INSTANCES_EXPIRED_LOOP
			}
		}
	}

	// if no invalidated items, return
	if len(b.invalidated) == 0 {
		log.Debug("No invalidated items, returning")
		return nil
	}
	// poll for invalidated items
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
	log := b.log.WithPrefix("invalidateInventoryCache ")
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

func (b *backend) DeleteProjectResources(backendType BackendType) error {
	log := b.log.WithPrefix("DeleteProjectResources ")
	log.Debug("Deleting project resources for backend type: %v", backendType)
	err := b.ForceRefreshInventory()
	if err != nil {
		return err
	}
	inventory := b.GetInventory()
	err = inventory.Instances.WithBackendType(backendType).WithNotState(LifeCycleStateTerminated).Terminate(time.Minute * 10)
	if err != nil {
		return err
	}
	inventory, err = b.GetRefreshedInventory()
	if err != nil {
		return err
	}
	err = inventory.Volumes.WithBackendType(backendType).DeleteVolumes(inventory.Firewalls.Describe(), time.Minute*10)
	if err != nil {
		return err
	}
	err = inventory.Images.WithBackendType(backendType).WithInAccount(true).DeleteImages(time.Minute * 10)
	if err != nil {
		return err
	}
	err = inventory.Firewalls.WithBackendType(backendType).Delete(time.Minute * 10)
	if err != nil {
		return err
	}
	err = b.CleanupDNS()
	if err != nil {
		return err
	}
	return nil
}

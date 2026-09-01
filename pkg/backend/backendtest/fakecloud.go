package backendtest

import (
	"io"
	"sync"
	"time"

	"github.com/citrusleaf/aerolab/pkg/backend/backends"
	"github.com/citrusleaf/aerolab/pkg/backend/clouds"
	"github.com/citrusleaf/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

// FakeCloud is a programmable test double implementing backends.Cloud. Every
// method records its name in Calls and returns the error stored in Errs[name]
// (nil if unset). Getter methods return the canned inventory slices. Action
// methods capture their instance arguments for later assertions.
//
// FakeCloud is safe for sequential use within a single test. Because it is
// typically registered in the process-global backend registry (see
// RegisterFakeCloud), tests using it must not run in parallel.
type FakeCloud struct {
	mu sync.Mutex

	// Calls records, in order, the names of the methods invoked.
	Calls []string
	// Errs maps a method name to the error it should return.
	Errs map[string]error

	// Canned inventory returned by the Get* methods.
	Instances backends.InstanceList
	Volumes   backends.VolumeList
	Images    backends.ImageList
	Networks  backends.NetworkList
	Firewalls backends.FirewallList

	// Canned scalar/list values.
	Zones          []string
	AvailableZones []string
	InstanceTypes  backends.InstanceTypeList
	VolumePrices   backends.VolumePriceList
	ExpirySystems  []*backends.ExpirySystem
	AccountID      string

	// ExecFunc, if non-nil, provides the InstancesExec result. Otherwise a
	// default (one empty ExecOutput per instance) is returned.
	ExecFunc func(backends.InstanceList, *backends.ExecInput) []*backends.ExecOutput

	// Captured action arguments.
	ExecInstances       backends.InstanceList
	ExecInput           *backends.ExecInput
	TerminatedInstances backends.InstanceList
	StoppedInstances    backends.InstanceList
	StartedInstances    backends.InstanceList
	HostKeyStore        *sshexec.HostKeyStore
	HostKeyStrict       bool
	Identity            *backends.Identity
	// CallerAccessInstances records the instances the last EnsureCallerAccess
	// call was asked to grant the caller access to.
	CallerAccessInstances backends.InstanceList
}

func (f *FakeCloud) record(name string) {
	f.mu.Lock()
	f.Calls = append(f.Calls, name)
	f.mu.Unlock()
}

func (f *FakeCloud) err(name string) error {
	if f.Errs == nil {
		return nil
	}
	return f.Errs[name]
}

// CallCount returns how many times the named method has been invoked.
func (f *FakeCloud) CallCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.Calls {
		if c == name {
			n++
		}
	}
	return n
}

// Reset clears all recorded calls and captured arguments.
func (f *FakeCloud) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = nil
	f.ExecInstances = nil
	f.ExecInput = nil
	f.TerminatedInstances = nil
	f.StoppedInstances = nil
	f.StartedInstances = nil
}

// --- basics ---

func (f *FakeCloud) SetConfig(configDir string, credentials *clouds.Credentials, project string, sshKeyDir string, log *logger.Logger, aerolabVersion string, workDir string, invalidateCacheFunc func(names ...string) error, listAllProjects bool) error {
	f.record("SetConfig")
	return f.err("SetConfig")
}

func (f *FakeCloud) SetHostKeyPolicy(store *sshexec.HostKeyStore, strict bool) {
	f.record("SetHostKeyPolicy")
	f.HostKeyStore, f.HostKeyStrict = store, strict
}

func (f *FakeCloud) SetIdentity(identity *backends.Identity) {
	f.record("SetIdentity")
	f.Identity = identity
}

func (f *FakeCloud) EnsureCallerAccess(instances backends.InstanceList) {
	f.record("EnsureCallerAccess")
	f.CallerAccessInstances = instances
}

func (f *FakeCloud) SetInventory(networks backends.NetworkList, firewalls backends.FirewallList, instances backends.InstanceList, volumes backends.VolumeList, images backends.ImageList) {
	f.record("SetInventory")
	f.Networks, f.Firewalls, f.Instances, f.Volumes, f.Images = networks, firewalls, instances, volumes, images
}

func (f *FakeCloud) ListEnabledZones() ([]string, error) {
	f.record("ListEnabledZones")
	return f.Zones, f.err("ListEnabledZones")
}

func (f *FakeCloud) ListAvailableZones() ([]string, error) {
	f.record("ListAvailableZones")
	return f.AvailableZones, f.err("ListAvailableZones")
}

func (f *FakeCloud) EnableZones(names ...string) error {
	f.record("EnableZones")
	return f.err("EnableZones")
}

func (f *FakeCloud) DisableZones(names ...string) error {
	f.record("DisableZones")
	return f.err("DisableZones")
}

// --- expiry ---

func (f *FakeCloud) ExpiryInstall(intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error {
	f.record("ExpiryInstall")
	return f.err("ExpiryInstall")
}

func (f *FakeCloud) ExpiryRemove(zones ...string) error {
	f.record("ExpiryRemove")
	return f.err("ExpiryRemove")
}

func (f *FakeCloud) ExpiryChangeConfiguration(logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error {
	f.record("ExpiryChangeConfiguration")
	return f.err("ExpiryChangeConfiguration")
}

func (f *FakeCloud) ExpiryList() ([]*backends.ExpirySystem, error) {
	f.record("ExpiryList")
	return f.ExpirySystems, f.err("ExpiryList")
}

func (f *FakeCloud) ExpiryChangeFrequency(intervalMinutes int, zones ...string) error {
	f.record("ExpiryChangeFrequency")
	return f.err("ExpiryChangeFrequency")
}

func (f *FakeCloud) ExpiryV7Check() (found bool, regions []string, err error) {
	f.record("ExpiryV7Check")
	return false, nil, f.err("ExpiryV7Check")
}

func (f *FakeCloud) VolumesChangeExpiry(volumes backends.VolumeList, expiry time.Time) error {
	f.record("VolumesChangeExpiry")
	return f.err("VolumesChangeExpiry")
}

func (f *FakeCloud) InstancesChangeExpiry(instances backends.InstanceList, expiry time.Time) error {
	f.record("InstancesChangeExpiry")
	return f.err("InstancesChangeExpiry")
}

// --- pricing ---

func (f *FakeCloud) GetVolumePrices() (backends.VolumePriceList, error) {
	f.record("GetVolumePrices")
	return f.VolumePrices, f.err("GetVolumePrices")
}

func (f *FakeCloud) GetInstanceTypes() (backends.InstanceTypeList, error) {
	f.record("GetInstanceTypes")
	return f.InstanceTypes, f.err("GetInstanceTypes")
}

// --- inventory ---

func (f *FakeCloud) GetVolumes() (backends.VolumeList, error) {
	f.record("GetVolumes")
	return f.Volumes, f.err("GetVolumes")
}

func (f *FakeCloud) GetInstances(_ backends.VolumeList, _ backends.NetworkList, _ backends.FirewallList) (backends.InstanceList, error) {
	f.record("GetInstances")
	return f.Instances, f.err("GetInstances")
}

func (f *FakeCloud) GetImages() (backends.ImageList, error) {
	f.record("GetImages")
	return f.Images, f.err("GetImages")
}

func (f *FakeCloud) GetNetworks() (backends.NetworkList, error) {
	f.record("GetNetworks")
	return f.Networks, f.err("GetNetworks")
}

func (f *FakeCloud) GetFirewalls(_ backends.NetworkList) (backends.FirewallList, error) {
	f.record("GetFirewalls")
	return f.Firewalls, f.err("GetFirewalls")
}

// --- create actions ---

func (f *FakeCloud) CreateFirewall(input *backends.CreateFirewallInput, waitDur time.Duration) (*backends.CreateFirewallOutput, error) {
	f.record("CreateFirewall")
	return nil, f.err("CreateFirewall")
}

func (f *FakeCloud) CreateVolume(input *backends.CreateVolumeInput) (*backends.CreateVolumeOutput, error) {
	f.record("CreateVolume")
	return nil, f.err("CreateVolume")
}

func (f *FakeCloud) CreateVolumeGetPrice(input *backends.CreateVolumeInput) (costGB float64, err error) {
	f.record("CreateVolumeGetPrice")
	return 0, f.err("CreateVolumeGetPrice")
}

func (f *FakeCloud) CreateImage(input *backends.CreateImageInput, waitDur time.Duration) (*backends.CreateImageOutput, error) {
	f.record("CreateImage")
	return nil, f.err("CreateImage")
}

func (f *FakeCloud) CreateInstances(input *backends.CreateInstanceInput, waitDur time.Duration) (*backends.CreateInstanceOutput, error) {
	f.record("CreateInstances")
	return nil, f.err("CreateInstances")
}

func (f *FakeCloud) CreateInstancesGetPrice(input *backends.CreateInstanceInput) (costPPH, costGB float64, err error) {
	f.record("CreateInstancesGetPrice")
	return 0, 0, f.err("CreateInstancesGetPrice")
}

// --- actions on multiple instances ---

func (f *FakeCloud) InstancesAddTags(instances backends.InstanceList, tags map[string]string) error {
	f.record("InstancesAddTags")
	return f.err("InstancesAddTags")
}

func (f *FakeCloud) InstancesRemoveTags(instances backends.InstanceList, tagKeys []string) error {
	f.record("InstancesRemoveTags")
	return f.err("InstancesRemoveTags")
}

func (f *FakeCloud) InstancesTerminate(instances backends.InstanceList, waitDur time.Duration) error {
	f.record("InstancesTerminate")
	f.TerminatedInstances = append(f.TerminatedInstances, instances...)
	return f.err("InstancesTerminate")
}

func (f *FakeCloud) InstancesStop(instances backends.InstanceList, force bool, waitDur time.Duration) error {
	f.record("InstancesStop")
	f.StoppedInstances = append(f.StoppedInstances, instances...)
	return f.err("InstancesStop")
}

func (f *FakeCloud) InstancesStart(instances backends.InstanceList, waitDur time.Duration) error {
	f.record("InstancesStart")
	f.StartedInstances = append(f.StartedInstances, instances...)
	return f.err("InstancesStart")
}

func (f *FakeCloud) InstancesExec(instances backends.InstanceList, e *backends.ExecInput) []*backends.ExecOutput {
	f.record("InstancesExec")
	f.ExecInstances = instances
	f.ExecInput = e
	if f.ExecFunc != nil {
		return f.ExecFunc(instances, e)
	}
	out := make([]*backends.ExecOutput, 0, len(instances))
	for _, inst := range instances {
		out = append(out, &backends.ExecOutput{Instance: inst, Output: &sshexec.ExecOutput{}})
	}
	return out
}

func (f *FakeCloud) InstancesGetSftpConfig(instances backends.InstanceList, username string) ([]*sshexec.ClientConf, error) {
	f.record("InstancesGetSftpConfig")
	if err := f.err("InstancesGetSftpConfig"); err != nil {
		return nil, err
	}
	out := make([]*sshexec.ClientConf, 0, len(instances))
	for range instances {
		out = append(out, &sshexec.ClientConf{Username: username})
	}
	return out, nil
}

func (f *FakeCloud) InstancesGetSSHKeyPath(instances backends.InstanceList) []string {
	f.record("InstancesGetSSHKeyPath")
	out := make([]string, 0, len(instances))
	for range instances {
		out = append(out, "")
	}
	return out
}

func (f *FakeCloud) InstancesAssignFirewalls(instances backends.InstanceList, fw backends.FirewallList) error {
	f.record("InstancesAssignFirewalls")
	return f.err("InstancesAssignFirewalls")
}

func (f *FakeCloud) InstancesRemoveFirewalls(instances backends.InstanceList, fw backends.FirewallList) error {
	f.record("InstancesRemoveFirewalls")
	return f.err("InstancesRemoveFirewalls")
}

func (f *FakeCloud) InstancesUpdateHostsFile(instances backends.InstanceList, hostsEntries []string, parallelSSHThreads int) error {
	f.record("InstancesUpdateHostsFile")
	return f.err("InstancesUpdateHostsFile")
}

// --- actions on multiple volumes ---

func (f *FakeCloud) VolumesAddTags(volumes backends.VolumeList, tags map[string]string, waitDur time.Duration) error {
	f.record("VolumesAddTags")
	return f.err("VolumesAddTags")
}

func (f *FakeCloud) VolumesRemoveTags(volumes backends.VolumeList, tagKeys []string, waitDur time.Duration) error {
	f.record("VolumesRemoveTags")
	return f.err("VolumesRemoveTags")
}

func (f *FakeCloud) DeleteVolumes(volumes backends.VolumeList, fw backends.FirewallList, waitDur time.Duration) error {
	f.record("DeleteVolumes")
	return f.err("DeleteVolumes")
}

func (f *FakeCloud) AttachVolumes(volumes backends.VolumeList, instance *backends.Instance, sharedMountData *backends.VolumeAttachShared, waitDur time.Duration) error {
	f.record("AttachVolumes")
	return f.err("AttachVolumes")
}

func (f *FakeCloud) DetachVolumes(volumes backends.VolumeList, instance *backends.Instance, waitDur time.Duration) error {
	f.record("DetachVolumes")
	return f.err("DetachVolumes")
}

func (f *FakeCloud) ResizeVolumes(volumes backends.VolumeList, newSizeGiB backends.StorageSize, waitDur time.Duration) error {
	f.record("ResizeVolumes")
	return f.err("ResizeVolumes")
}

// --- actions on images ---

func (f *FakeCloud) ImagesDelete(images backends.ImageList, waitDur time.Duration) error {
	f.record("ImagesDelete")
	return f.err("ImagesDelete")
}

func (f *FakeCloud) ImagesAddTags(images backends.ImageList, tags map[string]string) error {
	f.record("ImagesAddTags")
	return f.err("ImagesAddTags")
}

func (f *FakeCloud) ImagesRemoveTags(images backends.ImageList, tagKeys []string) error {
	f.record("ImagesRemoveTags")
	return f.err("ImagesRemoveTags")
}

// --- firewall actions ---

func (f *FakeCloud) FirewallsUpdate(fw backends.FirewallList, ports backends.PortsIn, waitDur time.Duration) error {
	f.record("FirewallsUpdate")
	return f.err("FirewallsUpdate")
}

func (f *FakeCloud) FirewallsDelete(fw backends.FirewallList, waitDur time.Duration) error {
	f.record("FirewallsDelete")
	return f.err("FirewallsDelete")
}

func (f *FakeCloud) FirewallsAddTags(fw backends.FirewallList, tags map[string]string, waitDur time.Duration) error {
	f.record("FirewallsAddTags")
	return f.err("FirewallsAddTags")
}

func (f *FakeCloud) FirewallsRemoveTags(fw backends.FirewallList, tagKeys []string, waitDur time.Duration) error {
	f.record("FirewallsRemoveTags")
	return f.err("FirewallsRemoveTags")
}

func (f *FakeCloud) CleanupDNS() error {
	f.record("CleanupDNS")
	return f.err("CleanupDNS")
}

// --- docker-only commands ---

func (f *FakeCloud) DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error {
	f.record("DockerCreateNetwork")
	return f.err("DockerCreateNetwork")
}

func (f *FakeCloud) DockerDeleteNetwork(region string, name string) error {
	f.record("DockerDeleteNetwork")
	return f.err("DockerDeleteNetwork")
}

func (f *FakeCloud) DockerPruneNetworks(region string) error {
	f.record("DockerPruneNetworks")
	return f.err("DockerPruneNetworks")
}

func (f *FakeCloud) DockerLoadImage(region string, reader io.Reader, projectLabels map[string]string) error {
	f.record("DockerLoadImage")
	return f.err("DockerLoadImage")
}

// --- network placement ---

func (f *FakeCloud) ResolveNetworkPlacement(placement string) (vpc *backends.Network, subnet *backends.Subnet, zone string, err error) {
	f.record("ResolveNetworkPlacement")
	return nil, nil, "", f.err("ResolveNetworkPlacement")
}

// --- VPC peering ---

func (f *FakeCloud) AcceptVPCPeering(peeringConnectionID string) error {
	f.record("AcceptVPCPeering")
	return f.err("AcceptVPCPeering")
}

func (f *FakeCloud) CreateRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string, force bool) error {
	f.record("CreateRoute")
	return f.err("CreateRoute")
}

func (f *FakeCloud) DeleteRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	f.record("DeleteRoute")
	return f.err("DeleteRoute")
}

func (f *FakeCloud) CreateBlackholeRoute(vpcID string, destinationCidrBlock string) error {
	f.record("CreateBlackholeRoute")
	return f.err("CreateBlackholeRoute")
}

func (f *FakeCloud) DeleteBlackholeRoute(vpcID string, destinationCidrBlock string) error {
	f.record("DeleteBlackholeRoute")
	return f.err("DeleteBlackholeRoute")
}

func (f *FakeCloud) AssociateVPCWithHostedZone(hostedZoneID string, vpcID string, region string) error {
	f.record("AssociateVPCWithHostedZone")
	return f.err("AssociateVPCWithHostedZone")
}

func (f *FakeCloud) GetVPCRouteCIDRs(vpcID string) ([]string, error) {
	f.record("GetVPCRouteCIDRs")
	return nil, f.err("GetVPCRouteCIDRs")
}

func (f *FakeCloud) FindAvailableCloudCIDR(vpcID string, requestedCIDR string) (cidr string, isRequested bool, err error) {
	f.record("FindAvailableCloudCIDR")
	return "", false, f.err("FindAvailableCloudCIDR")
}

func (f *FakeCloud) CheckRouteExists(vpcID string, peeringConnectionID string, destinationCidrBlock string) (bool, error) {
	f.record("CheckRouteExists")
	return false, f.err("CheckRouteExists")
}

func (f *FakeCloud) CheckVPCHostedZoneAssociation(hostedZoneID string, vpcID string) (bool, error) {
	f.record("CheckVPCHostedZoneAssociation")
	return false, f.err("CheckVPCHostedZoneAssociation")
}

// --- account ---

func (f *FakeCloud) GetAccountID() (string, error) {
	f.record("GetAccountID")
	return f.AccountID, f.err("GetAccountID")
}

// --- migration ---

func (f *FakeCloud) MigrateV7Resources(input *backends.MigrateV7Input) (*backends.MigrationResult, error) {
	f.record("MigrateV7Resources")
	return nil, f.err("MigrateV7Resources")
}

// compile-time assertion that FakeCloud satisfies backends.Cloud.
var _ backends.Cloud = (*FakeCloud)(nil)

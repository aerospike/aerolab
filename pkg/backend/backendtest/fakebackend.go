package backendtest

import (
	"io"
	"sync"
	"time"

	"github.com/citrusleaf/aerolab/pkg/backend/backends"
	"github.com/citrusleaf/aerolab/pkg/backend/clouds"
)

// FakeBackend is a programmable test double implementing backends.Backend. It
// serves a seeded *backends.Inventory and records the names of invoked methods
// in Calls. Each method returns the error stored in Errs[name] (nil if unset).
//
// Note: inventory list actions (e.g. Inventory.Instances.Terminate) do NOT go
// through FakeBackend; they dispatch through the global backend registry keyed
// by BackendType. To intercept those, register a FakeCloud via RegisterFakeCloud
// and give the fixture instances that backend type.
type FakeBackend struct {
	mu sync.Mutex

	// Inventory is returned by GetInventory / GetRefreshedInventory.
	Inventory *backends.Inventory
	// Credentials is returned by GetCredentials.
	Credentials *clouds.Credentials

	// Calls records, in order, the names of the methods invoked.
	Calls []string
	// Errs maps a method name to the error it should return.
	Errs map[string]error

	// Canned return values.
	EnabledRegions   []string
	AvailableZones   []string
	InstanceTypes    backends.InstanceTypeList
	VolumePrices     backends.VolumePriceList
	ExpiryListResult *backends.ExpiryList
	AccountID        string

	// Optional hooks for create actions; when nil the canned *Output fields are
	// returned instead.
	CreateInstancesFunc  func(*backends.CreateInstanceInput, time.Duration) (*backends.CreateInstanceOutput, error)
	CreateInstanceOutput *backends.CreateInstanceOutput
	CreateFirewallOutput *backends.CreateFirewallOutput
	CreateVolumeOutput   *backends.CreateVolumeOutput
	CreateImageOutput    *backends.CreateImageOutput
}

// NewFakeBackend returns a FakeBackend seeded with the given inventory (a nil
// inventory is replaced with an empty one) and an empty credentials object.
func NewFakeBackend(inv *backends.Inventory) *FakeBackend {
	if inv == nil {
		inv = NewInventory(nil)
	}
	return &FakeBackend{
		Inventory:   inv,
		Credentials: &clouds.Credentials{},
	}
}

func (f *FakeBackend) record(name string) {
	f.mu.Lock()
	f.Calls = append(f.Calls, name)
	f.mu.Unlock()
}

func (f *FakeBackend) err(name string) error {
	if f.Errs == nil {
		return nil
	}
	return f.Errs[name]
}

// CallCount returns how many times the named method has been invoked.
func (f *FakeBackend) CallCount(name string) int {
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

func (f *FakeBackend) GetCredentials() *clouds.Credentials {
	f.record("GetCredentials")
	return f.Credentials
}

func (f *FakeBackend) GetInventory() *backends.Inventory {
	f.record("GetInventory")
	if f.Inventory == nil {
		f.Inventory = NewInventory(nil)
	}
	return f.Inventory
}

func (f *FakeBackend) ForceRefreshInventory() error {
	f.record("ForceRefreshInventory")
	return f.err("ForceRefreshInventory")
}

func (f *FakeBackend) RefreshChangedInventory() error {
	f.record("RefreshChangedInventory")
	return f.err("RefreshChangedInventory")
}

func (f *FakeBackend) GetRefreshedInventory() (*backends.Inventory, error) {
	f.record("GetRefreshedInventory")
	return f.GetInventory(), f.err("GetRefreshedInventory")
}

func (f *FakeBackend) AddRegion(backendType backends.BackendType, names ...string) error {
	f.record("AddRegion")
	return f.err("AddRegion")
}

func (f *FakeBackend) RemoveRegion(backendType backends.BackendType, names ...string) error {
	f.record("RemoveRegion")
	return f.err("RemoveRegion")
}

func (f *FakeBackend) ListEnabledRegions(backendType backends.BackendType) (name []string, err error) {
	f.record("ListEnabledRegions")
	return f.EnabledRegions, f.err("ListEnabledRegions")
}

func (f *FakeBackend) ListAvailableZones(backendType backends.BackendType) (zones []string, err error) {
	f.record("ListAvailableZones")
	return f.AvailableZones, f.err("ListAvailableZones")
}

func (f *FakeBackend) CreateFirewall(input *backends.CreateFirewallInput, waitDur time.Duration) (*backends.CreateFirewallOutput, error) {
	f.record("CreateFirewall")
	return f.CreateFirewallOutput, f.err("CreateFirewall")
}

func (f *FakeBackend) CreateVolume(input *backends.CreateVolumeInput) (*backends.CreateVolumeOutput, error) {
	f.record("CreateVolume")
	return f.CreateVolumeOutput, f.err("CreateVolume")
}

func (f *FakeBackend) CreateVolumeGetPrice(input *backends.CreateVolumeInput) (costGB float64, err error) {
	f.record("CreateVolumeGetPrice")
	return 0, f.err("CreateVolumeGetPrice")
}

func (f *FakeBackend) CreateImage(input *backends.CreateImageInput, waitDur time.Duration) (*backends.CreateImageOutput, error) {
	f.record("CreateImage")
	return f.CreateImageOutput, f.err("CreateImage")
}

func (f *FakeBackend) CreateInstances(input *backends.CreateInstanceInput, waitDur time.Duration) (*backends.CreateInstanceOutput, error) {
	f.record("CreateInstances")
	if f.CreateInstancesFunc != nil {
		return f.CreateInstancesFunc(input, waitDur)
	}
	return f.CreateInstanceOutput, f.err("CreateInstances")
}

func (f *FakeBackend) CreateInstancesGetPrice(input *backends.CreateInstanceInput) (costPPH, costGB float64, err error) {
	f.record("CreateInstancesGetPrice")
	return 0, 0, f.err("CreateInstancesGetPrice")
}

func (f *FakeBackend) CleanupDNS() error {
	f.record("CleanupDNS")
	return f.err("CleanupDNS")
}

func (f *FakeBackend) DeleteProjectResources(backendType backends.BackendType) error {
	f.record("DeleteProjectResources")
	return f.err("DeleteProjectResources")
}

func (f *FakeBackend) ExpiryInstall(backendType backends.BackendType, intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error {
	f.record("ExpiryInstall")
	return f.err("ExpiryInstall")
}

func (f *FakeBackend) ExpiryRemove(backendType backends.BackendType, zones ...string) error {
	f.record("ExpiryRemove")
	return f.err("ExpiryRemove")
}

func (f *FakeBackend) ExpiryChangeFrequency(backendType backends.BackendType, intervalMinutes int, zones ...string) error {
	f.record("ExpiryChangeFrequency")
	return f.err("ExpiryChangeFrequency")
}

func (f *FakeBackend) ExpiryList() (*backends.ExpiryList, error) {
	f.record("ExpiryList")
	return f.ExpiryListResult, f.err("ExpiryList")
}

func (f *FakeBackend) ExpiryChangeConfiguration(backendType backends.BackendType, logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error {
	f.record("ExpiryChangeConfiguration")
	return f.err("ExpiryChangeConfiguration")
}

func (f *FakeBackend) ExpiryV7Check(backendType backends.BackendType) (found bool, regions []string, err error) {
	f.record("ExpiryV7Check")
	return false, nil, f.err("ExpiryV7Check")
}

func (f *FakeBackend) Close() error {
	f.record("Close")
	return f.err("Close")
}

func (f *FakeBackend) DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error {
	f.record("DockerCreateNetwork")
	return f.err("DockerCreateNetwork")
}

func (f *FakeBackend) DockerDeleteNetwork(region string, name string) error {
	f.record("DockerDeleteNetwork")
	return f.err("DockerDeleteNetwork")
}

func (f *FakeBackend) DockerPruneNetworks(region string) error {
	f.record("DockerPruneNetworks")
	return f.err("DockerPruneNetworks")
}

func (f *FakeBackend) DockerLoadImage(region string, reader io.Reader, projectLabels map[string]string) error {
	f.record("DockerLoadImage")
	return f.err("DockerLoadImage")
}

func (f *FakeBackend) GetVolumePrices(backendType backends.BackendType) (backends.VolumePriceList, error) {
	f.record("GetVolumePrices")
	return f.VolumePrices, f.err("GetVolumePrices")
}

func (f *FakeBackend) GetInstanceTypes(backendType backends.BackendType) (backends.InstanceTypeList, error) {
	f.record("GetInstanceTypes")
	return f.InstanceTypes, f.err("GetInstanceTypes")
}

func (f *FakeBackend) ResolveNetworkPlacement(backendType backends.BackendType, placement string) (vpc *backends.Network, subnet *backends.Subnet, zone string, err error) {
	f.record("ResolveNetworkPlacement")
	return nil, nil, "", f.err("ResolveNetworkPlacement")
}

func (f *FakeBackend) AcceptVPCPeering(backendType backends.BackendType, peeringConnectionID string) error {
	f.record("AcceptVPCPeering")
	return f.err("AcceptVPCPeering")
}

func (f *FakeBackend) CreateRoute(backendType backends.BackendType, vpcID string, peeringConnectionID string, destinationCidrBlock string, force bool) error {
	f.record("CreateRoute")
	return f.err("CreateRoute")
}

func (f *FakeBackend) DeleteRoute(backendType backends.BackendType, vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	f.record("DeleteRoute")
	return f.err("DeleteRoute")
}

func (f *FakeBackend) CreateBlackholeRoute(backendType backends.BackendType, vpcID string, destinationCidrBlock string) error {
	f.record("CreateBlackholeRoute")
	return f.err("CreateBlackholeRoute")
}

func (f *FakeBackend) DeleteBlackholeRoute(backendType backends.BackendType, vpcID string, destinationCidrBlock string) error {
	f.record("DeleteBlackholeRoute")
	return f.err("DeleteBlackholeRoute")
}

func (f *FakeBackend) AssociateVPCWithHostedZone(backendType backends.BackendType, hostedZoneID string, vpcID string, region string) error {
	f.record("AssociateVPCWithHostedZone")
	return f.err("AssociateVPCWithHostedZone")
}

func (f *FakeBackend) GetVPCRouteCIDRs(backendType backends.BackendType, vpcID string) ([]string, error) {
	f.record("GetVPCRouteCIDRs")
	return nil, f.err("GetVPCRouteCIDRs")
}

func (f *FakeBackend) FindAvailableCloudCIDR(backendType backends.BackendType, vpcID string, requestedCIDR string) (cidr string, isRequested bool, err error) {
	f.record("FindAvailableCloudCIDR")
	return "", false, f.err("FindAvailableCloudCIDR")
}

func (f *FakeBackend) CheckRouteExists(backendType backends.BackendType, vpcID string, peeringConnectionID string, destinationCidrBlock string) (bool, error) {
	f.record("CheckRouteExists")
	return false, f.err("CheckRouteExists")
}

func (f *FakeBackend) CheckVPCHostedZoneAssociation(backendType backends.BackendType, hostedZoneID string, vpcID string) (bool, error) {
	f.record("CheckVPCHostedZoneAssociation")
	return false, f.err("CheckVPCHostedZoneAssociation")
}

func (f *FakeBackend) GetAccountID(backendType backends.BackendType) (string, error) {
	f.record("GetAccountID")
	return f.AccountID, f.err("GetAccountID")
}

func (f *FakeBackend) MigrateV7Resources(backendType backends.BackendType, input *backends.MigrateV7Input) (*backends.MigrationResult, error) {
	f.record("MigrateV7Resources")
	return nil, f.err("MigrateV7Resources")
}

// compile-time assertion that FakeBackend satisfies backends.Backend.
var _ backends.Backend = (*FakeBackend)(nil)

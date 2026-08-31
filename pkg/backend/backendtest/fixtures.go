package backendtest

import (
	"strconv"
	"time"

	"github.com/citrusleaf/aerolab/pkg/backend/backends"
)

// InstanceOption mutates an Instance being built by NewInstance.
type InstanceOption func(*backends.Instance)

// WithBackendType sets the backend type of the instance. Defaults to
// BackendTypeDocker when not specified.
func WithBackendType(bt backends.BackendType) InstanceOption {
	return func(i *backends.Instance) { i.BackendType = bt }
}

// WithState sets the instance lifecycle state. Defaults to Running.
func WithState(s backends.LifeCycleState) InstanceOption {
	return func(i *backends.Instance) { i.InstanceState = s }
}

// WithPrivateIP sets the private IP address.
func WithPrivateIP(ip string) InstanceOption {
	return func(i *backends.Instance) { i.IP.Private = ip }
}

// WithPublicIP sets the public IP address.
func WithPublicIP(ip string) InstanceOption {
	return func(i *backends.Instance) { i.IP.Public = ip }
}

// WithOwner sets the owner.
func WithOwner(owner string) InstanceOption {
	return func(i *backends.Instance) { i.Owner = owner }
}

// WithInstanceType sets the instance type / machine type string.
func WithInstanceType(it string) InstanceOption {
	return func(i *backends.Instance) { i.InstanceType = it }
}

// WithExpires sets the expiry time.
func WithExpires(t time.Time) InstanceOption {
	return func(i *backends.Instance) { i.Expires = t }
}

// WithOS sets the operating system name and version.
func WithOS(name, version string) InstanceOption {
	return func(i *backends.Instance) { i.OperatingSystem = backends.OS{Name: name, Version: version} }
}

// WithArchitecture sets the CPU architecture.
func WithArchitecture(a backends.Architecture) InstanceOption {
	return func(i *backends.Instance) { i.Architecture = a }
}

// WithTags sets (replaces) the tags map.
func WithTags(tags map[string]string) InstanceOption {
	return func(i *backends.Instance) { i.Tags = tags }
}

// WithAerolabType sets the "aerolab.type" tag (e.g. "server", "aerospike",
// "client", "agi"). This is what InstanceList.WithType filters on, so cluster
// and client CLI command helpers need it set to find fixture instances.
func WithAerolabType(typ string) InstanceOption {
	return func(i *backends.Instance) {
		if i.Tags == nil {
			i.Tags = map[string]string{}
		}
		i.Tags["aerolab.type"] = typ
	}
}

// NewInstance builds a single Instance fixture for the given cluster name and
// node number, with sensible defaults (docker backend, Running state, a derived
// name and private IP). Override any field with the provided options.
func NewInstance(clusterName string, nodeNo int, opts ...InstanceOption) *backends.Instance {
	i := &backends.Instance{
		ClusterName:   clusterName,
		NodeNo:        nodeNo,
		BackendType:   backends.BackendTypeDocker,
		InstanceState: backends.LifeCycleStateRunning,
		InstanceID:    clusterName + "-" + itoa(nodeNo),
		Name:          clusterName + "-" + itoa(nodeNo),
		IP:            backends.IP{Private: "10.0.0." + itoa(nodeNo)},
	}
	for _, o := range opts {
		o(i)
	}
	return i
}

// NewCluster builds an InstanceList of `count` instances (node numbers 1..count)
// for the given cluster name, applying the same options to every node.
func NewCluster(clusterName string, count int, opts ...InstanceOption) backends.InstanceList {
	out := make(backends.InstanceList, 0, count)
	for n := 1; n <= count; n++ {
		out = append(out, NewInstance(clusterName, n, opts...))
	}
	return out
}

// NewInventory builds an *Inventory from the provided instances. Volumes,
// images, networks and firewalls default to empty lists; set them directly on
// the returned struct if needed.
func NewInventory(instances backends.InstanceList) *backends.Inventory {
	return &backends.Inventory{
		Networks:  backends.NetworkList{},
		Firewalls: backends.FirewallList{},
		Volumes:   backends.VolumeList{},
		Instances: instances,
		Images:    backends.ImageList{},
	}
}

// NewVolume builds a minimal Volume fixture.
func NewVolume(name string, bt backends.BackendType) *backends.Volume {
	return &backends.Volume{
		Name:        name,
		BackendType: bt,
		State:       backends.VolumeStateAvailable,
	}
}

// NewImage builds a minimal Image fixture.
func NewImage(name string, bt backends.BackendType) *backends.Image {
	return &backends.Image{
		Name:        name,
		BackendType: bt,
		State:       backends.VolumeStateAvailable,
	}
}

// NewNetwork builds a minimal Network fixture.
func NewNetwork(name string, bt backends.BackendType) *backends.Network {
	return &backends.Network{
		Name:        name,
		BackendType: bt,
		State:       backends.NetworkStateAvailable,
	}
}

// NewFirewall builds a minimal Firewall fixture.
func NewFirewall(name string, bt backends.BackendType) *backends.Firewall {
	return &backends.Firewall{
		Name:        name,
		BackendType: bt,
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

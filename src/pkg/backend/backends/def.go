package backends

import "github.com/aerospike/aerolab/pkg/backend"

var backends = make(map[backend.BackendType]Backend)

type Backend interface {
	ListEnabledZones() ([]string, error)
	EnableZone(name string) error
	DisableZone(name string) error
	GetVolumes() (backend.VolumeList, error)
	GetInstances() (backend.InstanceList, error)
}

func Register(b Backend, name backend.BackendType) {
	backends[name] = b
}

func Get() map[backend.BackendType]Backend {
	return backends
}

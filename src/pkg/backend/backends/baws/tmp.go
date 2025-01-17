package baws

import (
	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type b struct{}

func init() {
	backends.Register(&b{}, backend.BackendTypeAWS)
}

func (s *b) ListEnabledZones() ([]string, error) {
	return nil, nil
}
func (s *b) EnableZone(name string) error {
	return nil
}
func (s *b) DisableZone(name string) error {
	return nil
}
func (s *b) GetVolumes() (backend.VolumeList, error) {
	return nil, nil
}
func (s *b) GetInstances() (backend.InstanceList, error) {
	return nil, nil
}

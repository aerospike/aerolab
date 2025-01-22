package baws

import "github.com/aerospike/aerolab/pkg/backend"

func (s *b) GetVolumes() (backend.VolumeList, error) {
	// TODO: actually get volumes from AWS
	return nil, nil
}

func (s *b) VolumesAddTags(volumes backend.VolumeList, tags map[string]string, waitDur int) error {
	return nil
}

func (s *b) VolumesRemoveTags(volumes backend.VolumeList, tagKeys []string, waitDur int) error {
	return nil
}
func (s *b) DeleteVolumes(volumes backend.VolumeList, waitDur int) error {
	return nil
}

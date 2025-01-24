package baws

import (
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
)

// TODO: maybe, if speed is questionable: if dealing with functions that do not require the whole inventory, and we don't have a valid state cache, allow for partial inventory poll/get only (no need for volumes if we are attaching to an instance, duh)

func (s *b) GetVolumes() (backend.VolumeList, error) {
	// TODO: actually get volumes from AWS
	return nil, nil
}

func (s *b) VolumesAddTags(volumes backend.VolumeList, tags map[string]string, waitDur time.Duration) error {
	return nil
}

func (s *b) VolumesRemoveTags(volumes backend.VolumeList, tagKeys []string, waitDur time.Duration) error {
	return nil
}

func (s *b) DeleteVolumes(volumes backend.VolumeList, waitDur time.Duration) error {
	return nil
}

func (s *b) AttachVolumes(volumes backend.VolumeList, instance *backend.Instance, mountTargetDirectory *string) error {
	return nil
}

func (s *b) DetachVolumes(volumes backend.VolumeList, instance *backend.Instance) error {
	return nil
}

func (s *b) ResizeVolumes(volumes backend.VolumeList, newSize backend.StorageSize) error {
	return nil
}

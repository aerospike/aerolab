package bvagrant

import (
	"errors"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
)

// GetVolumes retrieves all Vagrant volumes (not implemented - Vagrant uses local filesystems).
//
// Returns:
//   - backends.VolumeList: empty list
//   - error: nil
func (s *b) GetVolumes() (backends.VolumeList, error) {
	log := s.log.WithPrefix("GetVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// Vagrant doesn't have a separate volume concept like cloud providers
	// Volumes are typically managed as synced folders in the Vagrantfile
	return backends.VolumeList{}, nil
}

// VolumesAddTags adds tags to volumes (not implemented for Vagrant).
func (s *b) VolumesAddTags(volumes backends.VolumeList, tags map[string]string, waitDur time.Duration) error {
	log := s.log.WithPrefix("VolumesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

// VolumesRemoveTags removes tags from volumes (not implemented for Vagrant).
func (s *b) VolumesRemoveTags(volumes backends.VolumeList, tagKeys []string, waitDur time.Duration) error {
	log := s.log.WithPrefix("VolumesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

// DeleteVolumes deletes volumes (not implemented for Vagrant).
func (s *b) DeleteVolumes(volumes backends.VolumeList, fw backends.FirewallList, waitDur time.Duration) error {
	log := s.log.WithPrefix("DeleteVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

// ResizeVolumes resizes volumes (not implemented for Vagrant).
func (s *b) ResizeVolumes(volumes backends.VolumeList, newSizeGiB backends.StorageSize, waitDur time.Duration) error {
	log := s.log.WithPrefix("ResizeVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

// AttachVolumes attaches volumes to instances (not implemented for Vagrant).
func (s *b) AttachVolumes(volumes backends.VolumeList, instance *backends.Instance, sharedMountData *backends.VolumeAttachShared, waitDur time.Duration) error {
	log := s.log.WithPrefix("AttachVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

// DetachVolumes detaches volumes from instances (not implemented for Vagrant).
func (s *b) DetachVolumes(volumes backends.VolumeList, instance *backends.Instance, waitDur time.Duration) error {
	log := s.log.WithPrefix("DetachVolumes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

// CreateVolumeGetPrice returns volume pricing (always 0 for Vagrant).
func (s *b) CreateVolumeGetPrice(input *backends.CreateVolumeInput) (costGB float64, err error) {
	log := s.log.WithPrefix("CreateVolumeGetPrice: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return 0, nil
}

// CreateVolume creates a volume (not implemented for Vagrant).
func (s *b) CreateVolume(input *backends.CreateVolumeInput) (output *backends.CreateVolumeOutput, err error) {
	log := s.log.WithPrefix("CreateVolume: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return nil, errors.New("not implemented")
}

// VolumesChangeExpiry changes volume expiry (not implemented for Vagrant).
func (s *b) VolumesChangeExpiry(volumes backends.VolumeList, expiry time.Time) error {
	log := s.log.WithPrefix("VolumesChangeExpiry: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

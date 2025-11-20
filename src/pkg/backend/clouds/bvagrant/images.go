package bvagrant

import (
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
)

// GetImages retrieves all Vagrant boxes/images.
//
// Returns:
//   - backends.ImageList: list of available Vagrant boxes
//   - error: nil on success, or an error describing what failed
//
// Note: This implementation returns a curated list of commonly used boxes.
// In a production environment, this could query the local Vagrant box cache
// or the Vagrant Cloud API.
func (s *b) GetImages() (backends.ImageList, error) {
	log := s.log.WithPrefix("GetImages: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// Return a curated list of commonly used boxes
	// In a production implementation, this could:
	// 1. Query local boxes with `vagrant box list`
	// 2. Query Vagrant Cloud API for available boxes
	// 3. Parse a configuration file with supported boxes

	images := backends.ImageList{
		&backends.Image{
			BackendType:  backends.BackendTypeVagrant,
			ImageId:      "ubuntu/jammy64",
			Name:         "ubuntu/jammy64",
			Architecture: backends.ArchitectureX8664,
			OSName:       "ubuntu",
			OSVersion:    "22.04",
			Username:     "vagrant",
			Public:       true,
			InAccount:    false,
			ZoneName:     "default",
			ZoneID:       "default",
		},
		&backends.Image{
			BackendType:  backends.BackendTypeVagrant,
			ImageId:      "ubuntu/focal64",
			Name:         "ubuntu/focal64",
			Architecture: backends.ArchitectureX8664,
			OSName:       "ubuntu",
			OSVersion:    "20.04",
			Username:     "vagrant",
			Public:       true,
			InAccount:    false,
			ZoneName:     "default",
			ZoneID:       "default",
		},
		&backends.Image{
			BackendType:  backends.BackendTypeVagrant,
			ImageId:      "generic/ubuntu2204",
			Name:         "generic/ubuntu2204",
			Architecture: backends.ArchitectureX8664,
			OSName:       "ubuntu",
			OSVersion:    "22.04",
			Username:     "vagrant",
			Public:       true,
			InAccount:    false,
			ZoneName:     "default",
			ZoneID:       "default",
		},
		&backends.Image{
			BackendType:  backends.BackendTypeVagrant,
			ImageId:      "generic/ubuntu2004",
			Name:         "generic/ubuntu2004",
			Architecture: backends.ArchitectureX8664,
			OSName:       "ubuntu",
			OSVersion:    "20.04",
			Username:     "vagrant",
			Public:       true,
			InAccount:    false,
			ZoneName:     "default",
			ZoneID:       "default",
		},
		&backends.Image{
			BackendType:  backends.BackendTypeVagrant,
			ImageId:      "centos/7",
			Name:         "centos/7",
			Architecture: backends.ArchitectureX8664,
			OSName:       "centos",
			OSVersion:    "7",
			Username:     "vagrant",
			Public:       true,
			InAccount:    false,
			ZoneName:     "default",
			ZoneID:       "default",
		},
		&backends.Image{
			BackendType:  backends.BackendTypeVagrant,
			ImageId:      "generic/rhel8",
			Name:         "generic/rhel8",
			Architecture: backends.ArchitectureX8664,
			OSName:       "rhel",
			OSVersion:    "8",
			Username:     "vagrant",
			Public:       true,
			InAccount:    false,
			ZoneName:     "default",
			ZoneID:       "default",
		},
		&backends.Image{
			BackendType:  backends.BackendTypeVagrant,
			ImageId:      "generic/rhel9",
			Name:         "generic/rhel9",
			Architecture: backends.ArchitectureX8664,
			OSName:       "rhel",
			OSVersion:    "9",
			Username:     "vagrant",
			Public:       true,
			InAccount:    false,
			ZoneName:     "default",
			ZoneID:       "default",
		},
		&backends.Image{
			BackendType:  backends.BackendTypeVagrant,
			ImageId:      "debian/bullseye64",
			Name:         "debian/bullseye64",
			Architecture: backends.ArchitectureX8664,
			OSName:       "debian",
			OSVersion:    "11",
			Username:     "vagrant",
			Public:       true,
			InAccount:    false,
			ZoneName:     "default",
			ZoneID:       "default",
		},
	}

	s.images = images
	return images, nil
}

// CreateImage creates a new Vagrant box/image (not implemented).
func (s *b) CreateImage(input *backends.CreateImageInput, waitDur time.Duration) (*backends.CreateImageOutput, error) {
	log := s.log.WithPrefix("CreateImage: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateImage")
}

// ImagesDelete deletes Vagrant boxes/images (not implemented).
func (s *b) ImagesDelete(images backends.ImageList, waitDur time.Duration) error {
	log := s.log.WithPrefix("ImagesDelete: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ImagesDelete")
}

// ImagesAddTags adds tags to images (not implemented for Vagrant).
func (s *b) ImagesAddTags(images backends.ImageList, tags map[string]string) error {
	log := s.log.WithPrefix("ImagesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ImagesAddTags")
}

// ImagesRemoveTags removes tags from images (not implemented for Vagrant).
func (s *b) ImagesRemoveTags(images backends.ImageList, tagKeys []string) error {
	log := s.log.WithPrefix("ImagesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ImagesRemoveTags")
}

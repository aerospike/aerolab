package bvagrant

import (
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
)

// GetVolumePrices returns volume pricing information (always empty for Vagrant).
//
// Returns:
//   - backends.VolumePriceList: empty list (Vagrant is free)
//   - error: nil
func (s *b) GetVolumePrices() (backends.VolumePriceList, error) {
	log := s.log.WithPrefix("GetVolumePrices: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// Vagrant is a local virtualization tool, there are no costs
	return backends.VolumePriceList{}, nil
}

// GetInstanceTypes returns instance type information for Vagrant.
//
// Returns:
//   - backends.InstanceTypeList: list of example instance types
//   - error: nil
//
// Note: Vagrant instance "types" are flexible and defined by the user.
// This returns example configurations that users can base their instances on.
func (s *b) GetInstanceTypes() (backends.InstanceTypeList, error) {
	log := s.log.WithPrefix("GetInstanceTypes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// Return example instance type configurations
	// Users can customize CPU and memory in their CreateInstanceParams
	types := backends.InstanceTypeList{
		&backends.InstanceType{
			Name:             "small",
			Region:           "default",
			CPUs:             2,
			GPUs:             0,
			MemoryGiB:        2,
			NvmeCount:        0,
			NvmeTotalSizeGiB: 0,
			Arch: backends.Architectures{
				backends.ArchitectureX8664,
			},
			PricePerHour: backends.InstanceTypePrice{
				OnDemand: 0,
				Spot:     0,
				Currency: "USD",
			},
		},
		&backends.InstanceType{
			Name:             "medium",
			Region:           "default",
			CPUs:             4,
			GPUs:             0,
			MemoryGiB:        4,
			NvmeCount:        0,
			NvmeTotalSizeGiB: 0,
			Arch: backends.Architectures{
				backends.ArchitectureX8664,
			},
			PricePerHour: backends.InstanceTypePrice{
				OnDemand: 0,
				Spot:     0,
				Currency: "USD",
			},
		},
		&backends.InstanceType{
			Name:             "large",
			Region:           "default",
			CPUs:             8,
			GPUs:             0,
			MemoryGiB:        8,
			NvmeCount:        0,
			NvmeTotalSizeGiB: 0,
			Arch: backends.Architectures{
				backends.ArchitectureX8664,
			},
			PricePerHour: backends.InstanceTypePrice{
				OnDemand: 0,
				Spot:     0,
				Currency: "USD",
			},
		},
		&backends.InstanceType{
			Name:             "xlarge",
			Region:           "default",
			CPUs:             16,
			GPUs:             0,
			MemoryGiB:        16,
			NvmeCount:        0,
			NvmeTotalSizeGiB: 0,
			Arch: backends.Architectures{
				backends.ArchitectureX8664,
			},
			PricePerHour: backends.InstanceTypePrice{
				OnDemand: 0,
				Spot:     0,
				Currency: "USD",
			},
		},
	}

	return types, nil
}

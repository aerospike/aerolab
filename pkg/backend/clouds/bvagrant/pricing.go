package bvagrant

import "github.com/aerospike/aerolab/pkg/backend/backends"

// GetVolumePrices, GetInstanceTypes, CreateInstancesGetPrice, and CreateVolumeGetPrice
// all return zero-cost synthetic entries: vagrant runs on the operator's own hardware,
// so there is no cloud billing to model.

func (s *b) GetVolumePrices() (backends.VolumePriceList, error) {
	return backends.VolumePriceList{
		{
			Type:           "vagrant-volume",
			Region:         "local",
			PricePerGBHour: 0,
			Currency:       "USD",
		},
	}, nil
}

func (s *b) GetInstanceTypes() (backends.InstanceTypeList, error) {
	return backends.InstanceTypeList{
		{
			Name:      "vagrant-instance",
			Region:    "local",
			CPUs:      0,
			MemoryGiB: 0,
			Arch:      []backends.Architecture{backends.ArchitectureX8664, backends.ArchitectureARM64},
			PricePerHour: backends.InstanceTypePrice{
				OnDemand: 0,
				Spot:     0,
				Currency: "USD",
			},
		},
	}, nil
}

func (s *b) CreateInstancesGetPrice(input *backends.CreateInstanceInput) (costPPH, costGB float64, err error) {
	return 0, 0, nil
}

func (s *b) CreateVolumeGetPrice(input *backends.CreateVolumeInput) (costGB float64, err error) {
	return 0, nil
}

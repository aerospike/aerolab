package bdocker

import (
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
)

func (s *b) GetVolumePrice(region string, volumeType string) (*backends.VolumePrice, error) {
	log := s.log.WithPrefix("GetVolumePrice: job=" + shortuuid.New() + " region=" + region + " volumeType=" + volumeType + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return &backends.VolumePrice{
		Type:           "docker-volume",
		Region:         region,
		PricePerGBHour: 0,
		Currency:       "USD",
	}, nil
}

func (s *b) GetVolumePrices() (backends.VolumePriceList, error) {
	log := s.log.WithPrefix("GetVolumePrices: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	regions, _ := s.ListEnabledZones()
	prices := backends.VolumePriceList{}
	for _, region := range regions {
		prices = append(prices, &backends.VolumePrice{
			Type:           "docker-volume",
			Region:         region,
			PricePerGBHour: 0,
			Currency:       "USD",
		})
	}
	return prices, nil
}

func (s *b) GetInstanceType(region string, instanceType string) (*backends.InstanceType, error) {
	log := s.log.WithPrefix("GetInstanceType: job=" + shortuuid.New() + " region=" + region + " instanceType=" + instanceType + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return &backends.InstanceType{
		Region:           region,
		Name:             "docker-instance",
		CPUs:             0,
		MemoryGiB:        0,
		Arch:             []backends.Architecture{backends.ArchitectureX8664, backends.ArchitectureARM64},
		NvmeCount:        0,
		NvmeTotalSizeGiB: 0,
		PricePerHour: backends.InstanceTypePrice{
			OnDemand: 0,
			Spot:     0,
			Currency: "USD",
		},
	}, nil
}

func (s *b) GetInstanceTypes() (backends.InstanceTypeList, error) {
	log := s.log.WithPrefix("GetInstanceTypes: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	types := backends.InstanceTypeList{}
	regions, _ := s.ListEnabledZones()
	for _, region := range regions {
		types = append(types, &backends.InstanceType{
			Region:           region,
			Name:             "docker-instance",
			CPUs:             0,
			MemoryGiB:        0,
			Arch:             []backends.Architecture{backends.ArchitectureX8664, backends.ArchitectureARM64},
			NvmeCount:        0,
			NvmeTotalSizeGiB: 0,
			PricePerHour: backends.InstanceTypePrice{
				OnDemand: 0,
				Spot:     0,
				Currency: "USD",
			},
		})
	}
	return types, nil
}

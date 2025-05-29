package backends

import (
	"fmt"
	"time"

	"github.com/aerospike/aerolab/pkg/structtags"
)

func (s *backend) CreateFirewall(input *CreateFirewallInput, waitDur time.Duration) (output *CreateFirewallOutput, err error) {
	start := time.Now()
	defer func() {
		s.log.Detail("CreateFirewall: err=%v, took=%v", err, time.Since(start))
	}()
	return s.enabledBackends[input.BackendType].CreateFirewall(input, waitDur)
}

func (s *backend) CreateVolume(input *CreateVolumeInput) (output *CreateVolumeOutput, err error) {
	start := time.Now()
	defer func() {
		s.log.Detail("CreateVolume: err=%v, took=%v", err, time.Since(start))
	}()
	return s.enabledBackends[input.BackendType].CreateVolume(input)
}

func (s *backend) CreateImage(input *CreateImageInput, waitDur time.Duration) (output *CreateImageOutput, err error) {
	start := time.Now()
	defer func() {
		s.log.Detail("CreateImage: err=%v, took=%v", err, time.Since(start))
	}()
	return s.enabledBackends[input.BackendType].CreateImage(input, waitDur)
}

func (s *backend) CreateInstances(input *CreateInstanceInput, waitDur time.Duration) (output *CreateInstanceOutput, err error) {
	start := time.Now()
	defer func() {
		s.log.Detail("CreateInstances: err=%v, took=%v", err, time.Since(start))
	}()
	if err := structtags.CheckRequired(input); err != nil {
		return nil, fmt.Errorf("required fields missing: %w", err)
	}
	return s.enabledBackends[input.BackendType].CreateInstances(input, waitDur)
}

func (s *backend) CreateInstancesGetPrice(input *CreateInstanceInput) (costPPH, costGB float64, err error) {
	start := time.Now()
	defer func() {
		s.log.Detail("CreateInstancesGetPrice: costPPH=%f, costGB=%f, err=%v, took=%v", costPPH, costGB, err, time.Since(start))
	}()
	return s.enabledBackends[input.BackendType].CreateInstancesGetPrice(input)
}

func (s *backend) CreateVolumeGetPrice(input *CreateVolumeInput) (costGB float64, err error) {
	start := time.Now()
	defer func() {
		s.log.Detail("CreateVolumeGetPrice: costGB=%f, err=%v, took=%v", costGB, err, time.Since(start))
	}()
	return s.enabledBackends[input.BackendType].CreateVolumeGetPrice(input)
}

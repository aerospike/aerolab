package backend

import "time"

func (s *backend) CreateFirewall(input *CreateFirewallInput, waitDur time.Duration) (output *CreateFirewallOutput, err error) {
	return cloudList[input.BackendType].CreateFirewall(input, waitDur)
}

func (s *backend) CreateVolume(input *CreateVolumeInput) (output *CreateVolumeOutput, err error) {
	return cloudList[input.BackendType].CreateVolume(input)
}

func (s *backend) CreateImage(input *CreateImageInput, waitDur time.Duration) (output *CreateImageOutput, err error) {
	return cloudList[input.BackendType].CreateImage(input, waitDur)
}

func (s *backend) CreateInstances(input *CreateInstanceInput, waitDur time.Duration) (output *CreateInstanceOutput, err error) {
	return cloudList[input.BackendType].CreateInstances(input, waitDur)
}

func (s *backend) CreateInstancesGetPrice(input *CreateInstanceInput) (costPPH, costGB float64, err error) {
	return cloudList[input.BackendType].CreateInstancesGetPrice(input)
}

func (s *backend) CreateVolumeGetPrice(input *CreateVolumeInput) (costGB float64, err error) {
	return cloudList[input.BackendType].CreateVolumeGetPrice(input)
}

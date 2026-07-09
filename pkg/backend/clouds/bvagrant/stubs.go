package bvagrant

import (
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
)

// Phase 1 placeholder methods — full implementations come in later phases.

func (s *b) ExpiryInstall(intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryInstall")
}

func (s *b) ExpiryRemove(zones ...string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryRemove")
}

func (s *b) ExpiryChangeConfiguration(logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryChangeConfiguration")
}

func (s *b) ExpiryList() ([]*backends.ExpirySystem, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryList")
}

func (s *b) ExpiryChangeFrequency(intervalMinutes int, zones ...string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryChangeFrequency")
}

func (s *b) ExpiryV7Check() (found bool, regions []string, err error) {
	return false, nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryV7Check")
}

func (s *b) VolumesChangeExpiry(volumes backends.VolumeList, expiry time.Time) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "VolumesChangeExpiry")
}

func (s *b) InstancesChangeExpiry(instances backends.InstanceList, expiry time.Time) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "InstancesChangeExpiry")
}

func (s *b) GetVolumePrices() (backends.VolumePriceList, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "GetVolumePrices")
}

func (s *b) GetInstanceTypes() (backends.InstanceTypeList, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "GetInstanceTypes")
}

func (s *b) GetVolumes() (backends.VolumeList, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "GetVolumes")
}

func (s *b) GetInstances(volumes backends.VolumeList, networks backends.NetworkList, firewalls backends.FirewallList) (backends.InstanceList, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "GetInstances")
}

func (s *b) GetImages() (backends.ImageList, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "GetImages")
}

func (s *b) GetNetworks() (backends.NetworkList, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "GetNetworks")
}

func (s *b) GetFirewalls(networks backends.NetworkList) (backends.FirewallList, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "GetFirewalls")
}

func (s *b) CreateVolume(input *backends.CreateVolumeInput) (*backends.CreateVolumeOutput, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateVolume")
}

func (s *b) CreateVolumeGetPrice(input *backends.CreateVolumeInput) (costGB float64, err error) {
	return 0, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateVolumeGetPrice")
}

func (s *b) CreateImage(input *backends.CreateImageInput, waitDur time.Duration) (*backends.CreateImageOutput, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateImage")
}

func (s *b) CreateInstances(input *backends.CreateInstanceInput, waitDur time.Duration) (*backends.CreateInstanceOutput, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateInstances")
}

func (s *b) CreateInstancesGetPrice(input *backends.CreateInstanceInput) (costPPH, costGB float64, err error) {
	return 0, 0, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateInstancesGetPrice")
}

func (s *b) InstancesAddTags(instances backends.InstanceList, tags map[string]string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "InstancesAddTags")
}

func (s *b) InstancesRemoveTags(instances backends.InstanceList, tagKeys []string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "InstancesRemoveTags")
}

func (s *b) InstancesTerminate(instances backends.InstanceList, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "InstancesTerminate")
}

func (s *b) InstancesStop(instances backends.InstanceList, force bool, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "InstancesStop")
}

func (s *b) InstancesStart(instances backends.InstanceList, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "InstancesStart")
}

func (s *b) InstancesExec(instances backends.InstanceList, e *backends.ExecInput) []*backends.ExecOutput {
	out := make([]*backends.ExecOutput, 0, len(instances))
	for _, inst := range instances {
		out = append(out, &backends.ExecOutput{Instance: inst, Output: &sshexec.ExecOutput{}})
	}
	return out
}

func (s *b) InstancesGetSftpConfig(instances backends.InstanceList, username string) ([]*sshexec.ClientConf, error) {
	out := make([]*sshexec.ClientConf, 0, len(instances))
	for range instances {
		out = append(out, &sshexec.ClientConf{Username: username})
	}
	return out, nil
}

func (s *b) InstancesGetSSHKeyPath(instances backends.InstanceList) []string {
	out := make([]string, 0, len(instances))
	for range instances {
		out = append(out, "")
	}
	return out
}

func (s *b) InstancesUpdateHostsFile(instances backends.InstanceList, hostsEntries []string, parallelSSHThreads int) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "InstancesUpdateHostsFile")
}

func (s *b) VolumesAddTags(volumes backends.VolumeList, tags map[string]string, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "VolumesAddTags")
}

func (s *b) VolumesRemoveTags(volumes backends.VolumeList, tagKeys []string, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "VolumesRemoveTags")
}

func (s *b) DeleteVolumes(volumes backends.VolumeList, fw backends.FirewallList, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DeleteVolumes")
}

func (s *b) ImagesDelete(images backends.ImageList, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ImagesDelete")
}

func (s *b) ImagesAddTags(images backends.ImageList, tags map[string]string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ImagesAddTags")
}

func (s *b) ImagesRemoveTags(images backends.ImageList, tagKeys []string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ImagesRemoveTags")
}

func (s *b) ResolveNetworkPlacement(placement string) (vpc *backends.Network, subnet *backends.Subnet, zone string, err error) {
	return nil, nil, "", backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ResolveNetworkPlacement")
}

package bvagrant

import (
	"io"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

func (s *b) AssociateVPCWithHostedZone(hostedZoneID string, vpcID string, region string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "AssociateVPCWithHostedZone")
}

func (s *b) DeleteRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DeleteRoute")
}

func (s *b) CreateRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string, force bool) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateRoute")
}

func (s *b) CreateBlackholeRoute(vpcID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateBlackholeRoute")
}

func (s *b) DeleteBlackholeRoute(vpcID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DeleteBlackholeRoute")
}

func (s *b) AcceptVPCPeering(peeringConnectionID string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "AcceptVPCPeering")
}

func (s *b) GetVPCRouteCIDRs(vpcID string) ([]string, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "GetVPCRouteCIDRs")
}

func (s *b) FindAvailableCloudCIDR(vpcID string, requestedCIDR string) (cidr string, isRequested bool, err error) {
	return "", false, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "FindAvailableCloudCIDR")
}

func (s *b) GetAccountID() (string, error) {
	return "", backends.ReturnNotImplemented(backends.BackendTypeVagrant, "GetAccountID")
}

func (s *b) MigrateV7Resources(input *backends.MigrateV7Input) (*backends.MigrationResult, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "MigrateV7Resources")
}

func (s *b) CheckRouteExists(vpcID string, peeringConnectionID string, destinationCidrBlock string) (bool, error) {
	return false, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CheckRouteExists")
}

func (s *b) CheckVPCHostedZoneAssociation(hostedZoneID string, vpcID string) (bool, error) {
	return false, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CheckVPCHostedZoneAssociation")
}

func (s *b) CreateFirewall(input *backends.CreateFirewallInput, waitDur time.Duration) (*backends.CreateFirewallOutput, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateFirewall")
}

func (s *b) FirewallsUpdate(fw backends.FirewallList, ports backends.PortsIn, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "FirewallsUpdate")
}

func (s *b) FirewallsDelete(fw backends.FirewallList, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "FirewallsDelete")
}

func (s *b) FirewallsAddTags(fw backends.FirewallList, tags map[string]string, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "FirewallsAddTags")
}

func (s *b) FirewallsRemoveTags(fw backends.FirewallList, tagKeys []string, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "FirewallsRemoveTags")
}

func (s *b) InstancesAssignFirewalls(instances backends.InstanceList, fw backends.FirewallList) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "InstancesAssignFirewalls")
}

func (s *b) InstancesRemoveFirewalls(instances backends.InstanceList, fw backends.FirewallList) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "InstancesRemoveFirewalls")
}

func (s *b) CleanupDNS() error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CleanupDNS")
}

func (s *b) DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DockerCreateNetwork")
}

func (s *b) DockerDeleteNetwork(region string, name string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DockerDeleteNetwork")
}

func (s *b) DockerPruneNetworks(region string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DockerPruneNetworks")
}

func (s *b) DockerLoadImage(region string, reader io.Reader, projectLabels map[string]string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DockerLoadImage")
}

func (s *b) AttachVolumes(volumes backends.VolumeList, instance *backends.Instance, sharedMountData *backends.VolumeAttachShared, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "AttachVolumes")
}

func (s *b) DetachVolumes(volumes backends.VolumeList, instance *backends.Instance, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DetachVolumes")
}

func (s *b) ResizeVolumes(volumes backends.VolumeList, newSizeGiB backends.StorageSize, waitDur time.Duration) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ResizeVolumes")
}

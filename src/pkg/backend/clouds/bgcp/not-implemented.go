package bgcp

import "github.com/aerospike/aerolab/pkg/backend/backends"

func (s *b) DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeGCP, "DockerCreateNetwork")
}

func (s *b) DockerDeleteNetwork(region string, name string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeGCP, "DockerDeleteNetwork")
}

func (s *b) DockerPruneNetworks(region string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeGCP, "DockerPruneNetworks")
}

func (s *b) AssociateVPCWithHostedZone(hostedZoneID string, vpcID string, region string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeGCP, "AssociateVPCWithHostedZone")
}

func (s *b) DeleteRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeGCP, "DeleteRoute")
}

func (s *b) CreateRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string, force bool) error {
	return backends.ReturnNotImplemented(backends.BackendTypeGCP, "CreateRoute")
}

func (s *b) CreateBlackholeRoute(vpcID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeGCP, "CreateBlackholeRoute")
}

func (s *b) DeleteBlackholeRoute(vpcID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeGCP, "DeleteBlackholeRoute")
}

func (s *b) AcceptVPCPeering(peeringConnectionID string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeGCP, "AcceptVPCPeering")
}

func (s *b) GetVPCRouteCIDRs(vpcID string) ([]string, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeGCP, "GetVPCRouteCIDRs")
}

func (s *b) FindAvailableCloudCIDR(vpcID string, requestedCIDR string) (cidr string, isRequested bool, err error) {
	return "", false, backends.ReturnNotImplemented(backends.BackendTypeGCP, "FindAvailableCloudCIDR")
}

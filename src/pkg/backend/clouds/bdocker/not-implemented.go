package bdocker

import "github.com/aerospike/aerolab/pkg/backend/backends"

func (s *b) AssociateVPCWithHostedZone(hostedZoneID string, vpcID string, region string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeDocker, "AssociateVPCWithHostedZone")
}

func (s *b) DeleteRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeDocker, "DeleteRoute")
}

func (s *b) CreateRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string, force bool) error {
	return backends.ReturnNotImplemented(backends.BackendTypeDocker, "CreateRoute")
}

func (s *b) CreateBlackholeRoute(vpcID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeDocker, "CreateBlackholeRoute")
}

func (s *b) DeleteBlackholeRoute(vpcID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeDocker, "DeleteBlackholeRoute")
}

func (s *b) AcceptVPCPeering(peeringConnectionID string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeDocker, "AcceptVPCPeering")
}

func (s *b) GetVPCRouteCIDRs(vpcID string) ([]string, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeDocker, "GetVPCRouteCIDRs")
}

func (s *b) FindAvailableCloudCIDR(vpcID string, requestedCIDR string) (cidr string, isRequested bool, err error) {
	return "", false, backends.ReturnNotImplemented(backends.BackendTypeDocker, "FindAvailableCloudCIDR")
}

func (s *b) GetAccountID() (string, error) {
	return "", backends.ReturnNotImplemented(backends.BackendTypeDocker, "GetAccountID")
}

func (s *b) MigrateV7Resources(input *backends.MigrateV7Input) (*backends.MigrationResult, error) {
	return nil, backends.ReturnNotImplemented(backends.BackendTypeDocker, "MigrateV7Resources")
}

func (s *b) CheckRouteExists(vpcID string, peeringConnectionID string, destinationCidrBlock string) (bool, error) {
	return false, backends.ReturnNotImplemented(backends.BackendTypeDocker, "CheckRouteExists")
}

func (s *b) CheckVPCHostedZoneAssociation(hostedZoneID string, vpcID string) (bool, error) {
	return false, backends.ReturnNotImplemented(backends.BackendTypeDocker, "CheckVPCHostedZoneAssociation")
}

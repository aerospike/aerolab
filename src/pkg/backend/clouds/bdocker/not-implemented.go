package bdocker

import "github.com/aerospike/aerolab/pkg/backend/backends"

func (s *b) AssociateVPCWithHostedZone(hostedZoneID string, vpcID string, region string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeDocker, "AssociateVPCWithHostedZone")
}

func (s *b) DeleteRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeDocker, "DeleteRoute")
}

func (s *b) CreateRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeDocker, "CreateRoute")
}

func (s *b) AcceptVPCPeering(peeringConnectionID string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeDocker, "AcceptVPCPeering")
}

func (s *b) GetAccountID() (string, error) {
	return "", backends.ReturnNotImplemented(backends.BackendTypeDocker, "GetAccountID")
}

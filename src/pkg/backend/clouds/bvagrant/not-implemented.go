package bvagrant

import "github.com/aerospike/aerolab/pkg/backend/backends"

// AcceptVPCPeering accepts VPC peering (not applicable for Vagrant).
func (s *b) AcceptVPCPeering(peeringConnectionID string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "AcceptVPCPeering")
}

// CreateRoute creates a VPC route (not applicable for Vagrant).
func (s *b) CreateRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateRoute")
}

// DeleteRoute deletes a VPC route (not applicable for Vagrant).
func (s *b) DeleteRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DeleteRoute")
}

// AssociateVPCWithHostedZone associates a VPC with a hosted zone (not applicable for Vagrant).
func (s *b) AssociateVPCWithHostedZone(hostedZoneID string, vpcID string, region string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "AssociateVPCWithHostedZone")
}

// GetAccountID returns the account ID (not applicable for Vagrant).
func (s *b) GetAccountID() (string, error) {
	return "", backends.ReturnNotImplemented(backends.BackendTypeVagrant, "GetAccountID")
}

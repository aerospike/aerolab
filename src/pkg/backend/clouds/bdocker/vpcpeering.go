package bdocker

import (
	"errors"
	"fmt"
)

// AcceptVPCPeering is not supported in Docker backend.
// Docker does not provide native VPC peering capabilities, and this functionality
// is not applicable to Docker-based deployments.
//
// Parameters:
//   - peeringConnectionID: The ID of the VPC peering connection (ignored)
//
// Returns:
//   - error: Always returns an error indicating this feature is not supported
//
// Usage:
//
//	err := backend.AcceptVPCPeering("some-id")
//	if err != nil {
//	    // This will always return an error
//	}
func (s *b) AcceptVPCPeering(peeringConnectionID string) error {
	return fmt.Errorf("AcceptVPCPeering is not supported in Docker backend")
}

// CreateRoute is not supported in Docker backend.
// Docker does not provide native VPC peering or routing capabilities, and this functionality
// is not applicable to Docker-based deployments.
//
// Parameters:
//   - vpcID: The ID of the VPC (ignored)
//   - peeringConnectionID: The ID of the VPC peering connection (ignored)
//   - destinationCidrBlock: The destination CIDR block (ignored)
//
// Returns:
//   - error: Always returns an error indicating this feature is not supported
//
// Usage:
//
//	err := backend.CreateRoute("vpc-id", "pcx-id", "10.0.0.0/16")
//	if err != nil {
//	    // This will always return an error
//	}
func (s *b) CreateRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	return fmt.Errorf("CreateRoute is not supported in Docker backend")
}

// DeleteRoute is not supported in Docker backend.
// Docker does not provide native VPC peering or routing capabilities, and this functionality
// is not applicable to Docker-based deployments.
//
// Parameters:
//   - vpcID: The ID of the VPC (ignored)
//   - peeringConnectionID: The ID of the VPC peering connection (ignored)
//   - destinationCidrBlock: The destination CIDR block (ignored)
//
// Returns:
//   - error: Always returns an error indicating this feature is not supported
//
// Usage:
//
//	err := backend.DeleteRoute("vpc-id", "pcx-id", "10.0.0.0/16")
//	if err != nil {
//	    // This will always return an error
//	}
func (s *b) DeleteRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	return errors.New("DeleteRoute is not implemented")
}

// AssociateVPCWithHostedZone is not supported in Docker backend.
// Docker does not provide native VPC or DNS capabilities, and this functionality
// is not applicable to Docker-based deployments.
//
// Parameters:
//   - hostedZoneID: The ID of the hosted zone (ignored)
//   - vpcID: The ID of the VPC (ignored)
//   - region: The region where the VPC is located (ignored)
//
// Returns:
//   - error: Always returns an error indicating this feature is not supported
//
// Usage:
//
//	err := backend.AssociateVPCWithHostedZone("zone-id", "vpc-id", "region")
//	if err != nil {
//	    // This will always return an error
//	}
func (s *b) AssociateVPCWithHostedZone(hostedZoneID string, vpcID string, region string) error {
	return fmt.Errorf("AssociateVPCWithHostedZone is not supported in Docker backend")
}

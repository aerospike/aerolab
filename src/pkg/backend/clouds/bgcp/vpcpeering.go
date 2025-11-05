package bgcp

import (
	"errors"
	"fmt"
)

// AcceptVPCPeering is not supported in GCP backend.
// GCP uses a different VPC peering model compared to AWS, and this functionality
// is not currently implemented in the Aerolab GCP backend.
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
	return fmt.Errorf("AcceptVPCPeering is not supported in GCP backend")
}

// CreateRoute is not supported in GCP backend.
// GCP uses a different VPC peering and routing model compared to AWS, and this functionality
// is not currently implemented in the Aerolab GCP backend.
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
	return fmt.Errorf("CreateRoute is not supported in GCP backend")
}

// DeleteRoute is not supported in GCP backend.
// GCP uses a different VPC peering and routing model compared to AWS, and this functionality
// is not currently implemented in the Aerolab GCP backend.
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

// AssociateVPCWithHostedZone is not supported in GCP backend.
// GCP uses a different DNS and VPC peering model compared to AWS, and this functionality
// is not currently implemented in the Aerolab GCP backend.
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
	return fmt.Errorf("AssociateVPCWithHostedZone is not supported in GCP backend")
}

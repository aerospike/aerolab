package bgcp

import (
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

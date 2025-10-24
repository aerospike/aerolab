package bdocker

import (
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

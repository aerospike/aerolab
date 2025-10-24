package bdocker

import (
	"fmt"
)

// GetAccountID is not supported in Docker backend.
// Docker does not have a concept of account IDs like cloud providers,
// and this functionality is not applicable to Docker-based deployments.
//
// Returns:
//   - string: Empty string (not used)
//   - error: Always returns an error indicating this feature is not supported
//
// Usage:
//
//	accountID, err := backend.GetAccountID()
//	if err != nil {
//	    // This will always return an error
//	}
func (s *b) GetAccountID() (string, error) {
	return "", fmt.Errorf("GetAccountID is not supported in Docker backend")
}

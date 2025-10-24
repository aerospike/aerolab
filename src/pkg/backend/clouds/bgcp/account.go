package bgcp

import (
	"fmt"

	"github.com/lithammer/shortuuid"
)

// GetAccountID retrieves the GCP project ID for the current credentials.
// This method returns the GCP project ID that was configured during backend initialization.
// In GCP, the project ID serves as the primary identifier for the account/project.
//
// Returns:
//   - string: The GCP project ID
//   - error: nil on success, or an error describing what failed
//
// Usage:
//
//	projectID, err := backend.GetAccountID()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("GCP Project ID: %s\n", projectID)
func (s *b) GetAccountID() (string, error) {
	log := s.log.WithPrefix("GetAccountID: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if s.project == "" {
		return "", fmt.Errorf("GCP project ID is not configured")
	}

	log.Detail("Retrieved GCP project ID: %s", s.project)
	return s.project, nil
}

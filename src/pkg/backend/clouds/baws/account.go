package baws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/lithammer/shortuuid"
)

// GetAccountID retrieves the AWS account ID for the current credentials.
// This method uses the AWS STS (Security Token Service) to get the caller identity,
// which includes the AWS account ID.
//
// Returns:
//   - string: The AWS account ID (12-digit number)
//   - error: nil on success, or an error describing what failed
//
// Usage:
//
//	accountID, err := backend.GetAccountID()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("AWS Account ID: %s\n", accountID)
func (s *b) GetAccountID() (string, error) {
	log := s.log.WithPrefix("GetAccountID: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// Get the first enabled zone to use for the STS client
	zones, err := s.ListEnabledZones()
	if err != nil {
		return "", fmt.Errorf("failed to get enabled zones: %w", err)
	}
	if len(zones) == 0 {
		return "", fmt.Errorf("no enabled zones found")
	}

	// Use the first zone to get the STS client
	zone := zones[0]
	cli, err := getStsClient(s.credentials, &zone)
	if err != nil {
		return "", fmt.Errorf("failed to get STS client: %w", err)
	}

	// Get the caller identity to retrieve the account ID
	input := &sts.GetCallerIdentityInput{}
	result, err := cli.GetCallerIdentity(context.TODO(), input)
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %w", err)
	}

	if result.Account == nil {
		return "", fmt.Errorf("account ID not found in caller identity response")
	}

	accountID := *result.Account
	log.Detail("Retrieved AWS account ID: %s", accountID)

	// Log additional identity information for debugging
	if result.UserId != nil {
		log.Detail("User ID: %s", *result.UserId)
	}
	if result.Arn != nil {
		log.Detail("ARN: %s", *result.Arn)
	}

	return accountID, nil
}

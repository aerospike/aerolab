package baws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/lithammer/shortuuid"
)

// AcceptVPCPeering accepts a VPC peering connection request.
// This method accepts a VPC peering connection that is in the "pending-acceptance" state.
// The caller must be the owner of the accepter VPC to perform this action.
//
// Parameters:
//   - peeringConnectionID: The ID of the VPC peering connection to accept
//
// Returns:
//   - error: nil on success, or an error describing what failed
//
// Usage:
//
//	err := backend.AcceptVPCPeering("pcx-1234567890abcdef0")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (s *b) AcceptVPCPeering(peeringConnectionID string) error {
	log := s.log.WithPrefix("AcceptVPCPeering: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if peeringConnectionID == "" {
		return fmt.Errorf("peering connection ID cannot be empty")
	}

	// Get the first enabled zone to use for the EC2 client
	zones, err := s.ListEnabledZones()
	if err != nil {
		return fmt.Errorf("failed to get enabled zones: %w", err)
	}
	if len(zones) == 0 {
		return fmt.Errorf("no enabled zones found")
	}

	// Use the first zone to get the EC2 client
	zone := zones[0]
	cli, err := getEc2Client(s.credentials, &zone)
	if err != nil {
		return fmt.Errorf("failed to get EC2 client: %w", err)
	}

	// First, describe the VPC peering connection to check its status
	describeInput := &ec2.DescribeVpcPeeringConnectionsInput{
		VpcPeeringConnectionIds: []string{peeringConnectionID},
	}

	describeResult, err := cli.DescribeVpcPeeringConnections(context.TODO(), describeInput)
	if err != nil {
		return fmt.Errorf("failed to describe VPC peering connection %s: %w", peeringConnectionID, err)
	}

	if len(describeResult.VpcPeeringConnections) == 0 {
		return fmt.Errorf("VPC peering connection %s not found", peeringConnectionID)
	}

	peeringConnection := describeResult.VpcPeeringConnections[0]

	// Check if the peering connection is in the correct state
	if peeringConnection.Status == nil {
		return fmt.Errorf("VPC peering connection %s has no status information", peeringConnectionID)
	}

	statusCode := string(peeringConnection.Status.Code)
	log.Detail("VPC peering connection %s status: %s", peeringConnectionID, statusCode)

	switch statusCode {
	case "active":
		return fmt.Errorf("VPC peering connection %s is already active", peeringConnectionID)
	case "deleted":
		return fmt.Errorf("VPC peering connection %s has been deleted", peeringConnectionID)
	case "deleting":
		return fmt.Errorf("VPC peering connection %s is being deleted", peeringConnectionID)
	case "failed":
		return fmt.Errorf("VPC peering connection %s has failed", peeringConnectionID)
	case "rejected":
		return fmt.Errorf("VPC peering connection %s has been rejected", peeringConnectionID)
	case "pending-acceptance":
		// This is the correct state, proceed with acceptance
		log.Detail("VPC peering connection %s is in pending-acceptance state, proceeding with acceptance", peeringConnectionID)
	default:
		return fmt.Errorf("VPC peering connection %s is in unexpected state: %s", peeringConnectionID, statusCode)
	}

	// Accept the VPC peering connection
	acceptInput := &ec2.AcceptVpcPeeringConnectionInput{
		VpcPeeringConnectionId: aws.String(peeringConnectionID),
	}

	acceptResult, err := cli.AcceptVpcPeeringConnection(context.TODO(), acceptInput)
	if err != nil {
		return fmt.Errorf("failed to accept VPC peering connection %s: %w", peeringConnectionID, err)
	}

	if acceptResult.VpcPeeringConnection == nil {
		return fmt.Errorf("no VPC peering connection returned after acceptance")
	}

	acceptedStatus := string(acceptResult.VpcPeeringConnection.Status.Code)
	log.Detail("VPC peering connection %s accepted, new status: %s", peeringConnectionID, acceptedStatus)

	// Log the VPC IDs involved in the peering connection
	if acceptResult.VpcPeeringConnection.AccepterVpcInfo != nil {
		log.Detail("Accepter VPC: %s", aws.ToString(acceptResult.VpcPeeringConnection.AccepterVpcInfo.VpcId))
	}
	if acceptResult.VpcPeeringConnection.RequesterVpcInfo != nil {
		log.Detail("Requester VPC: %s", aws.ToString(acceptResult.VpcPeeringConnection.RequesterVpcInfo.VpcId))
	}

	log.Detail("Successfully accepted VPC peering connection %s", peeringConnectionID)
	return nil
}

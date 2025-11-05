package baws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
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

// CreateRoute creates a route in the route table(s) for the specified VPC.
// This method automatically looks up route tables associated with the VPC and creates
// a route pointing to the VPC peering connection for the destination CIDR block.
//
// Parameters:
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//   - peeringConnectionID: The ID of the VPC peering connection (e.g., "pcx-1234567890abcdef0")
//   - destinationCidrBlock: The destination CIDR block (e.g., "10.128.0.0/19")
//
// Returns:
//   - error: nil on success, or an error describing what failed
//
// Usage:
//
//	err := backend.CreateRoute("vpc-1234567890abcdef0", "pcx-1234567890abcdef0", "10.128.0.0/19")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (s *b) CreateRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	log := s.log.WithPrefix("CreateRoute: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if vpcID == "" {
		return fmt.Errorf("VPC ID cannot be empty")
	}
	if peeringConnectionID == "" {
		return fmt.Errorf("peering connection ID cannot be empty")
	}
	if destinationCidrBlock == "" {
		return fmt.Errorf("destination CIDR block cannot be empty")
	}

	// Get enabled zones to find which region the VPC is in
	zones, err := s.ListEnabledZones()
	if err != nil {
		return fmt.Errorf("failed to get enabled zones: %w", err)
	}
	if len(zones) == 0 {
		return fmt.Errorf("no enabled zones found")
	}

	// Try to find the VPC in each zone to determine which region to use
	var ec2Cli *ec2.Client
	for _, zone := range zones {
		cli, err := getEc2Client(s.credentials, &zone)
		if err != nil {
			log.Detail("Failed to get EC2 client for zone %s: %s", zone, err)
			continue
		}

		// Check if VPC exists in this zone
		describeInput := &ec2.DescribeVpcsInput{
			VpcIds: []string{vpcID},
		}

		describeResult, err := cli.DescribeVpcs(context.TODO(), describeInput)
		if err != nil {
			log.Detail("VPC %s not found in zone %s: %s", vpcID, zone, err)
			continue
		}

		if len(describeResult.Vpcs) > 0 {
			ec2Cli = cli
			log.Detail("Found VPC %s in zone %s", vpcID, zone)
			break
		}
	}

	if ec2Cli == nil {
		return fmt.Errorf("VPC %s not found in any enabled zone", vpcID)
	}

	// Get route tables associated with the VPC
	describeRouteTablesInput := &ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcID},
			},
		},
	}

	routeTablesResult, err := ec2Cli.DescribeRouteTables(context.TODO(), describeRouteTablesInput)
	if err != nil {
		return fmt.Errorf("failed to describe route tables for VPC %s: %w", vpcID, err)
	}

	if len(routeTablesResult.RouteTables) == 0 {
		return fmt.Errorf("no route tables found for VPC %s", vpcID)
	}

	// Create route in each route table (or just the main one if it's explicitly marked)
	var routeTableIds []string
	for _, rt := range routeTablesResult.RouteTables {
		// Prefer main route table, but create in all if multiple exist
		routeTableId := aws.ToString(rt.RouteTableId)
		isMain := false
		if rt.Associations != nil {
			for _, assoc := range rt.Associations {
				if assoc.Main != nil && *assoc.Main {
					// This is the main route table, prioritize it
					isMain = true
					log.Detail("Found main route table: %s", routeTableId)
					break
				}
			}
		}
		// Add route tables to the list, prioritizing main route table
		found := false
		for _, id := range routeTableIds {
			if id == routeTableId {
				found = true
				break
			}
		}
		if !found {
			if isMain {
				routeTableIds = append([]string{routeTableId}, routeTableIds...)
			} else {
				routeTableIds = append(routeTableIds, routeTableId)
			}
		}
	}

	if len(routeTableIds) == 0 {
		return fmt.Errorf("no valid route tables found for VPC %s", vpcID)
	}

	// Create route in each route table
	for _, routeTableId := range routeTableIds {
		createRouteInput := &ec2.CreateRouteInput{
			RouteTableId:           aws.String(routeTableId),
			DestinationCidrBlock:   aws.String(destinationCidrBlock),
			VpcPeeringConnectionId: aws.String(peeringConnectionID),
		}

		_, err := ec2Cli.CreateRoute(context.TODO(), createRouteInput)
		if err != nil {
			// Check if route already exists (this is okay)
			// AWS returns error code "RouteAlreadyExists" when route already exists
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "routealreadyexists") || strings.Contains(errStr, "route already exists") {
				log.Detail("Route already exists in route table %s, continuing", routeTableId)
				continue
			}
			return fmt.Errorf("failed to create route in route table %s: %w", routeTableId, err)
		}
		log.Detail("Successfully created route in route table %s: %s -> %s", routeTableId, destinationCidrBlock, peeringConnectionID)
	}

	log.Detail("Successfully created routes for VPC %s", vpcID)
	return nil
}

// DeleteRoute deletes a route from the route table(s) for the specified VPC.
// This method automatically looks up route tables associated with the VPC and deletes
// the route matching the destination CIDR block and VPC peering connection.
//
// Parameters:
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//   - peeringConnectionID: The ID of the VPC peering connection (e.g., "pcx-1234567890abcdef0")
//   - destinationCidrBlock: The destination CIDR block (e.g., "10.128.0.0/19")
//
// Returns:
//   - error: nil on success, or an error describing what failed. If the route doesn't exist,
//     this is treated as success (no-op).
//
// Usage:
//
//	err := backend.DeleteRoute("vpc-1234567890abcdef0", "pcx-1234567890abcdef0", "10.128.0.0/19")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (s *b) DeleteRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string) error {
	log := s.log.WithPrefix("DeleteRoute: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if vpcID == "" {
		return fmt.Errorf("VPC ID cannot be empty")
	}
	if peeringConnectionID == "" {
		return fmt.Errorf("peering connection ID cannot be empty")
	}
	if destinationCidrBlock == "" {
		return fmt.Errorf("destination CIDR block cannot be empty")
	}

	// Get enabled zones to find which region the VPC is in
	zones, err := s.ListEnabledZones()
	if err != nil {
		return fmt.Errorf("failed to get enabled zones: %w", err)
	}
	if len(zones) == 0 {
		return fmt.Errorf("no enabled zones found")
	}

	// Try to find the VPC in each zone to determine which region to use
	var ec2Cli *ec2.Client
	for _, zone := range zones {
		cli, err := getEc2Client(s.credentials, &zone)
		if err != nil {
			log.Detail("Failed to get EC2 client for zone %s: %s", zone, err)
			continue
		}

		// Check if VPC exists in this zone
		describeInput := &ec2.DescribeVpcsInput{
			VpcIds: []string{vpcID},
		}

		describeResult, err := cli.DescribeVpcs(context.TODO(), describeInput)
		if err != nil {
			log.Detail("VPC %s not found in zone %s: %s", vpcID, zone, err)
			continue
		}

		if len(describeResult.Vpcs) > 0 {
			ec2Cli = cli
			log.Detail("Found VPC %s in zone %s", vpcID, zone)
			break
		}
	}

	if ec2Cli == nil {
		return fmt.Errorf("VPC %s not found in any enabled zone", vpcID)
	}

	// Get route tables associated with the VPC
	describeRouteTablesInput := &ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcID},
			},
		},
	}

	routeTablesResult, err := ec2Cli.DescribeRouteTables(context.TODO(), describeRouteTablesInput)
	if err != nil {
		return fmt.Errorf("failed to describe route tables for VPC %s: %w", vpcID, err)
	}

	if len(routeTablesResult.RouteTables) == 0 {
		log.Detail("No route tables found for VPC %s, nothing to delete", vpcID)
		return nil
	}

	// Delete route from each route table that has a matching route
	var deletedCount int
	for _, rt := range routeTablesResult.RouteTables {
		routeTableId := aws.ToString(rt.RouteTableId)

		// Check if this route table has a route matching our criteria
		routeFound := false
		if rt.Routes != nil {
			for _, route := range rt.Routes {
				routeDest := aws.ToString(route.DestinationCidrBlock)
				routePeering := aws.ToString(route.VpcPeeringConnectionId)

				if routeDest == destinationCidrBlock && routePeering == peeringConnectionID {
					routeFound = true
					break
				}
			}
		}

		if !routeFound {
			log.Detail("Route not found in route table %s, skipping", routeTableId)
			continue
		}

		// Delete the route
		deleteRouteInput := &ec2.DeleteRouteInput{
			RouteTableId:         aws.String(routeTableId),
			DestinationCidrBlock: aws.String(destinationCidrBlock),
		}

		_, err := ec2Cli.DeleteRoute(context.TODO(), deleteRouteInput)
		if err != nil {
			// Check if route doesn't exist (this is okay - might have been deleted already)
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "does not exist") || strings.Contains(errStr, "not found") {
				log.Detail("Route already deleted from route table %s, continuing", routeTableId)
				continue
			}
			return fmt.Errorf("failed to delete route from route table %s: %w", routeTableId, err)
		}
		log.Detail("Successfully deleted route from route table %s: %s -> %s", routeTableId, destinationCidrBlock, peeringConnectionID)
		deletedCount++
	}

	if deletedCount > 0 {
		log.Detail("Successfully deleted routes from %d route table(s) for VPC %s", deletedCount, vpcID)
	} else {
		log.Detail("No matching routes found to delete for VPC %s", vpcID)
	}
	return nil
}

// AssociateVPCWithHostedZone associates a VPC with a private hosted zone in Route53.
// This method associates the specified VPC with the hosted zone, allowing DNS queries
// from instances in the VPC to resolve records in the private hosted zone.
//
// Parameters:
//   - hostedZoneID: The ID of the hosted zone (e.g., "Z04089311NGVVH0FO3QGG")
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//   - region: The region where the VPC is located (e.g., "us-east-1")
//
// Returns:
//   - error: nil on success, or an error describing what failed
//
// Usage:
//
//	err := backend.AssociateVPCWithHostedZone("Z04089311NGVVH0FO3QGG", "vpc-1234567890abcdef0", "us-east-1")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (s *b) AssociateVPCWithHostedZone(hostedZoneID string, vpcID string, region string) error {
	log := s.log.WithPrefix("AssociateVPCWithHostedZone: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if hostedZoneID == "" {
		return fmt.Errorf("hosted zone ID cannot be empty")
	}
	if vpcID == "" {
		return fmt.Errorf("VPC ID cannot be empty")
	}
	if region == "" {
		return fmt.Errorf("region cannot be empty")
	}

	// Get Route53 client (Route53 is global, so region doesn't matter for the client)
	// But we need to use a region for the client creation, so use the VPC's region
	cli, err := getRoute53Client(s.credentials, &region)
	if err != nil {
		return fmt.Errorf("failed to get Route53 client: %w", err)
	}

	// Associate VPC with hosted zone
	associateInput := &route53.AssociateVPCWithHostedZoneInput{
		HostedZoneId: aws.String(hostedZoneID),
		VPC: &route53types.VPC{
			VPCId:     aws.String(vpcID),
			VPCRegion: route53types.VPCRegion(region),
		},
	}

	_, err = cli.AssociateVPCWithHostedZone(context.TODO(), associateInput)
	if err != nil {
		return fmt.Errorf("failed to associate VPC %s with hosted zone %s: %w", vpcID, hostedZoneID, err)
	}

	log.Detail("Successfully associated VPC %s with hosted zone %s", vpcID, hostedZoneID)
	return nil
}

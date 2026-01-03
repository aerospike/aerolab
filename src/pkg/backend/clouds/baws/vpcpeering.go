package baws

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/lithammer/shortuuid"
)

// isCloudCIDR checks if a CIDR block looks like an Aerospike Cloud database CIDR.
// Cloud database CIDRs typically follow the pattern 10.x.0.0/19 (starting from 10.128.0.0/19).
// This is used to identify placeholder routes that were created to reserve a CIDR.
func isCloudCIDR(cidr string) bool {
	// Cloud CIDRs are /19 blocks in the 10.x.0.0 range, typically starting from 10.128.0.0/19
	if !strings.HasSuffix(cidr, "/19") {
		return false
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	ip := ipNet.IP.To4()
	if ip == nil {
		return false
	}
	// Check if it's in the 10.x.0.0 range (first octet is 10, third and fourth octets are 0)
	return ip[0] == 10 && ip[2] == 0 && ip[3] == 0
}

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
// If a route with the same destination CIDR block already exists:
//   - If the route points to the same peering connection, returns success (no-op)
//   - If force=false and the route points to a different target, returns an error
//   - If force=true and the route points to a different target, deletes the existing route and creates the new one
//
// Parameters:
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//   - peeringConnectionID: The ID of the VPC peering connection (e.g., "pcx-1234567890abcdef0")
//   - destinationCidrBlock: The destination CIDR block (e.g., "10.128.0.0/19")
//   - force: If true, replaces any existing route with the same destination CIDR block
//
// Returns:
//   - error: nil on success, or an error describing what failed
//
// Usage:
//
//	err := backend.CreateRoute("vpc-1234567890abcdef0", "pcx-1234567890abcdef0", "10.128.0.0/19", false)
//	if err != nil {
//	    log.Fatal(err)
//	}
func (s *b) CreateRoute(vpcID string, peeringConnectionID string, destinationCidrBlock string, force bool) error {
	log := s.log.WithPrefix("CreateRoute: job=" + shortuuid.New() + " ")
	log.Detail("Start (force=%t)", force)
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

	// Find the route table to use - prefer main route table, otherwise use first one
	var targetRouteTable *types.RouteTable
	var mainRouteTable *types.RouteTable
	var firstRouteTable *types.RouteTable

	for i := range routeTablesResult.RouteTables {
		rt := &routeTablesResult.RouteTables[i]
		log.Detail("Enum: vpcId=%s routeTableId=%s", aws.ToString(rt.VpcId), aws.ToString(rt.RouteTableId))

		// Track the first route table we see
		if firstRouteTable == nil {
			firstRouteTable = rt
		}

		// Check if this is the main route table
		if rt.Associations != nil {
			for _, assoc := range rt.Associations {
				if assoc.Main != nil && *assoc.Main {
					mainRouteTable = rt
					log.Detail("Found main route table: %s", aws.ToString(rt.RouteTableId))
					break
				}
			}
		}

		// If we found the main route table, no need to continue searching
		if mainRouteTable != nil {
			break
		}
	}

	// Determine which route table to use
	if mainRouteTable != nil {
		targetRouteTable = mainRouteTable
		log.Detail("Using main route table: %s", aws.ToString(targetRouteTable.RouteTableId))
	} else if firstRouteTable != nil {
		targetRouteTable = firstRouteTable
		log.Detail("No main route table found, using first available: %s", aws.ToString(targetRouteTable.RouteTableId))
	} else {
		return fmt.Errorf("no valid route tables found for VPC %s", vpcID)
	}

	targetRouteTableId := aws.ToString(targetRouteTable.RouteTableId)

	// Check if a route with the same destination CIDR block already exists
	var existingRoute *types.Route
	if targetRouteTable.Routes != nil {
		for i := range targetRouteTable.Routes {
			route := &targetRouteTable.Routes[i]
			routeDest := aws.ToString(route.DestinationCidrBlock)
			if routeDest == destinationCidrBlock {
				existingRoute = route
				break
			}
		}
	}

	if existingRoute != nil {
		existingPeeringId := aws.ToString(existingRoute.VpcPeeringConnectionId)
		existingGatewayId := aws.ToString(existingRoute.GatewayId)
		existingNatGatewayId := aws.ToString(existingRoute.NatGatewayId)
		existingInstanceId := aws.ToString(existingRoute.InstanceId)
		existingNetworkInterfaceId := aws.ToString(existingRoute.NetworkInterfaceId)
		existingTransitGatewayId := aws.ToString(existingRoute.TransitGatewayId)

		log.Detail("Found existing route for %s: peeringId=%s gatewayId=%s natGatewayId=%s instanceId=%s networkInterfaceId=%s transitGatewayId=%s",
			destinationCidrBlock, existingPeeringId, existingGatewayId, existingNatGatewayId, existingInstanceId, existingNetworkInterfaceId, existingTransitGatewayId)

		// Check if the existing route points to the same peering connection
		if existingPeeringId == peeringConnectionID {
			log.Detail("Route already exists with the same peering connection ID in route table %s", targetRouteTableId)
			return nil
		}

		// Route exists but points to a different target
		// Allow replacement if:
		// - The route is a blackhole (State == "blackhole")
		// - The route is a placeholder (IGW target for cloud CIDRs like 10.*/19)
		// - force is true
		isBlackhole := existingRoute.State == types.RouteStateBlackhole
		isPlaceholder := strings.HasPrefix(existingGatewayId, "igw-") && isCloudCIDR(destinationCidrBlock)

		if !force && !isBlackhole && !isPlaceholder {
			return fmt.Errorf("route for %s already exists in route table %s but points to a different target (existing peering: %s, gateway: %s, nat: %s, instance: %s, interface: %s, transit: %s); use force=true to replace it",
				destinationCidrBlock, targetRouteTableId, existingPeeringId, existingGatewayId, existingNatGatewayId, existingInstanceId, existingNetworkInterfaceId, existingTransitGatewayId)
		}

		// Force is true, route is blackhole, or route is placeholder - delete the existing route first
		if isBlackhole {
			log.Detail("Route is blackhole, deleting existing route for %s in route table %s", destinationCidrBlock, targetRouteTableId)
		} else if isPlaceholder {
			log.Detail("Route is placeholder (IGW target for cloud CIDR), deleting existing route for %s in route table %s", destinationCidrBlock, targetRouteTableId)
		} else {
			log.Detail("Force=true, deleting existing route for %s in route table %s", destinationCidrBlock, targetRouteTableId)
		}
		deleteRouteInput := &ec2.DeleteRouteInput{
			RouteTableId:         aws.String(targetRouteTableId),
			DestinationCidrBlock: aws.String(destinationCidrBlock),
		}
		_, err := ec2Cli.DeleteRoute(context.TODO(), deleteRouteInput)
		if err != nil {
			return fmt.Errorf("failed to delete existing route for %s in route table %s: %w", destinationCidrBlock, targetRouteTableId, err)
		}
		log.Detail("Successfully deleted existing route for %s in route table %s", destinationCidrBlock, targetRouteTableId)
	}

	// Create route in the selected route table
	createRouteInput := &ec2.CreateRouteInput{
		RouteTableId:           aws.String(targetRouteTableId),
		DestinationCidrBlock:   aws.String(destinationCidrBlock),
		VpcPeeringConnectionId: aws.String(peeringConnectionID),
	}

	_, err = ec2Cli.CreateRoute(context.TODO(), createRouteInput)
	if err != nil {
		return fmt.Errorf("failed to create route in route table %s: %w", targetRouteTableId, err)
	}
	log.Detail("Successfully created route in route table %s: %s -> %s", targetRouteTableId, destinationCidrBlock, peeringConnectionID)

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

// CreateBlackholeRoute creates a placeholder route in the route table(s) for the specified VPC.
// Since AWS doesn't support creating true blackhole routes via API, this function creates
// a placeholder route using the Internet Gateway (from the 0.0.0.0/0 route) as the target.
// This reserves the CIDR block and prevents race conditions during cluster creation.
// The placeholder route will be automatically replaced when CreateRoute is called with
// a VPC peering connection for the same CIDR.
//
// Parameters:
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//   - destinationCidrBlock: The destination CIDR block to reserve (e.g., "10.130.0.0/19")
//
// Returns:
//   - error: nil on success, or an error describing what failed
//
// Usage:
//
//	err := backend.CreateBlackholeRoute("vpc-1234567890abcdef0", "10.130.0.0/19")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (s *b) CreateBlackholeRoute(vpcID string, destinationCidrBlock string) error {
	log := s.log.WithPrefix("CreateBlackholeRoute: job=" + shortuuid.New() + " ")
	log.Detail("Start (vpcID=%s, cidr=%s)", vpcID, destinationCidrBlock)
	defer log.Detail("End")

	if vpcID == "" {
		return fmt.Errorf("VPC ID cannot be empty")
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

	// Find the route table to use - prefer main route table, otherwise use first one
	var targetRouteTable *types.RouteTable
	var mainRouteTable *types.RouteTable
	var firstRouteTable *types.RouteTable

	for i := range routeTablesResult.RouteTables {
		rt := &routeTablesResult.RouteTables[i]
		log.Detail("Enum: vpcId=%s routeTableId=%s", aws.ToString(rt.VpcId), aws.ToString(rt.RouteTableId))

		// Track the first route table we see
		if firstRouteTable == nil {
			firstRouteTable = rt
		}

		// Check if this is the main route table
		if rt.Associations != nil {
			for _, assoc := range rt.Associations {
				if assoc.Main != nil && *assoc.Main {
					mainRouteTable = rt
					log.Detail("Found main route table: %s", aws.ToString(rt.RouteTableId))
					break
				}
			}
		}

		// If we found the main route table, no need to continue searching
		if mainRouteTable != nil {
			break
		}
	}

	// Determine which route table to use
	if mainRouteTable != nil {
		targetRouteTable = mainRouteTable
		log.Detail("Using main route table: %s", aws.ToString(targetRouteTable.RouteTableId))
	} else if firstRouteTable != nil {
		targetRouteTable = firstRouteTable
		log.Detail("No main route table found, using first available: %s", aws.ToString(targetRouteTable.RouteTableId))
	} else {
		return fmt.Errorf("no valid route tables found for VPC %s", vpcID)
	}

	targetRouteTableId := aws.ToString(targetRouteTable.RouteTableId)

	// Check if a route with the same destination CIDR block already exists
	if targetRouteTable.Routes != nil {
		for i := range targetRouteTable.Routes {
			route := &targetRouteTable.Routes[i]
			routeDest := aws.ToString(route.DestinationCidrBlock)
			if routeDest == destinationCidrBlock {
				return fmt.Errorf("route for %s already exists in route table %s", destinationCidrBlock, targetRouteTableId)
			}
		}
	}

	// Find the Internet Gateway ID from the 0.0.0.0/0 route
	// We use this IGW as a placeholder target since AWS doesn't support creating true blackhole routes via API
	var internetGatewayId string
	if targetRouteTable.Routes != nil {
		for i := range targetRouteTable.Routes {
			route := &targetRouteTable.Routes[i]
			routeDest := aws.ToString(route.DestinationCidrBlock)
			if routeDest == "0.0.0.0/0" {
				gatewayId := aws.ToString(route.GatewayId)
				if strings.HasPrefix(gatewayId, "igw-") {
					internetGatewayId = gatewayId
					log.Detail("Found Internet Gateway from 0.0.0.0/0 route: %s", internetGatewayId)
					break
				}
			}
		}
	}

	if internetGatewayId == "" {
		return fmt.Errorf("no Internet Gateway found in route table %s (no 0.0.0.0/0 route with igw- target)", targetRouteTableId)
	}

	// Create placeholder route using the Internet Gateway
	// This reserves the CIDR in the route table and will be replaced with the actual peering route later
	createRouteInput := &ec2.CreateRouteInput{
		RouteTableId:         aws.String(targetRouteTableId),
		DestinationCidrBlock: aws.String(destinationCidrBlock),
		GatewayId:            aws.String(internetGatewayId),
	}

	_, err = ec2Cli.CreateRoute(context.TODO(), createRouteInput)
	if err != nil {
		return fmt.Errorf("failed to create placeholder route in route table %s: %w", targetRouteTableId, err)
	}
	log.Detail("Successfully created placeholder route in route table %s: %s -> %s (IGW placeholder)", targetRouteTableId, destinationCidrBlock, internetGatewayId)

	return nil
}

// DeleteBlackholeRoute deletes a placeholder/blackhole route from the route table(s) for the specified VPC.
// This method deletes the route matching the destination CIDR block regardless of its target.
// It's used to clean up reserved CIDRs when cluster creation fails.
//
// Parameters:
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//   - destinationCidrBlock: The destination CIDR block (e.g., "10.130.0.0/19")
//
// Returns:
//   - error: nil on success, or an error describing what failed. If the route doesn't exist,
//     this is treated as success (no-op).
//
// Usage:
//
//	err := backend.DeleteBlackholeRoute("vpc-1234567890abcdef0", "10.130.0.0/19")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (s *b) DeleteBlackholeRoute(vpcID string, destinationCidrBlock string) error {
	log := s.log.WithPrefix("DeleteBlackholeRoute: job=" + shortuuid.New() + " ")
	log.Detail("Start (vpcID=%s, cidr=%s)", vpcID, destinationCidrBlock)
	defer log.Detail("End")

	if vpcID == "" {
		return fmt.Errorf("VPC ID cannot be empty")
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

	// Delete route from each route table that has a matching route (by CIDR only, regardless of target)
	var deletedCount int
	for _, rt := range routeTablesResult.RouteTables {
		routeTableId := aws.ToString(rt.RouteTableId)

		// Check if this route table has a route matching our CIDR
		routeFound := false
		if rt.Routes != nil {
			for _, route := range rt.Routes {
				routeDest := aws.ToString(route.DestinationCidrBlock)
				if routeDest == destinationCidrBlock {
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
		log.Detail("Successfully deleted placeholder route from route table %s: %s", routeTableId, destinationCidrBlock)
		deletedCount++
	}

	if deletedCount > 0 {
		log.Detail("Successfully deleted placeholder routes from %d route table(s) for VPC %s", deletedCount, vpcID)
	} else {
		log.Detail("No matching placeholder routes found to delete for VPC %s", vpcID)
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

// GetVPCRouteCIDRs returns all destination CIDR blocks from the route tables of the specified VPC.
// This is useful for checking if a CIDR block is already in use before creating a new route.
//
// Parameters:
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//
// Returns:
//   - []string: A list of destination CIDR blocks found in the VPC's route tables
//   - error: nil on success, or an error describing what failed
//
// Usage:
//
//	cidrs, err := backend.GetVPCRouteCIDRs("vpc-1234567890abcdef0")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, cidr := range cidrs {
//	    fmt.Println(cidr)
//	}
func (s *b) GetVPCRouteCIDRs(vpcID string) ([]string, error) {
	log := s.log.WithPrefix("GetVPCRouteCIDRs: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if vpcID == "" {
		return nil, fmt.Errorf("VPC ID cannot be empty")
	}

	// Get enabled zones to find which region the VPC is in
	zones, err := s.ListEnabledZones()
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled zones: %w", err)
	}
	if len(zones) == 0 {
		return nil, fmt.Errorf("no enabled zones found")
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
		return nil, fmt.Errorf("VPC %s not found in any enabled zone", vpcID)
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
		return nil, fmt.Errorf("failed to describe route tables for VPC %s: %w", vpcID, err)
	}

	// Collect all unique destination CIDR blocks
	cidrSet := make(map[string]struct{})
	for _, rt := range routeTablesResult.RouteTables {
		if rt.Routes != nil {
			for _, route := range rt.Routes {
				if route.DestinationCidrBlock != nil && *route.DestinationCidrBlock != "" {
					cidrSet[*route.DestinationCidrBlock] = struct{}{}
				}
			}
		}
	}

	// Convert set to slice
	cidrs := make([]string, 0, len(cidrSet))
	for cidr := range cidrSet {
		cidrs = append(cidrs, cidr)
	}

	log.Detail("Found %d unique CIDR blocks in VPC %s route tables", len(cidrs), vpcID)
	return cidrs, nil
}

// GetVPCPeeringCIDRs returns all CIDR blocks used by VPC peering connections for the given VPC.
// This includes both the requester and accepter CIDR blocks from active or pending peering connections.
//
// Parameters:
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//
// Returns:
//   - []string: A slice of unique CIDR blocks used by VPC peering connections
//   - error: nil on success, or an error describing what failed
func (s *b) GetVPCPeeringCIDRs(vpcID string) ([]string, error) {
	log := s.log.WithPrefix("GetVPCPeeringCIDRs: job=" + shortuuid.New() + " ")
	log.Detail("Start (vpcID=%s)", vpcID)
	defer log.Detail("End")

	if vpcID == "" {
		return nil, fmt.Errorf("VPC ID cannot be empty")
	}

	// Get enabled zones to find which region the VPC is in
	zones, err := s.ListEnabledZones()
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled zones: %w", err)
	}
	if len(zones) == 0 {
		return nil, fmt.Errorf("no enabled zones found")
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
		return nil, fmt.Errorf("VPC %s not found in any enabled zone", vpcID)
	}

	// Get VPC peering connections where this VPC is either the requester or accepter
	describeInput := &ec2.DescribeVpcPeeringConnectionsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("accepter-vpc-info.vpc-id"),
				Values: []string{vpcID},
			},
		},
	}

	// Collect CIDRs from peering connections
	cidrSet := make(map[string]struct{})

	// First, get connections where this VPC is the accepter
	result, err := ec2Cli.DescribeVpcPeeringConnections(context.TODO(), describeInput)
	if err != nil {
		return nil, fmt.Errorf("failed to describe VPC peering connections (accepter): %w", err)
	}

	for _, pc := range result.VpcPeeringConnections {
		// Skip deleted/failed/rejected connections
		if pc.Status != nil {
			statusCode := string(pc.Status.Code)
			if statusCode == "deleted" || statusCode == "failed" || statusCode == "rejected" || statusCode == "expired" {
				continue
			}
		}

		// Get requester CIDR (the cloud database side)
		if pc.RequesterVpcInfo != nil && pc.RequesterVpcInfo.CidrBlock != nil {
			cidr := *pc.RequesterVpcInfo.CidrBlock
			if cidr != "" {
				cidrSet[cidr] = struct{}{}
				log.Detail("Found requester CIDR from peering: %s", cidr)
			}
			// Also check CidrBlockSet for additional CIDRs
			for _, cb := range pc.RequesterVpcInfo.CidrBlockSet {
				if cb.CidrBlock != nil && *cb.CidrBlock != "" {
					cidrSet[*cb.CidrBlock] = struct{}{}
					log.Detail("Found requester CIDR from peering (CidrBlockSet): %s", *cb.CidrBlock)
				}
			}
		}
	}

	// Also get connections where this VPC is the requester (less common but possible)
	describeInput2 := &ec2.DescribeVpcPeeringConnectionsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("requester-vpc-info.vpc-id"),
				Values: []string{vpcID},
			},
		},
	}

	result2, err := ec2Cli.DescribeVpcPeeringConnections(context.TODO(), describeInput2)
	if err != nil {
		return nil, fmt.Errorf("failed to describe VPC peering connections (requester): %w", err)
	}

	for _, pc := range result2.VpcPeeringConnections {
		// Skip deleted/failed/rejected connections
		if pc.Status != nil {
			statusCode := string(pc.Status.Code)
			if statusCode == "deleted" || statusCode == "failed" || statusCode == "rejected" || statusCode == "expired" {
				continue
			}
		}

		// Get accepter CIDR (the other side)
		if pc.AccepterVpcInfo != nil && pc.AccepterVpcInfo.CidrBlock != nil {
			cidr := *pc.AccepterVpcInfo.CidrBlock
			if cidr != "" {
				cidrSet[cidr] = struct{}{}
				log.Detail("Found accepter CIDR from peering: %s", cidr)
			}
			// Also check CidrBlockSet for additional CIDRs
			for _, cb := range pc.AccepterVpcInfo.CidrBlockSet {
				if cb.CidrBlock != nil && *cb.CidrBlock != "" {
					cidrSet[*cb.CidrBlock] = struct{}{}
					log.Detail("Found accepter CIDR from peering (CidrBlockSet): %s", *cb.CidrBlock)
				}
			}
		}
	}

	// Convert set to slice
	cidrs := make([]string, 0, len(cidrSet))
	for cidr := range cidrSet {
		cidrs = append(cidrs, cidr)
	}

	log.Detail("Found %d unique CIDR blocks from VPC peering connections for VPC %s", len(cidrs), vpcID)
	return cidrs, nil
}

// FindAvailableCloudCIDR finds an available CIDR block that is not already in use in the VPC's route tables or peering connections.
// It starts from 10.130.0.0/19 and increments the second octet until it finds an available CIDR.
//
// Parameters:
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//   - requestedCIDR: The CIDR block to check. If empty, starts from 10.130.0.0/19
//
// Returns:
//   - cidr: The available CIDR block found
//   - isRequested: true if the returned CIDR is the same as requestedCIDR (or default if requestedCIDR was empty)
//   - error: nil on success, or an error describing what failed
//
// Usage:
//
//	cidr, isRequested, err := backend.FindAvailableCloudCIDR("vpc-1234567890abcdef0", "")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if !isRequested {
//	    fmt.Printf("Default CIDR was in use, using %s instead\n", cidr)
//	}
func (s *b) FindAvailableCloudCIDR(vpcID string, requestedCIDR string) (cidr string, isRequested bool, err error) {
	log := s.log.WithPrefix("FindAvailableCloudCIDR: job=" + shortuuid.New() + " ")
	log.Detail("Start (vpcID=%s, requestedCIDR=%s)", vpcID, requestedCIDR)
	defer log.Detail("End")

	// Get existing CIDRs from VPC route tables
	existingCIDRs, err := s.GetVPCRouteCIDRs(vpcID)
	if err != nil {
		return "", false, fmt.Errorf("failed to get existing CIDRs from route tables: %w", err)
	}

	// Get existing CIDRs from VPC peering connections
	peeringCIDRs, err := s.GetVPCPeeringCIDRs(vpcID)
	if err != nil {
		return "", false, fmt.Errorf("failed to get existing CIDRs from VPC peering connections: %w", err)
	}

	// Combine both sources
	existingCIDRs = append(existingCIDRs, peeringCIDRs...)
	log.Detail("Total CIDRs to check against: %d (route tables: %d, peering: %d)",
		len(existingCIDRs), len(existingCIDRs)-len(peeringCIDRs), len(peeringCIDRs))

	// Create a set for quick lookup
	existingSet := make(map[string]struct{})
	for _, c := range existingCIDRs {
		existingSet[c] = struct{}{}
	}

	// Default starting CIDR
	const defaultCIDR = "10.130.0.0/19"

	// If a specific CIDR was requested, check if it's available
	if requestedCIDR != "" && requestedCIDR != "default" {
		if _, exists := existingSet[requestedCIDR]; exists {
			return "", false, fmt.Errorf("requested CIDR %s is already in use in VPC %s route tables", requestedCIDR, vpcID)
		}
		// Also check for overlaps
		for existingCIDR := range existingSet {
			if cidrsOverlap(requestedCIDR, existingCIDR) {
				return "", false, fmt.Errorf("requested CIDR %s overlaps with existing CIDR %s in VPC %s route tables", requestedCIDR, existingCIDR, vpcID)
			}
		}
		log.Detail("Requested CIDR %s is available", requestedCIDR)
		return requestedCIDR, true, nil
	}

	// Check if default CIDR is available
	if _, exists := existingSet[defaultCIDR]; !exists {
		// Also check for overlaps
		hasOverlap := false
		for existingCIDR := range existingSet {
			if cidrsOverlap(defaultCIDR, existingCIDR) {
				hasOverlap = true
				log.Detail("Default CIDR %s overlaps with existing CIDR %s", defaultCIDR, existingCIDR)
				break
			}
		}
		if !hasOverlap {
			log.Detail("Default CIDR %s is available", defaultCIDR)
			return defaultCIDR, true, nil
		}
	} else {
		log.Detail("Default CIDR %s is already in use, finding next available", defaultCIDR)
	}

	// Default is not available, find the next one
	// Start from 10.130.0.0/19 and increment second octet (10.131.0.0/19, 10.132.0.0/19, etc.)
	for secondOctet := 130; secondOctet <= 255; secondOctet++ {
		candidateCIDR := fmt.Sprintf("10.%d.0.0/19", secondOctet)

		if _, exists := existingSet[candidateCIDR]; exists {
			log.Detail("CIDR %s is in use, trying next", candidateCIDR)
			continue
		}

		// Check for overlaps with existing CIDRs
		hasOverlap := false
		for existingCIDR := range existingSet {
			if cidrsOverlap(candidateCIDR, existingCIDR) {
				hasOverlap = true
				log.Detail("CIDR %s overlaps with existing CIDR %s, trying next", candidateCIDR, existingCIDR)
				break
			}
		}
		if hasOverlap {
			continue
		}

		log.Detail("Found available CIDR: %s", candidateCIDR)
		return candidateCIDR, false, nil
	}

	return "", false, fmt.Errorf("no available CIDR block found in range 10.130.0.0/19 - 10.255.0.0/19 for VPC %s", vpcID)
}

// CheckRouteExists checks if a route exists in the VPC route table pointing to the peering connection.
//
// Parameters:
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//   - peeringConnectionID: The ID of the VPC peering connection (e.g., "pcx-1234567890abcdef0")
//   - destinationCidrBlock: The destination CIDR block (e.g., "10.128.0.0/19")
//
// Returns:
//   - bool: true if the route exists, false otherwise
//   - error: nil on success, or an error describing what failed
func (s *b) CheckRouteExists(vpcID string, peeringConnectionID string, destinationCidrBlock string) (bool, error) {
	log := s.log.WithPrefix("CheckRouteExists: job=" + shortuuid.New() + " ")
	log.Detail("Start (vpcID=%s, peeringConnectionID=%s, cidr=%s)", vpcID, peeringConnectionID, destinationCidrBlock)
	defer log.Detail("End")

	if vpcID == "" || peeringConnectionID == "" || destinationCidrBlock == "" {
		return false, fmt.Errorf("vpcID, peeringConnectionID, and destinationCidrBlock are required")
	}

	// Get enabled zones to find which region the VPC is in
	zones, err := s.ListEnabledZones()
	if err != nil {
		return false, fmt.Errorf("failed to get enabled zones: %w", err)
	}
	if len(zones) == 0 {
		return false, fmt.Errorf("no enabled zones found")
	}

	// Try to find the VPC in each zone
	var ec2Cli *ec2.Client
	for _, zone := range zones {
		cli, err := getEc2Client(s.credentials, &zone)
		if err != nil {
			continue
		}
		describeInput := &ec2.DescribeVpcsInput{
			VpcIds: []string{vpcID},
		}
		describeResult, err := cli.DescribeVpcs(context.TODO(), describeInput)
		if err != nil {
			continue
		}
		if len(describeResult.Vpcs) > 0 {
			ec2Cli = cli
			break
		}
	}

	if ec2Cli == nil {
		return false, fmt.Errorf("VPC %s not found in any enabled zone", vpcID)
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
		return false, fmt.Errorf("failed to describe route tables for VPC %s: %w", vpcID, err)
	}

	// Check all route tables for a matching route
	for _, rt := range routeTablesResult.RouteTables {
		if rt.Routes != nil {
			for _, route := range rt.Routes {
				routeDest := aws.ToString(route.DestinationCidrBlock)
				routePeering := aws.ToString(route.VpcPeeringConnectionId)
				if routeDest == destinationCidrBlock && routePeering == peeringConnectionID {
					log.Detail("Found matching route in route table %s", aws.ToString(rt.RouteTableId))
					return true, nil
				}
			}
		}
	}

	log.Detail("No matching route found")
	return false, nil
}

// CheckVPCHostedZoneAssociation checks if a VPC is associated with a hosted zone.
//
// Parameters:
//   - hostedZoneID: The ID of the hosted zone (e.g., "Z04089311NGVVH0FO3QGG")
//   - vpcID: The ID of the VPC (e.g., "vpc-1234567890abcdef0")
//
// Returns:
//   - bool: true if the VPC is associated with the hosted zone, false otherwise
//   - error: nil on success, or an error describing what failed
func (s *b) CheckVPCHostedZoneAssociation(hostedZoneID string, vpcID string) (bool, error) {
	log := s.log.WithPrefix("CheckVPCHostedZoneAssociation: job=" + shortuuid.New() + " ")
	log.Detail("Start (hostedZoneID=%s, vpcID=%s)", hostedZoneID, vpcID)
	defer log.Detail("End")

	if hostedZoneID == "" || vpcID == "" {
		return false, fmt.Errorf("hostedZoneID and vpcID are required")
	}

	// Get enabled zones
	zones, err := s.ListEnabledZones()
	if err != nil {
		return false, fmt.Errorf("failed to get enabled zones: %w", err)
	}
	if len(zones) == 0 {
		return false, fmt.Errorf("no enabled zones found")
	}

	// Use the first zone to get the Route53 client
	zone := zones[0]
	cli, err := getRoute53Client(s.credentials, &zone)
	if err != nil {
		return false, fmt.Errorf("failed to get Route53 client: %w", err)
	}

	// Get hosted zone details
	getInput := &route53.GetHostedZoneInput{
		Id: aws.String(hostedZoneID),
	}

	result, err := cli.GetHostedZone(context.TODO(), getInput)
	if err != nil {
		return false, fmt.Errorf("failed to get hosted zone %s: %w", hostedZoneID, err)
	}

	// Check if VPC is in the list of associated VPCs
	for _, vpc := range result.VPCs {
		if aws.ToString(vpc.VPCId) == vpcID {
			log.Detail("VPC %s is associated with hosted zone %s", vpcID, hostedZoneID)
			return true, nil
		}
	}

	log.Detail("VPC %s is not associated with hosted zone %s", vpcID, hostedZoneID)
	return false, nil
}

// cidrsOverlap checks if two CIDR blocks overlap
// It excludes 0.0.0.0/0 (default route) from overlap checks since it's a routing rule, not an actual CIDR allocation
func cidrsOverlap(cidr1, cidr2 string) bool {
	// Skip default route - it's not an actual CIDR allocation conflict
	if cidr1 == "0.0.0.0/0" || cidr2 == "0.0.0.0/0" {
		return false
	}

	_, net1, err1 := net.ParseCIDR(cidr1)
	_, net2, err2 := net.ParseCIDR(cidr2)

	if err1 != nil || err2 != nil {
		return false
	}

	return net1.Contains(net2.IP) || net2.Contains(net1.IP)
}

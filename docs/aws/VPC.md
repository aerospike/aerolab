# Custom VPC setup notes

## VPC

1. Create VPC
2. Create Subnets for the VPC (if not automatically done)
3. For each subnet, edit subnet settings, and ensure "Auto-assign public IPv4" is ticked, or a public IP won't be assigned to the instances

## Internet gateway

By default a new VPC cannot be routed to the internet. This needs to be configured.

1. Create an Internet Gateway
2. Attach the Internet Gateway to the VPC
3. Under subnets, select one of the new subnets and check which route table is being use (table ID)
4. Navigate to Route Tables and select the relevant route table
5. ensure that a route exists (or add a route if it does not) as follows:
   * CIDR: 0.0.0.0/0
   * Gateway/Router: the internet gateway we have created and attached to thre VPC

## Summary

The following have been created or checked to ensure proper networking status:
1. VPC
2. Subnets with auto-assign of public IPv4
3. Internet Gateway
4. Internet Gateway attached to VPC
5. Internet Gateway added to the routing table in use by the VPC/subnets

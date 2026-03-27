//go:build !noaws

package expire

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/clouds/baws"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

func (h *ExpiryHandler) expireEksctl(region string) error {
	log.Println("EKS: Listing clusters")
	ekssvc, err := baws.GetEksClient(h.Credentials, &region)
	if err != nil {
		return fmt.Errorf("could not get EKS client: %w", err)
	}
	ec2svc, err := baws.GetEc2Client(h.Credentials, &region)
	if err != nil {
		return fmt.Errorf("could not get EC2 client: %w", err)
	}
	eksClusters, err := ekssvc.ListClusters(context.TODO(), &eks.ListClustersInput{})
	if err != nil {
		return fmt.Errorf("could not list EKS: %w", err)
	}
	now := time.Now()
	for _, eksClusterName := range eksClusters.Clusters {
		eksCluster, err := ekssvc.DescribeCluster(context.TODO(), &eks.DescribeClusterInput{Name: aws.String(eksClusterName)})
		if err != nil {
			return fmt.Errorf("could not describe EKS cluster: %w", err)
		}
		eksAt := eksCluster.Cluster.Tags["ExpireAt"]
		var expireAt time.Time
		if eksAt != "" {
			oldAtInt, err := strconv.Atoi(eksAt)
			if err == nil {
				expireAt = time.Unix(int64(oldAtInt), 0)
			}
		} else {
			initial := eksCluster.Cluster.Tags["initialExpiry"]
			if initial != "" {
				createTs := aws.ToTime(eksCluster.Cluster.CreatedAt)
				initialDuration, err := time.ParseDuration(initial)
				if err == nil {
					expireAt = createTs.Add(initialDuration)
				}
			}
		}
		if expireAt.IsZero() {
			log.Printf("EKS: Cluster=%s NoExpirySet", aws.ToString(eksCluster.Cluster.Name))
			continue
		}
		if expireAt.After(now) {
			log.Printf("EKS: Cluster=%s Expiry=%s NotExpired", aws.ToString(eksCluster.Cluster.Name), expireAt.Format(time.RFC3339))
			continue
		}
		log.Printf("EKS: Cluster=%s Expiry=%s Starting Expiry", aws.ToString(eksCluster.Cluster.Name), expireAt.Format(time.RFC3339))

		log.Print("EKS: Listing cloudformation stacks...")
		cfSvc, err := baws.GetCloudformationClient(h.Credentials, &region)
		if err != nil {
			return fmt.Errorf("could not get cloudformation client: %w", err)
		}
		stacks, err := cfSvc.ListStacks(context.TODO(), &cloudformation.ListStacksInput{})
		if err != nil {
			return fmt.Errorf("could not cloudformation.ListStacks: %w", err)
		}
		type astack struct {
			Name          string
			CreationTime  time.Time
			CurrentStatus string
		}
		stackList := []*astack{}
		for _, stack := range stacks.StackSummaries {
			if !strings.HasPrefix(aws.ToString(stack.StackName), "eksctl-"+aws.ToString(eksCluster.Cluster.Name)+"-") {
				continue
			}
			if stack.StackStatus == cftypes.StackStatusDeleteComplete {
				continue
			}
			stackList = append(stackList, &astack{
				Name:          aws.ToString(stack.StackName),
				CreationTime:  aws.ToTime(stack.CreationTime),
				CurrentStatus: string(stack.StackStatus),
			})
		}
		sort.Slice(stackList, func(i, j int) bool {
			return stackList[j].CreationTime.Before(stackList[i].CreationTime) // reverse sort since we want to delete in reverse order
		})
		// turn off deletion protection for all stacks before attempting deletion
		for _, stack := range stackList {
			log.Printf("EKS: Disabling termination protection on stack %s", stack.Name)
			_, err = cfSvc.UpdateTerminationProtection(context.TODO(), &cloudformation.UpdateTerminationProtectionInput{
				StackName:                   aws.String(stack.Name),
				EnableTerminationProtection: aws.Bool(false),
			})
			if err != nil {
				log.Printf("EKS: Warning: could not disable termination protection on stack %s: %s", stack.Name, err)
			}
		}
		for _, stack := range stackList {
			if stack.Name == "eksctl-"+aws.ToString(eksCluster.Cluster.Name)+"-cluster" {
				log.Print("EKS: Listing EBS Volumes")
				clusterName := aws.ToString(eksCluster.Cluster.Name)
				var eksVolumes []ec2types.Volume
				volPaginator := ec2.NewDescribeVolumesPaginator(ec2svc, &ec2.DescribeVolumesInput{
					Filters: []ec2types.Filter{
						{
							Name:   aws.String("tag:kubernetes.io/cluster/" + clusterName),
							Values: []string{"owned"},
						}, {
							Name:   aws.String("tag:KubernetesCluster"),
							Values: []string{clusterName},
						},
					},
				})
				for volPaginator.HasMorePages() {
					page, err := volPaginator.NextPage(context.TODO())
					if err != nil {
						return fmt.Errorf("could not ec2.DescribeVolumes: %w", err)
					}
					eksVolumes = append(eksVolumes, page.Volumes...)
				}
				log.Printf("EKS: Found %d EBS volumes for cluster %s (tag:KubernetesCluster tag:kubernetes.io/cluster/=owned)", len(eksVolumes), clusterName)
				expiryTime := time.Now().Format(time.RFC3339)
				for _, delvol := range eksVolumes {
					doNotExpire := false
					for _, tag := range delvol.Tags {
						if aws.ToString(tag.Key) == "aerolab/do-not-expire" {
							doNotExpire = true
							break
						}
					}
					if doNotExpire {
						log.Printf("EKS: Skipping volume %s (aerolab/do-not-expire tag set)", aws.ToString(delvol.VolumeId))
						continue
					}
					log.Printf("EKS: Tagging volume %s for aerolab standard volume expiry", aws.ToString(delvol.VolumeId))
					_, err = ec2svc.CreateTags(context.TODO(), &ec2.CreateTagsInput{
						Resources: []string{aws.ToString(delvol.VolumeId)},
						Tags: []ec2types.Tag{
							{Key: aws.String(baws.TAG_AEROLAB_VERSION), Value: aws.String("eks-expiry")},
							{Key: aws.String(baws.TAG_EXPIRES), Value: aws.String(expiryTime)},
							{Key: aws.String(baws.TAG_NAME), Value: aws.String("eks-" + clusterName)},
						},
					})
					if err != nil {
						log.Printf("EKS: Warning: could not tag volume %s for expiry: %s", aws.ToString(delvol.VolumeId), err)
					}
					log.Printf("EKS: Attempting direct delete of volume %s", aws.ToString(delvol.VolumeId))
					_, err = ec2svc.DeleteVolume(context.TODO(), &ec2.DeleteVolumeInput{
						VolumeId: delvol.VolumeId,
					})
					if err != nil {
						log.Printf("EKS: Warning: could not delete volume %s now (tagged for standard expiry, will be retried): %s", aws.ToString(delvol.VolumeId), err)
					}
				}
				log.Print("Checking IAM Identity Providers (tag:alpha.eksctl.io/cluster-name={CLUSTERNAME})")
				iamsvc, err := baws.GetIamClient(h.Credentials, &region)
				if err != nil {
					return fmt.Errorf("could not get iam client: %w", err)
				}
				oidc, err := iamsvc.ListOpenIDConnectProviders(context.TODO(), &iam.ListOpenIDConnectProvidersInput{})
				if err != nil {
					return fmt.Errorf("could not iam.ListOpenIDConnectProviders: %w", err)
				}
				oidcDelList := []string{}
				for _, oidcItem := range oidc.OpenIDConnectProviderList {
					tags, err := iamsvc.ListOpenIDConnectProviderTags(context.TODO(), &iam.ListOpenIDConnectProviderTagsInput{
						OpenIDConnectProviderArn: oidcItem.Arn,
					})
					if err != nil {
						return fmt.Errorf("could not iam.ListOpenIDConnectProviderTags on %s: %w", *oidcItem.Arn, err)
					}
					tagList := make(map[string]string)
					for _, tag := range tags.Tags {
						tagList[*tag.Key] = *tag.Value
					}
					if tagList["alpha.eksctl.io/cluster-name"] == aws.ToString(eksCluster.Cluster.Name) {
						oidcDelList = append(oidcDelList, *oidcItem.Arn)
					}
				}
				log.Printf("Deleting %d IAM Identity Provider for the k8s cluster", len(oidcDelList))
				for _, oidcDel := range oidcDelList {
					_, err = iamsvc.DeleteOpenIDConnectProvider(context.TODO(), &iam.DeleteOpenIDConnectProviderInput{
						OpenIDConnectProviderArn: aws.String(oidcDel),
					})
					if err != nil {
						return fmt.Errorf("could not iam.DeleteOpenIDConnectProvider on %s: %w", oidcDel, err)
					}
				}
			}
			// find the VPCs associated with the stack; then delete their security group, routes, and peerings, in that order
			// delete the stack after the extra VPC resources have been deleted
			log.Printf("EKS: Describing resources for stack %s to find VPCs", stack.Name)
			stackResources, err := cfSvc.DescribeStackResources(context.TODO(), &cloudformation.DescribeStackResourcesInput{
				StackName: aws.String(stack.Name),
			})
			if err != nil {
				log.Printf("EKS: Warning: could not describe stack resources for %s: %s", stack.Name, err)
			} else {
				// collect VPC IDs from the stack resources
				var vpcIds []string
				for _, res := range stackResources.StackResources {
					if aws.ToString(res.ResourceType) == "AWS::EC2::VPC" && res.PhysicalResourceId != nil {
						vpcIds = append(vpcIds, aws.ToString(res.PhysicalResourceId))
					}
				}
				for _, vpcId := range vpcIds {
					log.Printf("EKS: Cleaning up VPC resources for %s in stack %s", vpcId, stack.Name)

					// 1. Delete non-default security groups in the VPC
					sgOut, err := ec2svc.DescribeSecurityGroups(context.TODO(), &ec2.DescribeSecurityGroupsInput{
						Filters: []ec2types.Filter{
							{
								Name:   aws.String("vpc-id"),
								Values: []string{vpcId},
							},
						},
					})
					if err != nil {
						log.Printf("EKS: Warning: could not describe security groups for VPC %s: %s", vpcId, err)
					} else {
						// first revoke all ingress/egress rules that reference non-default SGs, to avoid dependency issues
						for _, sg := range sgOut.SecurityGroups {
							if aws.ToString(sg.GroupName) == "default" {
								continue
							}
							if len(sg.IpPermissions) > 0 {
								_, err = ec2svc.RevokeSecurityGroupIngress(context.TODO(), &ec2.RevokeSecurityGroupIngressInput{
									GroupId:       sg.GroupId,
									IpPermissions: sg.IpPermissions,
								})
								if err != nil {
									log.Printf("EKS: Warning: could not revoke ingress rules for SG %s: %s", aws.ToString(sg.GroupId), err)
								}
							}
							if len(sg.IpPermissionsEgress) > 0 {
								_, err = ec2svc.RevokeSecurityGroupEgress(context.TODO(), &ec2.RevokeSecurityGroupEgressInput{
									GroupId:       sg.GroupId,
									IpPermissions: sg.IpPermissionsEgress,
								})
								if err != nil {
									log.Printf("EKS: Warning: could not revoke egress rules for SG %s: %s", aws.ToString(sg.GroupId), err)
								}
							}
						}
						// now delete the non-default security groups
						for _, sg := range sgOut.SecurityGroups {
							if aws.ToString(sg.GroupName) == "default" {
								continue
							}
							log.Printf("EKS: Deleting security group %s (%s) in VPC %s", aws.ToString(sg.GroupId), aws.ToString(sg.GroupName), vpcId)
							_, err = ec2svc.DeleteSecurityGroup(context.TODO(), &ec2.DeleteSecurityGroupInput{
								GroupId: sg.GroupId,
							})
							if err != nil {
								log.Printf("EKS: Warning: could not delete security group %s: %s", aws.ToString(sg.GroupId), err)
							}
						}
					}

					// 2. Delete routes referencing VPC peering connections in the VPC's route tables
					rtOut, err := ec2svc.DescribeRouteTables(context.TODO(), &ec2.DescribeRouteTablesInput{
						Filters: []ec2types.Filter{
							{
								Name:   aws.String("vpc-id"),
								Values: []string{vpcId},
							},
						},
					})
					if err != nil {
						log.Printf("EKS: Warning: could not describe route tables for VPC %s: %s", vpcId, err)
					} else {
						for _, rt := range rtOut.RouteTables {
							for _, route := range rt.Routes {
								if route.VpcPeeringConnectionId == nil {
									continue
								}
								cidr := aws.ToString(route.DestinationCidrBlock)
								if cidr == "" {
									cidr = aws.ToString(route.DestinationIpv6CidrBlock)
								}
								if cidr == "" {
									continue
								}
								log.Printf("EKS: Deleting route %s -> peering %s in route table %s", cidr, aws.ToString(route.VpcPeeringConnectionId), aws.ToString(rt.RouteTableId))
								deleteRouteInput := &ec2.DeleteRouteInput{
									RouteTableId: rt.RouteTableId,
								}
								if route.DestinationCidrBlock != nil {
									deleteRouteInput.DestinationCidrBlock = route.DestinationCidrBlock
								} else {
									deleteRouteInput.DestinationIpv6CidrBlock = route.DestinationIpv6CidrBlock
								}
								_, err = ec2svc.DeleteRoute(context.TODO(), deleteRouteInput)
								if err != nil {
									log.Printf("EKS: Warning: could not delete route %s in route table %s: %s", cidr, aws.ToString(rt.RouteTableId), err)
								}
							}
						}
					}

					// 3. Delete VPC peering connections associated with this VPC
					peeringOut, err := ec2svc.DescribeVpcPeeringConnections(context.TODO(), &ec2.DescribeVpcPeeringConnectionsInput{
						Filters: []ec2types.Filter{
							{
								Name:   aws.String("requester-vpc-info.vpc-id"),
								Values: []string{vpcId},
							},
						},
					})
					if err != nil {
						log.Printf("EKS: Warning: could not describe VPC peering connections for VPC %s: %s", vpcId, err)
					} else {
						for _, peering := range peeringOut.VpcPeeringConnections {
							log.Printf("EKS: Deleting VPC peering connection %s for VPC %s", aws.ToString(peering.VpcPeeringConnectionId), vpcId)
							_, err = ec2svc.DeleteVpcPeeringConnection(context.TODO(), &ec2.DeleteVpcPeeringConnectionInput{
								VpcPeeringConnectionId: peering.VpcPeeringConnectionId,
							})
							if err != nil {
								log.Printf("EKS: Warning: could not delete VPC peering connection %s: %s", aws.ToString(peering.VpcPeeringConnectionId), err)
							}
						}
					}
					// also check accepter side
					peeringOut, err = ec2svc.DescribeVpcPeeringConnections(context.TODO(), &ec2.DescribeVpcPeeringConnectionsInput{
						Filters: []ec2types.Filter{
							{
								Name:   aws.String("accepter-vpc-info.vpc-id"),
								Values: []string{vpcId},
							},
						},
					})
					if err != nil {
						log.Printf("EKS: Warning: could not describe accepter VPC peering connections for VPC %s: %s", vpcId, err)
					} else {
						for _, peering := range peeringOut.VpcPeeringConnections {
							log.Printf("EKS: Deleting VPC peering connection %s (accepter) for VPC %s", aws.ToString(peering.VpcPeeringConnectionId), vpcId)
							_, err = ec2svc.DeleteVpcPeeringConnection(context.TODO(), &ec2.DeleteVpcPeeringConnectionInput{
								VpcPeeringConnectionId: peering.VpcPeeringConnectionId,
							})
							if err != nil {
								log.Printf("EKS: Warning: could not delete VPC peering connection %s: %s", aws.ToString(peering.VpcPeeringConnectionId), err)
							}
						}
					}
				}
			}
			if stack.CurrentStatus != string(cftypes.StackStatusDeleteInProgress) {
				log.Printf("EKS: Scheduling deletion of stack %s", stack.Name)
				_, err = cfSvc.DeleteStack(context.TODO(), &cloudformation.DeleteStackInput{
					StackName: aws.String(stack.Name),
				})
				if err != nil {
					return fmt.Errorf("could not cloudformation.DeleteStack: %w", err)
				}
			}
			log.Printf("EKS: Waiting on %s to be deleted", stack.Name)
			waiter := cloudformation.NewStackDeleteCompleteWaiter(cfSvc)
			err = waiter.Wait(context.TODO(), &cloudformation.DescribeStacksInput{
				StackName: aws.String(stack.Name),
			}, 10*time.Minute)
			if err != nil {
				return fmt.Errorf("could not cloudformation.WaitUntilStackDeleteComplete: %w", err)
			}
		}
		log.Println("EKS: Cluster Expired")
	}
	return nil
}

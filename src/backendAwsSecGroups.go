package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/bestmethod/inslice"
)

func (d *backendAws) GetAZName(subnetID string) (string, error) {
	if strings.HasPrefix(subnetID, "subnet-") {
		snets, err := d.ec2svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
			SubnetIds: aws.StringSlice([]string{subnetID}),
		})
		if err != nil {
			return "", fmt.Errorf("could not find subnet: %s", err)
		}
		if len(snets.Subnets) == 0 {
			return "", fmt.Errorf("could not find subnet")
		}
		return *snets.Subnets[0].AvailabilityZone, nil
	}

	out, err := d.resolveVPC()
	if err != nil {
		return "", fmt.Errorf("could not resolve default VPC: %s", err)
	}
	if len(out.Vpcs) == 0 {
		return "", fmt.Errorf("could not find default VPC, does not exist in AWS account; use the appropriate command switch to specify the security group and subnet ID to use")
	}
	vpc := aws.StringValue(out.Vpcs[0].VpcId)
	if len(out.Vpcs) > 1 {
		log.Printf("WARN: more than 1 default VPC found, choosing first one in list: %s", vpc)
	}

	filters := []*ec2.Filter{
		{
			Name:   aws.String("default-for-az"),
			Values: aws.StringSlice([]string{"true"}),
		},
		{
			Name:   aws.String("vpc-id"),
			Values: aws.StringSlice([]string{vpc}),
		},
	}
	if subnetID != "" {
		filters = append(filters, &ec2.Filter{
			Name:   aws.String("availability-zone"),
			Values: aws.StringSlice([]string{subnetID}),
		})
	}
	sout, err := d.ec2svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: filters,
	})
	if err != nil {
		return "", fmt.Errorf("could not resolve default subnet: %s", err)
	}
	if len(sout.Subnets) == 0 {
		return "", fmt.Errorf("could not find default subnet, does not exist in AWS account; use the appropriate command switch to specify the subnet ID to use")
	}
	return *sout.Subnets[0].AvailabilityZone, nil
}

func (d *backendAws) resolveVPC() (*ec2.DescribeVpcsOutput, error) {
	return d.resolveVPCdo(true)
}

func (d *backendAws) resolveVPCdo(create bool) (*ec2.DescribeVpcsOutput, error) {
	out, err := d.ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("is-default"),
				Values: aws.StringSlice([]string{"true"}),
			},
		},
	})
	if err != nil && create {
		_, errx := d.ec2svc.CreateDefaultVpc(&ec2.CreateDefaultVpcInput{})
		if errx != nil {
			return out, fmt.Errorf("%s :: %s", err, errx)
		}
		errx = d.ec2svc.WaitUntilVpcExists(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("is-default"),
					Values: aws.StringSlice([]string{"true"}),
				},
			},
		})
		if errx != nil {
			return out, fmt.Errorf("%s :: %s", err, errx)
		}
		errx = d.ec2svc.WaitUntilVpcAvailable(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("is-default"),
					Values: aws.StringSlice([]string{"true"}),
				},
			},
		})
		if errx != nil {
			return out, fmt.Errorf("%s :: %s", err, errx)
		}
		return d.resolveVPCdo(false)
	}
	return out, err
}

func (d *backendAws) resolveSecGroupAndSubnet(secGroupID string, subnetID string, printID bool, namePrefixes []string, isAgi bool) (secGroup string, subnet string, err error) {
	var vpc string
	if secGroupID == "" || !strings.HasPrefix(subnetID, "subnet-") {
		if !strings.HasPrefix(subnetID, "subnet-") {
			out, err := d.resolveVPC()
			if err != nil {
				return "", "", fmt.Errorf("could not resolve default VPC: %s", err)
			}
			if len(out.Vpcs) == 0 {
				return "", "", fmt.Errorf("could not find default VPC, does not exist in AWS account; use the appropriate command switch to specify the security group and subnet ID to use")
			}
			vpc = aws.StringValue(out.Vpcs[0].VpcId)
			if len(out.Vpcs) > 1 {
				log.Printf("WARN: more than 1 default VPC found, choosing first one in list: %s", vpc)
			}
		} else {
			out, err := d.ec2svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
				SubnetIds: aws.StringSlice([]string{subnetID}),
			})
			if err != nil {
				return "", "", fmt.Errorf("could not resolve given subnet: %s", err)
			}
			if len(out.Subnets) == 0 {
				return "", "", fmt.Errorf("could not find given subnet")
			}
			vpc = aws.StringValue(out.Subnets[0].VpcId)
		}
	}

	if strings.HasPrefix(subnetID, "subnet-") {
		subnet = subnetID
	} else {
		filters := []*ec2.Filter{
			{
				Name:   aws.String("default-for-az"),
				Values: aws.StringSlice([]string{"true"}),
			},
			{
				Name:   aws.String("vpc-id"),
				Values: aws.StringSlice([]string{vpc}),
			},
		}
		if subnetID != "" {
			filters = append(filters, &ec2.Filter{
				Name:   aws.String("availability-zone"),
				Values: aws.StringSlice([]string{subnetID}),
			})
		}
		out, err := d.ec2svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
			Filters: filters,
		})
		if err != nil {
			return "", "", fmt.Errorf("could not resolve default subnet: %s", err)
		}
		if len(out.Subnets) == 0 {
			return "", "", fmt.Errorf("could not find default subnet, does not exist in AWS account; use the appropriate command switch to specify the subnet ID to use")
		}
		subnet = aws.StringValue(out.Subnets[0].SubnetId)
		if len(out.Subnets) > 1 {
			log.Printf("WARN: more than 1 default subnet found for vpc %s, choosing first one in list: %s", vpc, subnet)
		}
	}
	if printID {
		log.Printf("Using subnet ID %s", subnet)
	}

	if secGroupID != "" {
		secGroup = secGroupID
		if printID {
			log.Printf("Using security group ID %s", secGroup)
		}
	} else {
		groupNames := namePrefixes
		if d.server {
			groupNames = append(groupNames, "AeroLabServer")
		} else {
			groupNames = append(groupNames, "AeroLabClient")
		}
		var secGroupList []string
		for _, groupPrefix := range groupNames {
			groupName := groupPrefix + "-" + strings.TrimPrefix(vpc, "vpc-")
			var out *ec2.DescribeSecurityGroupsOutput
			var err error
			if vpc == "" {
				out, err = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
					GroupNames: aws.StringSlice([]string{groupName}),
				})
			} else {
				out, err = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
					Filters: []*ec2.Filter{
						{
							Name:   aws.String("vpc-id"),
							Values: aws.StringSlice([]string{vpc}),
						},
						{
							Name:   aws.String("group-name"),
							Values: aws.StringSlice([]string{groupName}),
						},
					},
				})
			}
			if err != nil && !strings.Contains(err.Error(), "InvalidGroup.NotFound") {
				return "", "", fmt.Errorf("could not resolve security groups: %s", err)
			}
			if (err != nil && strings.Contains(err.Error(), "InvalidGroup.NotFound")) || len(out.SecurityGroups) == 0 {
				log.Print("Managed Security groups not found in VPC for given subnet, creating...")
				secGroupsA, err := d.createSecGroups(vpc, groupPrefix, isAgi)
				if err != nil {
					return "", "", fmt.Errorf("could create security groups: %s", err)
				}
				for _, sssg := range secGroupsA {
					if !inslice.HasString(secGroupList, sssg) {
						secGroupList = append(secGroupList, sssg)
					}
				}
				secGroup = strings.Join(secGroupList, ",")
				log.Printf("Using security group IDs %s", secGroup)
			} else {
				if groupPrefix != "AeroLabServer" && groupPrefix != "AeroLabClient" {
					fwfound := false
					myIp := getip2()
					parsedIp := net.ParseIP(myIp)
					for _, perms := range out.SecurityGroups[0].IpPermissions {
						if aws.Int64Value(perms.FromPort) == -1 || aws.Int64Value(perms.ToPort) == -1 {
							continue
						}
						for _, permRange := range perms.IpRanges {
							_, cidr, _ := net.ParseCIDR(*permRange.CidrIp)
							if cidr.Contains(parsedIp) {
								fwfound = true
								break
							}
						}
						if fwfound {
							break
						}
					}
					if !fwfound {
						log.Println("Security group CIDR doesn't allow this command to complete, re-locking security groups with the caller's IP")
						err = d.LockSecurityGroups(myIp, true, vpc, groupPrefix, isAgi)
						if err != nil {
							return secGroup, subnet, err
						}
					}
				}
				secGroupA := aws.StringValue(out.SecurityGroups[0].GroupId)
				if !inslice.HasString(secGroupList, secGroupA) {
					secGroupList = append(secGroupList, secGroupA)
				}
				secGroup = strings.Join(secGroupList, ",")
				log.Printf("Using security group ID %s name %s", secGroupA, groupName)
			}
		}
	}

	return
}

func (d *backendAws) createSecGroups(vpc string, namePrefix string, isAgi bool) (secGroups []string, err error) {
	var secGroupIds []string
	groupNames := []string{"AeroLabServer-" + strings.TrimPrefix(vpc, "vpc-"), "AeroLabClient-" + strings.TrimPrefix(vpc, "vpc-"), namePrefix + "-" + strings.TrimPrefix(vpc, "vpc-")}

	// create groups
	for i, groupName := range groupNames {
		out, err := d.ec2svc.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
			Description: aws.String(groupName),
			GroupName:   aws.String(groupName),
			VpcId:       aws.String(vpc),
		})
		if err != nil {
			if !strings.Contains(err.Error(), "InvalidGroup.Duplicate") || i > 1 {
				return nil, fmt.Errorf("could not create default server security group for AeroLab in vpc %s: %s", vpc, err)
			} else {
				secGroupIds = append(secGroupIds, "EXISTS")
				continue
			}
		}
		if i == 0 && d.server {
			secGroups = append(secGroups, aws.StringValue(out.GroupId))
		} else if i == 1 && d.client {
			secGroups = append(secGroups, aws.StringValue(out.GroupId))
		} else if i == 2 {
			secGroups = append(secGroups, aws.StringValue(out.GroupId))
		}
		secGroupIds = append(secGroupIds, aws.StringValue(out.GroupId))
		err = d.ec2svc.WaitUntilSecurityGroupExists(&ec2.DescribeSecurityGroupsInput{
			GroupIds: []*string{out.GroupId},
		})
		if err != nil {
			if i == 2 {
				d.deleteSecGroups(vpc, namePrefix, false)
			} else {
				d.deleteSecGroups(vpc, namePrefix, true)
			}
			return nil, fmt.Errorf("an error occurred while waiting for security group to exist after creation for AeroLab in vpc %s: %s", vpc, err)
		}
	}

	// add ingress rule for inter-comms
	for groupNo, groupId := range secGroupIds {
		if groupNo > 1 {
			break
		}
		if groupId == "EXISTS" {
			continue
		}
		_, err := d.ec2svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: aws.String(groupId),
			IpPermissions: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("-1"),
					FromPort:   aws.Int64(-1),
					ToPort:     aws.Int64(-1),
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							Description: aws.String("serverGroup"),
							GroupId:     aws.String(secGroupIds[0]),
							VpcId:       aws.String(vpc),
						},
						{
							Description: aws.String("clientGroup"),
							GroupId:     aws.String(secGroupIds[1]),
							VpcId:       aws.String(vpc),
						},
					},
				},
			},
		})
		if err != nil {
			d.deleteSecGroups(vpc, namePrefix, true)
			return nil, fmt.Errorf("an error occurred while adding ingress intercomms for security group to exist after creation for AeroLab in vpc %s: %s", vpc, err)
		}
	}

	ip := "0.0.0.0/0"
	if !isAgi {
		ip = getip2()
		if !strings.Contains(ip, "/") {
			ip = ip + "/32"
		}
	}
	// add ingress rule for port 22
	for groupNo, groupId := range secGroupIds {
		if groupNo < 2 {
			continue
		}
		_, err := d.ec2svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: aws.String(groupId),
			IpPermissions: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(22),
					ToPort:     aws.Int64(22),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp:      aws.String(ip),
							Description: aws.String("ssh from anywhere"),
						},
					},
				}, {
					IpProtocol: aws.String("icmp"),
					FromPort:   aws.Int64(-1),
					ToPort:     aws.Int64(-1),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp:      aws.String("0.0.0.0/0"),
							Description: aws.String("icmp from anywhere"),
						},
					},
				},
			},
		})
		if err != nil {
			d.deleteSecGroups(vpc, namePrefix, false)
			return nil, fmt.Errorf("an error occurred while adding ingress port 22 for security group to exist after creation for AeroLab in vpc %s: %s", vpc, err)
		}
	}

	// ingress rule for client for special ports (grafana, vscode), apply on server too, in case we want access from the great beyond
	extraports := []int64{80, 443}
	if !isAgi {
		extraports = []int64{3000, 80, 443, 8080, 8888, 9200}
	}
	for groupNo, groupId := range secGroupIds {
		if groupNo < 2 {
			continue
		}
		for _, port := range extraports {
			_, err := d.ec2svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
				GroupId: aws.String(groupId),
				IpPermissions: []*ec2.IpPermission{
					{
						IpProtocol: aws.String("tcp"),
						FromPort:   aws.Int64(port),
						ToPort:     aws.Int64(port),
						IpRanges: []*ec2.IpRange{
							{
								CidrIp:      aws.String(ip),
								Description: aws.String("allow " + strconv.Itoa(int(port)) + " from anywhere"),
							},
						},
					},
				},
			})
			if err != nil {
				d.deleteSecGroups(vpc, namePrefix, false)
				return nil, fmt.Errorf("an error occurred while adding ingress port 22 for security group to exist after creation for AeroLab in vpc %s: %s", vpc, err)
			}
		}
	}
	return secGroups, nil
}

func (d *backendAws) deleteSecGroups(vpc string, namePrefix string, internal bool) error {
	// prerequisite - remove dependencies between groups if such exist
	var group1, group2, group3 *ec2.DescribeSecurityGroupsOutput
	var err1, err2, err3 error
	if vpc == "" {
		if internal {
			group1, err1 = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
				GroupNames: aws.StringSlice([]string{"AeroLabServer-" + strings.TrimPrefix(vpc, "vpc-")}),
			})
			group2, err2 = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
				GroupNames: aws.StringSlice([]string{"AeroLabClient-" + strings.TrimPrefix(vpc, "vpc-")}),
			})
		}
		group3, err3 = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
			GroupNames: aws.StringSlice([]string{namePrefix + "-" + strings.TrimPrefix(vpc, "vpc-")}),
		})
	} else {
		if internal {
			group1, err1 = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("vpc-id"),
						Values: aws.StringSlice([]string{vpc}),
					},
					{
						Name:   aws.String("group-name"),
						Values: aws.StringSlice([]string{"AeroLabServer-" + strings.TrimPrefix(vpc, "vpc-")}),
					},
				},
			})
			group2, err2 = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("vpc-id"),
						Values: aws.StringSlice([]string{vpc}),
					},
					{
						Name:   aws.String("group-name"),
						Values: aws.StringSlice([]string{"AeroLabClient-" + strings.TrimPrefix(vpc, "vpc-")}),
					},
				},
			})
		}
		group3, err3 = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("vpc-id"),
					Values: aws.StringSlice([]string{vpc}),
				},
				{
					Name:   aws.String("group-name"),
					Values: aws.StringSlice([]string{namePrefix + "-" + strings.TrimPrefix(vpc, "vpc-")}),
				},
			},
		})
	}
	if internal && err1 == nil && err2 == nil && len(group1.SecurityGroups) > 0 && len(group2.SecurityGroups) > 0 {
		_, err := d.ec2svc.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
			GroupId: group1.SecurityGroups[0].GroupId,
			IpPermissions: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("-1"),
					FromPort:   aws.Int64(-1),
					ToPort:     aws.Int64(-1),
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: group2.SecurityGroups[0].GroupId,
							VpcId:   aws.String(vpc),
						},
					},
				},
			},
		}) // remove server deps from client group
		if err != nil {
			log.Printf("WARN: could not remove server dependency from client group: %s", err)
		}
		_, err = d.ec2svc.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
			GroupId: group2.SecurityGroups[0].GroupId,
			IpPermissions: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("-1"),
					FromPort:   aws.Int64(-1),
					ToPort:     aws.Int64(-1),
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						{
							GroupId: group1.SecurityGroups[0].GroupId,
							VpcId:   aws.String(vpc),
						},
					},
				},
			},
		}) // remove client deps from server group
		if err != nil {
			log.Printf("WARN: could not remove client dependency from server group: %s", err)
		}
	}
	var nerr error
	if vpc == "" {
		if internal {
			_, err := d.ec2svc.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
				GroupName: aws.String("AeroLabServer-" + strings.TrimPrefix(vpc, "vpc-")),
			})
			if err != nil {
				nerr = fmt.Errorf("failed to delete AeroLabServer group: %s", err)
			}
			_, err = d.ec2svc.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
				GroupName: aws.String("AeroLabClient-" + strings.TrimPrefix(vpc, "vpc-")),
			})
			if err != nil {
				if nerr == nil {
					nerr = fmt.Errorf("failed to delete AeroLabClient group: %s", err)
				} else {
					nerr = fmt.Errorf("%s ;; failed to delete AeroLabClient group: %s", nerr, err)
				}
			}
		}
		_, err := d.ec2svc.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
			GroupName: aws.String(namePrefix + "-" + strings.TrimPrefix(vpc, "vpc-")),
		})
		if err != nil {
			if nerr == nil {
				nerr = fmt.Errorf("failed to delete %s group: %s", namePrefix, err)
			} else {
				nerr = fmt.Errorf("%s ;; failed to delete %s group: %s", nerr, namePrefix, err)
			}
		}
	} else {
		if internal {
			if len(group1.SecurityGroups) > 0 {
				_, err := d.ec2svc.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
					GroupId: group1.SecurityGroups[0].GroupId,
				})
				if err != nil {
					nerr = fmt.Errorf("failed to delete AeroLabServer group: %s", err)
				}
			}
			if len(group2.SecurityGroups) > 0 {
				_, err := d.ec2svc.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
					GroupId: group2.SecurityGroups[0].GroupId,
				})
				if err != nil {
					if nerr == nil {
						nerr = fmt.Errorf("failed to delete AeroLabClient group: %s", err)
					} else {
						nerr = fmt.Errorf("%s ;; failed to delete AeroLabClient group: %s", nerr, err)
					}
				}
			}
		}
		if err3 == nil && len(group3.SecurityGroups) > 0 {
			_, err := d.ec2svc.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
				GroupId: group3.SecurityGroups[0].GroupId,
			})
			if err != nil {
				if nerr == nil {
					nerr = fmt.Errorf("failed to delete external group: %s", err)
				} else {
					nerr = fmt.Errorf("%s ;; failed to delete external group: %s", nerr, err)
				}
			}
		}
	}
	return nerr
}

// ignoring performLocking as this command already does what it's supposed to
func (d *backendAws) AssignSecurityGroups(clusterName string, names []string, vpc string, remove bool, performLocking bool) error {
	var instIds []*ec2.Instance
	var secGroupIds []string
	// find all instanceIds for a given cluster; if 0 found, error
	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + awsTagUsedBy),
				Values: []*string{aws.String(awsTagUsedByValue)},
			},
		},
	}
	instances, err := d.ec2svc.DescribeInstances(&filter)
	if err != nil {
		return fmt.Errorf("could not run DescribeInstances\n%s", err)
	}
	for _, reservation := range instances.Reservations {
		for _, instance := range reservation.Instances {
			instance := instance
			if *instance.State.Code != int64(48) {
				for _, tag := range instance.Tags {
					if *tag.Key == awsTagClusterName {
						if *tag.Value == clusterName {
							instIds = append(instIds, instance)
						}
					}
				}
			}
		}
	}
	if len(instIds) == 0 {
		return errors.New("cluster not found")
	}
	// list security groups, if groups with given names not found, error
	secGroups, err := d.listSecurityGroups(false)
	if err != nil {
		return err
	}

	for _, name := range names {
		found := false
		for _, sg := range secGroups {
			if strings.Split(sg.AWS.SecurityGroupName, "-")[0] == name {
				found = true
				secGroupIds = append(secGroupIds, sg.AWS.SecurityGroupID)
				break
			}
		}
		if !found {
			return fmt.Errorf("security group with prefix %s not found", name)
		}
	}
	for _, inst := range instIds {
		sgids := secGroupIds
		if !remove {
			for _, sgold := range inst.SecurityGroups {
				if !inslice.HasString(sgids, *sgold.GroupId) {
					sgids = append(sgids, *sgold.GroupId)
				}
			}
		} else {
			sgids = []string{}
			for _, sgold := range inst.SecurityGroups {
				if !inslice.HasString(secGroupIds, *sgold.GroupId) {
					sgids = append(sgids, *sgold.GroupId)
				}
			}
		}
		_, err := d.ec2svc.ModifyInstanceAttribute(&ec2.ModifyInstanceAttributeInput{
			Groups:     aws.StringSlice(sgids),
			InstanceId: inst.InstanceId,
		})
		if err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	// for each instance, modify the security group assignments
	return nil
}

func (d *backendAws) DeleteSecurityGroups(vpc string, namePrefix string, internal bool) error {
	if vpc == "" {
		out, err := d.ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("is-default"),
					Values: aws.StringSlice([]string{"true"}),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("could not resolve default VPC: %s", err)
		}
		if len(out.Vpcs) == 0 {
			return fmt.Errorf("could not find default VPC, does not exist in AWS account; use the appropriate command switch to specify the VPC to use")
		}
		vpc = aws.StringValue(out.Vpcs[0].VpcId)
		if len(out.Vpcs) > 1 {
			log.Printf("WARN: more than 1 default VPC found, choosing first one in list: %s", vpc)
		}
	}
	return d.deleteSecGroups(vpc, namePrefix, internal)
}

func (d *backendAws) LockSecurityGroups(ip string, lockSSH bool, vpc string, namePrefix string, isAgi bool) error {
	portList := []int64{3000, 80, 443, 8080, 8888, 9200}
	if isAgi {
		portList = []int64{80, 443}
	}
	if lockSSH {
		portList = append(portList, 22)
	}
	// if VPC is "", find default-vpc
	if vpc == "" {
		out, err := d.ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("is-default"),
					Values: aws.StringSlice([]string{"true"}),
				},
			},
		})
		if err != nil {
			return err
		}
		if len(out.Vpcs) == 0 {
			return errors.New("default VPC not found")
		}
		vpc = aws.StringValue(out.Vpcs[0].VpcId)
	}
	return d.lockSecurityGroups(ip, portList, namePrefix+"-"+strings.TrimPrefix(vpc, "vpc-"), vpc, namePrefix, isAgi)
}

func (d *backendAws) lockSecurityGroups(ip string, portList []int64, secGroupName string, vpc string, namePrefix string, isAgi bool) error {
	var sgi *ec2.DescribeSecurityGroupsInput
	if vpc == "" {
		sgi = &ec2.DescribeSecurityGroupsInput{
			GroupNames: aws.StringSlice([]string{secGroupName}),
		}
	} else {
		sgi = &ec2.DescribeSecurityGroupsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("vpc-id"),
					Values: aws.StringSlice([]string{vpc}),
				},
				{
					Name:   aws.String("group-name"),
					Values: aws.StringSlice([]string{secGroupName}),
				},
			},
		}
	}
	groups, err := d.ec2svc.DescribeSecurityGroups(sgi)
	if err != nil {
		return fmt.Errorf("could not get AeroLabClient security group in selected region: %s", err)
	}
	if len(groups.SecurityGroups) == 0 {
		return errors.New("could not find security group")
	}
	group := groups.SecurityGroups[0]

	for _, port := range portList {
		for _, rule := range group.IpPermissions {
			if aws.Int64Value(rule.FromPort) != port {
				continue
			}
			_, err = d.ec2svc.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
				GroupId: group.GroupId,
				IpPermissions: []*ec2.IpPermission{
					rule,
				},
			})
			if err != nil {
				return fmt.Errorf("an error occurred while removing ingress ports for security group: %s", err)
			}
		}
	}
	if namePrefix == "AeroLabServer" || namePrefix == "AeroLabClient" {
		return nil
	}
	if isAgi {
		ip = "0.0.0.0/0"
	} else {
		if ip == "discover-caller-ip" {
			ip = getip2()
		}
		if !strings.Contains(ip, "/") {
			ip = ip + "/32"
		}
	}
	for _, port := range portList {
		_, err := d.ec2svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: group.GroupId,
			IpPermissions: []*ec2.IpPermission{
				{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(port),
					ToPort:     aws.Int64(port),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp:      aws.String(ip),
							Description: aws.String("allow " + strconv.Itoa(int(port)) + " from anywhere"),
						},
					},
				},
			},
		})
		if err != nil {
			return fmt.Errorf("an error occurred while adding ingress ports for security group in vpc %s", err)
		}
	}

	return nil
}

type IP struct {
	Query string
}

func getip2() string {
	req, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return err.Error()
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err.Error()
	}

	var ip IP
	json.Unmarshal(body, &ip)

	return ip.Query
}

func (d *backendAws) CreateSecurityGroups(vpc string, namePrefix string, isAgi bool) error {
	if vpc == "" {
		out, err := d.ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("is-default"),
					Values: aws.StringSlice([]string{"true"}),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("could not resolve default VPC: %s", err)
		}
		if len(out.Vpcs) == 0 {
			return fmt.Errorf("could not find default VPC, does not exist in AWS account; use the appropriate command switch to specify the VPC to use")
		}
		vpc = aws.StringValue(out.Vpcs[0].VpcId)
		if len(out.Vpcs) > 1 {
			log.Printf("WARN: more than 1 default VPC found, choosing first one in list: %s", vpc)
		}
	}
	_, err := d.createSecGroups(vpc, namePrefix, isAgi)
	return err
}

func (d *backendAws) ListSecurityGroups() error {
	_, err := d.listSecurityGroups(true)
	return err
}

func (d *backendAws) listSecurityGroups(stdout bool) ([]inventoryFirewallRule, error) {
	out, err := d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{})
	if err != nil {
		return nil, err
	}
	rules := []inventoryFirewallRule{}
	for _, sg := range out.SecurityGroups {
		nIps := []string{}
		for _, sga := range sg.IpPermissions {
			if *sga.IpProtocol == "icmp" {
				continue
			}
			for _, sgb := range sga.IpRanges {
				if !inslice.HasString(nIps, *sgb.CidrIp) {
					nIps = append(nIps, *sgb.CidrIp)
				}
			}
		}
		if len(nIps) == 0 && (strings.HasPrefix(aws.StringValue(sg.GroupName), "AeroLabServer-") || strings.HasPrefix(aws.StringValue(sg.GroupName), "AeroLabClient-")) {
			nIps = []string{"internal"}
		}
		rules = append(rules, inventoryFirewallRule{
			AWS: &inventoryFirewallRuleAWS{
				VPC:               aws.StringValue(sg.VpcId),
				SecurityGroupName: aws.StringValue(sg.GroupName),
				SecurityGroupID:   aws.StringValue(sg.GroupId),
				IPs:               nIps,
				Region:            a.opts.Config.Backend.Region,
			},
		})
	}
	if stdout {
		var output []string
		for _, sg := range out.SecurityGroups {
			/*
				if !strings.HasPrefix(aws.StringValue(sg.GroupName), "AeroLabServer-") && !strings.HasPrefix(aws.StringValue(sg.GroupName), "AeroLabClient-") {
					continue
				}
			*/
			output = append(output, fmt.Sprintf("%s\t%s\t%s", aws.StringValue(sg.VpcId), aws.StringValue(sg.GroupName), aws.StringValue(sg.GroupId)))
		}
		sort.Strings(output)
		w := tabwriter.NewWriter(os.Stdout, 1, 1, 4, ' ', 0)
		fmt.Fprintln(w, "VPC_ID\tSecurityGroupName\tSecurityGroupID")
		fmt.Fprintln(w, "------\t-----------------\t---------------")
		for _, line := range output {
			fmt.Fprint(w, line+"\n")
		}
		w.Flush()
	}
	return rules, nil
}

type vpc struct {
	cidr string
	name string
}

func (d *backendAws) ListSubnets() error {
	_, err := d.listSubnets(true)
	return err
}
func (d *backendAws) listSubnets(stdout bool) ([]inventorySubnetAWS, error) {
	ij := []inventorySubnetAWS{}
	out, err := d.ec2svc.DescribeSubnets(&ec2.DescribeSubnetsInput{})
	if err != nil {
		return nil, err
	}
	outv, err := d.ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, err
	}
	vpcList := make(map[string]*vpc)
	for _, avpc := range outv.Vpcs {
		nameTag := ""
		for _, tag := range avpc.Tags {
			if strings.ToLower(aws.StringValue(tag.Key)) == "name" {
				nameTag = aws.StringValue(tag.Value)
				break
			}
		}
		vpcList[aws.StringValue(avpc.VpcId)] = &vpc{
			name: nameTag,
			cidr: aws.StringValue(avpc.CidrBlock),
		}
	}
	foundVPCs := []string{}
	var w *tabwriter.Writer
	if stdout {
		w = tabwriter.NewWriter(os.Stdout, 1, 1, 4, ' ', 0)
		fmt.Fprintln(w, "VPC_ID\tVPC_Name\tVPC_Cidr\tAvailZone\tSubnet_ID\tSubnet_CIDR\tAZ-Default\tSubnet_Name\tAutoPublicIP")
		fmt.Fprintln(w, "------\t--------\t--------\t---------\t---------\t-----------\t----------\t-----------\t------------")
	}
	lines := []string{}
	for _, sub := range out.Subnets {
		nameTag := ""
		for _, tag := range sub.Tags {
			if strings.ToLower(aws.StringValue(tag.Key)) == "name" {
				nameTag = aws.StringValue(tag.Value)
				break
			}
		}
		avpc, ok := vpcList[aws.StringValue(sub.VpcId)]
		if !ok {
			avpc = new(vpc)
		} else {
			if !inslice.HasString(foundVPCs, aws.StringValue(sub.VpcId)) {
				foundVPCs = append(foundVPCs, aws.StringValue(sub.VpcId))
			}
		}
		autoIP := "no (enable to use with aerolab)"
		if aws.BoolValue(sub.MapPublicIpOnLaunch) {
			autoIP = "yes (ok)"
		}
		ij = append(ij, inventorySubnetAWS{
			VpcId:            aws.StringValue(sub.VpcId),
			VpcName:          avpc.name,
			VpcCidr:          avpc.cidr,
			AvailabilityZone: aws.StringValue(sub.AvailabilityZone),
			SubnetId:         aws.StringValue(sub.SubnetId),
			SubnetCidr:       aws.StringValue(sub.CidrBlock),
			IsAzDefault:      aws.BoolValue(sub.DefaultForAz),
			SubnetName:       nameTag,
			AutoPublicIP:     aws.BoolValue(sub.MapPublicIpOnLaunch),
		})
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%t\t%s\t%s\n", aws.StringValue(sub.VpcId), avpc.name, avpc.cidr, aws.StringValue(sub.AvailabilityZone), aws.StringValue(sub.SubnetId), aws.StringValue(sub.CidrBlock), aws.BoolValue(sub.DefaultForAz), nameTag, autoIP))
	}
	if stdout {
		sort.Strings(lines)
		for _, line := range lines {
			fmt.Fprint(w, line)
		}
	}
	for _, foundVPC := range foundVPCs {
		delete(vpcList, foundVPC)
	}
	for id, vpc := range vpcList {
		ij = append(ij, inventorySubnetAWS{
			VpcId:   id,
			VpcName: vpc.name,
			VpcCidr: vpc.cidr,
		})
		if stdout {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", id, vpc.name, vpc.cidr, "", "", "", "", "", "")
		}
	}
	if stdout {
		w.Flush()
	}
	return ij, nil
}

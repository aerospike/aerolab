package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/bestmethod/inslice"
)

func (d *backendAws) resolveVPC() (*ec2.DescribeVpcsOutput, error) {
	return d.resolveVPCdo(true)
}

func (d *backendAws) resolveVPCdo(create bool) (*ec2.DescribeVpcsOutput, error) {
	out, err := d.ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
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
				&ec2.Filter{
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
				&ec2.Filter{
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

func (d *backendAws) resolveSecGroupAndSubnet(secGroupID string, subnetID string, printID bool) (secGroup string, subnet string, err error) {
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
			&ec2.Filter{
				Name:   aws.String("default-for-az"),
				Values: aws.StringSlice([]string{"true"}),
			},
			&ec2.Filter{
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
		groupName := "AeroLabServer"
		if d.client {
			groupName = "AeroLabClient"
		}
		groupName = groupName + "-" + strings.TrimPrefix(vpc, "vpc-")
		var out *ec2.DescribeSecurityGroupsOutput
		var err error
		if vpc == "" {
			out, err = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
				GroupNames: aws.StringSlice([]string{groupName}),
			})
		} else {
			out, err = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
				Filters: []*ec2.Filter{
					&ec2.Filter{
						Name:   aws.String("vpc-id"),
						Values: aws.StringSlice([]string{vpc}),
					},
					&ec2.Filter{
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
			log.Print("Managed Security groups not found in VPC for given subnet, creating AeroLabServer and AeroLabClient")
			secGroup, err = d.createSecGroups(vpc)
			if err != nil {
				return "", "", fmt.Errorf("could create security groups: %s", err)
			}
			log.Print("WARN: Created unrestricted security group for aerolab clients; to lock these down, use: aerolab config aws lock-security-groups")
		} else {
			secGroup = aws.StringValue(out.SecurityGroups[0].GroupId)
			log.Printf("Using security group ID %s name %s", secGroup, groupName)
		}
	}

	return
}

func (d *backendAws) createSecGroups(vpc string) (secGroup string, err error) {
	var secGroupIds []string
	groupNames := []string{"AeroLabServer-" + strings.TrimPrefix(vpc, "vpc-"), "AeroLabClient-" + strings.TrimPrefix(vpc, "vpc-")}

	// create groups
	for i, groupName := range groupNames {
		out, err := d.ec2svc.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
			Description: aws.String(groupName),
			GroupName:   aws.String(groupName),
			VpcId:       aws.String(vpc),
		})
		if err != nil {
			return "", fmt.Errorf("could not create default server security group for AeroLab in vpc %s: %s", vpc, err)
		}
		if i == 0 && d.server {
			secGroup = aws.StringValue(out.GroupId)
		} else if i == 1 && d.client {
			secGroup = aws.StringValue(out.GroupId)
		}
		secGroupIds = append(secGroupIds, aws.StringValue(out.GroupId))
		err = d.ec2svc.WaitUntilSecurityGroupExists(&ec2.DescribeSecurityGroupsInput{
			GroupIds: []*string{out.GroupId},
		})
		if err != nil {
			d.deleteSecGroups(vpc)
			return "", fmt.Errorf("an error occurred while waiting for security group to exist after creation for AeroLab in vpc %s: %s", vpc, err)
		}
	}

	// add ingress rule for inter-comms
	for _, groupId := range secGroupIds {
		_, err := d.ec2svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: aws.String(groupId),
			IpPermissions: []*ec2.IpPermission{
				&ec2.IpPermission{
					IpProtocol: aws.String("-1"),
					FromPort:   aws.Int64(-1),
					ToPort:     aws.Int64(-1),
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						&ec2.UserIdGroupPair{
							Description: aws.String("serverGroup"),
							GroupId:     aws.String(secGroupIds[0]),
							VpcId:       aws.String(vpc),
						},
						&ec2.UserIdGroupPair{
							Description: aws.String("clientGroup"),
							GroupId:     aws.String(secGroupIds[1]),
							VpcId:       aws.String(vpc),
						},
					},
				},
			},
		})
		if err != nil {
			d.deleteSecGroups(vpc)
			return "", fmt.Errorf("an error occurred while adding ingress intercomms for security group to exist after creation for AeroLab in vpc %s: %s", vpc, err)
		}
	}

	// add ingress rule for port 22
	for _, groupId := range secGroupIds {
		_, err := d.ec2svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: aws.String(groupId),
			IpPermissions: []*ec2.IpPermission{
				&ec2.IpPermission{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(22),
					ToPort:     aws.Int64(22),
					IpRanges: []*ec2.IpRange{
						&ec2.IpRange{
							CidrIp:      aws.String("0.0.0.0/0"),
							Description: aws.String("ssh from anywhere"),
						},
					},
				},
			},
		})
		if err != nil {
			d.deleteSecGroups(vpc)
			return "", fmt.Errorf("an error occurred while adding ingress port 22 for security group to exist after creation for AeroLab in vpc %s: %s", vpc, err)
		}
	}

	// ingress rule for client for special ports (grafana, jupyter, vscode)
	groupId := secGroupIds[1]
	for _, port := range []int64{3000, 8080, 8888, 9200} {
		_, err := d.ec2svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: aws.String(groupId),
			IpPermissions: []*ec2.IpPermission{
				&ec2.IpPermission{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(port),
					ToPort:     aws.Int64(port),
					IpRanges: []*ec2.IpRange{
						&ec2.IpRange{
							CidrIp:      aws.String("0.0.0.0/0"),
							Description: aws.String("allow " + strconv.Itoa(int(port)) + " from anywhere"),
						},
					},
				},
			},
		})
		if err != nil {
			d.deleteSecGroups(vpc)
			return "", fmt.Errorf("an error occurred while adding ingress port 22 for security group to exist after creation for AeroLab in vpc %s: %s", vpc, err)
		}
	}
	return secGroup, nil
}

func (d *backendAws) deleteSecGroups(vpc string) error {
	// prerequisite - remove dependencies between groups if such exist
	var group1, group2 *ec2.DescribeSecurityGroupsOutput
	var err1, err2 error
	if vpc == "" {
		group1, err1 = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
			GroupNames: aws.StringSlice([]string{"AeroLabServer-" + strings.TrimPrefix(vpc, "vpc-")}),
		})
		group2, err2 = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
			GroupNames: aws.StringSlice([]string{"AeroLabClient-" + strings.TrimPrefix(vpc, "vpc-")}),
		})
	} else {
		group1, err1 = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
			Filters: []*ec2.Filter{
				&ec2.Filter{
					Name:   aws.String("vpc-id"),
					Values: aws.StringSlice([]string{vpc}),
				},
				&ec2.Filter{
					Name:   aws.String("group-name"),
					Values: aws.StringSlice([]string{"AeroLabServer-" + strings.TrimPrefix(vpc, "vpc-")}),
				},
			},
		})
		group2, err2 = d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
			Filters: []*ec2.Filter{
				&ec2.Filter{
					Name:   aws.String("vpc-id"),
					Values: aws.StringSlice([]string{vpc}),
				},
				&ec2.Filter{
					Name:   aws.String("group-name"),
					Values: aws.StringSlice([]string{"AeroLabClient-" + strings.TrimPrefix(vpc, "vpc-")}),
				},
			},
		})
	}
	if err1 == nil && err2 == nil && len(group1.SecurityGroups) > 0 && len(group2.SecurityGroups) > 0 {
		_, err := d.ec2svc.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
			GroupId: group1.SecurityGroups[0].GroupId,
			IpPermissions: []*ec2.IpPermission{
				&ec2.IpPermission{
					IpProtocol: aws.String("-1"),
					FromPort:   aws.Int64(-1),
					ToPort:     aws.Int64(-1),
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						&ec2.UserIdGroupPair{
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
				&ec2.IpPermission{
					IpProtocol: aws.String("-1"),
					FromPort:   aws.Int64(-1),
					ToPort:     aws.Int64(-1),
					UserIdGroupPairs: []*ec2.UserIdGroupPair{
						&ec2.UserIdGroupPair{
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
	} else {
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
	return nerr
}

func (d *backendAws) DeleteSecurityGroups(vpc string) error {
	if vpc == "" {
		out, err := d.ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				&ec2.Filter{
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
	return d.deleteSecGroups(vpc)
}

func (d *backendAws) LockSecurityGroups(ip string, lockSSH bool, vpc string) error {
	portList := []int64{3000, 8080, 8888, 9200}
	if lockSSH {
		portList = append(portList, 22)
	}
	// if VPC is "", find default-vpc
	if vpc == "" {
		out, err := d.ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				&ec2.Filter{
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
	err := d.lockSecurityGroups(ip, portList, "AeroLabClient-"+strings.TrimPrefix(vpc, "vpc-"), vpc)
	if err != nil {
		return err
	}
	if lockSSH {
		return d.lockSecurityGroups(ip, []int64{22}, "AeroLabServer-"+strings.TrimPrefix(vpc, "vpc-"), vpc)
	}
	return nil
}

func (d *backendAws) lockSecurityGroups(ip string, portList []int64, secGroupName string, vpc string) error {
	var sgi *ec2.DescribeSecurityGroupsInput
	if vpc == "" {
		sgi = &ec2.DescribeSecurityGroupsInput{
			GroupNames: aws.StringSlice([]string{secGroupName}),
		}
	} else {
		sgi = &ec2.DescribeSecurityGroupsInput{
			Filters: []*ec2.Filter{
				&ec2.Filter{
					Name:   aws.String("vpc-id"),
					Values: aws.StringSlice([]string{vpc}),
				},
				&ec2.Filter{
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

	if ip == "discover-caller-ip" {
		ip = getip2()
	}
	if !strings.Contains(ip, "/") {
		ip = ip + "/32"
	}
	for _, port := range portList {
		_, err := d.ec2svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: group.GroupId,
			IpPermissions: []*ec2.IpPermission{
				&ec2.IpPermission{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int64(port),
					ToPort:     aws.Int64(port),
					IpRanges: []*ec2.IpRange{
						&ec2.IpRange{
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

func (d *backendAws) CreateSecurityGroups(vpc string) error {
	if vpc == "" {
		out, err := d.ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				&ec2.Filter{
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
	_, err := d.createSecGroups(vpc)
	return err
}

func (d *backendAws) ListSecurityGroups() error {
	out, err := d.ec2svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{})
	if err != nil {
		return err
	}
	var output []string
	for _, sg := range out.SecurityGroups {
		if !strings.HasPrefix(aws.StringValue(sg.GroupName), "AeroLabServer-") && !strings.HasPrefix(aws.StringValue(sg.GroupName), "AeroLabClient-") {
			continue
		}
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
	return nil
}

type vpc struct {
	cidr string
	name string
}

func (d *backendAws) ListSubnets() error {
	out, err := d.ec2svc.DescribeSubnets(&ec2.DescribeSubnetsInput{})
	if err != nil {
		return err
	}
	outv, err := d.ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{})
	if err != nil {
		return err
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
	w := tabwriter.NewWriter(os.Stdout, 1, 1, 4, ' ', 0)
	fmt.Fprintln(w, "VPC_ID\tVPC_Name\tVPC_Cidr\tAvailZone\tSubnet_ID\tSubnet_CIDR\tAZ-Default\tSubnet_Name\tAutoPublicIP")
	fmt.Fprintln(w, "------\t--------\t--------\t---------\t---------\t-----------\t----------\t-----------\t------------")
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
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%t\t%s\t%s\n", aws.StringValue(sub.VpcId), avpc.name, avpc.cidr, aws.StringValue(sub.AvailabilityZone), aws.StringValue(sub.SubnetId), aws.StringValue(sub.CidrBlock), aws.BoolValue(sub.DefaultForAz), nameTag, autoIP))
	}
	sort.Strings(lines)
	for _, line := range lines {
		fmt.Fprint(w, line)
	}

	for _, foundVPC := range foundVPCs {
		delete(vpcList, foundVPC)
	}
	for id, vpc := range vpcList {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", id, vpc.name, vpc.cidr, "", "", "", "", "", "")
	}
	w.Flush()
	return nil
}

package baws

import (
	"context"
	"errors"
	"maps"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/lithammer/shortuuid"
)

func (s *b) GetFirewalls(networks backends.NetworkList) (backends.FirewallList, error) {
	log := s.log.WithPrefix("GetFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backends.FirewallList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	wg.Add(len(zones))
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s owned: start", zone)
			defer log.Detail("zone=%s owned: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			listFilters := []types.Filter{
				{
					Name:   aws.String("tag-key"),
					Values: []string{TAG_AEROLAB_VERSION},
				},
			}
			if !s.listAllProjects {
				listFilters = append(listFilters, types.Filter{
					Name:   aws.String("tag:" + TAG_AEROLAB_PROJECT),
					Values: []string{s.project},
				})
			}
			paginator := ec2.NewDescribeSecurityGroupsPaginator(cli, &ec2.DescribeSecurityGroupsInput{
				Filters: listFilters,
			})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				for _, sec := range out.SecurityGroups {
					tags := make(map[string]string)
					for _, t := range sec.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}
					// vpcId - match with network
					nets := networks.WithNetID(aws.ToString(sec.VpcId))
					var net *backends.Network
					if nets.Count() > 0 {
						net = nets.Describe()[0]
					} else {
						net = nil
					}
					// make ports from IpPermissions
					ports := backends.PortsOut{}
					for _, perm := range sec.IpPermissions {
						prot := backends.ProtocolAll
						switch aws.ToString(perm.IpProtocol) {
						case "tcp":
							prot = backends.ProtocolTCP
						case "udp":
							prot = backends.ProtocolUDP
						case "icmp":
							prot = backends.ProtocolICMP
						}
						for _, ipRange := range perm.IpRanges {
							permx := perm
							permx.IpRanges = []types.IpRange{ipRange}
							permx.UserIdGroupPairs = nil
							ports = append(ports, &backends.PortOut{
								Port: backends.Port{
									FromPort:   int(aws.ToInt32(perm.FromPort)),
									ToPort:     int(aws.ToInt32(perm.ToPort)),
									SourceCidr: aws.ToString(ipRange.CidrIp),
									SourceId:   "",
									Protocol:   prot,
								},
								BackendSpecific: permx,
							})
						}
						for _, srcGrp := range perm.UserIdGroupPairs {
							permx := perm
							permx.IpRanges = nil
							permx.UserIdGroupPairs = []types.UserIdGroupPair{srcGrp}
							ports = append(ports, &backends.PortOut{
								Port: backends.Port{
									FromPort:   int(aws.ToInt32(perm.FromPort)),
									ToPort:     int(aws.ToInt32(perm.ToPort)),
									SourceCidr: "",
									SourceId:   aws.ToString(srcGrp.GroupId),
									Protocol:   prot,
								},
								BackendSpecific: permx,
							})
						}
					}
					ilock.Lock()
					i = append(i, &backends.Firewall{
						BackendType: backends.BackendTypeAWS,
						Name:        aws.ToString(sec.GroupName),
						FirewallID:  aws.ToString(sec.GroupId),
						Description: aws.ToString(sec.Description),
						ZoneName:    zone,
						ZoneID:      zone,
						Owner:       tags[TAG_OWNER],
						Tags:        tags,
						Ports:       ports,
						Network:     net,
					})
					ilock.Unlock()
				}
			}
		}(zone)
	}
	wg.Wait()
	if errs == nil {
		s.firewalls = i
	}
	return i, errs
}

func (s *b) FirewallsUpdate(fw backends.FirewallList, ports backends.PortsIn, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsUpdate: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(fw) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall)
	fwIds := make(map[string]backends.FirewallList)
	for _, firewall := range fw {
		firewall := firewall
		if _, ok := fwIds[firewall.ZoneID]; !ok {
			fwIds[firewall.ZoneID] = backends.FirewallList{}
		}
		fwIds[firewall.ZoneID] = append(fwIds[firewall.ZoneID], firewall)
	}
	wg := new(sync.WaitGroup)
	var errs error
	for zone, ids := range fwIds {
		wg.Add(1)
		go func(zone string, ids backends.FirewallList) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			err := func() error {
				cli, err := getEc2Client(s.credentials, &zone)
				if err != nil {
					return err
				}
				for _, id := range ids {
					err := s.firewallHandleUpdate(cli, id, ports)
					if err != nil {
						return err
					}
				}
				return nil
			}()
			if err != nil {
				errs = errors.Join(errs, err)
			}
		}(zone, ids)
	}
	wg.Wait()
	return errs
}

func (s *b) firewallHandleUpdate(cli *ec2.Client, fw *backends.Firewall, ports backends.PortsIn) error {
	deleteRules := &ec2.RevokeSecurityGroupIngressInput{
		GroupId: aws.String(fw.FirewallID),
	}
	addRules := &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(fw.FirewallID),
	}
	for _, port := range ports {
		switch port.Action {
		case backends.PortActionAdd:
			if port.SourceCidr != "" {
				addRules.IpPermissions = append(addRules.IpPermissions, types.IpPermission{
					IpRanges: []types.IpRange{
						{
							CidrIp: aws.String(port.SourceCidr),
						},
					},
					FromPort:   aws.Int32(int32(port.FromPort)),
					ToPort:     aws.Int32(int32(port.ToPort)),
					IpProtocol: aws.String(port.Protocol),
				})
			}
			if port.SourceId != "" {
				addRules.IpPermissions = append(addRules.IpPermissions, types.IpPermission{
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId: aws.String(port.SourceId),
						},
					},
					FromPort:   aws.Int32(int32(port.FromPort)),
					ToPort:     aws.Int32(int32(port.ToPort)),
					IpProtocol: aws.String(port.Protocol),
				})
			}
		case backends.PortActionDelete:
			for _, fwport := range fw.Ports {
				// if ToPort and FromPort are not specified, we pass this check (all ports)
				if port.ToPort != 0 && fwport.ToPort != port.ToPort {
					continue
				}
				if port.FromPort != 0 && fwport.FromPort != port.FromPort {
					continue
				}
				// if protocol is ProtocolAll, we pass this check (all match)
				if port.Protocol != backends.ProtocolAll && port.Protocol != fwport.Protocol {
					continue
				}
				// if sourceCidr is not specified, we pass this check (all match)
				if port.SourceCidr != "" && port.SourceCidr != fwport.SourceCidr {
					continue
				}
				// if sourceId is not specified, we pass this check (all match)
				if port.SourceId != "" && port.SourceId != fwport.SourceId {
					continue
				}
				// all checks passed, rule match
				deleteRules.IpPermissions = append(deleteRules.IpPermissions, fwport.BackendSpecific.(types.IpPermission))
			}
		default:
			return errors.New("action on port not specified")
		}
	}
	// first run revoke to delete any old rules
	if len(deleteRules.IpPermissions) > 0 {
		_, err := cli.RevokeSecurityGroupIngress(context.TODO(), deleteRules)
		if err != nil {
			return err
		}
	}
	// now run authorize to add new rules
	if len(addRules.IpPermissions) > 0 {
		_, err := cli.AuthorizeSecurityGroupIngress(context.TODO(), addRules)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) FirewallsDelete(fw backends.FirewallList, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsDelete: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(fw) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall)
	fwIds := make(map[string][]string)
	for _, firewall := range fw {
		if _, ok := fwIds[firewall.ZoneID]; !ok {
			fwIds[firewall.ZoneID] = []string{}
		}
		fwIds[firewall.ZoneID] = append(fwIds[firewall.ZoneID], firewall.FirewallID)
	}
	wg := new(sync.WaitGroup)
	var errs error
	for zone, ids := range fwIds {
		wg.Add(1)
		go func(zone string, ids []string) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			err := func() error {
				cli, err := getEc2Client(s.credentials, &zone)
				if err != nil {
					return err
				}
				for _, id := range ids {
					_, err = cli.DeleteSecurityGroup(context.TODO(), &ec2.DeleteSecurityGroupInput{
						GroupId: aws.String(id),
					})
					if err != nil {
						return err
					}
				}
				return nil
			}()
			if err != nil {
				errs = errors.Join(errs, err)
			}
		}(zone, ids)
	}
	wg.Wait()
	return errs
}

func (s *b) FirewallsAddTags(fw backends.FirewallList, tags map[string]string, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(fw) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall)
	fwIds := make(map[string][]string)
	for _, firewall := range fw {
		if _, ok := fwIds[firewall.ZoneID]; !ok {
			fwIds[firewall.ZoneID] = []string{}
		}
		fwIds[firewall.ZoneID] = append(fwIds[firewall.ZoneID], firewall.FirewallID)
	}
	tagsOut := []types.Tag{}
	for k, v := range tags {
		tagsOut = append(tagsOut, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	for zone, ids := range fwIds {
		log.Detail("zone=%s start", zone)
		defer log.Detail("zone=%s end", zone)
		cli, err := getEc2Client(s.credentials, &zone)
		if err != nil {
			return err
		}
		_, err = cli.CreateTags(context.TODO(), &ec2.CreateTagsInput{
			Resources: ids,
			Tags:      tagsOut,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) FirewallsRemoveTags(fw backends.FirewallList, tagKeys []string, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(fw) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall)
	fwIds := make(map[string][]string)
	for _, firewall := range fw {
		if _, ok := fwIds[firewall.ZoneID]; !ok {
			fwIds[firewall.ZoneID] = []string{}
		}
		fwIds[firewall.ZoneID] = append(fwIds[firewall.ZoneID], firewall.FirewallID)
	}
	tagsOut := []types.Tag{}
	for _, k := range tagKeys {
		tagsOut = append(tagsOut, types.Tag{
			Key: aws.String(k),
		})
	}
	for zone, ids := range fwIds {
		log.Detail("zone=%s start", zone)
		defer log.Detail("zone=%s end", zone)
		cli, err := getEc2Client(s.credentials, &zone)
		if err != nil {
			return err
		}
		_, err = cli.DeleteTags(context.TODO(), &ec2.DeleteTagsInput{
			Resources: ids,
			Tags:      tagsOut,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) CreateFirewall(input *backends.CreateFirewallInput, waitDur time.Duration) (output *backends.CreateFirewallOutput, err error) {
	log := s.log.WithPrefix("CreateFirewall: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// generate all tags in tags variable
	tags := make(map[string]string)
	maps.Copy(tags, input.Tags)
	tags[TAG_OWNER] = input.Owner
	tags[TAG_AEROLAB_PROJECT] = s.project
	tags[TAG_AEROLAB_VERSION] = s.aerolabVersion
	// create PortsOut
	portsOut := backends.PortsOut{}
	for _, port := range input.Ports {
		if port.SourceCidr != "" {
			portsOut = append(portsOut, &backends.PortOut{
				Port: *port,
				BackendSpecific: types.IpPermission{
					FromPort:   aws.Int32(int32(port.FromPort)),
					IpProtocol: aws.String(port.Protocol),
					ToPort:     aws.Int32(int32(port.ToPort)),
					IpRanges: []types.IpRange{
						{
							CidrIp: aws.String(port.SourceCidr),
						},
					},
				},
			})
		}
		if port.SourceId != "" {
			portsOut = append(portsOut, &backends.PortOut{
				Port: *port,
				BackendSpecific: types.IpPermission{
					FromPort:   aws.Int32(int32(port.FromPort)),
					IpProtocol: aws.String(port.Protocol),
					ToPort:     aws.Int32(int32(port.ToPort)),
					UserIdGroupPairs: []types.UserIdGroupPair{
						{
							GroupId: aws.String(port.SourceId),
						},
					},
				},
			})
		}
	}
	// create output var
	output = &backends.CreateFirewallOutput{
		Firewall: &backends.Firewall{
			BackendType:     input.BackendType,
			Name:            input.Name,
			Description:     input.Description,
			FirewallID:      "", // to be filled after successful creation
			ZoneName:        input.Network.ZoneName,
			ZoneID:          input.Network.ZoneID,
			Owner:           input.Owner,
			Tags:            tags,
			Ports:           portsOut,
			Network:         input.Network,
			BackendSpecific: nil, // unused
		},
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall)
	cli, err := getEc2Client(s.credentials, &input.Network.ZoneName)
	if err != nil {
		return output, err
	}
	log.Detail("CreateSecurityGroup")
	out, err := cli.CreateSecurityGroup(context.TODO(), &ec2.CreateSecurityGroupInput{
		Description: aws.String(input.Description),
		GroupName:   aws.String(input.Name),
		VpcId:       aws.String(input.Network.NetworkId),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeSecurityGroup,
				Tags: func(tags map[string]string) []types.Tag {
					out := make([]types.Tag, 0, len(tags))
					for k, v := range tags {
						out = append(out, types.Tag{
							Key:   aws.String(k),
							Value: aws.String(v),
						})
					}
					return out
				}(tags),
			},
		},
	})
	if err != nil {
		return output, err
	}
	output.Firewall.FirewallID = aws.ToString(out.GroupId)
	if len(input.Ports) > 0 {
		if waitDur == 0 {
			waitDur = 1 * time.Minute
		}
	}

	if waitDur == 0 && len(input.Ports) > 0 {
		waitDur = 1 * time.Minute
	}

	if waitDur > 0 {
		log.Detail("WaitSecurityGroup")
		err := ec2.NewSecurityGroupExistsWaiter(cli, func(o *ec2.SecurityGroupExistsWaiterOptions) {
			o.MinDelay = 1 * time.Second
			o.MaxDelay = 5 * time.Second
		}).Wait(context.TODO(), &ec2.DescribeSecurityGroupsInput{
			GroupIds: []string{output.Firewall.FirewallID},
		}, waitDur)
		if err != nil {
			return output, err
		}
	}

	if len(input.Ports) > 0 {
		pin := backends.PortsIn{}
		for _, port := range input.Ports {
			sid := port.SourceId
			if sid == "self" {
				sid = output.Firewall.FirewallID
			}
			pin = append(pin, &backends.PortIn{
				Port: backends.Port{
					FromPort:   port.FromPort,
					ToPort:     port.ToPort,
					SourceCidr: port.SourceCidr,
					SourceId:   sid,
					Protocol:   port.Protocol,
				},
				Action: backends.PortActionAdd,
			})
		}
		log.Detail("FirewallsUpdate")
		err = s.FirewallsUpdate(backends.FirewallList{output.Firewall}, pin, waitDur)
		if err != nil {
			return output, err
		}
	}
	return output, nil
}

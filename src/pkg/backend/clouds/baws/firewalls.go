package baws

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/lithammer/shortuuid"
)

// TODO CreateFirewalls call

func (s *b) GetFirewalls(networks backend.NetworkList) (backend.FirewallList, error) {
	log := s.log.WithPrefix("GetFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backend.FirewallList
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
			paginator := ec2.NewDescribeSecurityGroupsPaginator(cli, &ec2.DescribeSecurityGroupsInput{
				Filters: []types.Filter{
					{
						Name:   aws.String("tag-key"),
						Values: []string{TAG_AEROLAB_VERSION},
					}, {
						Name:   aws.String("tag:" + TAG_AEROLAB_PROJECT),
						Values: []string{s.project},
					},
				},
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
					var net *backend.Network
					if nets.Count() > 0 {
						net = nets.Describe()[0]
					} else {
						net = nil
					}
					// make ports from IpPermissions
					ports := backend.PortsOut{}
					for _, perm := range sec.IpPermissions {
						prot := backend.ProtocolAll
						switch aws.ToString(perm.IpProtocol) {
						case "tcp":
							prot = backend.ProtocolTCP
						case "udp":
							prot = backend.ProtocolUDP
						case "icmp":
							prot = backend.ProtocolICMP
						}
						for _, ipRange := range perm.IpRanges {
							permx := perm
							permx.IpRanges = []types.IpRange{ipRange}
							permx.UserIdGroupPairs = nil
							ports = append(ports, &backend.PortOut{
								Port: backend.Port{
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
							ports = append(ports, &backend.PortOut{
								Port: backend.Port{
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
					i = append(i, &backend.Firewall{
						BackendType: backend.BackendTypeAWS,
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
	return i, errs
}

func (s *b) FirewallsUpdate(fw backend.FirewallList, ports backend.PortsIn, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsUpdate: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(fw) == 0 {
		return nil
	}
	fwIds := make(map[string]backend.FirewallList)
	for _, firewall := range fw {
		firewall := firewall
		if _, ok := fwIds[firewall.ZoneID]; !ok {
			fwIds[firewall.ZoneID] = backend.FirewallList{}
		}
		fwIds[firewall.ZoneID] = append(fwIds[firewall.ZoneID], firewall)
	}
	wg := new(sync.WaitGroup)
	var errs error
	for zone, ids := range fwIds {
		wg.Add(1)
		go func(zone string, ids backend.FirewallList) {
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

func (s *b) firewallHandleUpdate(cli *ec2.Client, fw *backend.Firewall, ports backend.PortsIn) error {
	deleteRules := &ec2.RevokeSecurityGroupIngressInput{
		GroupId: aws.String(fw.FirewallID),
	}
	addRules := &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(fw.FirewallID),
	}
	for _, port := range ports {
		switch port.Action {
		case backend.PortActionAdd:
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
		case backend.PortActionDelete:
			for _, fwport := range fw.Ports {
				// if ToPort and FromPort are not specified, we pass this check (all ports)
				if port.ToPort != 0 && fwport.ToPort != port.ToPort {
					continue
				}
				if port.FromPort != 0 && fwport.FromPort != port.FromPort {
					continue
				}
				// if protocol is ProtocolAll, we pass this check (all match)
				if port.Protocol != backend.ProtocolAll && port.Protocol != fwport.Protocol {
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
	_, err := cli.RevokeSecurityGroupIngress(context.TODO(), deleteRules)
	if err != nil {
		return err
	}
	// now run authorize to add new rules
	_, err = cli.AuthorizeSecurityGroupIngress(context.TODO(), addRules)
	if err != nil {
		return err
	}
	return nil
}

func (s *b) FirewallsDelete(fw backend.FirewallList, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsDelete: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(fw) == 0 {
		return nil
	}
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

func (s *b) FirewallsAddTags(fw backend.FirewallList, tags map[string]string, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(fw) == 0 {
		return nil
	}
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

func (s *b) FirewallsRemoveTags(fw backend.FirewallList, tagKeys []string, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(fw) == 0 {
		return nil
	}
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

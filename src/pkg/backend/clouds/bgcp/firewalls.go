package bgcp

import (
	"context"
	"strconv"
	"strings"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
)

type FirewallDetail struct {
	Metadata *metadata `json:"metadata" yaml:"metadata"`
}

type PortDetail struct {
	TargetTags        []string `json:"target_tags" yaml:"target_tags"`
	DestinationRanges []string `json:"destination_ranges" yaml:"destination_ranges"`
}

func (s *b) GetFirewalls(networks backends.NetworkList) (backends.FirewallList, error) {
	log := s.log.WithPrefix("GetFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backends.FirewallList
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	log.Detail("owned: start")
	defer log.Detail("owned: end")
	ctx := context.Background()
	client, err := compute.NewFirewallsRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()
	it := client.List(ctx, &computepb.ListFirewallsRequest{
		Project: s.credentials.Project,
		Filter:  proto.String(LABEL_FILTER_AEROLAB),
	})
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		m := &metadata{}
		err = m.decodeFromDescriptionField(stringValue(pair.Description))
		if err != nil {
			return nil, err
		}
		if !s.listAllProjects {
			if m.AerolabProject != s.project {
				continue
			}
		}
		network := networks.WithName(getValueFromURL(pair.GetNetwork()))
		if network.Count() == 0 {
			continue
		}
		net := network.Describe()[0]
		// get port mapping
		ports := backends.PortsOut{}
		for _, perm := range pair.GetAllowed() {
			prot := backends.ProtocolAll
			switch perm.GetIPProtocol() {
			case "tcp":
				prot = backends.ProtocolTCP
			case "udp":
				prot = backends.ProtocolUDP
			case "icmp":
				prot = backends.ProtocolICMP
			default:
				continue
			}
			for _, ipRange := range perm.GetPorts() {
				fromPort := 0
				toPort := 0
				if strings.Contains(ipRange, "-") {
					parts := strings.Split(ipRange, "-")
					fromPort, _ = strconv.Atoi(parts[0])
					toPort, _ = strconv.Atoi(parts[1])
				} else {
					fromPort, _ = strconv.Atoi(ipRange)
					toPort = fromPort
				}
				for _, srcIP := range pair.GetSourceRanges() {
					ports = append(ports, &backends.PortOut{
						Port: backends.Port{
							FromPort:   fromPort,
							ToPort:     toPort,
							SourceCidr: srcIP,
							SourceId:   "",
							Protocol:   prot,
						},
						BackendSpecific: &PortDetail{
							TargetTags:        pair.GetTargetTags(),
							DestinationRanges: pair.GetDestinationRanges(),
						},
					})
				}
				for _, srcGRP := range pair.GetSourceTags() {
					ports = append(ports, &backends.PortOut{
						Port: backends.Port{
							FromPort:   fromPort,
							ToPort:     toPort,
							SourceCidr: "",
							SourceId:   srcGRP,
							Protocol:   prot,
						},
						BackendSpecific: &PortDetail{
							TargetTags:        pair.GetTargetTags(),
							DestinationRanges: pair.GetDestinationRanges(),
						},
					})
				}
			}
		}
		// fill list
		i = append(i, &backends.Firewall{
			BackendType: backends.BackendTypeGCP,
			Name:        pair.GetName(),
			FirewallID:  pair.GetSelfLink(),
			Description: pair.GetDescription(),
			ZoneName:    "",
			ZoneID:      "",
			Owner:       m.Owner,
			Tags:        m.Custom,
			Ports:       ports,
			Network:     net,
			BackendSpecific: &FirewallDetail{
				Metadata: m,
			},
		})
	}
	s.firewalls = i
	return i, nil
}

func (s *b) FirewallsUpdate(fw backends.FirewallList, ports backends.PortsIn, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsUpdate: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(fw) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall)
	log.Detail("start")
	defer log.Detail("end")
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewFirewallsRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()
	for _, fw := range fw {
		err := s.firewallHandleUpdate(ctx, client, fw, ports)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) firewallHandleUpdate(ctx context.Context, cli *compute.FirewallsClient, fw *backends.Firewall, ports backends.PortsIn) error {
	allow := []*computepb.Allowed{} // protocol and port list
	dstCidr := []string{}           // allow if destination is in this range
	srcCidr := []string{}           // allow if source is in this range
	srcTags := []string{}           // allow if source has this tag
	targetTags := []string{}        // apply this firewall to instances with these tags

NEXTPORT:
	for _, port := range fw.Ports {
		// Skip if this port is marked for deletion
		for _, p := range ports {
			if p.Action == backends.PortActionDelete && p.Protocol == port.Protocol && p.FromPort == port.FromPort && p.ToPort == port.ToPort {
				if (p.SourceId == "" || p.SourceId == port.SourceId) && (p.SourceCidr == "" || p.SourceCidr == port.SourceCidr) {
					continue NEXTPORT
				}
			}
		}
		for _, p := range port.BackendSpecific.(*PortDetail).DestinationRanges {
			dstCidr = append(dstCidr, p)
		}
		if port.SourceCidr != "" {
			srcCidr = append(srcCidr, port.SourceCidr)
		}
		if port.SourceId != "" {
			srcTags = append(srcTags, port.SourceId)
		}
		for _, p := range port.BackendSpecific.(*PortDetail).TargetTags {
			targetTags = append(targetTags, p)
		}
		if port.FromPort == port.ToPort {
			allow = append(allow, &computepb.Allowed{
				IPProtocol: &port.Protocol,
				Ports:      []string{strconv.Itoa(port.FromPort)},
			})
		} else {
			allow = append(allow, &computepb.Allowed{
				IPProtocol: &port.Protocol,
				Ports:      []string{strconv.Itoa(port.FromPort) + "-" + strconv.Itoa(port.ToPort)},
			})
		}
	}
	for _, port := range ports {
		switch port.Action {
		case backends.PortActionAdd:
			allowed := &computepb.Allowed{
				IPProtocol: &port.Protocol,
			}
			if port.FromPort == port.ToPort {
				allowed.Ports = []string{strconv.Itoa(port.FromPort)}
			} else {
				allowed.Ports = []string{strconv.Itoa(port.FromPort) + "-" + strconv.Itoa(port.ToPort)}
			}
			allow = append(allow, allowed)
			if port.SourceCidr != "" {
				srcCidr = append(srcCidr, port.SourceCidr)
			}
			if port.SourceId != "" {
				srcTags = append(srcTags, port.SourceId)
			}
			targetTags = append(targetTags, fw.Name)
		}
	}

	op, err := cli.Update(ctx, &computepb.UpdateFirewallRequest{
		Firewall: fw.Name,
		Project:  s.credentials.Project,
		FirewallResource: &computepb.Firewall{
			Name:              &fw.Name,
			Allowed:           allow,
			DestinationRanges: dstCidr,
			Network:           &fw.Network.NetworkId,
			SourceRanges:      srcCidr,
			SourceTags:        srcTags,
			TargetTags:        targetTags,
		},
	})
	if err != nil {
		return err
	}
	err = op.Wait(ctx)
	if err != nil {
		return err
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

	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewFirewallsRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()
	ops := []*compute.Operation{}
	for _, fw := range fw {
		op, err := client.Delete(ctx, &computepb.DeleteFirewallRequest{
			Firewall: fw.Name,
			Project:  s.credentials.Project,
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	for _, op := range ops {
		err = op.Wait(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) FirewallsAddTags(fw backends.FirewallList, tags map[string]string, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(fw) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall)
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewFirewallsRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()
	ops := []*compute.Operation{}
	for _, fw := range fw {
		m := &metadata{}
		err := deepCopy(fw.BackendSpecific.(*FirewallDetail).Metadata, m)
		if err != nil {
			return err
		}
		for k, v := range tags {
			m.Custom[k] = v
		}
		desc := m.encodeToDescriptionField()
		op, err := client.Update(ctx, &computepb.UpdateFirewallRequest{
			Firewall: fw.Name,
			Project:  s.credentials.Project,
			FirewallResource: &computepb.Firewall{
				Description: &desc,
			},
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	for _, op := range ops {
		err = op.Wait(ctx)
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
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewFirewallsRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return err
	}
	defer client.Close()
	ops := []*compute.Operation{}
	for _, fw := range fw {
		m := &metadata{}
		err := deepCopy(fw.BackendSpecific.(*FirewallDetail).Metadata, m)
		if err != nil {
			return err
		}
		for _, k := range tagKeys {
			delete(m.Custom, k)
		}
		desc := m.encodeToDescriptionField()
		op, err := client.Update(ctx, &computepb.UpdateFirewallRequest{
			Firewall: fw.Name,
			Project:  s.credentials.Project,
			FirewallResource: &computepb.Firewall{
				Description: &desc,
			},
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	for _, op := range ops {
		err = op.Wait(ctx)
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
	tags := &FirewallDetail{
		Metadata: &metadata{
			Owner:          input.Owner,
			AerolabProject: s.project,
			AerolabVersion: s.aerolabVersion,
			Custom:         input.Tags,
		},
	}
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
			FirewallID:      input.Name,
			ZoneName:        input.Network.ZoneName,
			ZoneID:          input.Network.ZoneID,
			Owner:           input.Owner,
			Tags:            tags.Metadata.Custom,
			Ports:           portsOut,
			Network:         input.Network,
			BackendSpecific: tags,
		},
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateFirewall)
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	ctx := context.Background()
	client, err := compute.NewFirewallsRESTClient(ctx, option.WithHTTPClient(cli))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	description := tags.Metadata.encodeToDescriptionField()
	op, err := client.Insert(ctx, &computepb.InsertFirewallRequest{
		Project: s.credentials.Project,
		FirewallResource: &computepb.Firewall{
			Name:        &input.Name,
			Network:     &input.Network.NetworkId,
			Description: &description,
		},
	})
	if err != nil {
		return nil, err
	}
	err = op.Wait(ctx)
	if err != nil {
		return output, err
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

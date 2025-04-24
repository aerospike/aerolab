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
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type FirewallDetail struct {
	Resource *computepb.Firewall `json:"resource" yaml:"resource"`
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
	})
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		m, err := decodeFromDescriptionField(stringValue(pair.Description))
		if err != nil {
			log.Detail("failed to decode metadata for firewall %s: %s", pair.Name, err)
			continue
		}
		if !s.listAllProjects {
			if m[TAG_AEROLAB_PROJECT] != s.project && !strings.HasPrefix(pair.GetName(), TAG_FIREWALL_NAME_PREFIX) {
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
			BackendType:     backends.BackendTypeGCP,
			Name:            pair.GetName(),
			FirewallID:      pair.GetSelfLink(),
			Description:     pair.GetDescription(),
			ZoneName:        "",
			ZoneID:          "",
			Owner:           m[TAG_AEROLAB_OWNER],
			Tags:            m,
			Ports:           ports,
			Network:         net,
			BackendSpecific: &FirewallDetail{Resource: pair},
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
		err := s.firewallHandleUpdate(ctx, client, fw, ports, waitDur)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *b) firewallHandleUpdate(ctx context.Context, cli *compute.FirewallsClient, fw *backends.Firewall, ports backends.PortsIn, waitDur time.Duration) error {
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
		dstCidr = append(dstCidr, port.BackendSpecific.(*PortDetail).DestinationRanges...)
		if port.SourceCidr != "" {
			srcCidr = append(srcCidr, port.SourceCidr)
		}
		if port.SourceId != "" {
			srcTags = append(srcTags, port.SourceId)
		}
		targetTags = append(targetTags, port.BackendSpecific.(*PortDetail).TargetTags...)
		prot := port.Protocol
		if prot == backends.ProtocolAll {
			prot = "all"
		}
		if port.FromPort != -1 {
			if port.FromPort == port.ToPort {
				allow = append(allow, &computepb.Allowed{
					IPProtocol: &prot,
					Ports:      []string{strconv.Itoa(port.FromPort)},
				})
			} else {
				allow = append(allow, &computepb.Allowed{
					IPProtocol: &prot,
					Ports:      []string{strconv.Itoa(port.FromPort) + "-" + strconv.Itoa(port.ToPort)},
				})
			}
		} else {
			allow = append(allow, &computepb.Allowed{
				IPProtocol: &prot,
			})
		}
	}
	for _, port := range ports {
		switch port.Action {
		case backends.PortActionAdd:
			prot := port.Protocol
			if prot == backends.ProtocolAll {
				prot = "all"
			}
			allowed := &computepb.Allowed{
				IPProtocol: &prot,
			}
			if port.FromPort != -1 {
				if port.FromPort == port.ToPort {
					allowed.Ports = []string{strconv.Itoa(port.FromPort)}
				} else {
					allowed.Ports = []string{strconv.Itoa(port.FromPort) + "-" + strconv.Itoa(port.ToPort)}
				}
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
	desc := encodeToDescriptionField(fw.Tags)
	op, err := cli.Update(ctx, &computepb.UpdateFirewallRequest{
		Firewall: fw.Name,
		Project:  s.credentials.Project,
		FirewallResource: &computepb.Firewall{
			Name:              &fw.Name,
			Description:       &desc,
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
	if waitDur > 0 {
		ctx, cancel := context.WithTimeout(ctx, waitDur)
		defer cancel()
		err = op.Wait(ctx)
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
	if waitDur > 0 {
		ctx, cancel := context.WithTimeout(ctx, waitDur)
		defer cancel()
		for _, op := range ops {
			err = op.Wait(ctx)
			if err != nil {
				return err
			}
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
		m, err := decodeFromDescriptionField(fw.Description)
		if err != nil {
			return err
		}
		for k, v := range tags {
			m[k] = v
		}
		desc := encodeToDescriptionField(m)
		res := fw.BackendSpecific.(*FirewallDetail).Resource
		res.Description = &desc
		op, err := client.Update(ctx, &computepb.UpdateFirewallRequest{
			Firewall:         fw.Name,
			Project:          s.credentials.Project,
			FirewallResource: res,
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	if waitDur > 0 {
		ctx, cancel := context.WithTimeout(ctx, waitDur)
		defer cancel()
		for _, op := range ops {
			err = op.Wait(ctx)
			if err != nil {
				return err
			}
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
		m, err := decodeFromDescriptionField(fw.Description)
		if err != nil {
			return err
		}
		for _, k := range tagKeys {
			delete(m, k)
		}
		desc := encodeToDescriptionField(m)
		res := fw.BackendSpecific.(*FirewallDetail).Resource
		res.Description = &desc
		op, err := client.Update(ctx, &computepb.UpdateFirewallRequest{
			Firewall:         fw.Name,
			Project:          s.credentials.Project,
			FirewallResource: res,
		})
		if err != nil {
			return err
		}
		ops = append(ops, op)
	}
	if waitDur > 0 {
		ctx, cancel := context.WithTimeout(ctx, waitDur)
		defer cancel()
		for _, op := range ops {
			err = op.Wait(ctx)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *b) CreateFirewall(input *backends.CreateFirewallInput, waitDur time.Duration) (output *backends.CreateFirewallOutput, err error) {
	log := s.log.WithPrefix("CreateFirewall: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// generate all tags in tags variable
	m := make(map[string]string)
	for k, v := range input.Tags {
		m[k] = v
	}
	m[TAG_AEROLAB_OWNER] = input.Owner
	m[TAG_AEROLAB_PROJECT] = s.project
	m[TAG_AEROLAB_VERSION] = s.aerolabVersion
	// create PortsOut
	portsOut := backends.PortsOut{}
	for _, port := range input.Ports {
		if port.SourceCidr != "" {
			portsOut = append(portsOut, &backends.PortOut{
				Port: *port,
				BackendSpecific: &PortDetail{
					TargetTags:        []string{input.Name},
					DestinationRanges: []string{},
				},
			})
		}
		if port.SourceId != "" {
			portsOut = append(portsOut, &backends.PortOut{
				Port: *port,
				BackendSpecific: &PortDetail{
					TargetTags:        []string{input.Name},
					DestinationRanges: []string{},
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
			Tags:            m,
			Ports:           portsOut,
			Network:         input.Network,
			BackendSpecific: &FirewallDetail{},
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

	description := encodeToDescriptionField(m)
	op, err := client.Insert(ctx, &computepb.InsertFirewallRequest{
		Project: s.credentials.Project,
		FirewallResource: &computepb.Firewall{
			Name:        &input.Name,
			Network:     &input.Network.NetworkId,
			Description: &description,
			Allowed: []*computepb.Allowed{
				{
					IPProtocol: aws.String("tcp"),
					Ports:      []string{"22"},
				},
			},
			TargetTags: []string{
				input.Name,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if waitDur > 0 {
		ctx, cancel := context.WithTimeout(ctx, waitDur)
		defer cancel()
		err = op.Wait(ctx)
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

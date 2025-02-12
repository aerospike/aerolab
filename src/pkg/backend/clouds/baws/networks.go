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

func (s *b) GetNetworks() (backend.NetworkList, error) {
	log := s.log.WithPrefix("GetNetworks: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backend.NetworkList
	ilock := new(sync.Mutex)
	var j backend.SubnetList
	jlock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	wg.Add(len(zones) * 2)
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s network: start", zone)
			defer log.Detail("zone=%s network: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			paginator := ec2.NewDescribeVpcsPaginator(cli, &ec2.DescribeVpcsInput{})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				for _, vpc := range out.Vpcs {
					tags := make(map[string]string)
					for _, t := range vpc.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}
					// if aerolab managed, but wrong project, ignore
					if v, ok := tags[TAG_AEROLAB_PROJECT]; ok && v != s.project {
						continue
					}
					managed := false
					if _, ok := tags[TAG_AEROLAB_VERSION]; ok {
						managed = true
					}
					state := backend.NetworkStateUnknown
					switch vpc.State {
					case types.VpcStatePending:
						state = backend.NetworkStateConfiguring
					case types.VpcStateAvailable:
						state = backend.NetworkStateAvailable
					}
					ilock.Lock()
					i = append(i, &backend.Network{
						BackendType:      backend.BackendTypeAWS,
						Name:             tags[TAG_NAME],
						Description:      tags[TAG_DESCRIPTION],
						NetworkId:        aws.ToString(vpc.VpcId),
						Cidr:             aws.ToString(vpc.CidrBlock),
						ZoneName:         zone,
						ZoneID:           zone,
						Owner:            tags[TAG_OWNER],
						Tags:             tags,
						IsDefault:        aws.ToBool(vpc.IsDefault),
						IsAerolabManaged: managed,
						State:            state,
						Subnets:          nil,
						BackendSpecific:  nil,
					})
					ilock.Unlock()
				}
			}
		}(zone)
	}
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s subnet: start", zone)
			defer log.Detail("zone=%s subnet: end", zone)
			cli, err := getEc2Client(s.credentials, &zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			paginator := ec2.NewDescribeSubnetsPaginator(cli, &ec2.DescribeSubnetsInput{})
			for paginator.HasMorePages() {
				out, err := paginator.NextPage(context.TODO())
				if err != nil {
					errs = errors.Join(errs, err)
					return
				}
				for _, subnet := range out.Subnets {
					tags := make(map[string]string)
					for _, t := range subnet.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}
					// if aerolab managed, but wrong project, ignore
					if v, ok := tags[TAG_AEROLAB_PROJECT]; ok && v != s.project {
						continue
					}
					managed := false
					if _, ok := tags[TAG_AEROLAB_VERSION]; ok {
						managed = true
					}
					state := backend.NetworkStateUnknown
					switch subnet.State {
					case types.SubnetStatePending:
						state = backend.NetworkStateConfiguring
					case types.SubnetStateAvailable:
						state = backend.NetworkStateAvailable
					}
					jlock.Lock()
					j = append(j, &backend.Subnet{
						BackendType:      backend.BackendTypeAWS,
						Name:             tags[TAG_NAME],
						Description:      tags[TAG_DESCRIPTION],
						SubnetId:         aws.ToString(subnet.SubnetId),
						Cidr:             aws.ToString(subnet.CidrBlock),
						ZoneName:         zone,
						ZoneID:           zone,
						Owner:            tags[TAG_OWNER],
						Tags:             tags,
						IsDefault:        aws.ToBool(subnet.DefaultForAz),
						IsAerolabManaged: managed,
						State:            state,
						PublicIP:         aws.ToBool(subnet.MapPublicIpOnLaunch),
						NetworkId:        aws.ToString(subnet.VpcId),
						Network:          nil,
						BackendSpecific:  nil,
					})
					jlock.Unlock()
				}
			}
		}(zone)
	}
	wg.Wait()
	// create pointers between subnets and networks
	log.Detail("Merging networks and subnets")
	for _, subnet := range j {
		for _, vpc := range i {
			if vpc.NetworkId != subnet.NetworkId {
				continue
			}
			vpc.Subnets = append(vpc.Subnets, subnet)
			subnet.Network = vpc
			break
		}
	}
	if errs == nil {
		s.networks = i
	}
	return i, errs
}

func (s *b) NetworksDelete(networks backend.NetworkList, waitDur time.Duration) error {
	log := s.log.WithPrefix("NetworksDelete: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(networks) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backend.CacheInvalidateNetwork)
	// first delete subnets
	subnets := backend.SubnetList{}
	for _, network := range networks {
		subnets = append(subnets, network.Subnets...)
	}
	err := s.NetworksDeleteSubnets(subnets, waitDur)
	if err != nil {
		return err
	}
	// delete networks
	list := make(map[string]backend.NetworkList)
	for _, network := range networks {
		network := network
		if _, ok := list[network.ZoneName]; !ok {
			list[network.ZoneName] = backend.NetworkList{}
		}
		list[network.ZoneName] = append(list[network.ZoneName], network)
	}
	wg := new(sync.WaitGroup)
	var errs error
	for zone, net := range list {
		wg.Add(1)
		go func(zone string, net backend.NetworkList) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			err := func() error {
				cli, err := getEc2Client(s.credentials, &zone)
				if err != nil {
					return err
				}
				for _, network := range net {
					_, err = cli.DeleteVpc(context.TODO(), &ec2.DeleteVpcInput{
						VpcId: aws.String(network.NetworkId),
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
		}(zone, net)
	}
	wg.Wait()
	return errs
}

func (s *b) NetworksDeleteSubnets(subnets backend.SubnetList, waitDur time.Duration) error {
	log := s.log.WithPrefix("NetworksDeleteSubnets: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(subnets) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backend.CacheInvalidateNetwork)
	list := make(map[string]backend.SubnetList)
	for _, subnet := range subnets {
		subnet := subnet
		if _, ok := list[subnet.ZoneName]; !ok {
			list[subnet.ZoneName] = backend.SubnetList{}
		}
		list[subnet.ZoneName] = append(list[subnet.ZoneName], subnet)
	}
	wg := new(sync.WaitGroup)
	var errs error
	for zone, sub := range list {
		wg.Add(1)
		go func(zone string, sub backend.SubnetList) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			err := func() error {
				cli, err := getEc2Client(s.credentials, &zone)
				if err != nil {
					return err
				}
				for _, subnet := range sub {
					_, err = cli.DeleteSubnet(context.TODO(), &ec2.DeleteSubnetInput{
						SubnetId: aws.String(subnet.SubnetId),
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
		}(zone, sub)
	}
	wg.Wait()
	return errs
}

func (s *b) NetworksAddTags(networks backend.NetworkList, tags map[string]string) error {
	log := s.log.WithPrefix("NetworksAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(networks) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backend.CacheInvalidateNetwork)
	netIds := make(map[string][]string)
	for _, net := range networks {
		if _, ok := netIds[net.ZoneID]; !ok {
			netIds[net.ZoneID] = []string{}
		}
		netIds[net.ZoneID] = append(netIds[net.ZoneID], net.NetworkId)
	}
	tagsOut := []types.Tag{}
	for k, v := range tags {
		tagsOut = append(tagsOut, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	for zone, ids := range netIds {
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

func (s *b) NetworksRemoveTags(networks backend.NetworkList, tagKeys []string) error {
	log := s.log.WithPrefix("NetworksRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(networks) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backend.CacheInvalidateNetwork)
	netIds := make(map[string][]string)
	for _, net := range networks {
		if _, ok := netIds[net.ZoneID]; !ok {
			netIds[net.ZoneID] = []string{}
		}
		netIds[net.ZoneID] = append(netIds[net.ZoneID], net.NetworkId)
	}
	tagsOut := []types.Tag{}
	for _, k := range tagKeys {
		tagsOut = append(tagsOut, types.Tag{
			Key: aws.String(k),
		})
	}
	for zone, ids := range netIds {
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

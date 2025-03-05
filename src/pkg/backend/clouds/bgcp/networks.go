package bgcp

import (
	"context"
	"errors"
	"sync"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bgcp/connect"
	"github.com/lithammer/shortuuid"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type NetworkDetail struct {
	Subnets []string `json:"subnets" yaml:"subnets"`
}

func (s *b) GetNetworks() (backends.NetworkList, error) {
	log := s.log.WithPrefix("GetNetworks: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backends.NetworkList
	var j backends.SubnetList
	wg := new(sync.WaitGroup)
	cli, err := connect.GetClient(s.credentials, log.WithPrefix("AUTH: "))
	if err != nil {
		return nil, err
	}
	defer cli.CloseIdleConnections()
	wg.Add(2)
	var errs error
	go func() {
		defer wg.Done()
		log.Detail("networks: start")
		defer log.Detail("networks: end")
		ctx := context.Background()
		client, err := compute.NewNetworksRESTClient(ctx, option.WithHTTPClient(cli))
		if err != nil {
			errs = errors.Join(errs, err)
			return
		}
		defer client.Close()
		it := client.List(ctx, &computepb.ListNetworksRequest{
			Project: s.credentials.Project,
		})
		for {
			pair, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			i = append(i, &backends.Network{
				BackendType:      backends.BackendTypeGCP,
				Name:             pair.GetName(),
				Description:      pair.GetDescription(),
				NetworkId:        pair.GetSelfLink(),
				Cidr:             pair.GetIPv4Range(),
				ZoneName:         "",
				ZoneID:           "",
				Owner:            "",
				Tags:             nil,
				IsDefault:        true,
				IsAerolabManaged: false,
				State:            backends.NetworkStateAvailable,
				Subnets:          nil,
				BackendSpecific: &NetworkDetail{
					Subnets: pair.GetSubnetworks(),
				},
			})
		}
	}()
	go func() {
		defer wg.Done()
		log.Detail("subnets: start")
		defer log.Detail("subnets: end")
		ctx := context.Background()
		client, err := compute.NewSubnetworksRESTClient(ctx, option.WithHTTPClient(cli))
		if err != nil {
			errs = errors.Join(errs, err)
			return
		}
		defer client.Close()
		it := client.List(ctx, &computepb.ListSubnetworksRequest{
			Project: s.credentials.Project,
		})
		for {
			pair, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			j = append(j, &backends.Subnet{
				BackendType:      backends.BackendTypeGCP,
				Name:             pair.GetName(),
				Description:      pair.GetDescription(),
				SubnetId:         pair.GetSelfLink(),
				Cidr:             pair.GetIpCidrRange(),
				ZoneName:         getValueFromURL(pair.GetRegion()),
				ZoneID:           pair.GetRegion(),
				Owner:            "",
				Tags:             nil,
				IsDefault:        true,
				IsAerolabManaged: false,
				State:            backends.NetworkStateAvailable,
				PublicIP:         true,
				NetworkId:        pair.GetNetwork(),
				Network:          nil,
				BackendSpecific:  nil,
			})
		}
	}()
	wg.Wait()
	// create pointers between subnets and networks
	log.Detail("Merging networks and subnets")
	for _, subnet := range j {
		for _, vpc := range i {
			if getValueFromURL(vpc.NetworkId) != getValueFromURL(subnet.NetworkId) {
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

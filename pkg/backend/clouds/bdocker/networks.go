package bdocker

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/citrusleaf/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
	"github.com/moby/moby/client"
)

type NetworkDetails struct {
	Scope      string
	Driver     string
	Internal   bool
	Attachable bool
	Ingress    bool
	Options    map[string]string
	Created    time.Time
}

// GetNetworkDetails safely extracts *NetworkDetails from BackendSpecific, initializing it if needed.
// This handles cases where BackendSpecific might be nil, a map (from JSON/YAML deserialization),
// or already the correct type.
func GetNetworkDetails(net *backends.Network) *NetworkDetails {
	if net.BackendSpecific == nil {
		net.BackendSpecific = &NetworkDetails{}
		return net.BackendSpecific.(*NetworkDetails)
	}
	if nd, ok := net.BackendSpecific.(*NetworkDetails); ok {
		return nd
	}
	// If it's a map (from JSON/YAML deserialization), try to convert it
	if m, ok := net.BackendSpecific.(map[string]any); ok {
		jsonBytes, err := json.Marshal(m)
		if err == nil {
			var nd NetworkDetails
			if err := json.Unmarshal(jsonBytes, &nd); err == nil {
				net.BackendSpecific = &nd
				return &nd
			}
		}
	}
	// If conversion failed or it's something else, create a new NetworkDetails
	net.BackendSpecific = &NetworkDetails{}
	return net.BackendSpecific.(*NetworkDetails)
}

// prefixString renders a CIDR the way the Docker API used to hand it to us: a
// dotted-quad string, or empty when the daemon reported no value. The zero
// netip.Prefix stringifies to "invalid Prefix", which would leak into subnet
// IDs and CIDR fields.
func prefixString(p netip.Prefix) string {
	if !p.IsValid() {
		return ""
	}
	return p.String()
}

// addrString is prefixString for a bare address.
func addrString(a netip.Addr) string {
	if !a.IsValid() {
		return ""
	}
	return a.String()
}

func (s *b) GetNetworks() (backends.NetworkList, error) {
	log := s.log.WithPrefix("GetNetworks: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backends.NetworkList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	wg.Add(len(zones))
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s network: start", zone)
			defer log.Detail("zone=%s network: end", zone)
			cli, err := s.getDockerClient(zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			out, err := cli.NetworkList(context.Background(), client.NetworkListOptions{})
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			for _, network := range out.Items {
				cidr := ""
				if len(network.IPAM.Config) > 0 {
					cidr = prefixString(network.IPAM.Config[0].Subnet)
				}
				description := ""
				if val, ok := network.Labels["description"]; ok {
					description = val
				}
				owner := ""
				if val, ok := network.Labels["owner"]; ok {
					owner = val
				}
				subnets := backends.SubnetList{}
				for i, subnet := range network.IPAM.Config {
					subnetID := prefixString(subnet.Subnet) + "-" + addrString(subnet.Gateway)
					subnets = append(subnets, &backends.Subnet{
						BackendType:      backends.BackendTypeDocker,
						Name:             subnetID,
						Description:      prefixString(subnet.IPRange),
						SubnetId:         subnetID,
						NetworkId:        network.ID,
						Cidr:             prefixString(subnet.Subnet),
						ZoneName:         zone,
						ZoneID:           zone,
						Owner:            owner,
						Tags:             map[string]string{},
						IsDefault:        i == 0,
						IsAerolabManaged: false,
						State:            backends.NetworkStateAvailable,
						PublicIP:         false,
						Network:          nil,
						BackendSpecific:  subnet,
					})
				}
				ilock.Lock()
				i = append(i, &backends.Network{
					BackendType:      backends.BackendTypeDocker,
					Name:             network.Name,
					Description:      description,
					NetworkId:        network.ID,
					Cidr:             cidr,
					ZoneName:         zone,
					ZoneID:           zone,
					Owner:            owner,
					Tags:             network.Labels,
					IsDefault:        network.Name == "bridge",
					IsAerolabManaged: false,
					State:            backends.NetworkStateAvailable,
					Subnets:          subnets,
					BackendSpecific: &NetworkDetails{
						Scope:      network.Scope,
						Driver:     network.Driver,
						Internal:   network.Internal,
						Attachable: network.Attachable,
						Ingress:    network.Ingress,
						Options:    network.Options,
						Created:    network.Created,
					},
				})
				ilock.Unlock()
			}
		}(zone)
	}
	wg.Wait()
	if errs == nil {
		s.networks = i
	}
	return i, errs
}

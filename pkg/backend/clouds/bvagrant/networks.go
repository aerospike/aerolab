package bvagrant

import "github.com/aerospike/aerolab/pkg/backend/backends"

// GetNetworks returns a single synthetic network describing the configured (or default)
// vagrant private-network subnet. Vagrant has no VPC/subnet management of its own — this
// exists purely so callers that enumerate Networks see something sensible for "local".
func (s *b) GetNetworks() (backends.NetworkList, error) {
	subnet := defaultSubnet
	if s.credentials != nil && s.credentials.Subnet != "" {
		subnet = s.credentials.Subnet
	}

	net := &backends.Network{
		BackendType:      backends.BackendTypeVagrant,
		Name:             "local",
		Description:      "Local vagrant private network",
		NetworkId:        "local",
		Cidr:             subnet,
		ZoneName:         "local",
		ZoneID:           "local",
		Owner:            "",
		Tags:             map[string]string{},
		IsDefault:        true,
		IsAerolabManaged: true,
		State:            backends.NetworkStateAvailable,
		Subnets: backends.SubnetList{
			{
				BackendType:      backends.BackendTypeVagrant,
				Name:             "local",
				SubnetId:         "local",
				NetworkId:        "local",
				Cidr:             subnet,
				ZoneName:         "local",
				ZoneID:           "local",
				Tags:             map[string]string{},
				IsDefault:        true,
				IsAerolabManaged: true,
				State:            backends.NetworkStateAvailable,
			},
		},
	}

	s.networks = backends.NetworkList{net}
	return s.networks, nil
}

package bvagrant

import (
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
)

// GetNetworks retrieves all Vagrant networks (not implemented - Vagrant uses provider-specific networking).
//
// Returns:
//   - backends.NetworkList: empty list
//   - error: nil
func (s *b) GetNetworks() (backends.NetworkList, error) {
	log := s.log.WithPrefix("GetNetworks: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// Vagrant networking is provider-specific and not managed separately
	// Networks are defined in Vagrantfiles
	return backends.NetworkList{}, nil
}

// DockerCreateNetwork creates a network (not applicable for Vagrant).
func (s *b) DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error {
	log := s.log.WithPrefix("DockerCreateNetwork: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DockerCreateNetwork")
}

// DockerDeleteNetwork deletes a network (not applicable for Vagrant).
func (s *b) DockerDeleteNetwork(region string, name string) error {
	log := s.log.WithPrefix("DockerDeleteNetwork: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DockerDeleteNetwork")
}

// DockerPruneNetworks prunes unused networks (not applicable for Vagrant).
func (s *b) DockerPruneNetworks(region string) error {
	log := s.log.WithPrefix("DockerPruneNetworks: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "DockerPruneNetworks")
}

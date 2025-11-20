package bvagrant

import (
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
)

// GetFirewalls retrieves all Vagrant firewalls (not implemented - Vagrant uses OS-level firewalls).
//
// Parameters:
//   - networkList: network list for firewall association
//
// Returns:
//   - backends.FirewallList: empty list
//   - error: nil
func (s *b) GetFirewalls(networkList backends.NetworkList) (backends.FirewallList, error) {
	log := s.log.WithPrefix("GetFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// Vagrant doesn't have a firewall concept like cloud providers
	// Firewalls are typically managed by the guest OS
	return backends.FirewallList{}, nil
}

// CreateFirewall creates a firewall (not implemented for Vagrant).
func (s *b) CreateFirewall(input *backends.CreateFirewallInput, waitDur time.Duration) (*backends.CreateFirewallOutput, error) {
	log := s.log.WithPrefix("CreateFirewall: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "CreateFirewall")
}

// FirewallsUpdate updates firewall rules (not implemented for Vagrant).
func (s *b) FirewallsUpdate(fw backends.FirewallList, ports backends.PortsIn, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsUpdate: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "FirewallsUpdate")
}

// FirewallsDelete deletes firewalls (not implemented for Vagrant).
func (s *b) FirewallsDelete(fw backends.FirewallList, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsDelete: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "FirewallsDelete")
}

// FirewallsAddTags adds tags to firewalls (not implemented for Vagrant).
func (s *b) FirewallsAddTags(fw backends.FirewallList, tags map[string]string, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "FirewallsAddTags")
}

// FirewallsRemoveTags removes tags from firewalls (not implemented for Vagrant).
func (s *b) FirewallsRemoveTags(fw backends.FirewallList, tagKeys []string, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "FirewallsRemoveTags")
}

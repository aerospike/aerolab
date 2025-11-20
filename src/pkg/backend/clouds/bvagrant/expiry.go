package bvagrant

import (
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
)

// ExpiryInstall installs the expiry system (not implemented for Vagrant).
//
// The expiry system is primarily designed for cloud providers where resources
// incur costs. For Vagrant, which runs locally, automated expiry is less critical.
func (s *b) ExpiryInstall(intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error {
	log := s.log.WithPrefix("ExpiryInstall: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryInstall")
}

// ExpiryRemove removes the expiry system (not implemented for Vagrant).
func (s *b) ExpiryRemove(zones ...string) error {
	log := s.log.WithPrefix("ExpiryRemove: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryRemove")
}

// ExpiryChangeConfiguration changes expiry system configuration (not implemented for Vagrant).
func (s *b) ExpiryChangeConfiguration(logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error {
	log := s.log.WithPrefix("ExpiryChangeConfiguration: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryChangeConfiguration")
}

// ExpiryList lists expiry system installations (not implemented for Vagrant).
func (s *b) ExpiryList() ([]*backends.ExpirySystem, error) {
	log := s.log.WithPrefix("ExpiryList: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return nil, backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryList")
}

// ExpiryChangeFrequency changes expiry check frequency (not implemented for Vagrant).
func (s *b) ExpiryChangeFrequency(intervalMinutes int, zones ...string) error {
	log := s.log.WithPrefix("ExpiryChangeFrequency: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.ReturnNotImplemented(backends.BackendTypeVagrant, "ExpiryChangeFrequency")
}

package bvagrant

import (
	"github.com/aerospike/aerolab/pkg/backend/backends"
)

// Vagrant runs on the operator's own machine, with a single fixed "local" zone and no
// remote daemon to install an expiry sweeper into (unlike the cloud backends, which
// install a periodic Lambda/systemd-timer/etc.). These mirror bdocker/expiry.go's shape,
// which is itself a set of TODO no-ops for the same reason (docker containers are also
// local-only and have no expiry daemon yet).

func (s *b) ExpiryInstall(intervalMinutes int, logLevel int, expireEksctl bool, cleanupDNS bool, force bool, onUpdateKeepOriginalSettings bool, zones ...string) error {
	// TODO: implement
	return nil
}

func (s *b) ExpiryRemove(zones ...string) error {
	// TODO: implement
	return nil
}

func (s *b) ExpiryChangeConfiguration(logLevel int, expireEksctl bool, cleanupDNS bool, zones ...string) error {
	// TODO: implement
	return nil
}

func (s *b) ExpiryList() ([]*backends.ExpirySystem, error) {
	// TODO: implement
	return []*backends.ExpirySystem{}, nil
}

func (s *b) ExpiryChangeFrequency(intervalMinutes int, zones ...string) error {
	// TODO: implement
	return nil
}

// ExpiryV7Check always returns false for vagrant, as there was no v7 expiry system for it.
func (s *b) ExpiryV7Check() (found bool, regions []string, err error) {
	return false, nil, nil
}

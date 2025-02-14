package baws

import "github.com/aerospike/aerolab/pkg/backend"

func (s *b) ExpiryInstall(zones ...string) error {
	return nil
}

func (s *b) ExpiryRemove(zones ...string) error {
	return nil
}

func (s *b) ExpiryList() ([]*backend.ExpirySystem, error) {
	return []*backend.ExpirySystem{}, nil
}

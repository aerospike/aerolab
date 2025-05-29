package bdocker

import (
	"errors"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/lithammer/shortuuid"
)

func (s *b) GetFirewalls(networks backends.NetworkList) (backends.FirewallList, error) {
	log := s.log.WithPrefix("GetFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return backends.FirewallList{}, nil
}

func (s *b) FirewallsUpdate(fw backends.FirewallList, ports backends.PortsIn, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsUpdate: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

func (s *b) FirewallsDelete(fw backends.FirewallList, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsDelete: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

func (s *b) FirewallsAddTags(fw backends.FirewallList, tags map[string]string, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

func (s *b) FirewallsRemoveTags(fw backends.FirewallList, tagKeys []string, waitDur time.Duration) error {
	log := s.log.WithPrefix("FirewallsRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

func (s *b) CreateFirewall(input *backends.CreateFirewallInput, waitDur time.Duration) (output *backends.CreateFirewallOutput, err error) {
	log := s.log.WithPrefix("CreateFirewall: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return nil, errors.New("not implemented")
}

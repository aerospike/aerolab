package baws

import "errors"

func (s *b) DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error {
	return errors.New("not implemented")
}

func (s *b) DockerDeleteNetwork(region string, name string) error {
	return errors.New("not implemented")
}

func (s *b) DockerPruneNetworks(region string) error {
	return errors.New("not implemented")
}

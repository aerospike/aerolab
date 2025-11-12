package baws

import "github.com/aerospike/aerolab/pkg/backend/backends"

func (s *b) DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeAWS, "DockerCreateNetwork")
}

func (s *b) DockerDeleteNetwork(region string, name string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeAWS, "DockerDeleteNetwork")
}

func (s *b) DockerPruneNetworks(region string) error {
	return backends.ReturnNotImplemented(backends.BackendTypeAWS, "DockerPruneNetworks")
}

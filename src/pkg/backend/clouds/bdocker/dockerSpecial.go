package bdocker

import (
	"context"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/lithammer/shortuuid"
)

func (s *b) DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error {
	log := s.log.WithPrefix("DockerCreateNetwork: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	defer s.invalidateCacheFunc(backends.CacheInvalidateNetwork)
	cli, err := s.getDockerClient(region)
	if err != nil {
		return err
	}
	var options map[string]string
	if mtu != "" {
		options = map[string]string{
			"com.docker.network.driver.mtu": mtu,
		}
	}
	var ipam *network.IPAM
	if subnet != "" {
		ipam = &network.IPAM{
			Config: []network.IPAMConfig{
				{Subnet: subnet},
			},
		}
	}
	_, err = cli.NetworkCreate(context.Background(), name, network.CreateOptions{
		Driver:     driver,
		IPAM:       ipam,
		Options:    options,
		Attachable: true,
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *b) DockerDeleteNetwork(region string, name string) error {
	log := s.log.WithPrefix("DockerDeleteNetwork: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	defer s.invalidateCacheFunc(backends.CacheInvalidateNetwork)
	cli, err := s.getDockerClient(region)
	if err != nil {
		return err
	}
	err = cli.NetworkRemove(context.Background(), name)
	if err != nil {
		return err
	}
	return nil
}

func (s *b) DockerPruneNetworks(region string) error {
	log := s.log.WithPrefix("DockerPruneNetworks: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	defer s.invalidateCacheFunc(backends.CacheInvalidateNetwork)
	cli, err := s.getDockerClient(region)
	if err != nil {
		return err
	}
	_, err = cli.NetworksPrune(context.Background(), filters.NewArgs())
	if err != nil {
		return err
	}
	return nil
}

package bdocker

import (
	"fmt"

	"github.com/docker/docker/client"
)

func (s *b) getDockerClient(region string) (*client.Client, error) {
	if (region == "default" || region == "") && s.credentials.EnableDefaultFromEnv {
		return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	}

	regionDefinition, ok := s.credentials.Regions[region]
	if !ok {
		return nil, fmt.Errorf("region %s not found", region)
	}

	opts := []client.Opt{client.WithAPIVersionNegotiation()}

	if regionDefinition.DockerHost != "" {
		opts = append(opts, client.WithHost(regionDefinition.DockerHost))
	} else {
		return nil, fmt.Errorf("docker host is not set for region %s", region)
	}

	if regionDefinition.DockerCertPath != "" {
		opts = append(opts, client.WithTLSClientConfig(regionDefinition.DockerCertPath, regionDefinition.DockerKeyPath, regionDefinition.DockerCaPath))
	}

	if regionDefinition.Timeout != 0 {
		opts = append(opts, client.WithTimeout(regionDefinition.Timeout))
	}

	return client.NewClientWithOpts(opts...)
}

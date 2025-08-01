package backends

import "fmt"

func (b *backend) DockerCreateNetwork(region string, name string, driver string, subnet string, mtu string) error {
	if _, ok := cloudList[BackendTypeDocker]; !ok {
		return fmt.Errorf("backend type %s not enabled", BackendTypeDocker)
	}
	return cloudList[BackendTypeDocker].DockerCreateNetwork(region, name, driver, subnet, mtu)
}

func (b *backend) DockerDeleteNetwork(region string, name string) error {
	if _, ok := cloudList[BackendTypeDocker]; !ok {
		return fmt.Errorf("backend type %s not enabled", BackendTypeDocker)
	}
	return cloudList[BackendTypeDocker].DockerDeleteNetwork(region, name)
}

func (b *backend) DockerPruneNetworks(region string) error {
	if _, ok := cloudList[BackendTypeDocker]; !ok {
		return fmt.Errorf("backend type %s not enabled", BackendTypeDocker)
	}
	return cloudList[BackendTypeDocker].DockerPruneNetworks(region)
}

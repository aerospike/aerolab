package bvagrant

type SSHInfo struct {
	HostName     string
	Port         int
	User         string
	IdentityFile string
}

type BoxInfo struct {
	Name     string
	Provider string
	Version  string
}

type runner interface {
	Up(dir string, machines []string, provider string, verbose bool) error
	Halt(dir string, machines []string) error
	Destroy(dir string, machines []string) error
	Status(dir string) (map[string]string, error)
	SSHConfig(dir string, machine string) (SSHInfo, error)
	BoxList() ([]BoxInfo, error)
	BoxAdd(name, location string) error
	BoxRemove(name string) error
	Package(dir, machine, outputPath string) error
	Version() (string, error)
	PluginList() ([]string, error)
}

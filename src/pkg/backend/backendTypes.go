package backend

type BackendType string

const (
	BackendTypeAWS    = "aws"
	BackendTypeGCP    = "gcp"
	BackendTypeDocker = "docker"
)

func ListBackendTypes() []BackendType {
	return []BackendType{
		BackendTypeAWS,
		BackendTypeGCP,
		BackendTypeDocker,
	}
}

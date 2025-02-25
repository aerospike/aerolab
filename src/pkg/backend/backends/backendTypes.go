package backends

type BackendType string

const (
	BackendTypeAWS    = "aws"
	BackendTypeGCP    = "gcp"
	BackendTypeDocker = "docker"
)

func ListBackendTypes() []BackendType {
	types := []BackendType{}
	for n := range cloudList {
		types = append(types, n)
	}
	return types
}

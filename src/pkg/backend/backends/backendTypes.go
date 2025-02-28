package backends

type BackendType string

const (
	BackendTypeAWS    BackendType = "aws"
	BackendTypeGCP    BackendType = "gcp"
	BackendTypeDocker BackendType = "docker"
)

func ListBackendTypes() []BackendType {
	types := []BackendType{}
	for n := range cloudList {
		types = append(types, n)
	}
	return types
}

package scripts

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed install-docker.sh
var InstallDocker []byte

//go:embed graph-start.sh
var startGraph string

//go:embed graph-config.properties
var graphProperties string

//go:embed vector-install.sh
var vectorInstall string

//go:embed eksctl-bootstrap.sh
var eksctlBootstrap string

//go:embed netLossDelay.sh
var netLossDelay string

// docker login details
type DockerLogin struct {
	URL  string
	User string
	Pass string
}

// seeds:      []string{"ip:port","ip:port",...}
// properties: extra properties, for example []string{"aerospike.graph.index.vertex.properties=property1,property2"}
func GetGraphConfig(seeds []string, namespace string, properties []string, rammb int) []byte {
	if rammb > 0 {
		properties = append(properties, fmt.Sprintf("aerospike.graph-service.heap.max=%dm", rammb))
	}
	return []byte(fmt.Sprintf(graphProperties, strings.Join(seeds, ", "), namespace, strings.Join(properties, "\n")))
}

// for on-cloud deployments, installs docker and starts graph inside
// for properties file path, ex: /etc/aerospike-graph/aerospike-graph.properties
func GetCloudGraphScript(propertiesFilePath string, extraParams string, imageName string, login *DockerLogin) []byte {
	if login == nil {
		return append(InstallDocker, []byte(fmt.Sprintf(startGraph, "aerospike-graph", propertiesFilePath, extraParams, imageName))...)
	}
	// add login to InstallDocker
	loginScript := []byte(fmt.Sprintf("\ndocker login --username '%s' --password '%s'", login.User, strings.ReplaceAll(login.Pass, "'", "\\'")))
	if login.URL != "" {
		loginScript = append(loginScript, []byte(fmt.Sprintf(" %s", login.URL))...)
	}
	loginScript = append(loginScript, '\n')
	installScript := append(InstallDocker, loginScript...)
	return append(installScript, []byte(fmt.Sprintf(startGraph, "aerospike-graph", propertiesFilePath, extraParams, imageName))...)
}

// asvec["amd64|arm64"] = downloadUrl
func GetVectorScript(isDocker bool, packaging string, asvec map[string]string, vectorSeed string) []byte {
	dockerVal := "0"
	if isDocker {
		dockerVal = "1"
	}
	debVal := "0"
	if packaging == "deb" {
		debVal = "1"
	}
	return []byte(fmt.Sprintf(vectorInstall, dockerVal, debVal, asvec["amd64"], asvec["arm64"], vectorSeed))
}

func GetEksctlBootstrapScript() []byte {
	return []byte(eksctlBootstrap)
}

func GetNetLossDelay() string {
	return netLossDelay
}

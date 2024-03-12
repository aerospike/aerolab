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

// seeds:      []string{"ip:port","ip:port",...}
// properties: extra properties, for example []string{"aerospike.graph.index.vertex.properties=property1,property2"}
func GetGraphConfig(seeds []string, namespace string, properties []string) []byte {
	return []byte(fmt.Sprintf(graphProperties, strings.Join(seeds, ", "), namespace, strings.Join(properties, "\n")))
}

// for on-cloud deployments, installs docker and starts graph inside
// for properties file path, ex: /etc/aerospike-graph/aerospike-graph.properties
func GetCloudGraphScript(ramMB int, propertiesFilePath string, extraParams string, imageName string) []byte {
	return append(InstallDocker, []byte(fmt.Sprintf(startGraph, "aerospike-graph", ramMB, propertiesFilePath, extraParams, imageName))...)
}

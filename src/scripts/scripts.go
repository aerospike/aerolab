package scripts

import (
	_ "embed"
	"fmt"
	"strings"
)

/* -- graph --
1. Save contents of GetGraphConfig to the destPath on client
2. Save contents of GetGraphScript to a temp location on client
3. Run the script from point 2 on client
*/

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

// for local deployments, just starts graph on local machine
func GetDockerGraphScript(clientName string, ramMB int, propertiesFilePath string, extraParams string) []byte {
	return []byte(fmt.Sprintf(startGraph, clientName, ramMB, propertiesFilePath, extraParams))
}

// for on-cloud deployments, installs docker and starts graph inside
// for properties file path, ex: /etc/aerospike-graph/aerospike-graph.properties
func GetCloudGraphScript(ramMB int, propertiesFilePath string, extraParams string) []byte {
	return append(InstallDocker, []byte(fmt.Sprintf(startGraph, "aerospike-graph", ramMB, propertiesFilePath, extraParams))...)
}

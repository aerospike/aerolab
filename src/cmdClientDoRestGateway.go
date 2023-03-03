package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type TypeRestGatewayVersion string

type clientCreateRestGatewayCmd struct {
	clientCreateBaseCmd
	Version     TypeRestGatewayVersion `short:"v" long:"version" description:"rest gw version; default=latest"`
	ClusterName TypeClusterName        `short:"C" long:"cluster-name" description:"cluster name to connect to" default:"mydc"`
	Username    string                 `short:"U" long:"username" description:"connect username"`
	Password    string                 `short:"P" long:"password" description:"connect password"`
	chDirCmd
}

type clientAddRestGatewayCmd struct {
	ClientName  TypeClientName         `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines    TypeMachines           `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	Version     TypeRestGatewayVersion `short:"v" long:"version" description:"rest gw version; default=latest"`
	ClusterName TypeClusterName        `short:"C" long:"cluster-name" description:"cluster name to connect to" default:"mydc"`
	Username    string                 `short:"U" long:"username" description:"connect username"`
	Password    string                 `short:"P" long:"password" description:"connect password"`
	StartScript flags.Filename         `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	url         string
	dirName     string
	fileName    string
	seedNode    string
	machines    []int
	Help        helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCreateRestGatewayCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroName != TypeDistro("ubuntu") || (c.DistroVersion != TypeDistroVersion("22.04") && c.DistroVersion != TypeDistroVersion("latest")) {
		return fmt.Errorf("RG is only supported on ubuntu:22.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}

	url, err := c.Version.GetDownloadURL()
	if err != nil {
		return err
	}
	b.WorkOnServers()
	clist, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clist, string(c.ClusterName)) {
		return errors.New("cluster not found")
	}
	ips, err := b.GetNodeIpMap(string(c.ClusterName), true)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		ips, err = b.GetNodeIpMap(string(c.ClusterName), false)
		if err != nil {
			return err
		}
		if len(ips) == 0 {
			return errors.New("node IPs not found")
		}
	}
	for _, ip := range ips {
		if ip != "" {
			a.opts.Client.Add.RestGateway.seedNode = ip
			break
		}
	}
	b.WorkOnClients()
	if a.opts.Client.Add.RestGateway.seedNode == "" {
		return errors.New("could not find an IP for a node in the given cluster - are all the nodes down?")
	}
	machines, err := c.createBase(args, "rest-gateway")
	if err != nil {
		return err
	}
	a.opts.Client.Add.RestGateway.ClientName = c.ClientName
	a.opts.Client.Add.RestGateway.StartScript = c.StartScript
	a.opts.Client.Add.RestGateway.Machines = TypeMachines(intSliceToString(machines, ","))
	a.opts.Client.Add.RestGateway.Version = c.Version
	a.opts.Client.Add.RestGateway.url = url
	a.opts.Client.Add.RestGateway.ClusterName = c.ClusterName
	a.opts.Client.Add.RestGateway.Username = c.Username
	a.opts.Client.Add.RestGateway.Password = c.Password
	a.opts.Client.Add.RestGateway.machines = machines
	return a.opts.Client.Add.RestGateway.addRestGateway(args)
}

func (c *clientAddRestGatewayCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.addRestGateway(args)
}

func (c *clientAddRestGatewayCmd) addRestGateway(args []string) error {
	b.WorkOnClients()
	var err error
	if c.url == "" {
		c.url, err = c.Version.GetDownloadURL()
		if err != nil {
			return err
		}
	}
	c.dirName, c.fileName = c.Version.GetJarPath()
	if c.seedNode == "" {
		b.WorkOnServers()
		clist, err := b.ClusterList()
		if err != nil {
			return err
		}
		if !inslice.HasString(clist, string(c.ClusterName)) {
			return errors.New("cluster not found")
		}
		ips, err := b.GetNodeIpMap(string(c.ClusterName), true)
		if err != nil {
			return err
		}
		if len(ips) == 0 {
			ips, err = b.GetNodeIpMap(string(c.ClusterName), false)
			if err != nil {
				return err
			}
			if len(ips) == 0 {
				return errors.New("node IPs not found")
			}
		}
		for _, ip := range ips {
			a.opts.Client.Add.RestGateway.seedNode = ip
			break
		}
		b.WorkOnClients()
	}
	script := c.installScript()
	err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/opt/install-gw.sh", strings.NewReader(script), len(script)}}, c.machines)
	if err != nil {
		return err
	}
	a.opts.Attach.Client.ClientName = c.ClientName
	a.opts.Attach.Client.Machine = c.Machines
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "/opt/install-gw.sh"})
	if err != nil {
		return err
	}
	a.opts.Attach.Client.Detach = true
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "/opt/autoload/01-restgw.sh"})
	if err != nil {
		return fmt.Errorf("failed to restart rest gateway: %s", err)
	}
	log.Print("Done")
	log.Print("Documentation can be found at: https://aerospike.github.io/aerospike-rest-gateway/")
	log.Print("Rest gateway logs are on the nodes in /var/log/, use 'client attach' command to explore the logs; connect with browser or curl to get the data")
	log.Print("Startup parameters are in /opt/autoload/01-restgw.sh on each node")
	return nil
}

func (c *clientAddRestGatewayCmd) installScript() string {
	return fmt.Sprintf(`set -e
apt-get update
apt-get -y install openjdk-19-jre openjdk-19-jre-headless curl wget
cd /opt
wget %s
tar -zxvf %s.tgz

mkdir -p /opt/autoload
cat <<'EOF' > /opt/autoload/01-restgw.sh
SEED=%s
AUTH_USER=%s
AUTH_PASS=%s
[ "${AUTH_USER}" == "" ] && AUTH_PARAMS="" || AUTH_PARAMS=(--aerospike.restclient.clientpolicy.user=${AUTH_USER} --aerospike.restclient.clientpolicy.password=${AUTH_PASS})
cd /opt/%s
nohup java -server -Dcom.sun.management.jmxremote -Dcom.sun.management.jmxremote.port=8082 -Dcom.sun.management.jmxremote.rmi.port=8082 -Dcom.sun.management.jmxremote.local.only=false -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false -XX:+UseG1GC -Xms2048m -Xmx2048m -jar ./%s --aerospike.restclient.hostname=${SEED} ${AUTH_PARAMS[@]} --logging.file.name=/var/log/restclient.log > /var/log/restclient_console.log 2>&1 &
EOF

chmod 755 /opt/autoload/01-restgw.sh
`, c.url, c.dirName, c.seedNode, c.Username, c.Password, c.dirName, c.fileName)
}

// returns url or error
// if version is "", it will find the latest
func (version *TypeRestGatewayVersion) GetDownloadURL() (string, error) {
	baseUrl := "https://download.aerospike.com/artifacts/aerospike-rest-gateway/"
	if *version == "" {
		client := &http.Client{}
		req, err := http.NewRequest("GET", baseUrl, nil)
		if err != nil {
			return "", err
		}
		response, err := client.Do(req)
		if err != nil {
			return "", err
		}

		if response.StatusCode != 200 {
			err = fmt.Errorf("error code: %d, URL: %s", response.StatusCode, baseUrl)
			return "", err
		}

		responseData, err := io.ReadAll(response.Body)
		if err != nil {
			return "", err
		}
		ver := ""
		for _, line := range strings.Split(string(responseData), "\n") {
			if strings.Contains(line, "folder.gif") {
				rp := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+[\.]*[0-9]*[^/]*`)
				nver := rp.FindString(line)
				if ver == "" {
					ver = nver
				} else {
					if VersionCheck(nver, ver) == -1 {
						ver = nver
					}
				}
			}
		}
		if ver == "" {
			return "", errors.New("required version not found")
		}
		*version = TypeRestGatewayVersion(ver)
	}

	fn := "aerospike-rest-gateway-"
	if VersionCheck("2.0.2", string(*version)) == -1 {
		fn = "aerospike-client-rest-"
	}
	dlUrl := baseUrl + string(*version) + "/" + fn + string(*version) + ".tgz"

	client := &http.Client{}
	req, err := http.NewRequest("HEAD", dlUrl, nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if response.StatusCode == 300 {
		return "", fmt.Errorf("version %s not found at %s", string(*version), baseUrl)
	}

	if response.StatusCode != 200 {
		err = fmt.Errorf("error code: %d, URL: %s", response.StatusCode, baseUrl)
		return "", err
	}
	return dlUrl, nil
}

func (version *TypeRestGatewayVersion) GetJarPath() (dirName string, fileName string) {
	if VersionCheck("2.0.2", string(*version)) == -1 {
		return "aerospike-client-rest-" + string(*version), "as-rest-client-" + string(*version) + ".jar"
	}
	return "aerospike-rest-gateway-" + string(*version), "as-rest-gateway-" + string(*version) + ".jar"
}

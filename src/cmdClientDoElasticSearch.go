package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type clientCreateElasticSearchCmd struct {
	clientCreateBaseCmd
	RamLimit int  `long:"mem-limit" description:"By Default ES will use most of any machine RAM; set this to a number of GB to limit each ES instance" default:"0"`
	JustDoIt bool `long:"confirm" description:"set this parameter to confirm any warning questions without being asked to press ENTER to continue"`
}

type clientAddElasticSearchCmd struct {
	ClientName    TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines      TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	StartScript   flags.Filename `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	RamLimit      int            `long:"mem-limit" description:"By Default ES will use most of any machine RAM; set this to a number of GB to limit each ES instance" default:"0"`
	existingNodes []int
	Help          helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCreateElasticSearchCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type == "docker" && !strings.Contains(c.Docker.ExposePortsToHost, ":9200") {
		if c.Docker.NoAutoExpose {
			fmt.Println("Docker backend is in use, but elasticsearch access port is not being forwarded. If using Docker Desktop, use '-e 9200:9200' parameter in order to forward port 9200. This can only be done for one elasticsearch node. Press ENTER to continue regardless.")
			if !c.JustDoIt {
				bufio.NewReader(os.Stdin).ReadBytes('\n')
			}
		} else {
			c.Docker.ExposePortsToHost = strings.Trim("9200,"+c.Docker.ExposePortsToHost, ",")
		}
	}
	if c.ClientCount > 1 && c.RamLimit == 0 && a.opts.Config.Backend.Type == "docker" {
		return fmt.Errorf("more than one elasticsearch node cannot be started without specifying RAM limits in docker - elasticsearch default will cause OOM-kills")
	}
	if c.DistroName != TypeDistro("ubuntu") || (c.DistroVersion != TypeDistroVersion("22.04") && c.DistroVersion != TypeDistroVersion("latest")) {
		return fmt.Errorf("ES is only supported on ubuntu:22.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	b.WorkOnClients()
	clusters, err := b.ClusterList()
	if err != nil {
		return err
	}
	if inslice.HasString(clusters, c.ClientName.String()) {
		a.opts.Client.Add.ElasticSearch.existingNodes, err = b.NodeListInCluster(c.ClientName.String())
		if err != nil {
			return err
		}
	}
	if len(a.opts.Client.Add.ElasticSearch.existingNodes) > 0 && c.RamLimit == 0 && a.opts.Config.Backend.Type == "docker" {
		return fmt.Errorf("more than one elasticsearch node cannot be started without specifying RAM limits in docker - elasticsearch default will cause OOM-kills")
	}
	machines, err := c.createBase(args, "elasticsearch")
	if err != nil {
		return err
	}
	if c.PriceOnly {
		return nil
	}

	a.opts.Client.Add.ElasticSearch.ClientName = c.ClientName
	a.opts.Client.Add.ElasticSearch.StartScript = c.StartScript
	a.opts.Client.Add.ElasticSearch.Machines = TypeMachines(intSliceToString(machines, ","))
	a.opts.Client.Add.ElasticSearch.RamLimit = c.RamLimit
	return a.opts.Client.Add.ElasticSearch.addElasticSearch(args)
}

func (c *clientAddElasticSearchCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.addElasticSearch(args)
}

func (c *clientAddElasticSearchCmd) addElasticSearch(args []string) error {
	isDocker := false
	if a.opts.Config.Backend.Type == "docker" {
		isDocker = true
		out, err := exec.Command("docker", "run", "--rm", "-i", "--privileged", "ubuntu:22.04", "sysctl", "-w", "vm.max_map_count=262144").CombinedOutput()
		if err != nil {
			fmt.Println("Workaround `sysctl -w vm.max_map_count=262144` for docker failed, elasticsearch might fail to start...")
			fmt.Println(err)
			fmt.Println(string(out))
		}
	}
	b.WorkOnClients()
	masterNode := 1
	if len(c.existingNodes) == 0 {
		script := c.installScriptAllNodes(c.RamLimit, isDocker) + c.installScriptMasterNode()
		err := b.CopyFilesToCluster(c.ClientName.String(), []fileList{{filePath: "/root/install.sh", fileContents: script, fileSize: len(script)}}, []int{1})
		if err != nil {
			return err
		}
		a.opts.Attach.Client.ClientName = c.ClientName
		a.opts.Attach.Client.Detach = false
		a.opts.Attach.Client.Machine = TypeMachines("1")
		err = a.opts.Attach.Client.run([]string{"/bin/bash", "/root/install.sh"})
		if err != nil {
			return err
		}
		// start connector
		a.opts.Attach.Client.ClientName = c.ClientName
		a.opts.Attach.Client.Detach = true
		a.opts.Attach.Client.Machine = TypeMachines("1")
		err = a.opts.Attach.Client.run([]string{"/bin/bash", "/opt/autoload/02-elasticsearch-connector"})
		if err != nil {
			return err
		}
	} else {
		masterNode = c.existingNodes[0]
	}
	nodes := []int{}
	for _, node := range strings.Split(c.Machines.String(), ",") {
		nodeInt, err := strconv.Atoi(node)
		if err != nil {
			return err
		}
		nodes = append(nodes, nodeInt)
	}
	for _, node := range nodes {
		if node == masterNode {
			continue
		}
		out, err := b.RunCommands(c.ClientName.String(), [][]string{{"/usr/share/elasticsearch/bin/elasticsearch-create-enrollment-token", "-s", "node"}}, []int{masterNode})
		if err != nil {
			return err
		}
		token := string(out[0])
		out, err = b.RunCommands(c.ClientName.String(), [][]string{{"cat", "/etc/aerospike-elasticsearch-outbound/truststore.pkcs12"}}, []int{masterNode})
		if err != nil {
			return err
		}
		cert := base64.StdEncoding.EncodeToString(out[0])
		script := c.installScriptAllNodes(c.RamLimit, isDocker) + c.installScriptSlaveNodesOnSlaves(token, cert)
		err = b.CopyFilesToCluster(c.ClientName.String(), []fileList{{filePath: "/root/install.sh", fileContents: script, fileSize: len(script)}}, []int{node})
		if err != nil {
			return err
		}
		a.opts.Attach.Client.ClientName = c.ClientName
		a.opts.Attach.Client.Detach = false
		a.opts.Attach.Client.Machine = TypeMachines(strconv.Itoa(node))
		err = a.opts.Attach.Client.run([]string{"/bin/bash", "/root/install.sh"})
		if err != nil {
			return err
		}
		// start connector
		a.opts.Attach.Client.ClientName = c.ClientName
		a.opts.Attach.Client.Detach = true
		a.opts.Attach.Client.Machine = TypeMachines(strconv.Itoa(node))
		err = a.opts.Attach.Client.run([]string{"/bin/bash", "/opt/autoload/02-elasticsearch-connector"})
		if err != nil {
			return err
		}
	}
	fmt.Print(`
ES username/password is: elastic/elastic

Usage examples to query in browser:
 * https://ELASTICIP:9200/test/_search
 * https://ELASTICIP:9200/test/_search?size=50
 * https://ELASTICIP:9200/test/_search?q=mybin:binvalue
 * https://ELASTICIP:9200/test/_search?q=metadata.set:myset

For best results, use FireFox, as it has a built-in JSON explorer features. It also accepts invalid slef-signed certificates that ES provides, while Chrome doesn't allow to continue.

`)
	if a.opts.Config.Backend.Type == "docker" {
		log.Print("Execute `aerolab inventory list` to get access URL.")
	}
	log.Println("Done")
	log.Println("WARN: Deprecation notice: the way clients are created and deployed is changing. A new way will be published in AeroLab 7.2 and the current client creation methods will be removed in AeroLab 8.0")
	return nil
}

func (c *clientAddElasticSearchCmd) installScriptAllNodes(ramlimit int, isDocker bool) string {
	script := `#!/bin/bash
apt-get update
apt-get -y install apt-transport-https ca-certificates curl gnupg wget openjdk-8-jre
echo "deb https://artifacts.elastic.co/packages/8.x/apt stable main" | tee /etc/apt/sources.list.d/elastic-8.x.list
curl -L https://artifacts.elastic.co/GPG-KEY-elasticsearch |apt-key add -
apt-get update && apt-get -y install elasticsearch
mkdir /nonexistent
chown elasticsearch /nonexistent
sed -i 's/elasticsearch:\(.*\):\/bin\/false/elasticsearch:\1:\/bin\/bash/g' /etc/passwd
`
	if ramlimit > 0 {
		script = script + fmt.Sprintf(`cat <<'EOF' > /etc/elasticsearch/jvm.options.d/ram.options
-Xms%dg
-Xmx%dg
EOF
`, ramlimit, ramlimit)
	}
	script = script + `cd /root
wget https://download.aerospike.com/artifacts/enterprise/aerospike-elasticsearch/1.0.0/aerospike-elasticsearch-outbound-1.0.0.all.deb
dpkg -i aerospike-elasticsearch-outbound-1.0.0.all.deb
sed -i -E 's/(.*)port: 9200/\1port: 9200\n\1scheme: https/g' /etc/aerospike-elasticsearch-outbound/aerospike-elasticsearch-outbound.yml
printf "  auth-config:\n    type: basic\n    username: elastic\n    password-file: /etc/aerospike-elasticsearch-outbound/password.conf\n" >> /etc/aerospike-elasticsearch-outbound/aerospike-elasticsearch-outbound.yml
printf "elastic" > /etc/aerospike-elasticsearch-outbound/password.conf
printf "  tls-config:\n    trust-store:\n      store-file: /etc/aerospike-elasticsearch-outbound/truststore.pkcs12\n      store-password-file: /etc/aerospike-elasticsearch-outbound/password.conf\n" >> /etc/aerospike-elasticsearch-outbound/aerospike-elasticsearch-outbound.yml
printf "doc-id:\n  source: digest\n" >> /etc/aerospike-elasticsearch-outbound/aerospike-elasticsearch-outbound.yml
`
	if isDocker {
		script = script + `mkdir -p /opt/autoload
echo 'nohup /opt/aerospike-elasticsearch-outbound/bin/aerospike-elasticsearch-outbound -f /etc/aerospike-elasticsearch-outbound/aerospike-elasticsearch-outbound.yml > /var/log/aerospike-elasticsearch.log 2>&1 &' > /opt/autoload/02-elasticsearch-connector
chmod 755 /opt/autoload/*
`
	} else {
		script = script + `mkdir -p /opt/autoload
echo 'systemctl start aerospike-elasticsearch-outbound' > /opt/autoload/02-elasticsearch-connector
chmod 755 /opt/autoload/*
`
	}
	script = script + `echo 'sysctl -w vm.max_map_count=262144; su - elasticsearch -c "/usr/share/elasticsearch/bin/elasticsearch -d"' > /opt/autoload/01-elasticsearch; chmod 755 /opt/autoload/*
`
	return script
}

func (c *clientAddElasticSearchCmd) installScriptMasterNode() string {
	return `
keytool -importcert -storetype PKCS12 -keystore /etc/aerospike-elasticsearch-outbound/truststore.pkcs12 -storepass elastic -alias ca -file /etc/elasticsearch/certs/http_ca.crt -noprompt
sed -i 's/#network.host: .*/network.host: 0.0.0.0/g' /etc/elasticsearch/elasticsearch.yml
sed -i 's/#transport.host: .*/transport.host: 0.0.0.0/g' /etc/elasticsearch/elasticsearch.yml
sed -i 's/#http.host: .*/http.host: 0.0.0.0/g' /etc/elasticsearch/elasticsearch.yml
/bin/bash /opt/autoload/01-elasticsearch
printf "elastic\nelastic\n" |/usr/share/elasticsearch/bin/elasticsearch-reset-password -u elastic -f -i -b
`
}

func (c *clientAddElasticSearchCmd) installScriptSlaveNodesOnSlaves(tokenFromMaster string, cert string) string {
	return fmt.Sprintf(`
cat <<'EOF' > /root/cert.b64
%s
EOF
cat /root/cert.b64 |base64 -d > /etc/aerospike-elasticsearch-outbound/truststore.pkcs12
echo "y" |/usr/share/elasticsearch/bin/elasticsearch-reconfigure-node --enrollment-token %s
/bin/bash /opt/autoload/01-elasticsearch
`, strings.Trim(cert, "\r\t\n "), strings.Trim(tokenFromMaster, "\r\t\n "))
}

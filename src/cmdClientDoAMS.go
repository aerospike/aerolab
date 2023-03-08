package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type clientCreateAMSCmd struct {
	clientCreateBaseCmd
	ConnectClusters TypeClusterName `short:"s" long:"clusters" default:"mydc" description:"comma-separated list of clusters to configure as source for this AMS"`
	chDirCmd
}

type clientAddAMSCmd struct {
	ClientName  TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines    TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	StartScript flags.Filename `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	osSelectorCmd
	nodes           map[string][]string // destination map[cluster][]nodeIPs
	Aws             clientAddAMSCmdAws  `no-flag:"true"`
	ConnectClusters TypeClusterName     `short:"s" long:"clusters" default:"" description:"comma-separated list of clusters to configure as source for this AMS"`
	Help            helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientAddAMSCmdAws struct {
	IsArm bool `long:"arm" hidden:"true" description:"indicate installing on an arm instance"`
}

func init() {
	addBackendSwitch("client.add.ams", "aws", &a.opts.Client.Add.AMS.Aws)
}

func (c *clientCreateAMSCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroName != TypeDistro("ubuntu") || (c.DistroVersion != TypeDistroVersion("22.04") && c.DistroVersion != TypeDistroVersion("latest")) {
		return fmt.Errorf("AMS is only supported on ubuntu:22.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	nodes, err := c.checkClustersExist(c.ConnectClusters.String())
	if err != nil {
		return err
	}
	machines, err := c.createBase(args, "ams")
	if err != nil {
		return err
	}
	a.opts.Client.Add.AMS.nodes = nodes
	a.opts.Client.Add.AMS.ClientName = c.ClientName
	a.opts.Client.Add.AMS.StartScript = c.StartScript
	a.opts.Client.Add.AMS.Machines = TypeMachines(intSliceToString(machines, ","))
	a.opts.Client.Add.AMS.ConnectClusters = c.ConnectClusters
	a.opts.Client.Add.AMS.Aws.IsArm = c.Aws.IsArm
	a.opts.Client.Add.AMS.DistroName = c.DistroName
	a.opts.Client.Add.AMS.DistroVersion = c.DistroVersion
	return a.opts.Client.Add.AMS.addAMS(args)
}

func (c *clientAddAMSCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroName != TypeDistro("ubuntu") || (c.DistroVersion != TypeDistroVersion("22.04") && c.DistroVersion != TypeDistroVersion("latest")) {
		return fmt.Errorf("AMS is only supported on ubuntu:22.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	nodes, err := c.checkClustersExist(c.ConnectClusters.String())
	if err != nil {
		return err
	}
	c.nodes = nodes
	return c.addAMS(args)
}

// return map[clusterName][]nodeIPs
func (c *clientCreateAMSCmd) checkClustersExist(clusters string) (map[string][]string, error) {
	return a.opts.Client.Add.AMS.checkClustersExist(clusters)
}

func (c *clientConfigureAMSCmd) checkClustersExist(clusters string) (map[string][]string, error) {
	return a.opts.Client.Add.AMS.checkClustersExist(clusters)
}

// return map[clusterName][]nodeIPs
func (c *clientAddAMSCmd) checkClustersExist(clusters string) (map[string][]string, error) {
	cnames := []string{}
	clusters = strings.Trim(clusters, "\r\n\t ")
	if len(clusters) > 0 {
		cnames = strings.Split(clusters, ",")
	}
	ret := make(map[string][]string)
	clist, err := b.ClusterList()
	if err != nil {
		return nil, err
	}
	// first pass check clusters exist
	for _, cname := range cnames {
		if !inslice.HasString(clist, cname) {
			return nil, fmt.Errorf("cluster %s does not exist", cname)
		}
	}
	// 2nd pass enumerate node IPs
	for _, cname := range cnames {
		ips, err := b.GetClusterNodeIps(cname)
		if err != nil {
			return nil, err
		}
		ret[cname] = ips
	}
	return ret, nil
}

func (c *clientAddAMSCmd) addAMS(args []string) error {
	b.WorkOnClients()
	a.opts.Attach.Client.ClientName = c.ClientName
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	a.opts.Attach.Client.Machine = c.Machines

	// install:prometheus
	err := a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "apt-get update && apt-get -y install prometheus"})
	if err != nil {
		return fmt.Errorf("failed to install prometheus: %s", err)
	}
	// install:grafana
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "apt-get -y install apt-transport-https software-properties-common wget && wget -q -O /usr/share/keyrings/grafana.key https://packages.grafana.com/gpg.key && echo \"deb [signed-by=/usr/share/keyrings/grafana.key] https://packages.grafana.com/oss/deb stable main\" > /etc/apt/sources.list.d/grafana.list && apt-get update && apt-get -y install grafana"})
	if err != nil {
		return fmt.Errorf("failed to install grafana: %s", err)
	}
	// configure:prometheus
	err = a.opts.Attach.Client.run([]string{"wget", "-q", "-O", "/etc/prometheus/aerospike_rules.yaml", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/prometheus/aerospike_rules.yml"})
	if err != nil {
		return fmt.Errorf("failed to configure prometheus (dl rules file): %s", err)
	}
	err = a.opts.Attach.Client.run([]string{"sed", "-i.bak", "-E", "s/^rule_files:/rule_files:\\n  - \"\\/etc\\/prometheus\\/aerospike_rules.yaml\"/g", "/etc/prometheus/prometheus.yml"})
	if err != nil {
		return fmt.Errorf("failed to configure prometheus (sed1): %s", err)
	}
	err = a.opts.Attach.Client.run([]string{"sed", "-i.bak2", "-E", "s/^scrape_configs:/scrape_configs:\\n  - job_name: aerospike\\n    static_configs:\\n      - targets: [] #TODO_ASD_TARGETS\\n/g", "/etc/prometheus/prometheus.yml"})
	if err != nil {
		return fmt.Errorf("failed to configure prometheus (sed2): %s", err)
	}
	allnodes := []string{}
	for _, nodes := range c.nodes {
		for _, node := range nodes {
			allnodes = append(allnodes, node+":9145")
		}
	}
	ips := "'" + strings.Join(allnodes, "','") + "'"
	err = a.opts.Attach.Client.run([]string{"sed", "-i.bak3", "-E", "s/.*TODO_ASD_TARGETS/      - targets: [" + ips + "] #TODO_ASD_TARGETS/g", "/etc/prometheus/prometheus.yml"})
	if err != nil {
		return fmt.Errorf("failed to configure prometheus (sed3): %s", err)
	}
	// configure:grafana
	err = a.opts.Attach.Client.run([]string{"chmod", "664", "/etc/grafana/grafana.ini"})
	if err != nil {
		return fmt.Errorf("failed to configure grafana (chmod): %s", err)
	}
	err = a.opts.Attach.Client.run([]string{"sed", "-i.bak", "-E", "s/^\\[paths\\]$/[paths]\\nprovisioning = \\/etc\\/grafana\\/provisioning/g", "/etc/grafana/grafana.ini"})
	if err != nil {
		return fmt.Errorf("failed to configure grafana (sed ini): %s", err)
	}
	err = a.opts.Attach.Client.run([]string{"grafana-cli", "plugins", "install", "camptocamp-prometheus-alertmanager-datasource"})
	if err != nil {
		return fmt.Errorf("failed to configure grafana (plugin install): %s", err)
	}
	err = a.opts.Attach.Client.run([]string{"wget", "-q", "-O", "/etc/grafana/provisioning/datasources/all.yaml", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/provisioning/datasources/all.yaml"})
	if err != nil {
		return fmt.Errorf("failed to configure grafana (wget datasource): %s", err)
	}
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "echo -e '  - name: Loki\n    type: loki\n    access: proxy\n    url: http://localhost:3100\n    jsonData:\n      maxLines: 1000\n' >> /etc/grafana/provisioning/datasources/all.yaml"})
	if err != nil {
		return fmt.Errorf("failed to add loki to grafana: %s", err)
	}
	err = a.opts.Attach.Client.run([]string{"sed", "-i.bak", "s/prometheus:9090/127.0.0.1:9090/g", "/etc/grafana/provisioning/datasources/all.yaml"})
	if err != nil {
		return fmt.Errorf("failed to configure grafana (sed datasource): %s", err)
	}
	dashboards := [][]string{
		{"mkdir", "-p", "/var/lib/grafana/dashboards"},
		{"wget", "-q", "-O", "/var/lib/grafana/dashboards/alerts.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/alerts.json"},
		{"wget", "-q", "-O", "/var/lib/grafana/dashboards/cluster.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/cluster.json"},
		{"wget", "-q", "-O", "/var/lib/grafana/dashboards/exporters.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/exporters.json"},
		{"wget", "-q", "-O", "/var/lib/grafana/dashboards/latency.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/latency.json"},
		{"wget", "-q", "-O", "/var/lib/grafana/dashboards/namespace.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/namespace.json"},
		{"wget", "-q", "-O", "/var/lib/grafana/dashboards/node.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/node.json"},
		{"wget", "-q", "-O", "/var/lib/grafana/dashboards/users.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/users.json"},
		{"wget", "-q", "-O", "/var/lib/grafana/dashboards/xdr.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/xdr.json"},
		{"wget", "-q", "-O", "/var/lib/grafana/dashboards/asbench.json", "https://raw.githubusercontent.com/aerospike/aerolab/master/scripts/asbench2.json"},
	}
	for _, dashboard := range dashboards {
		err = a.opts.Attach.Client.run(dashboard)
		if err != nil {
			return fmt.Errorf("failed to configure grafana (%v): %s", dashboard, err)
		}
	}
	// expand nodes and install dashboards yaml initializer
	f, err := os.CreateTemp("", "aerolab-ams.yaml")
	if err != nil {
		return err
	}
	_, err = f.WriteString(c.dashboardsYaml())
	if err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	fName := f.Name()
	f.Close()
	defer os.Remove(fName)
	a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
	a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
	a.opts.Files.Upload.Files.Source = flags.Filename(fName)
	a.opts.Files.Upload.Files.Destination = flags.Filename("/etc/grafana/provisioning/dashboards/dashboards.yaml")
	a.opts.Files.Upload.IsClient = true
	err = a.opts.Files.Upload.runUpload(args)
	if err != nil {
		return err
	}
	// (re)start prometheus
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "service prometheus stop; sleep 2; service prometheus start"})
	if err != nil {
		return fmt.Errorf("failed to restart prometheus: %s", err)
	}
	// (re)start grafana
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "chmod 777 /etc/grafana/provisioning/dashboards/dashboards.yaml; [ ! -f /.dockerenv ] && systemctl daemon-reload && systemctl enable grafana-server; service grafana-server start; sleep 3; pidof grafana; exit $?"})
	if err != nil {
		return fmt.Errorf("failed to restart grafana: %s", err)
	}

	// install ams startup script
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "mkdir -p /opt/autoload; echo 'service prometheus start' > /opt/autoload/01-prometheus; echo 'service grafana-server start' > /opt/autoload/02-grafana; chmod 755 /opt/autoload/*"})
	if err != nil {
		return fmt.Errorf("failed to install startup script: %s", err)
	}

	// loki - install
	b.WorkOnClients()
	var nodes []int
	err = c.Machines.ExpandNodes(string(c.ClientName))
	if err != nil {
		return err
	}
	nodesList, err := b.NodeListInCluster(string(c.ClientName))
	if err != nil {
		return err
	}
	if c.Machines == "" {
		nodes = nodesList
	} else {
		for _, nodeString := range strings.Split(c.Machines.String(), ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				return err
			}
			if !inslice.HasInt(nodesList, nodeInt) {
				return fmt.Errorf("node %d does not exist in cluster", nodeInt)
			}
			nodes = append(nodes, nodeInt)
		}
	}
	if len(nodes) == 0 {
		err = errors.New("found 0 nodes in cluster")
		return err
	}
	// arm fill
	isArm := c.Aws.IsArm
	if a.opts.Config.Backend.Type == "docker" {
		if b.Arch() == TypeArchArm {
			isArm = true
		} else {
			isArm = false
		}
	}
	lokiScript, lokiSize := installLokiScript(isArm)
	err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{filePath: "/opt/install-loki.sh", fileContents: strings.NewReader(lokiScript), fileSize: lokiSize}}, nodes)
	if err != nil {
		return fmt.Errorf("failed to install loki download script: %s", err)
	}
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "/opt/install-loki.sh"})
	if err != nil {
		return fmt.Errorf("failed to install loki: %s", err)
	}

	// install loki startup script
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "mkdir -p /opt/autoload; echo 'nohup /usr/bin/loki -config.file=/etc/loki/loki.yaml -log-config-reverse-order > /var/log/loki.log 2>&1 &' > /opt/autoload/03-loki; chmod 755 /opt/autoload/*"})
	if err != nil {
		return fmt.Errorf("failed to install loki startup script: %s", err)
	}

	// start loki
	a.opts.Attach.Client.Detach = true
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "/opt/autoload/03-loki"})
	if err != nil {
		return fmt.Errorf("failed to restart loki: %s", err)
	}

	// install early/late scripts
	if string(c.StartScript) != "" {
		a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
		a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
		a.opts.Files.Upload.Files.Source = flags.Filename(c.StartScript)
		a.opts.Files.Upload.Files.Destination = flags.Filename("/usr/local/bin/start.sh")
		a.opts.Files.Upload.IsClient = true
		err = a.opts.Files.Upload.runUpload(args)
		if err != nil {
			return err
		}
	}
	log.Printf("To access grafana, visit the client IP on port 3000 from your browser. Do `aerolab client list` to get IPs. Username:Password is admin:admin")
	log.Print("Done")
	log.Print("NOTE: Remember to install the aerospike-prometheus-exporter on the Aerospike server nodes, using `aerolab cluster add exporter` command")
	if a.opts.Config.Backend.Type == "aws" {
		log.Print("NOTE: if allowing for AeroLab to manage AWS Security Group, if not already done so, consider restricting access by using: aerolab config aws lock-security-groups")
	}
	return nil
}

func (c *clientAddAMSCmd) dashboardsYaml() string {
	return `apiVersion: 1

providers:
- name: 'default'
  orgId: 1
  folder: ''
  type: file
  disableDeletion: false
  editable: true
  allowUiUpdates: true
  options:
    path: /var/lib/grafana/dashboards
`
}

func installLokiScript(isArm bool) (script string, size int) {
	arch := "amd64"
	if isArm {
		arch = "arm64"
	}
	script = `apt-get update && apt-get -y install wget curl unzip || exit 1
cd /root
wget https://github.com/grafana/loki/releases/download/v2.5.0/loki-linux-` + arch + `.zip || exit 1
unzip loki-linux-` + arch + `.zip || exit 1
mv loki-linux-` + arch + ` /usr/bin/loki || exit 1
wget https://github.com/grafana/loki/releases/download/v2.5.0/logcli-linux-` + arch + `.zip || exit 1
unzip logcli-linux-` + arch + `.zip || exit 1
mv logcli-linux-` + arch + ` /usr/bin/logcli || exit 1
chmod 755 /usr/bin/logcli /usr/bin/loki || exit 1
mkdir -p /etc/loki /data-logs/loki
cat <<'EOF' > /etc/loki/loki.yaml
auth_enabled: false
server:
  http_listen_port: 3100
  grpc_listen_port: 9096
common:
  path_prefix: /data-logs/loki
  storage:
    filesystem:
      chunks_directory: /data-logs/loki/chunks
      rules_directory: /data-logs/loki/rules
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory
schema_config:
  configs:
    - from: 2020-10-24
      store: boltdb-shipper
      object_store: filesystem
      schema: v11
      index:
        prefix: index_
        period: 24h
analytics:
  reporting_enabled: false
EOF
`
	size = len(script)
	return
}

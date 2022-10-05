package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bestmethod/inslice"
	"github.com/jessevdk/go-flags"
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
	ConnectClusters TypeClusterName     `short:"s" long:"clusters" default:"mydc" description:"comma-separated list of clusters to configure as source for this AMS"`
	Help            helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientAddAMSCmdAws struct {
	IsArm bool `long:"arm" description:"indicate installing on an arm instance"`
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
		allnodes = append(allnodes, nodes...)
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
	err = a.opts.Attach.Client.run([]string{"sed", "-i.bak", "s/prometheus:9090/127.0.0.1:9090/g", "/etc/grafana/provisioning/datasources/all.yaml"})
	if err != nil {
		return fmt.Errorf("failed to configure grafana (sed datasource): %s", err)
	}
	dashboards := [][]string{
		[]string{"mkdir", "-p", "/var/lib/grafana/dashboards"},
		[]string{"wget", "-q", "-O", "/var/lib/grafana/dashboards/alerts.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/alerts.json"},
		[]string{"wget", "-q", "-O", "/var/lib/grafana/dashboards/cluster.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/cluster.json"},
		[]string{"wget", "-q", "-O", "/var/lib/grafana/dashboards/exporters.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/exporters.json"},
		[]string{"wget", "-q", "-O", "/var/lib/grafana/dashboards/latency.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/latency.json"},
		[]string{"wget", "-q", "-O", "/var/lib/grafana/dashboards/namespace.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/namespace.json"},
		[]string{"wget", "-q", "-O", "/var/lib/grafana/dashboards/node.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/node.json"},
		[]string{"wget", "-q", "-O", "/var/lib/grafana/dashboards/users.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/users.json"},
		[]string{"wget", "-q", "-O", "/var/lib/grafana/dashboards/xdr.json", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/dashboards/xdr.json"},
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
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "chmod 777 /etc/grafana/provisioning/dashboards/dashboards.yaml; which systemctl; [ $? -eq 0 ] && systemctl daemon-reload && systemctl enable grafana-server; service grafana-server start; sleep 5; service grafana-server stop; sleep 5; service grafana-server start"})
	if err != nil {
		return fmt.Errorf("failed to restart grafana: %s", err)
	}

	// install ams startup script
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "mkdir -p /opt/autoload; echo 'service prometheus start' > /opt/autoload/01-prometheus; echo 'service grafana-server start' > /opt/autoload/02-grafana; chmod 755 /opt/autoload/*"})
	if err != nil {
		return fmt.Errorf("failed to install startup script: %s", err)
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
	log.Print("Done")
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

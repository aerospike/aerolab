package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/parallelize"
	"github.com/bestmethod/inslice"
)

type clientConfigureToolsCmd struct {
	ClientName TypeClientName  `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines   TypeMachines    `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	ConnectAMS TypeClusterName `short:"m" long:"ams" default:"ams" description:"AMS client machine name"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureToolsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running client.configure.tools")
	a.opts.Attach.Client.ClientName = c.ClientName
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	a.opts.Attach.Client.Machine = c.Machines
	b.WorkOnClients()
	nodeList, err := c.checkClustersExist(c.ConnectAMS.String())
	if err != nil {
		return err
	}
	allnodes := []string{}
	for _, nodes := range nodeList {
		for _, node := range nodes {
			allnodes = append(allnodes, node+":3100")
		}
	}
	if len(allnodes) == 0 {
		return errors.New("found 0 AMS machines")
	}
	if len(allnodes) > 1 {
		log.Printf("Found more than 1 AMS machine, will point log consolidator at the first one: %s", allnodes[0])
	}
	ip := allnodes[0] // this will have ip:3100
	// resolve tools nodes list
	err = c.Machines.ExpandNodes(c.ClientName.String())
	if err != nil {
		return fmt.Errorf("could not expand node list: %s", err)
	}
	tnodes := []int{}
	for _, tnode := range strings.Split(c.Machines.String(), ",") {
		no, _ := strconv.Atoi(tnode)
		tnodes = append(tnodes, no)
	}
	returns := parallelize.MapLimit(tnodes, c.ParallelThreads, func(tnode int) error {
		// store IP on tools nodes
		err = b.CopyFilesToCluster(c.ClientName.String(), []fileList{{filePath: "/opt/asbench-grafana.ip", fileContents: ip, fileSize: len(ip)}}, []int{tnode})
		if err != nil {
			return fmt.Errorf("could not upload file 1: %s", err)
		}
		// arm fill
		isArm := false
		if a.opts.Config.Backend.Type == "docker" {
			if b.Arch() == TypeArchArm {
				isArm = true
			} else {
				isArm = false
			}
		} else {
			// login to node to work out if it's arm
			out, err := b.RunCommands(c.ClientName.String(), [][]string{{"uname", "-p"}}, []int{tnode})
			if err != nil {
				return fmt.Errorf("could not extablish node architecture: %s; %s", err, string(out[0]))
			}
			if strings.Contains(string(out[0]), "arm") || strings.Contains(string(out[0]), "aarch") {
				isArm = true
			}
		}
		// install promtail if not found
		promScript := promTailScript(isArm)
		err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{filePath: "/opt/install-promtail.sh", fileContents: promScript, fileSize: len(promScript)}}, []int{tnode})
		if err != nil {
			return fmt.Errorf("failed to install loki download script: %s", err)
		}
		// install
		out, err := b.RunCommands(c.ClientName.String(), [][]string{{"/bin/bash", "/opt/install-promtail.sh"}}, []int{tnode})
		if err != nil {
			if len(out) > 0 {
				return fmt.Errorf("%s :: %s", err, string(out[0]))
			} else {
				return err
			}
		}
		// install promtail config file
		promScript = promTailConf()
		err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{filePath: "/opt/configure-promtail.sh", fileContents: promScript, fileSize: len(promScript)}}, []int{tnode})
		if err != nil {
			return fmt.Errorf("failed to install conf script: %s", err)
		}
		out, err = b.RunCommands(c.ClientName.String(), [][]string{{"/bin/bash", "/opt/configure-promtail.sh"}}, []int{tnode})
		if err != nil {
			if len(out) > 0 {
				return fmt.Errorf("%s :: %s", err, string(out[0]))
			} else {
				return err
			}
		}
		// install promtail startup script
		out, err = b.RunCommands(c.ClientName.String(), [][]string{{"/bin/bash", "-c", "mkdir -p /opt/autoload; echo 'nohup /usr/bin/promtail -config.file=/etc/promtail/promtail.yaml -log-config-reverse-order > /var/log/promtail.log 2>&1 &' > /opt/autoload/10-promtail; chmod 755 /opt/autoload/*"}}, []int{tnode})
		if err != nil {
			if len(out) > 0 {
				return fmt.Errorf("%s :: %s", err, string(out[0]))
			} else {
				return err
			}
		}
		// kill promtail
		b.RunCommands(c.ClientName.String(), [][]string{{"pkill", "-9", "promtail"}}, []int{tnode})
		return nil
	})
	isError := false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %d returned %s", tnodes[i], ret)
			isError = true
		}
	}
	if isError {
		return errors.New("some nodes returned errors")
	}

	// (re)start promtail
	defer backendRestoreTerminal()
	a.opts.Attach.Client.Detach = true
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "/opt/autoload/10-promtail"})
	if err != nil {
		return fmt.Errorf("failed to restart promtail: %s", err)
	}
	backendRestoreTerminal()
	log.Print("Done")
	return nil
}

func promTailConf() string {
	return `cat <<EOF > /etc/promtail/promtail.yaml
server:
  http_listen_port: 9080
  grpc_listen_port: 0
positions:
  filename: /var/promtail/positions.yaml
clients:
  - url: http://$(cat /opt/asbench-grafana.ip)/loki/api/v1/push
scrape_configs:
  - job_name: asbench
    static_configs:
      - targets:
          - localhost
        labels:
          job: asbench
          __path__: /var/log/asbench_*.log
          host: $(hostname)
    pipeline_stages:
      - match:
          selector: '{job="asbench"}'
          stages:
          - regex:
              source: filename
              expression: "/var/log/asbench_(?P<instance>.*)\\\\.log"
          - labels:
              instance:

EOF
`
}

func promTailScript(isArm bool) string {
	arch := "amd64"
	if isArm {
		arch = "arm64"
	}
	script := `apt-get update
apt-get -y install unzip 
cd /root
wget https://github.com/grafana/loki/releases/download/v2.5.0/promtail-linux-` + arch + `.zip
unzip promtail-linux-` + arch + `.zip
mv promtail-linux-` + arch + ` /usr/bin/promtail
chmod 755 /usr/bin/promtail
mkdir -p /etc/promtail /var/promtail
`
	return script
}

// return map[clusterName][]nodeIPs
func (c *clientConfigureToolsCmd) checkClustersExist(clusters string) (map[string][]string, error) {
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

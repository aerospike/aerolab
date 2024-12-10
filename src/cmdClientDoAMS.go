package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/parallelize"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
	"gopkg.in/yaml.v3"
)

type clientCreateAMSCmd struct {
	clientCreateBaseCmd
	ConnectClusters TypeClusterName `short:"s" long:"clusters" description:"comma-separated list of clusters to configure as source for this AMS"`
	ConnectClients  TypeClientName  `short:"S" long:"clients" description:"comma-separated list of (graph) clients to configure as source for this AMS"`
	ConnectVector   TypeClientName  `short:"V" long:"vector" description:"comma-separated list of vector clients to configure as source for this AMS"`
	Dashboards      flags.Filename  `long:"dashboards" description:"dashboards list file, see https://github.com/aerospike/aerolab/blob/master/docs/usage/monitoring/dashboards.md"`
	DebugDashboards bool            `long:"debug-dashboards" hidden:"true"`
	JustDoIt        bool            `long:"confirm" description:"set this parameter to confirm any warning questions without being asked to press ENTER to continue" webdisable:"true" webset:"true"`
	chDirCmd
}

type clientAddAMSCmd struct {
	ClientName  TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines    TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	StartScript flags.Filename `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	osSelectorCmd
	nodes           map[string][]string // destination map[cluster][]nodeIPs
	clients         map[string][]string // destination map[cluster][]nodeIPs
	vector          map[string][]string
	ConnectClusters TypeClusterName `short:"s" long:"clusters" default:"" description:"comma-separated list of clusters to configure as source for this AMS"`
	ConnectClients  TypeClientName  `short:"S" long:"clients" description:"comma-separated list of clients to configure as source for this AMS"`
	ConnectVector   TypeClientName  `short:"V" long:"vector" description:"comma-separated list of vector clients to configure as source for this AMS"`
	Dashboards      flags.Filename  `long:"dashboards" description:"dashboards list file, see https://github.com/aerospike/aerolab/blob/master/docs/usage/monitoring/dashboards.md"`
	parallelThreadsCmd
	DebugDashboards bool               `long:"debug-dashboards" hidden:"true"`
	Aws             clientAddAMSCmdAws `no-flag:"true"`
	Gcp             clientAddAMSCmdAws `no-flag:"true"`
	Help            helpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientAddAMSCmdAws struct {
	IsArm bool `long:"arm" hidden:"true" description:"indicate installing on an arm instance"`
}

func init() {
	addBackendSwitch("client.add.ams", "aws", &a.opts.Client.Add.AMS.Aws)
	addBackendSwitch("client.add.ams", "gcp", &a.opts.Client.Add.AMS.Gcp)
}

func (c *clientCreateAMSCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type == "docker" && !strings.Contains(c.Docker.ExposePortsToHost, ":3000") {
		if c.Docker.NoAutoExpose {
			fmt.Println("Docker backend is in use, but AMS access port is not being forwarded. If using Docker Desktop, use '-e 3000:3000' parameter in order to forward port 3000 for grafana. This can only be done for one system. Press ENTER to continue regardless.")
			if !c.JustDoIt {
				var ignoreMe string
				fmt.Scanln(&ignoreMe)
			}
		} else {
			c.Docker.ExposePortsToHost = strings.Trim("3000:3000,"+c.Docker.ExposePortsToHost, ",")
		}
	}
	if c.DistroVersion == "latest" {
		c.DistroVersion = "24.04"
	}
	if c.DistroName != TypeDistro("ubuntu") || (c.DistroVersion != TypeDistroVersion("24.04") && c.DistroVersion != TypeDistroVersion("latest")) {
		return fmt.Errorf("AMS is only supported on ubuntu:24.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	custom := []CustomAMSDashboard{}
	if c.Dashboards != "" {
		cDashFile, err := os.ReadFile(strings.TrimPrefix(string(c.Dashboards), "+"))
		if err != nil {
			return err
		}
		err = yaml.Unmarshal(cDashFile, &custom)
		if err != nil {
			return err
		}
	}
	var nodeList map[string][]string
	var clientList map[string][]string
	var vectorList map[string][]string
	var err error
	if c.ConnectClusters != "" {
		nodeList, err = c.checkClustersExist(c.ConnectClusters.String())
		if err != nil {
			return err
		}
	}
	if c.ConnectClients != "" {
		b.WorkOnClients()
		clientList, err = c.checkClustersExist(c.ConnectClients.String())
		if err != nil {
			return err
		}
	}
	if c.ConnectVector != "" {
		b.WorkOnClients()
		vectorList, err = c.checkClustersExist(c.ConnectVector.String())
		if err != nil {
			return err
		}
	}
	machines, err := c.createBase(args, "ams")
	if err != nil {
		return err
	}
	if c.PriceOnly {
		return nil
	}

	a.opts.Client.Add.AMS.nodes = nodeList
	a.opts.Client.Add.AMS.clients = clientList
	a.opts.Client.Add.AMS.vector = vectorList
	a.opts.Client.Add.AMS.ClientName = c.ClientName
	a.opts.Client.Add.AMS.StartScript = c.StartScript
	a.opts.Client.Add.AMS.Machines = TypeMachines(intSliceToString(machines, ","))
	a.opts.Client.Add.AMS.ConnectClusters = c.ConnectClusters
	a.opts.Client.Add.AMS.Aws.IsArm = c.Aws.IsArm
	a.opts.Client.Add.AMS.Gcp.IsArm = c.Gcp.IsArm
	a.opts.Client.Add.AMS.DistroName = c.DistroName
	a.opts.Client.Add.AMS.DistroVersion = c.DistroVersion
	a.opts.Client.Add.AMS.Dashboards = c.Dashboards
	a.opts.Client.Add.AMS.ParallelThreads = c.ParallelThreads
	a.opts.Client.Add.AMS.DebugDashboards = c.DebugDashboards
	return a.opts.Client.Add.AMS.addAMS(args)
}

func (c *clientAddAMSCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.DistroVersion == "latest" {
		c.DistroVersion = "24.04"
	}
	if c.DistroName != TypeDistro("ubuntu") || (c.DistroVersion != TypeDistroVersion("24.04") && c.DistroVersion != TypeDistroVersion("latest")) {
		return fmt.Errorf("AMS is only supported on ubuntu:24.04, selected %s:%s", c.DistroName, c.DistroVersion)
	}
	if c.ConnectClusters != "" {
		nodes, err := c.checkClustersExist(c.ConnectClusters.String())
		if err != nil {
			return err
		}
		c.nodes = nodes
	}
	if c.ConnectClients != "" {
		b.WorkOnClients()
		clients, err := c.checkClustersExist(c.ConnectClients.String())
		if err != nil {
			return err
		}
		c.clients = clients
	}
	if c.ConnectVector != "" {
		b.WorkOnClients()
		vectorList, err := c.checkClustersExist(c.ConnectVector.String())
		if err != nil {
			return err
		}
		c.vector = vectorList
	}
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

type GitHubDirEntry struct {
	Name        string `json:"name"`         // ends with ".json" - file name to store as; or folder name to explore (/ add to suffix of Path)
	Path        string `json:"path"`         // ends with ".json" for file; store name/path, stripping the 'config/grafana/dashboards/' first
	Size        int    `json:"size"`         // not zero = file, zero = folder
	DownloadURL string `json:"download_url"` // not empty = file, empty = folder
	Type        string `json:"type"`         // dir or file
}

type CustomAMSDashboard struct {
	Destination string  `yaml:"destination"`
	FromFile    *string `yaml:"fromFile,omitempty"`
	FromUrl     *string `yaml:"fromUrl,omitempty"`
}

func GitHubBuildDashboardList(baseUrl string, basePath string, currentPath string) (mkdirs [][]string, dashboards [][]string, err error) {
	GitHubDirList := []GitHubDirEntry{}
	req, err := http.NewRequest("GET", baseUrl+currentPath, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Add("Accept", "application/vnd.github+json")
	req.Header.Add("X-GitHub-Api-Version", "2022-11-28")
	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		err = fmt.Errorf("GET '%s': exit code (%d)", baseUrl+currentPath, response.StatusCode)
		return nil, nil, err
	}
	err = json.NewDecoder(response.Body).Decode(&GitHubDirList)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range GitHubDirList {
		if entry.Size != 0 && strings.HasSuffix(entry.Name, ".json") && entry.Type == "file" {
			newPath := strings.TrimPrefix(entry.Path, basePath)
			dashboards = append(dashboards, []string{"wget", "-q", "-O", "/var/lib/grafana/dashboards" + newPath, entry.DownloadURL})
		} else if entry.Size == 0 && entry.DownloadURL == "" && entry.Type == "dir" {
			newPath := strings.TrimPrefix(entry.Path, basePath)
			mkdirs = append(mkdirs, []string{"mkdir", "-p", "/var/lib/grafana/dashboards" + newPath})
			newMkdir, newDash, err := GitHubBuildDashboardList(baseUrl, basePath, entry.Path)
			if err != nil {
				return nil, nil, err
			}
			dashboards = append(dashboards, newDash...)
			mkdirs = append(mkdirs, newMkdir...)
		}
	}
	return mkdirs, dashboards, nil
}

func (c *clientAddAMSCmd) addAMS(args []string) error {
	b.WorkOnClients()
	a.opts.Attach.Client.ClientName = c.ClientName
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	a.opts.Attach.Client.Machine = c.Machines

	// custom dashboards - read
	customDashboards := []CustomAMSDashboard{}
	if c.Dashboards != "" {
		cDashFile, err := os.ReadFile(strings.TrimPrefix(string(c.Dashboards), "+"))
		if err != nil {
			return err
		}
		err = yaml.Unmarshal(cDashFile, &customDashboards)
		if err != nil {
			return err
		}
	}
	customDashboards = append(customDashboards, CustomAMSDashboard{
		FromUrl:     aws.String("https://raw.githubusercontent.com/aerospike/aerolab/master/scripts/asbench2.json"),
		Destination: "/var/lib/grafana/dashboards/asbench.json",
	})

	b.WorkOnClients()
	var nodes []int
	err := c.Machines.ExpandNodes(string(c.ClientName))
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

	defer backendRestoreTerminal()
	// TODO: upload and run install script
	installScript := c.installScript()
	err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{filePath: "/opt/installer.sh", fileContents: installScript, fileSize: len(installScript)}}, nodesList)
	if err != nil {
		return err
	}
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "/opt/installer.sh"})
	if err != nil {
		return fmt.Errorf("failed to run installer: %s", err)
	}

	allnodes := []string{}
	allnodeExp := []string{}
	allClients := []string{}
	allVector := []string{}
	for _, nodes := range c.nodes {
		for _, node := range nodes {
			allnodes = append(allnodes, node+":9145")
			allnodeExp = append(allnodeExp, node+":9100")
		}
	}
	for _, nodes := range c.clients {
		for _, node := range nodes {
			allClients = append(allClients, node+":9090")
		}
	}
	for _, nodes := range c.vector {
		for _, node := range nodes {
			allVector = append(allVector, node+":5040")
		}
	}
	ips := "'" + strings.Join(allnodes, "','") + "'"
	nips := "'" + strings.Join(allnodeExp, "','") + "'"
	cips := "'" + strings.Join(allClients, "','") + "'"
	vips := "'" + strings.Join(allVector, "','") + "'"
	if len(allnodes) != 0 || len(allnodeExp) != 0 {
		err = a.opts.Attach.Client.run([]string{"sed", "-i.bakAsd", "-E", "s/.*TODO_ASD_TARGETS/      - targets: [" + ips + "] #TODO_ASD_TARGETS/g", "/etc/prometheus/prometheus.yml"})
		if err != nil {
			return fmt.Errorf("failed to configure prometheus (sed3): %s", err)
		}
		err = a.opts.Attach.Client.run([]string{"sed", "-i.bakAsdNode", "-E", "s/.*TODO_ASDN_TARGETS/      - targets: [" + nips + "] #TODO_ASDN_TARGETS/g", "/etc/prometheus/prometheus.yml"})
		if err != nil {
			return fmt.Errorf("failed to configure prometheus (sed3.1): %s", err)
		}
	}
	if len(allClients) != 0 {
		err = a.opts.Attach.Client.run([]string{"sed", "-i.bakGraph", "-E", "s/.*TODO_CLIENT_TARGETS/      - targets: [" + cips + "] #TODO_CLIENT_TARGETS/g", "/etc/prometheus/prometheus.yml"})
		if err != nil {
			return fmt.Errorf("failed to configure prometheus (sed.1): %s", err)
		}
	}
	if len(allVector) != 0 {
		err = a.opts.Attach.Client.run([]string{"sed", "-i.bakVector", "-E", "s/.*TODO_VECTOR_TARGETS/      - targets: [" + vips + "] #TODO_VECTOR_TARGETS/g", "/etc/prometheus/prometheus.yml"})
		if err != nil {
			return fmt.Errorf("failed to configure prometheus (sed.1): %s", err)
		}
	}
	// dashboards
	log.Println("Downloading dashboards...")
	// first make basics
	dashboards := [][]string{
		{"wget", "-q", "-O", "/etc/grafana/provisioning/dashboards/all.yaml", "https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/provisioning/dashboards/all.yaml"},
		{"mkdir", "-p", "/var/lib/grafana/dashboards"},
	}
	for _, dashboard := range dashboards {
		if c.DebugDashboards {
			log.Printf("PRE: running %v", dashboard)
		}
		err = a.opts.Attach.Client.run(dashboard)
		if err != nil {
			return fmt.Errorf("failed to configure grafana (%v): %s", dashboard, err)
		}
	}
	mkdirList := []string{}
	// install default dashboards
	if c.Dashboards == "" || strings.HasPrefix(string(c.Dashboards), "+") {
		baseUrl := "https://api.github.com/repos/aerospike/aerospike-monitoring/contents/"
		currentPath := "config/grafana/dashboards"
		newMkdir, newDash, err := GitHubBuildDashboardList(baseUrl, currentPath, currentPath)
		if err != nil {
			return err
		}
		for _, mkdir := range newMkdir {
			if !inslice.HasString(mkdirList, mkdir[2]) {
				if c.DebugDashboards {
					log.Printf("MKDIR: running %v", mkdir)
				}
				mkdirList = append(mkdirList, mkdir[2])
				err = a.opts.Attach.Client.run(mkdir)
				if err != nil {
					return fmt.Errorf("failed to configure grafana (%v): %s", mkdir, err)
				}
			} else {
				if c.DebugDashboards {
					log.Printf("MKDIR: duplicate, skipping %v", mkdir)
				}
			}
		}
		// this does not need to be sequential, can be done in parallel
		returns := parallelize.MapLimit(newDash, c.ParallelThreads, func(dashboard []string) error {
			if c.DebugDashboards {
				log.Printf("WGET: running %v", dashboard)
			}
			tries := 0
			for {
				err = a.opts.Attach.Client.run(dashboard)
				if err != nil {
					tries++
					if tries == 5 {
						return fmt.Errorf("failed to configure grafana (%v): %s", dashboard, err)
					} else {
						time.Sleep(time.Second * 2)
					}
				} else {
					break
				}
			}
			return nil
		})
		isError := false
		for _, ret := range returns {
			if ret != nil {
				log.Print(ret)
				isError = true
			}
		}
		if isError {
			return errors.New("some commands returned errors")
		}
	}
	// custom dashboards
	if len(customDashboards) > 0 {
		dashboards = [][]string{}
		nFiles := []fileList{}
		for _, custom := range customDashboards {
			fc := []byte{}
			if custom.FromUrl != nil && *custom.FromUrl != "" {
				resp, err := http.Get(*custom.FromUrl)
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode > 299 {
					return fmt.Errorf("URL %s returned %v:%s status code", *custom.FromUrl, resp.StatusCode, resp.Status)
				}
				fc, err = io.ReadAll(resp.Body)
				if err != nil {
					return err
				}
			}
			if custom.FromFile != nil && *custom.FromFile != "" {
				fc, err = os.ReadFile(*custom.FromFile)
				if err != nil {
					return err
				}
			}
			destDir, _ := path.Split(custom.Destination)
			if destDir != "/var/lib/grafana/dashboards" && destDir != "/var/lib/grafana/dashboards/" {
				dashboards = append(dashboards, []string{"mkdir", "-p", destDir})
			}
			nFiles = append(nFiles, fileList{
				filePath:     custom.Destination,
				fileContents: string(fc),
				fileSize:     len(fc),
			})
		}
		for _, mkdir := range dashboards {
			if !inslice.HasString(mkdirList, mkdir[2]) {
				if c.DebugDashboards {
					log.Printf("MKDIR: custom running %v", mkdir)
				}
				mkdirList = append(mkdirList, mkdir[2])
				err = a.opts.Attach.Client.run(mkdir)
				if err != nil {
					return fmt.Errorf("failed to configure grafana (%v): %s", mkdir, err)
				}
			} else {
				if c.DebugDashboards {
					log.Printf("MKDIR: custom duplicate, skipping %v", mkdir)
				}
			}
		}
		// can parallelize
		returns := parallelize.MapLimit(nFiles, c.ParallelThreads, func(nFile fileList) error {
			if c.DebugDashboards {
				log.Printf("UPLOAD: custom uploading %v", nFile.filePath)
			}
			tries := 0
			for {
				err = b.CopyFilesToCluster(c.ClientName.String(), []fileList{nFile}, nodes)
				if err != nil {
					tries++
					if tries == 5 {
						return err
					} else {
						time.Sleep(time.Second * 2)
					}
				} else {
					break
				}
			}
			return nil
		})
		isError := false
		for _, ret := range returns {
			if ret != nil {
				log.Print(ret)
				isError = true
			}
		}
		if isError {
			return errors.New("some commands returned errors")
		}
	}
	log.Println("Installing services...")
	// (re)start prometheus
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "service prometheus stop; sleep 2; service prometheus start"})
	if err != nil {
		return fmt.Errorf("failed to restart prometheus: %s", err)
	}
	// (re)start grafana
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "chmod 777 /etc/grafana/provisioning/dashboards/all.yaml; [ ! -f /.dockerenv ] && systemctl daemon-reload && systemctl enable grafana-server; service grafana-server start; sleep 3; pidof grafana; exit $?"})
	if err != nil {
		return fmt.Errorf("failed to restart grafana: %s", err)
	}

	// install ams startup script
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "mkdir -p /opt/autoload; echo 'service prometheus start' > /opt/autoload/01-prometheus; echo 'service grafana-server start' > /opt/autoload/02-grafana; chmod 755 /opt/autoload/*"})
	if err != nil {
		return fmt.Errorf("failed to install startup script: %s", err)
	}

	// loki - install
	// arm fill
	isArm := c.Aws.IsArm
	if a.opts.Config.Backend.Type == "gcp" {
		isArm = c.Gcp.IsArm
	}
	if a.opts.Config.Backend.Type == "docker" {
		if b.Arch() == TypeArchArm {
			isArm = true
		} else {
			isArm = false
		}
	}
	lokiScript, lokiSize := installLokiScript(isArm)
	err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{filePath: "/opt/install-loki.sh", fileContents: lokiScript, fileSize: lokiSize}}, nodes)
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
		a.opts.Files.Upload.Files.Destination = "/usr/local/bin/start.sh"
		a.opts.Files.Upload.IsClient = true
		a.opts.Files.Upload.doLegacy = true
		err = a.opts.Files.Upload.runUpload(args)
		if err != nil {
			return err
		}
	}
	backendRestoreTerminal()
	log.Printf("Username:Password is admin:admin")
	log.Print("Done")
	log.Print("NOTE: Remember to install the aerospike-prometheus-exporter on the Aerospike server nodes, using `aerolab cluster add exporter` command")
	log.Print("Execute `aerolab inventory list` to get access URL.")
	if a.opts.Config.Backend.Type == "aws" {
		log.Print("NOTE: if allowing for AeroLab to manage AWS Security Group, if not already done so, consider restricting access by using: aerolab config aws lock-security-groups")
	}
	if a.opts.Config.Backend.Type == "gcp" {
		log.Print("NOTE: if not already done so, consider restricting access by using: aerolab config gcp lock-firewall-rules")
	}
	log.Println("WARN: Deprecation notice: the way clients are created and deployed is changing. A new design will be explored during AeroLab's version 7's lifecycle and the current client creation methods will be removed in AeroLab 8.0")
	return nil
}

func installLokiScript(isArm bool) (script string, size int) {
	arch := "amd64"
	if isArm {
		arch = "arm64"
	}
	script = `apt-get update && apt-get -y install wget curl unzip || exit 1
cd /root
wget https://github.com/grafana/loki/releases/download/v3.3.0/loki-linux-` + arch + `.zip || exit 1
unzip loki-linux-` + arch + `.zip || exit 1
mv loki-linux-` + arch + ` /usr/bin/loki || exit 1
wget https://github.com/grafana/loki/releases/download/v3.3.0/logcli-linux-` + arch + `.zip || exit 1
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

func (c *clientAddAMSCmd) installScript() string {
	return `function grafana() {
	set -x
	set -e
	mkdir -p /etc/apt/keyrings/
	wget -q -O - https://apt.grafana.com/gpg.key | gpg --dearmor > /etc/apt/keyrings/grafana.gpg
	echo "deb [signed-by=/etc/apt/keyrings/grafana.gpg] https://apt.grafana.com stable main" > /etc/apt/sources.list.d/grafana.list
	apt-get update
	apt-get -y install grafana
}

function grafana_fallback() {
	set -x
	set -e
    platform=amd64
    [[ $(uname -m) =~ arm ]] && platform=arm64
    [[ $(uname -p) =~ arm ]] && platform=arm64
	apt-get install -y adduser libfontconfig1 musl
	wget https://dl.grafana.com/oss/release/grafana_11.3.1_amd64.deb
	dpkg -i grafana_10.4.1_${platform}.deb
}

set -x
set -e
apt-get update
apt-get -y install prometheus apt-transport-https software-properties-common wget

set +e
ret=$(grafana)
if [ $? -ne 0 ]
then
	echo ${ret}
	echo "WARNING: Grafana failed to install from apt repos, trying manual download"
	ret=$(grafana_fallback)
	if [ $? -ne 0 ]
	then
		echo ${ret}
		exit 1
	fi
fi

set -e
wget -q -O /etc/prometheus/aerospike_rules.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/prometheus/aerospike_rules.yml
sed -i.bak -E 's/^rule_files:/rule_files:\n  - "\/etc\/prometheus\/aerospike_rules.yaml"/g' /etc/prometheus/prometheus.yml
sed -i.bak -E 's/- job_name: node/- job_name: nodelocal/g' /etc/prometheus/prometheus.yml
sed -i.bak -E 's/^scrape_configs:/scrape_configs:\n  - job_name: aerospike\n    static_configs:\n      - targets: [] #TODO_ASD_TARGETS\n  - job_name: node\n    static_configs:\n      - targets: [] #TODO_ASDN_TARGETS\n  - job_name: clients\n    static_configs:\n      - targets: [] #TODO_CLIENT_TARGETS\n  - job_name: vector\n    metrics_path: \/manage\/rest\/v1\/prometheus\n    static_configs:\n      - targets: [] #TODO_VECTOR_TARGETS\n/g' /etc/prometheus/prometheus.yml
chmod 664 /etc/grafana/grafana.ini
sed -i.bak -E 's/^\[paths\]$/[paths]\nprovisioning = \/etc\/grafana\/provisioning/g' /etc/grafana/grafana.ini
grafana-cli plugins install camptocamp-prometheus-alertmanager-datasource
grafana-cli plugins install grafana-polystat-panel
wget -q -O /etc/grafana/provisioning/datasources/all.yaml https://raw.githubusercontent.com/aerospike/aerospike-monitoring/master/config/grafana/provisioning/datasources/all.yaml
echo -e '  - name: Loki\n    type: loki\n    access: proxy\n    url: http://localhost:3100\n    jsonData:\n      maxLines: 1000\n' >> /etc/grafana/provisioning/datasources/all.yaml
sed -i.bak 's/prometheus:9090/127.0.0.1:9090/g' /etc/grafana/provisioning/datasources/all.yaml
`
}

package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/parallelize"
	"github.com/aerospike/aerolab/scripts"
	"github.com/bestmethod/inslice"
)

type clientCreateGraphCmd struct {
	clientCreateNoneCmd
	ClusterName     TypeClusterName `short:"C" long:"cluster-name" description:"cluster name to seed from" default:"mydc"`
	Seed            string          `long:"seed" description:"specify a seed IP:PORT instead of providing a ClusterName; if this parameter is provided, ClusterName is ignored"`
	Namespace       string          `short:"m" long:"namespace" description:"namespace name to configure graph to use" default:"test"`
	ExtraProperties []string        `short:"e" long:"extra" description:"extra properties to add; can be specified multiple times; ex: -e 'aerospike.client.timeout=2000'"`
	AMS             string          `long:"ams" description:"name of an AMS client to add this machine to prometheus configs to"`
	RAMMb           int             `long:"ram-mb" description:"manually specify amount of RAM MiB to use"`
	GraphImage      string          `long:"graph-image" description:"graph is installed using docker images; docker image to use for graph installation" default:"aerospike/aerospike-graph-service"`
	DockerLoginUser string          `long:"docker-user" description:"login to docker registry for graph installation"`
	DockerLoginPass string          `long:"docker-pass" description:"login to docker registry for graph installation"`
	DockerLoginURL  string          `long:"docker-url" description:"login to docker registry for graph installation"`
	JustDoIt        bool            `long:"confirm" description:"set this parameter to confirm any warning questions without being asked to press ENTER to continue" webdisable:"true" webset:"true"`
	seedip          string
	seedport        string
	chDirCmd
}

func (c *clientCreateGraphCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type == "docker" && !strings.Contains(c.Docker.ExposePortsToHost, ":8182") {
		if c.Docker.NoAutoExpose {
			fmt.Println("Docker backend is in use, but graph access port is not being forwarded. If using Docker Desktop, use '-e 8182:8182' parameter in order to forward port 8182. Press ENTER to continue regardless.")
			if !c.JustDoIt {
				var ignoreMe string
				fmt.Scanln(&ignoreMe)
			}
		} else {
			c.Docker.ExposePortsToHost = strings.Trim("8182:8182,"+c.Docker.ExposePortsToHost, ",")
		}
	}
	if c.Seed == "" {
		fmt.Println("Getting cluster list")
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
				c.seedip = ip
				break
			}
		}
		c.seedport = "3000"
		if a.opts.Config.Backend.Type == "docker" {
			inv, err := b.Inventory("", []int{InventoryItemClusters})
			if err != nil {
				return err
			}
			for _, item := range inv.Clusters {
				if item.ClusterName == c.ClusterName.String() {
					if item.PrivateIp != "" && item.DockerExposePorts != "" {
						c.seedport = item.DockerExposePorts
						c.seedip = item.PrivateIp
					}
				}
			}
		}
	} else {
		addr, err := net.ResolveTCPAddr("tcp", c.Seed)
		if err != nil {
			return err
		}
		c.seedport = strconv.Itoa(addr.Port)
		c.seedip = addr.IP.String()
	}
	b.WorkOnClients()
	if c.seedip == "" {
		return errors.New("could not find an IP for a node in the given cluster - are all the nodes down?")
	}

	// AMS section - verify it exists
	if c.AMS != "" {
		clients, err := b.ClusterList()
		if err != nil {
			return err
		}
		if !inslice.HasString(clients, c.AMS) {
			return errors.New("AMS client not found")
		}
		b.WorkOnClients()
	}

	// continue installation
	if a.opts.Config.Backend.Type == "docker" && c.RAMMb == 0 {
		log.Println("For local docker deployment, defaulting to 4G RAM limit.")
		c.RAMMb = 4096
	}
	graphConfig := scripts.GetGraphConfig([]string{c.seedip + ":" + c.seedport}, c.Namespace, c.ExtraProperties, c.RAMMb)

	if a.opts.Config.Backend.Type == "docker" {
		if c.DockerLoginUser != "" && c.DockerLoginPass != "" {
			log.Println("Docker Login...")
			params := []string{"login", "--username", c.DockerLoginUser, "--password", c.DockerLoginPass}
			if c.DockerLoginURL != "" {
				params = append(params, c.DockerLoginURL)
			}
			out, err := exec.Command("docker", params...).CombinedOutput()
			if err != nil {
				return fmt.Errorf("%s\n%s", err, string(out))
			}
		}
		log.Println("Running client.create.graph")
		if c.ClientCount > 1 {
			return errors.New("on docker, only one client can be dpeloyed at a time")
		}
		// install on docker
		curDir, err := os.Getwd()
		if err != nil {
			return err
		}
		confFile := filepath.Join(curDir, "aerospike-graph.properties")
		if _, err := os.Stat(confFile); err == nil {
			log.Printf("WARNING: configuration file %s already exists. Press ENTER to override and continue.", confFile)
			if !c.JustDoIt {
				var ignoreMe string
				fmt.Scanln(&ignoreMe)
			}
		}
		log.Printf("Saving configuration file to %s", confFile)
		err = os.WriteFile(confFile, graphConfig, 0644)
		if err != nil {
			return err
		}
		c.Docker.clientCustomDockerImage = c.GraphImage
		log.Println("Pulling and running dockerized aerospike-graph, this may take a while...")
		machines, err := c.createBase([]string{"-v", confFile + ":/opt/aerospike-graph/aerospike-graph.properties"}, "graph")
		if err != nil {
			return err
		}
		if c.AMS != "" {
			mstring := []string{}
			for _, n := range machines {
				mstring = append(mstring, strconv.Itoa(n))
			}
			err = c.ConfigureAMS(strings.Join(mstring, ","))
			if err != nil {
				return err
			}
		}
		log.Print("Done")
		log.Print("\nCommon tasks and commands:")
		log.Print(" * access gremlin console:          docker run -it --rm tinkerpop/gremlin-console")
		log.Printf(" * access terminal on graph server: aerolab attach client -n %s", c.ClientName)
		log.Print(" * visit https://gdotv.com/ to download a Graph IDE and Visualization tool")
		return nil
	}

	// install on cloud
	machines, err := c.createBase(args, "graph")
	if err != nil {
		return err
	}
	if c.PriceOnly {
		return nil
	}
	log.Println("Continuing graph installation...")
	returns := parallelize.MapLimit(machines, c.ParallelThreads, func(node int) error {
		var dockerLogin *scripts.DockerLogin
		if c.DockerLoginUser != "" && c.DockerLoginPass != "" {
			dockerLogin = &scripts.DockerLogin{
				URL:  c.DockerLoginURL,
				User: c.DockerLoginUser,
				Pass: c.DockerLoginPass,
			}
		}
		graphScript := scripts.GetCloudGraphScript("/etc/aerospike-graph.properties", "", c.GraphImage, dockerLogin)
		err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/etc/aerospike-graph.properties", string(graphConfig), len(graphConfig)}, {"/tmp/install-graph.sh", string(graphScript), len(graphScript)}}, []int{node})
		if err != nil {
			return err
		}
		defer backendRestoreTerminal()
		err = b.AttachAndRun(string(c.ClientName), node, []string{"/bin/bash", "/tmp/install-graph.sh"}, false)
		if err != nil {
			return err
		}
		return nil
	})
	isError := false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %d returned %s", machines[i], ret)
			isError = true
		}
	}
	if isError {
		return errors.New("some nodes returned errors")
	}
	if c.AMS != "" {
		mstring := []string{}
		for _, n := range machines {
			mstring = append(mstring, strconv.Itoa(n))
		}
		err = c.ConfigureAMS(strings.Join(mstring, ","))
		if err != nil {
			return err
		}
	}
	log.Println("Done")
	log.Print("Common tasks and commands:")
	log.Print(" * access gremlin console locally:         docker run -it --rm tinkerpop/gremlin-console")
	log.Printf(" * access gremlin console on graph server: aerolab attach client -n %s -- docker run -it --rm tinkerpop/gremlin-console", c.ClientName)
	log.Printf(" * access terminal on graph server:        aerolab attach client -n %s", c.ClientName)
	log.Print(" * visit https://gdotv.com/ to download a Graph IDE and Visualization tool")
	log.Print("Example creating a dedicated gremlin-console client:")
	log.Print(" * create an empty client: aerolab client create base -n mygremlin [...]")
	log.Print(" * donwload docker script: aerolab client attach -n mygremlin -- curl -fsSL https://get.docker.com -o /tmp/get-docker.sh")
	log.Print(" * install docker        : aerolab client attach -n mygremlin -- bash /tmp/get-docker.sh")
	log.Print(" * run gremlin-console   : aerolab client attach -n mygremlin -- docker run -it --rm tinkerpop/gremlin-console")
	return nil
}

func (c *clientCreateGraphCmd) ConfigureAMS(machines string) error {
	a.opts.Client.Configure.AMS.ClientName = TypeClientName(c.AMS)
	a.opts.Client.Configure.AMS.ConnectClients = c.ClientName
	a.opts.Client.Configure.AMS.Machines = ""
	return a.opts.Client.Configure.AMS.Execute(nil)
}

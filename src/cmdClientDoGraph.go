package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
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
	clientCreateBaseCmd
	ClusterName     TypeClusterName `short:"C" long:"cluster-name" description:"cluster name to seed from" default:"mydc"`
	Namespace       string          `short:"m" long:"namespace" description:"namespace name to configure graph to use" default:"test"`
	ExtraProperties []string        `short:"e" long:"extra" description:"extra properties to add; can be specified multiple times; ex: -e 'aerospike.client.timeout=2000'"`
	RAMMb           int             `long:"ram-mb" description:"manually specify amount of RAM MiB to use; default-docker: 4G; default-cloud: 90pct"`
	JustDoIt        bool            `long:"confirm" description:"set this parameter to confirm any warning questions without being asked to press ENTER to continue" webdisable:"true" webset:"true"`
	seedip          string
	seedport        string
	chDirCmd
}

func (c *clientCreateGraphCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
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
	b.WorkOnClients()
	if c.seedip == "" {
		return errors.New("could not find an IP for a node in the given cluster - are all the nodes down?")
	}

	graphConfig := scripts.GetGraphConfig([]string{c.seedip + ":" + c.seedport}, c.Namespace, c.ExtraProperties)

	if a.opts.Config.Backend.Type == "docker" {
		log.Println("Running client.create.graph")
		if c.ClientCount > 1 {
			return errors.New("on docker, only one client can be dpeloyed at a time")
		}
		// install on docker
		if c.RAMMb == 0 {
			c.RAMMb = 4096
		}
		curDir, err := os.Getwd()
		if err != nil {
			return err
		}
		confFile := filepath.Join(curDir, "aerospike-graph.properties")
		if _, err := os.Stat(confFile); err == nil {
			log.Printf("WARNING: configuration file %s already exists. Press ENTER to continue.", confFile)
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
		netParams := ""
		if c.Docker.NetworkName != "" {
			netParams = "--network " + c.Docker.NetworkName
		}
		graphScript := scripts.GetDockerGraphScript(fmt.Sprintf(dockerNameHeader+"%s_%d", c.ClientName, 1), c.RAMMb, confFile, netParams)
		log.Println("Pulling and running dockerized aerospike-graph, this may take a while...")
		out, err := exec.Command("bash", "-c", string(graphScript)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ERROR: %s: %s", err, string(out))
		}
		log.Print("Done")
		log.Print("Common tasks and commands:")
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
		memSizeMb := c.RAMMb
		if c.RAMMb == 0 {
			out, err := b.RunCommands(string(c.ClientName), [][]string{{"free", "-b"}}, []int{node})
			if err != nil {
				nout := ""
				for _, n := range out {
					nout = nout + "\n" + string(n)
				}
				return fmt.Errorf("error on client %s: %s: %s", c.ClusterName, nout, err)
			}
			scanner := bufio.NewScanner(bytes.NewReader(out[0]))
			for scanner.Scan() {
				line := scanner.Text()
				if !strings.HasPrefix(line, "Mem:") {
					continue
				}
				memBytes := strings.Fields(line)
				if len(memBytes) < 2 {
					return errors.New("memory line corrupt from free -m")
				}
				memSizeMb, err = strconv.Atoi(memBytes[1])
				if err != nil {
					return fmt.Errorf("could not get memory from %s: %s", memBytes[1], err)
				}
				memSizeMb = memSizeMb / 1024 / 1024
			}
			if memSizeMb == 0 {
				return errors.New("could not find memory size from free -b")
			}
			sysSizeMb := memSizeMb
			memSizeMb = int(float64(memSizeMb) * 0.9)
			if memSizeMb < 1024 {
				log.Println("RAM size minimum falls below 1GB, setting to 1GB and hoping for the best")
				memSizeMb = 1024
			}
			log.Printf("Client %s Machine %d TotalSizeMB=%d GraphSizeMB=%d", c.ClientName, node, sysSizeMb, memSizeMb)
		}
		graphScript := scripts.GetCloudGraphScript(memSizeMb, "/etc/aerospike-graph.properties", "")
		err = b.CopyFilesToCluster(string(c.ClientName), []fileList{{"/etc/aerospike-graph.properties", string(graphConfig), len(graphConfig)}, {"/tmp/install-graph.sh", string(graphScript), len(graphScript)}}, []int{node})
		if err != nil {
			return err
		}
		a.opts.Attach.Client.ClientName = c.ClientName
		a.opts.Attach.Client.Machine = TypeMachines(strconv.Itoa(node))
		defer backendRestoreTerminal()
		err = a.opts.Attach.Client.run([]string{"/bin/bash", "/tmp/install-graph.sh"})
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
	log.Println("Done")
	log.Print("Common tasks and commands:")
	log.Print(" * access gremlin console locally:         docker run -it --rm tinkerpop/gremlin-console")
	log.Printf(" * access gremlin console on graph server: aerolab attach client -n %s -- docker run -it --rm tinkerpop/gremlin-console", c.ClientName)
	log.Printf(" * access terminal on graph server:        aerolab attach client -n %s", c.ClientName)
	log.Print(" * visit https://gdotv.com/ to download a Graph IDE and Visualization tool")
	return nil
}

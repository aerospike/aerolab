package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/parallelize"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type clusterAddExporterCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	CustomConf  flags.Filename  `short:"o" long:"custom-conf" description:"To deploy a custom ape.toml configuration file, specify it's path here"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	clusterStartStopDestroyCmd
}

func (c *clusterAddExporterCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running cluster.add.exporter")
	err := c.Nodes.ExpandNodes(string(c.ClusterName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClusterName), c.Nodes.String())
	if err != nil {
		return err
	}
	if len(cList) > 1 {
		return fmt.Errorf("only one cluster can be specified at a time")
	}

	//arms
	armlist := []int{}
	amdlist := []int{}
	for _, cluster := range cList {
		nlist := nodes[cluster]
		for _, node := range nlist {
			isArm, err := b.IsNodeArm(cluster, node)
			if err != nil {
				return err
			}
			if isArm {
				armlist = append(armlist, node)
			} else {
				amdlist = append(amdlist, node)
			}
		}
	}

	// find url
	pUrl, pV, err := aeroFindUrlX(promUrl, "latest", "", "")
	if err != nil {
		return fmt.Errorf("could not locate prometheus url: %s", err)
	}
	log.Printf("Installing version %s of prometheus exporter", pV)
	pUrlAmd := pUrl + "aerospike-prometheus-exporter_" + pV + "_x86_64.tar.gz"
	pUrlArm := pUrl + "aerospike-prometheus-exporter_" + pV + "_aarch64.tar.gz"
	nodeExpAmd := "https://github.com/prometheus/node_exporter/releases/download/v1.5.0/node_exporter-1.5.0.linux-amd64.tar.gz"
	nodeExpArm := "https://github.com/prometheus/node_exporter/releases/download/v1.5.0/node_exporter-1.5.0.linux-arm64.tar.gz"

	// install amd
	commands := [][]string{
		{"/bin/bash", "-c", "kill $(pidof aerospike-prometheus-exporter) >/dev/null 2>&1; sleep 2; kill -9 $(pidof aerospike-prometheus-exporter) >/dev/null 2>&1 || exit 0"},
		{"/bin/bash", "-c", "kill $(pidof node_exporter) >/dev/null 2>&1; sleep 2; kill -9 $(pidof node_exporter) >/dev/null 2>&1 || exit 0"},
		{"wget", pUrlAmd, "-O", "/aerospike-prometheus-exporter.tgz"},
		{"/bin/bash", "-c", "cd / && tar -xvzf aerospike-prometheus-exporter.tgz"},
		{"wget", nodeExpAmd, "-O", "/node-exporter.tgz"},
		{"/bin/bash", "-c", "cd / && tar -xvzf node-exporter.tgz && mv node_exporter-*/node_exporter /usr/bin/node_exporter"},
		{"/bin/bash", "-c", "mkdir -p /opt/autoload"},
	}
	if a.opts.Config.Backend.Type == "docker" {
		commands = append(commands, []string{"/bin/bash", "-c", "mkdir -p /opt/autoload && echo \"pidof aerospike-prometheus-exporter; [ \\$? -eq 0 ] && exit 0; bash -c 'nohup aerospike-prometheus-exporter --config /etc/aerospike-prometheus-exporter/ape.toml >/var/log/exporter.log 2>&1 & jobs -p %1'\" > /opt/autoload/01-exporter; chmod 755 /opt/autoload/01-exporter"})
	}
	for _, cluster := range cList {
		returns := parallelize.MapLimit(amdlist, c.ParallelThreads, func(node int) error {
			out, err := b.RunCommands(cluster, commands, []int{node})
			if err != nil {
				nout := ""
				for _, n := range out {
					nout = nout + "\n" + string(n)
				}
				return fmt.Errorf("error on cluster %s: %s: %s", cluster, nout, err)
			}
			cts := "pidof node_exporter; [ \\$? -eq 0 ] && exit 0; bash -c 'nohup /usr/bin/node_exporter >/var/log/node_exporter.log 2>&1 & jobs -p %1'"
			ctsr := strings.NewReader(cts)
			err = b.CopyFilesToClusterReader(cluster, []fileListReader{{filePath: "/opt/autoload/01-node-exporter", fileContents: ctsr, fileSize: len(cts)}}, []int{node})
			if err != nil {
				return err
			}
			return nil
		})
		isError := false
		for i, ret := range returns {
			if ret != nil {
				log.Printf("Node %d returned %s", amdlist[i], ret)
				isError = true
			}
		}
		if isError {
			return errors.New("some nodes returned errors")
		}
	}

	// install arm
	commands = [][]string{
		{"/bin/bash", "-c", "kill $(pidof aerospike-prometheus-exporter) >/dev/null 2>&1; sleep 2; kill -9 $(pidof aerospike-prometheus-exporter) >/dev/null 2>&1 || exit 0"},
		{"/bin/bash", "-c", "kill $(pidof node_exporter) >/dev/null 2>&1; sleep 2; kill -9 $(pidof node_exporter) >/dev/null 2>&1 || exit 0"},
		{"wget", pUrlArm, "-O", "/aerospike-prometheus-exporter.tgz"},
		{"/bin/bash", "-c", "cd / && tar -xvzf aerospike-prometheus-exporter.tgz"},
		{"wget", nodeExpArm, "-O", "/node-exporter.tgz"},
		{"/bin/bash", "-c", "cd / && tar -xvzf node-exporter.tgz && mv node_exporter-*/node_exporter /usr/bin/node_exporter"},
		{"/bin/bash", "-c", "mkdir -p /opt/autoload"},
	}
	if a.opts.Config.Backend.Type == "docker" {
		commands = append(commands, []string{"/bin/bash", "-c", "mkdir -p /opt/autoload && echo \"pidof aerospike-prometheus-exporter; [ \\$? -eq 0 ] && exit 0; bash -c 'nohup aerospike-prometheus-exporter --config /etc/aerospike-prometheus-exporter/ape.toml >/var/log/exporter.log 2>&1 & jobs -p %1'\" > /opt/autoload/01-exporter; chmod 755 /opt/autoload/01-exporter"})
	}
	for _, cluster := range cList {
		returns := parallelize.MapLimit(armlist, c.ParallelThreads, func(node int) error {
			out, err := b.RunCommands(cluster, commands, []int{node})
			if err != nil {
				nout := ""
				for _, n := range out {
					nout = nout + "\n" + string(n)
				}
				return fmt.Errorf("error on cluster %s: %s: %s", cluster, nout, err)
			}
			cts := "pidof node_exporter; [ \\$? -eq 0 ] && exit 0; bash -c 'nohup /usr/bin/node_exporter >/var/log/node_exporter.log 2>&1 & jobs -p %1'"
			ctsr := strings.NewReader(cts)
			err = b.CopyFilesToClusterReader(cluster, []fileListReader{{filePath: "/opt/autoload/01-node-exporter", fileContents: ctsr, fileSize: len(cts)}}, []int{node})
			if err != nil {
				return err
			}
			return nil
		})
		isError := false
		for i, ret := range returns {
			if ret != nil {
				log.Printf("Node %d returned %s", armlist[i], ret)
				isError = true
			}
		}
		if isError {
			return errors.New("some nodes returned errors")
		}
	}

	// install custom ape.toml
	if c.CustomConf != "" {
		for _, cluster := range cList {
			a.opts.Files.Upload.ClusterName = TypeClusterName(cluster)
			a.opts.Files.Upload.IsClient = false
			a.opts.Files.Upload.Nodes = TypeNodes("all")
			a.opts.Files.Upload.Files.Source = c.CustomConf
			a.opts.Files.Upload.Files.Destination = flags.Filename("/etc/aerospike-prometheus-exporter/ape.toml")
			a.opts.Files.Upload.doLegacy = true
			a.opts.Files.Upload.ParallelThreads = c.ParallelThreads
			err = a.opts.Files.Upload.runUpload(args)
			if err != nil {
				return err
			}
		}
	}

	// patch in the expose ports if on docker
	if a.opts.Config.Backend.Type == "docker" {
		inv, err := b.Inventory("", []int{InventoryItemClusters})
		if err != nil {
			return err
		}
		b.WorkOnServers()
		returns := parallelize.MapLimit(inv.Clusters, c.ParallelThreads, func(item inventoryCluster) error {
			if item.ClusterName != c.ClusterName.String() {
				return nil
			}
			if item.DockerExposePorts == "" {
				return nil
			}
			nodeNo, err := strconv.Atoi(item.NodeNo)
			if err != nil {
				return err
			}
			out, err := b.RunCommands(string(c.ClusterName), [][]string{{"cat", "/etc/aerospike-prometheus-exporter/ape.toml"}}, []int{nodeNo})
			if err != nil {
				return err
			}
			scanner := bufio.NewScanner(bytes.NewReader(out[0]))
			in := ""
			found := false
			for scanner.Scan() {
				line := scanner.Text()
				linex := strings.Trim(line, "\r\t\n ")
				if strings.HasPrefix(linex, "db_port") {
					in = in + "db_port = " + item.DockerExposePorts + "\n"
					found = true
				} else {
					in = in + line + "\n"
				}
			}
			if !found {
				scanner = bufio.NewScanner(bytes.NewReader(out[0]))
				in = ""
				for scanner.Scan() {
					line := scanner.Text()
					linex := strings.Trim(line, "\r\t\n ")
					if strings.HasPrefix(linex, "[Aerospike]") {
						in = in + line + "\n" + "db_port = " + item.DockerExposePorts + "\n"
					} else {
						in = in + line + "\n"
					}
				}
			}
			err = b.CopyFilesToClusterReader(item.ClusterName, []fileListReader{{"/etc/aerospike-prometheus-exporter/ape.toml", strings.NewReader(in), len(in)}}, []int{nodeNo})
			if err != nil {
				log.Printf("ERROR: could not install ape.toml after docker port patching: %s", err)
			}
			return nil
		})
		isError := false
		for i, ret := range returns {
			if ret != nil {
				log.Printf("Node %s returned %s", inv.Clusters[i].NodeNo, ret)
				isError = true
			}
		}
		if isError {
			return errors.New("some nodes returned errors")
		}
	}

	// start
	commands = [][]string{
		{"/bin/bash", "/opt/autoload/01-node-exporter"},
	}
	for _, cluster := range cList {
		returns := parallelize.MapLimit(nodes[cluster], c.ParallelThreads, func(node int) error {
			out, err := b.RunCommands(cluster, commands, []int{node})
			if err != nil {
				nout := ""
				for _, n := range out {
					nout = nout + "\n" + string(n)
				}
				return fmt.Errorf("error on cluster %s: %s: %s", cluster, nout, err)
			}
			return nil
		})
		isError := false
		for i, ret := range returns {
			if ret != nil {
				log.Printf("Node %d returned %s", nodes[cluster][i], ret)
				isError = true
			}
		}
		if isError {
			return errors.New("some nodes returned errors")
		}
	}
	if a.opts.Config.Backend.Type == "docker" {
		commands = [][]string{
			{"/bin/bash", "/opt/autoload/01-exporter"},
		}
		for _, cluster := range cList {
			returns := parallelize.MapLimit(nodes[cluster], c.ParallelThreads, func(node int) error {
				out, err := b.RunCommands(cluster, commands, []int{node})
				if err != nil {
					nout := ""
					for _, n := range out {
						nout = nout + "\n" + string(n)
					}
					return fmt.Errorf("error on cluster %s: %s: %s", cluster, nout, err)
				}
				return nil
			})
			isError := false
			for i, ret := range returns {
				if ret != nil {
					log.Printf("Node %d returned %s", nodes[cluster][i], ret)
					isError = true
				}
			}
			if isError {
				return errors.New("some nodes returned errors")
			}
		}
	} else {
		commands = [][]string{
			{"/bin/bash", "-c", "systemctl daemon-reload"},
			{"/bin/bash", "-c", "kill -9 `pidof aerospike-prometheus-exporter` 2>/dev/null || echo starting"},
			{"/bin/bash", "-c", "systemctl stop aerospike-prometheus-exporter"},
			{"/bin/bash", "-c", "systemctl enable aerospike-prometheus-exporter"},
			{"/bin/bash", "-c", "systemctl start aerospike-prometheus-exporter"},
		}
		for _, cluster := range cList {
			returns := parallelize.MapLimit(nodes[cluster], c.ParallelThreads, func(node int) error {
				out, err := b.RunCommands(cluster, commands, []int{node})
				if err != nil {
					nout := ""
					for _, n := range out {
						nout = nout + "\n" + string(n)
					}
					return fmt.Errorf("error on cluster %s: %s: %s", cluster, nout, err)
				}
				return nil
			})
			isError := false
			for i, ret := range returns {
				if ret != nil {
					log.Printf("Node %d returned %s", nodes[cluster][i], ret)
					isError = true
				}
			}
			if isError {
				return errors.New("some nodes returned errors")
			}
		}
	}
	log.Print("Done")
	log.Print("NOTE: Remember to install the AMS stack client to monitor the cluster, using `aerolab client create ams` command")
	return nil
}

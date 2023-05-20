package main

import (
	"fmt"
	"log"
	"strings"

	flags "github.com/rglonek/jeddevdk-goflags"
)

type clusterAddExporterCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	CustomConf  flags.Filename  `short:"o" long:"custom-conf" description:"To deploy a custom ape.toml configuration file, specify it's path here"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
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
		out, err := b.RunCommands(cluster, commands, amdlist)
		if err != nil {
			nout := ""
			for _, n := range out {
				nout = nout + "\n" + string(n)
			}
			return fmt.Errorf("error on cluster %s: %s: %s", cluster, nout, err)
		}
		cts := "pidof node_exporter; [ \\$? -eq 0 ] && exit 0; bash -c 'nohup /usr/bin/node_exporter >/var/log/node_exporter.log 2>&1 & jobs -p %1'"
		ctsr := strings.NewReader(cts)
		err = b.CopyFilesToCluster(cluster, []fileList{{filePath: "/opt/autoload/01-node-exporter", fileContents: ctsr, fileSize: len(cts)}}, amdlist)
		if err != nil {
			log.Print(err)
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
		out, err := b.RunCommands(cluster, commands, armlist)
		if err != nil {
			nout := ""
			for _, n := range out {
				nout = nout + "\n" + string(n)
			}
			return fmt.Errorf("error on cluster %s: %s: %s", cluster, nout, err)
		}
		cts := "pidof node_exporter; [ \\$? -eq 0 ] && exit 0; bash -c 'nohup /usr/bin/node_exporter >/var/log/node_exporter.log 2>&1 & jobs -p %1'"
		ctsr := strings.NewReader(cts)
		err = b.CopyFilesToCluster(cluster, []fileList{{filePath: "/opt/autoload/01-node-exporter", fileContents: ctsr, fileSize: len(cts)}}, armlist)
		if err != nil {
			log.Print(err)
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
			err = a.opts.Files.Upload.runUpload(args)
			if err != nil {
				return err
			}
		}
	}

	// start
	commands = [][]string{
		{"/bin/bash", "/opt/autoload/01-node-exporter"},
	}
	for _, cluster := range cList {
		out, err := b.RunCommands(cluster, commands, nodes[cluster])
		if err != nil {
			nout := ""
			for _, n := range out {
				nout = nout + "\n" + string(n)
			}
			return fmt.Errorf("error on cluster %s: %s: %s", cluster, nout, err)
		}
	}
	if a.opts.Config.Backend.Type == "docker" {
		commands = [][]string{
			{"/bin/bash", "/opt/autoload/01-exporter"},
		}
		for _, cluster := range cList {
			out, err := b.RunCommands(cluster, commands, nodes[cluster])
			if err != nil {
				nout := ""
				for _, n := range out {
					nout = nout + "\n" + string(n)
				}
				return fmt.Errorf("error on cluster %s: %s: %s", cluster, nout, err)
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
			out, err := b.RunCommands(cluster, commands, nodes[cluster])
			if err != nil {
				nout := ""
				for _, n := range out {
					nout = nout + "\n" + string(n)
				}
				return fmt.Errorf("error on cluster %s: %s: %s", cluster, nout, err)
			}
		}
	}
	log.Print("Done")
	log.Print("NOTE: Remember to install the AMS stack client to monitor the cluster, using `aerolab client create ams` command")
	return nil
}

package main

import (
	"fmt"
	"log"

	flags "github.com/rglonek/jeddevdk-goflags"
)

type clusterAddExporterCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"mydc"`
	CustomConf  flags.Filename  `short:"o" long:"custom-conf" description:"To deploy a custom ape.toml configuration file, specify it's path here"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
	clusterStartStopDestroyCmd
}

func (c *clusterAddExporterCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running cluster.add.exporter")
	cList, nodes, err := c.getBasicData(string(c.ClusterName), "all")
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

	// install amd
	commands := [][]string{
		[]string{"/bin/bash", "-c", "kill $(pidof aerospike-prometheus-exporter) >/dev/null 2>&1; sleep 2; kill -9 $(pidof aerospike-prometheus-exporter) >/dev/null 2>&1 || exit 0"},
		[]string{"wget", pUrlAmd, "-O", "/aerospike-prometheus-exporter.tgz"},
		[]string{"/bin/bash", "-c", "cd / && tar -xvzf aerospike-prometheus-exporter.tgz"},
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
	}

	// install arm
	commands = [][]string{
		[]string{"/bin/bash", "-c", "kill $(pidof aerospike-prometheus-exporter) >/dev/null 2>&1; sleep 2; kill -9 $(pidof aerospike-prometheus-exporter) >/dev/null 2>&1 || exit 0"},
		[]string{"wget", pUrlArm, "-O", "/aerospike-prometheus-exporter.tgz"},
		[]string{"/bin/bash", "-c", "cd / && tar -xvzf aerospike-prometheus-exporter.tgz"},
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
	if a.opts.Config.Backend.Type == "docker" {
		commands = [][]string{
			[]string{"/bin/bash", "/opt/autoload/01-exporter"},
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
			[]string{"/bin/bash", "-c", "systemctl daemon-reload"},
			[]string{"/bin/bash", "-c", "kill -9 `pidof aerospike-prometheus-exporter` 2>/dev/null || echo starting"},
			[]string{"/bin/bash", "-c", "systemctl stop aerospike-prometheus-exporter"},
			[]string{"/bin/bash", "-c", "systemctl enable aerospike-prometheus-exporter"},
			[]string{"/bin/bash", "-c", "systemctl start aerospike-prometheus-exporter"},
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
	return nil
}

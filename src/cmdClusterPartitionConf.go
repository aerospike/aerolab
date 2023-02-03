package main

import (
	"log"
)

// TODO whole thing here

type clusterPartitionConfCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes            TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks      TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterPartitions TypeFilterRange `short:"p" long:"filter-partitions" description:"Select partitions on each disk by number, ex: 1,2,4-8" default:"ALL"`
	FilterType       string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme|ebs" default:"ALL"`
	Namespace        string          `short:"m" long:"namespace" description:"namespace to modify the settings for; default: first found namespace" default:""`
	ConfDest         string          `short:"o" long:"configure" description:"what to configure the selections as; options: device|shadow|allflash" default:""`
	Help             helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterPartitionConfCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running cluster.partition.conf")
	log.Print("Done")
	return nil
}

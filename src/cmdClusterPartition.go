package main

import "log"

type clusterPartitionCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	// TODO: switches for: filters, partitions, etc
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterPartitionCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running cluster.partition")
	log.Print("Done")
	return nil
}

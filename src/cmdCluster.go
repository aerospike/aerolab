package main

import (
	"log"
	"os"
)

type clusterCmd struct {
	Create    clusterCreateCmd    `command:"create" subcommands-optional:"true" description:"Create a new cluster"`
	List      clusterListCmd      `command:"list" subcommands-optional:"true" description:"List clusters"`
	Start     clusterStartCmd     `command:"start" subcommands-optional:"true" description:"Start cluster"`
	Stop      clusterStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop cluster"`
	Grow      clusterGrowCmd      `command:"grow" subcommands-optional:"true" description:"Add nodes to cluster"`
	Destroy   clusterDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy cluster"`
	Add       clusterAddCmd       `command:"add" subcommands-optional:"true" description:"Add features to clusters, ex: ams"`
	Partition clusterPartitionCmd `command:"partition" subcommands-optional:"true" description:"node disk partitioner"`
	Help      helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

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

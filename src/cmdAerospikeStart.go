package main

import "os"

type aerospikeStartCmd struct {
	aerospikeStartSelectorCmd
	parallelThreadsCmd
}

type aerospikeStartSelectorCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *aerospikeStartCmd) Execute(args []string) error {
	return c.run(args, "start", os.Stdout)
}

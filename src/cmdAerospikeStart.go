package main

type aerospikeStartCmd struct {
	aerospikeStartSelectorCmd
	ParallelThreads int `short:"t" long:"threads" description:"Run on this many nodes in parallel" default:"50"`
}

type aerospikeStartSelectorCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *aerospikeStartCmd) Execute(args []string) error {
	return c.run(args, "start")
}

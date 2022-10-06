package main

import "os"

type clusterCmd struct {
	Create  clusterCreateCmd  `command:"create" subcommands-optional:"true" description:"Create a new cluster"`
	List    clusterListCmd    `command:"list" subcommands-optional:"true" description:"List clusters"`
	Start   clusterStartCmd   `command:"start" subcommands-optional:"true" description:"Start cluster"`
	Stop    clusterStopCmd    `command:"stop" subcommands-optional:"true" description:"Stop cluster"`
	Grow    clusterGrowCmd    `command:"grow" subcommands-optional:"true" description:"Add nodes to cluster"`
	Destroy clusterDestroyCmd `command:"destroy" subcommands-optional:"true" description:"Destroy cluster"`
	Add     clusterAddCmd     `command:"add" subcommands-optional:"true" description:"Add features to clusters, ex: ams"`
	Help    helpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

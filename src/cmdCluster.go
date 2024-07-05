package main

import (
	"os"
)

type clusterCmd struct {
	Create    clusterCreateCmd    `command:"create" subcommands-optional:"true" description:"Create a new cluster" webicon:"fas fa-circle-plus" invwebforce:"true"`
	List      clusterListCmd      `command:"list" subcommands-optional:"true" description:"List clusters" webicon:"fas fa-list"`
	Start     clusterStartCmd     `command:"start" subcommands-optional:"true" description:"Start cluster" webicon:"fas fa-play" invwebforce:"true"`
	Stop      clusterStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop cluster" webicon:"fas fa-stop" invwebforce:"true"`
	Grow      clusterGrowCmd      `command:"grow" subcommands-optional:"true" description:"Add nodes to cluster" webicon:"fas fa-circle-plus" invwebforce:"true"`
	Destroy   clusterDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy cluster" webicon:"fas fa-trash" invwebforce:"true"`
	Add       clusterAddCmd       `command:"add" subcommands-optional:"true" description:"Add features to clusters, ex: ams" webicon:"fas fa-gear"`
	Partition clusterPartitionCmd `command:"partition" subcommands-optional:"true" description:"node disk partitioner" webicon:"fas fa-divide"`
	Attach    attachShellCmd      `command:"attach" subcommands-optional:"true" description:"symlink to: attach shell" webicon:"fas fa-terminal" simplemode:"false"`
	Share     clusterShareCmd     `command:"share" subcommands-optional:"true" description:"AWS/GCP: share the cluster by importing a provided ssh public key file" webicon:"fas fa-share"`
	Help      helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

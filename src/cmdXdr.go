package main

import (
	"os"
)

type xdrCmd struct {
	Connect        xdrConnectCmd        `command:"connect" subcommands-optional:"true" description:"Connect clusters and namespaces via XDR" webicon:"fas fa-link"`
	CreateClusters xdrCreateClustersCmd `command:"create-clusters" subcommands-optional:"true" description:"Create clusters connected via XDR" webicon:"fas fa-circle-plus"`
	Help           helpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *xdrCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

package main

import "fmt"

type clusterListCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	f, e := b.ClusterListFull()
	if e != nil {
		return e
	}
	fmt.Println(f)
	return nil
}

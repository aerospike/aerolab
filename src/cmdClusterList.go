package main

import "fmt"

type clusterListCmd struct {
	Json bool    `short:"j" long:"json" description:"Provide output in json format"`
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	f, e := b.ClusterListFull(c.Json)
	if e != nil {
		return e
	}
	fmt.Println(f)
	return nil
}

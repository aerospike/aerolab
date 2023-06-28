package main

import (
	"os"
)

type inventoryCmd struct {
	List          inventoryListCmd          `command:"list" subcommands-optional:"true" description:"List clusters, clients and templates"`
	InstanceTypes inventoryInstanceTypesCmd `command:"instance-types" subcommands-optional:"true" description:"Lookup GCP|AWS available instance types"`
	Help          helpCmd                   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *inventoryCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

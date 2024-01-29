package main

import "os"

type dataCmd struct {
	Insert dataInsertCmd `command:"insert" subcommands-optional:"true" description:"Insert data into an Aerospike cluster" webicon:"fas fa-circle-plus"`
	Delete dataDeleteCmd `command:"delete" subcommands-optional:"true" description:"Delete data inserted via AeroLab" webicon:"fas fa-circle-minus"`
	Help   helpCmd       `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *dataCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

package main

import (
	"os"
)

type logsCmd struct {
	Get  logsGetCmd  `command:"get" subcommands-optional:"true" description:"Download logs from Aerospike logs"`
	Show logsShowCmd `command:"show" subcommands-optional:"true" description:"Print logs from an Aerospike node"`
	Help helpCmd     `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *logsCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

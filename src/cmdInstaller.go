package main

import (
	"os"
)

type installerCmd struct {
	ListVersions installerListVersionsCmd `command:"list-versions" subcommands-optional:"true" description:"List Aerospike versions"`
	Download     installerDownloadCmd     `command:"download" subcommands-optional:"true" description:"Download Aerospike installer"`
	Help         helpCmd                  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *installerCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

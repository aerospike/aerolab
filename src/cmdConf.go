package main

import "os"

type confCmd struct {
	Generator confGeneratorCmd `command:"generate" subcommands-optional:"true" description:"Generate or modify Aerospike configuration files"`
	FixMesh   confFixMeshCmd   `command:"fix-mesh" subcommands-optional:"true" description:"Fix mesh configuration in the cluster"`
	Help      helpCmd          `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *confCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

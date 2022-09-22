package main

import "os"

type confCmd struct {
	FixMesh confFixMeshCmd `command:"fix-mesh" subcommands-optional:"true" description:"Fix mesh configuration in the cluster"`
	Help    helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *confCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

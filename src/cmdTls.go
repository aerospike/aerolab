package main

import "os"

type tlsCmd struct {
	Generate tlsGenerateCmd `command:"generate" subcommands-optional:"true" description:"Generate TLS certificates" webicon:"fas fa-passport"`
	Copy     tlsCopyCmd     `command:"copy" subcommands-optional:"true" description:"Copy certificates to other nodes,clusters or clients" webicon:"fas fa-copy"`
	Help     helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *tlsCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

package main

import "os"

type templateCmd struct {
	List   templateListCmd   `command:"list" subcommands-optional:"true" description:"List available templates"`
	Delete templateDeleteCmd `command:"destroy" subcommands-optional:"true" description:"Delete a template image"`
	Create templateCreateCmd `command:"create" subcommands-optional:"true" description:"Create a new template"`
	Help   helpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *templateCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

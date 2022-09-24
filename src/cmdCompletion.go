package main

import "os"

type completionCmd struct {
	Bash completionBashCmd `command:"bash" subcommands-optional:"true" description:"Install completion script for bash"`
	Zsh  completionZshCmd  `command:"zsh" subcommands-optional:"true" description:"Install completion script for zsh"`
	Help helpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *completionCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

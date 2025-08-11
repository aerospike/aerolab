package cmd

import (
	_ "embed"
)

//go:embed cmdCompletion.sh.tpl
var completionBash string

type CompletionCmd struct {
	Bash CompletionBashCmd `command:"bash" subcommands-optional:"true" description:"Install completion script for bash" webicon:"fas fa-tent"`
	Zsh  CompletionZshCmd  `command:"zsh" subcommands-optional:"true" description:"Install completion script for zsh" webicon:"fas fa-tents"`
	Help HelpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CompletionCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

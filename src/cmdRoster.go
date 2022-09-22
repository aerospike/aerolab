package main

import (
	"os"
)

type rosterCmd struct {
	Show  rosterShowCmd  `command:"show" subcommands-optional:"true" description:"Show roster in the cluster namespace"`
	Apply rosterApplyCmd `command:"apply" subcommands-optional:"true" description:"Apply a roster to the cluster namespace"`
	Cheat rosterCheatCmd `command:"cheat" subcommands-optional:"true" description:"Quick strong consistency cheat-sheet"`
	Help  helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *rosterCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

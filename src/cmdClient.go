package main

import (
	"os"
)

type clientCmd struct {
	Create    clientCreateCmd    `command:"create" subcommands-optional:"true" description:"Create new client machines" webicon:"fas fa-circle-plus"`
	Add       clientAddCmd       `command:"add" hidden:"true" subcommands-optional:"true" description:"Add features to existing client machines"`
	Configure clientConfigureCmd `command:"configure" subcommands-optional:"true" description:"(re)configure some clients, such as ams" webicon:"fas fa-gear"`
	List      clientListCmd      `command:"list" subcommands-optional:"true" description:"List client machine groups" webicon:"fas fa-list"`
	Start     clientStartCmd     `command:"start" subcommands-optional:"true" description:"Start a client machine group" webicon:"fas fa-play"`
	Stop      clientStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop a client machine group" webicon:"fas fa-stop"`
	Grow      clientGrowCmd      `command:"grow" subcommands-optional:"true" description:"Grow a client machine group" webicon:"fas fa-circle-plus"`
	Destroy   clientDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy client(s)" webicon:"fas fa-trash" invwebforce:"true"`
	Attach    attachClientCmd    `command:"attach" subcommands-optional:"true" description:"symlink to: attach client" webicon:"fas fa-terminal"`
	Share     clientShareCmd     `command:"share" subcommands-optional:"true" description:"share a client with other users - wrapper around ssh-copy-id" webicon:"fas fa-share"`
	Help      helpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

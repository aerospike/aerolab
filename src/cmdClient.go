package main

import (
	"os"
)

type clientCmd struct {
	Create    clientCreateCmd    `command:"create" subcommands-optional:"true" description:"Create new client machines"`
	Add       clientAddCmd       `command:"add" hidden:"true" subcommands-optional:"true" description:"Add features to existing client machines"`
	Configure clientConfigureCmd `command:"configure" subcommands-optional:"true" description:"(re)configure some clients, such as ams"`
	List      clientListCmd      `command:"list" subcommands-optional:"true" description:"List client machine groups"`
	Start     clientStartCmd     `command:"start" subcommands-optional:"true" description:"Start a client machine group"`
	Stop      clientStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop a client machine group"`
	Grow      clientGrowCmd      `command:"grow" subcommands-optional:"true" description:"Grow a client machine group"`
	Destroy   clientDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy client(s)"`
	Attach    attachClientCmd    `command:"attach" subcommands-optional:"true" description:"symlink to: attach client"`
	Share     clientShareCmd     `command:"share" subcommands-optional:"true" description:"share a client with other users - wrapper around ssh-copy-id"`
	Help      helpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

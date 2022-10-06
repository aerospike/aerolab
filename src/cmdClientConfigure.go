package main

import "os"

type clientConfigureCmd struct {
	AMS  clientConfigureAMSCmd `command:"ams" subcommands-optional:"true" description:"change which clusters prometheus points at"`
	Help helpCmd               `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

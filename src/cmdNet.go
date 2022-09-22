package main

import "os"

type netCmd struct {
	Block     netBlockCmd     `command:"block" subcommands-optional:"true" description:"Block a port"`
	Unblock   netUnblockCmd   `command:"unblock" subcommands-optional:"true" description:"Unblock a port"`
	List      netListCmd      `command:"list" subcommands-optional:"true" description:"List blocked ports"`
	LossDelay netLossDelayCmd `command:"loss-delay" subcommands-optional:"true" description:"Simulate packet loss or latencies"`
	Help      helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *netCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

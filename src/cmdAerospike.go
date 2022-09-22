package main

import "os"

type aerospikeCmd struct {
	Start   aerospikeStartCmd   `command:"start" subcommands-optional:"true" description:"Start aerospike"`
	Stop    aerospikeStopCmd    `command:"stop" subcommands-optional:"true" description:"Stop aerospike"`
	Restart aerospikeRestartCmd `command:"restart" subcommands-optional:"true" description:"Restart aerospike"`
	Status  aerospikeStatusCmd  `command:"status" subcommands-optional:"true" description:"Aerospike daemon status"`
	Upgrade aerospikeUpgradeCmd `command:"upgrade" subcommands-optional:"true" description:"Upgrade aerospike daemon"`
	Help    helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *aerospikeCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

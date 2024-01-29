package main

import "os"

type attachCmd struct {
	Shell  attachShellCmd  `command:"shell" subcommands-optional:"true" description:"Attach to shell" webicon:"fas fa-terminal"`
	Client attachClientCmd `command:"client" subcommands-optional:"true" description:"Attach to client machine shell" webicon:"fas fa-tv"`
	Aql    attachAqlCmd    `command:"aql" subcommands-optional:"true" description:"Run aql on node" webicon:"fas fa-database"`
	Asadm  attachAsadmCmd  `command:"asadm" subcommands-optional:"true" description:"Run asadm on node" webicon:"fas fa-hammer"`
	Asinfo attachAsinfoCmd `command:"asinfo" subcommands-optional:"true" description:"Run asinfo on node" webicon:"fas fa-circle-info"`
	AGI    agiAttachCmd    `command:"agi" subcommands-optional:"true" description:"Attach to an AGI node" webicon:"fas fa-chart-line"`
	Trino  attachCmdTrino  `command:"trino" subcommands-optional:"true" description:"Attach to trino shell" webicon:"fas fa-tachograph-digital"`
	Help   attachCmdHelp   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *attachCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

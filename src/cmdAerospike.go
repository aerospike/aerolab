package main

import (
	"os"
)

type aerospikeCmd struct {
	Start     aerospikeStartCmd     `command:"start" subcommands-optional:"true" description:"Start aerospike" webicon:"fas fa-play"`
	Stop      aerospikeStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop aerospike" webicon:"fas fa-stop"`
	Restart   aerospikeRestartCmd   `command:"restart" subcommands-optional:"true" description:"Restart aerospike" webicon:"fas fa-forward-step"`
	Status    aerospikeStatusCmd    `command:"status" subcommands-optional:"true" description:"Aerospike daemon status" webicon:"fas fa-circle-question"`
	Upgrade   aerospikeUpgradeCmd   `command:"upgrade" subcommands-optional:"true" description:"Upgrade aerospike daemon" webicon:"fas fa-circle-arrow-up"`
	ColdStart aerospikeColdStartCmd `command:"cold-start" subcommands-optional:"true" description:"Cold-Start aerospike" webicon:"fas fa-play"`
	IsStable  aerospikeIsStableCmd  `command:"is-stable" subcommands-optional:"true" description:"Check if, and optionally wait until, cluster is stable" webicon:"fas fa-circle-question"`
	Help      helpCmd               `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *aerospikeCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

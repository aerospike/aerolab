package cmd

type AerospikeCmd struct {
	Start     AerospikeStartCmd     `command:"start" subcommands-optional:"true" description:"Start aerospike" webicon:"fas fa-play"`
	Stop      AerospikeStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop aerospike" webicon:"fas fa-stop"`
	Restart   AerospikeRestartCmd   `command:"restart" subcommands-optional:"true" description:"Restart aerospike" webicon:"fas fa-forward-step"`
	Status    AerospikeStatusCmd    `command:"status" subcommands-optional:"true" description:"Aerospike daemon status" webicon:"fas fa-circle-question"`
	Upgrade   AerospikeUpgradeCmd   `command:"upgrade" subcommands-optional:"true" description:"Upgrade aerospike daemon" webicon:"fas fa-circle-arrow-up"`
	ColdStart AerospikeColdStartCmd `command:"cold-start" subcommands-optional:"true" description:"Cold-Start aerospike" webicon:"fas fa-play"`
	IsStable  AerospikeIsStableCmd  `command:"is-stable" subcommands-optional:"true" description:"Check if, and optionally wait until, cluster is stable" webicon:"fas fa-circle-question"`
	Help      HelpCmd               `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AerospikeCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

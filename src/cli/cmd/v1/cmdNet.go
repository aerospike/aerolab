package cmd

type NetCmd struct {
	Block     NetBlockCmd     `command:"block" subcommands-optional:"true" description:"Block a port" webicon:"fas fa-lock"`
	Unblock   NetUnblockCmd   `command:"unblock" subcommands-optional:"true" description:"Unblock a port" webicon:"fas fa-unlock"`
	List      NetListCmd      `command:"list" subcommands-optional:"true" description:"List blocked ports" webicon:"fas fa-list"`
	LossDelay NetLossDelayCmd `command:"loss-delay" subcommands-optional:"true" description:"Simulate packet loss or latencies" webicon:"fas fa-building-shield"`
	Help      HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *NetCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

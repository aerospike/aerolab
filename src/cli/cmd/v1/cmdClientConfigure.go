package cmd

type ClientConfigureCmd struct {
	Tools    ClientConfigureToolsCmd    `command:"tools" subcommands-optional:"true" description:"Configure aerospike-tools" webicon:"fas fa-toolbox"`
	AMS      ClientConfigureAMSCmd      `command:"ams" subcommands-optional:"true" description:"Configure AMS (Prometheus/Grafana)" webicon:"fas fa-layer-group"`
	Firewall ClientConfigureFirewallCmd `command:"firewall" subcommands-optional:"true" description:"Configure firewall rules" webicon:"fas fa-shield-alt"`
	Expiry   ClientChangeExpiryCmd      `command:"expiry" subcommands-optional:"true" description:"Change expiry" webicon:"fas fa-clock"`
	Help     HelpCmd                    `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientConfigureCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

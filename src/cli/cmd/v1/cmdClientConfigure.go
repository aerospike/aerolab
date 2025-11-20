package cmd

type ClientConfigureCmd struct {
	Tools       ClientConfigureToolsCmd       `command:"tools" subcommands-optional:"true" description:"Configure aerospike-tools" webicon:"fas fa-toolbox"`
	AMS         ClientConfigureAMSCmd         `command:"ams" subcommands-optional:"true" description:"Configure AMS (Prometheus/Grafana)" webicon:"fas fa-layer-group"`
	VSCode      ClientConfigureVSCodeCmd      `command:"vscode" subcommands-optional:"true" description:"Configure VSCode" webicon:"fas fa-code"`
	Firewall    ClientConfigureFirewallCmd    `command:"firewall" subcommands-optional:"true" description:"Configure firewall rules" webicon:"fas fa-shield-alt"`
	Help        HelpCmd                       `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientConfigureCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}


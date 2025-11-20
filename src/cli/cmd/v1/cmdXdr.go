package cmd

type XdrCmd struct {
	Connect        XdrConnectCmd        `command:"connect" subcommands-optional:"true" description:"Connect clusters and namespaces via XDR" webicon:"fas fa-link"`
	CreateClusters XdrCreateClustersCmd `command:"create-clusters" subcommands-optional:"true" description:"Create clusters connected via XDR" webicon:"fas fa-circle-plus"`
	Help           HelpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *XdrCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

package cmd

type ClusterAddCmd struct {
	//Exporter ClusterAddExporterCmd `command:"exporter" subcommands-optional:"true" description:"Install ams exporter in a cluster or clusters" webicon:"fas fa-layer-group"`
	//Firewall ClusterAddFirewallCmd `command:"firewall" subcommands-optional:"true" description:"Add firewall rules to an existing cluster" webicon:"fas fa-fire"`
	//Expiry   ClusterAddExpiryCmd   `command:"expiry" subcommands-optional:"true" description:"Add or change hours until expiry for a cluster (aws|gcp only)" webicon:"fas fa-user-xmark"`
	//AeroLab  ClusterAddAerolabCmd  `command:"aerolab" subcommands-optional:"true" description:"Deploy aerolab binary on a cluster" webicon:"fas fa-flask"`
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterAddCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

// TODO all the Add commands

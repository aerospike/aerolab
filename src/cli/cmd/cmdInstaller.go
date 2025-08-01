package cmd

type InstallerCmd struct {
	ListVersions InstallerListVersionsCmd `command:"list-versions" subcommands-optional:"true" description:"List Aerospike versions" webicon:"fas fa-list"`
	Download     InstallerDownloadCmd     `command:"download" subcommands-optional:"true" description:"Download Aerospike installer" webicon:"fas fa-download"`
	Help         HelpCmd                  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstallerCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

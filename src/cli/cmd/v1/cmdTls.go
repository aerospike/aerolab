package cmd

type TlsCmd struct {
	Generate TlsGenerateCmd `command:"generate" subcommands-optional:"true" description:"Generate TLS certificates" webicon:"fas fa-passport"`
	Copy     TlsCopyCmd     `command:"copy" subcommands-optional:"true" description:"Copy certificates to other nodes or clusters" webicon:"fas fa-copy"`
	Help     HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *TlsCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

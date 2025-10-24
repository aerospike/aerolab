package cmd

type LogsCmd struct {
	Show LogsShowCmd `command:"show" subcommands-optional:"true" description:"Print logs from an Aerospike node" webicon:"fas fa-eye"`
	Get  LogsGetCmd  `command:"get" subcommands-optional:"true" description:"Download logs from Aerospike logs" webicon:"fas fa-file-export" webcommandtype:"download"`
	Help HelpCmd     `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *LogsCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

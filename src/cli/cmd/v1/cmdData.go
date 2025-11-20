package cmd

type DataCmd struct {
	Insert DataInsertCmd `command:"insert" subcommands-optional:"true" description:"Insert data into an Aerospike cluster" webicon:"fas fa-circle-plus"`
	Delete DataDeleteCmd `command:"delete" subcommands-optional:"true" description:"Delete data inserted via AeroLab" webicon:"fas fa-circle-minus"`
	Help   HelpCmd       `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *DataCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

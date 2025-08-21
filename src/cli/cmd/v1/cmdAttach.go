package cmd

type AttachCmd struct {
	Shell  AttachShellCmd  `command:"shell" subcommands-optional:"true" description:"Attach to shell" webicon:"fas fa-terminal" simplemode:"false"`
	Client AttachClientCmd `command:"client" subcommands-optional:"true" description:"Attach to client machine shell" webicon:"fas fa-tv" simplemode:"false"`
	Aql    AttachAqlCmd    `command:"aql" subcommands-optional:"true" description:"Run aql on node" webicon:"fas fa-database" simplemode:"false"`
	Asadm  AttachAsadmCmd  `command:"asadm" subcommands-optional:"true" description:"Run asadm on node" webicon:"fas fa-hammer" simplemode:"false"`
	Asinfo AttachAsinfoCmd `command:"asinfo" subcommands-optional:"true" description:"Run asinfo on node" webicon:"fas fa-circle-info" simplemode:"false"`
	AGI    AttachAGICmd    `command:"agi" subcommands-optional:"true" description:"Attach to an AGI node" webicon:"fas fa-chart-line" simplemode:"false"`
	Trino  AttachTrinoCmd  `command:"trino" subcommands-optional:"true" description:"Attach to trino shell" webicon:"fas fa-tachograph-digital" simplemode:"false"`
	Help   HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AttachCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

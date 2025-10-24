package cmd

type RosterCmd struct {
	Show  RosterShowCmd  `command:"show" subcommands-optional:"true" description:"Show roster in the cluster namespace" webicon:"fas fa-eye"`
	Apply RosterApplyCmd `command:"apply" subcommands-optional:"true" description:"Apply a roster to the cluster namespace" webicon:"fas fa-right-to-bracket"`
	Cheat RosterCheatCmd `command:"cheat" subcommands-optional:"true" description:"Quick strong consistency cheat-sheet" webicon:"fas fa-comment"`
	Help  HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *RosterCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

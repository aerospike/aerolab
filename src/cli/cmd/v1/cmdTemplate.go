package cmd

type TemplateCmd struct {
	Create  TemplateCreateCmd  `command:"create" subcommands-optional:"true" description:"Create a template" webicon:"fas fa-plus"`
	List    TemplateListCmd    `command:"list" subcommands-optional:"true" description:"List templates" webicon:"fas fa-list"`
	Destroy TemplateDestroyCmd `command:"destroy" subcommands-optional:"true" description:"Destroy a template" webicon:"fas fa-trash"`
	Vacuum  TemplateVacuumCmd  `command:"vacuum" subcommands-optional:"true" description:"Vacuum a template" webicon:"fas fa-broom"`
	Help    HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *TemplateCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

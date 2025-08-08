package cmd

type TemplateCmd struct {
	List   TemplateListCmd   `command:"list" subcommands-optional:"true" description:"List available templates" webicon:"fas fa-list"`
	Create TemplateCreateCmd `command:"create" subcommands-optional:"true" description:"Create a new template" webicon:"fas fa-circle-plus" invwebforce:"true"`
	Delete TemplateDeleteCmd `command:"destroy" subcommands-optional:"true" description:"Delete a template image" webicon:"fas fa-trash" invwebforce:"true"`
	Vacuum TemplateVacuumCmd `command:"vacuum" subcommands-optional:"true" description:"Remove danging unfinished templates left over from create failures" webicon:"fas fa-broom"`
	Help   HelpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *TemplateCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

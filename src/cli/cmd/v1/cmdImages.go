package cmd

type ImagesCmd struct {
	List   ImagesListCmd   `command:"list" subcommands-optional:"true" description:"List available images" webicon:"fas fa-list"`
	Create ImagesCreateCmd `command:"create" subcommands-optional:"true" description:"Create a new image" webicon:"fas fa-circle-plus" invwebforce:"true"`
	Delete ImagesDeleteCmd `command:"destroy" subcommands-optional:"true" description:"Delete an image" webicon:"fas fa-trash" invwebforce:"true"`
	Vacuum ImagesVacuumCmd `command:"vacuum" subcommands-optional:"true" description:"Remove danging unfinished images left over from create failures" webicon:"fas fa-broom"`
	Help   HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ImagesCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

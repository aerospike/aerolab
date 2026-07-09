package vagrant

// A BoxListCommand specifies the options and output of vagrant box list.
type BoxListCommand struct {
	BaseCommand
	ErrorResponse
	BoxListResponse
}

// BoxList returns the available vagrant boxes. After setting options as
// appropriate, you must call Run() or Start() followed by Wait() to execute.
// Output will be in Boxes and any error will be in Error.
func (client *VagrantClient) BoxList() *BoxListCommand {
	return &BoxListCommand{
		BaseCommand:     newBaseCommand(client),
		BoxListResponse: newBoxListResponse(),
	}
}

func (cmd *BoxListCommand) init() error {
	return cmd.BaseCommand.init(&cmd.BoxListResponse, "box", "list")
}

// Run the command
func (cmd *BoxListCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *BoxListCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

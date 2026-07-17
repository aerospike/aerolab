package vagrant

// StatusCommand specifies options and output from vagrant status
type StatusCommand struct {
	BaseCommand
	MachineNameArgument
	StatusResponse
}

// Status will return the status of vagrant machines. After setting options as
// appropriate, you must call Run() or Start() followed by Wait() to execute.
// Output will be in Status and any error will be in Error.
func (client *VagrantClient) Status() *StatusCommand {
	return &StatusCommand{
		BaseCommand:    newBaseCommand(client),
		StatusResponse: newStatusResponse(),
	}
}

func (cmd *StatusCommand) init() error {
	if cmd.MachineName != "" {
		return cmd.BaseCommand.init(&cmd.StatusResponse, "status", cmd.MachineName)
	}
	return cmd.BaseCommand.init(&cmd.StatusResponse, "status")
}

// Run the command
func (cmd *StatusCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *StatusCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

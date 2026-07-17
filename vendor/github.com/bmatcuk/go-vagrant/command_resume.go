package vagrant

// A ResumeCommand specifies the options and output of vagrant resume.
type ResumeCommand struct {
	BaseCommand
	MachineNameArgument
	ErrorResponse
	ProvisioningArgument
}

// Resume will restart a vagrant machine that has been suspended or halted.
// After setting options as appropriate, you must call Run() or Start()
// followed by Wait() to execute. Errors will be recorded in Error.
func (client *VagrantClient) Resume() *ResumeCommand {
	return &ResumeCommand{
		BaseCommand:   newBaseCommand(client),
		ErrorResponse: newErrorResponse(),
	}
}

func (cmd *ResumeCommand) init() error {
	args := cmd.appendMachineName(cmd.buildArguments())
	return cmd.BaseCommand.init(&cmd.ErrorResponse, "resume", args...)
}

// Run the command
func (cmd *ResumeCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *ResumeCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

package vagrant

// ReloadCommand specifies the options and output of vagrant reload
type ReloadCommand struct {
	BaseCommand
	MachineNameArgument
	ErrorResponse
	ProvisioningArgument
}

// Reload will halt and restart vagrant machines, reloading config from the
// Vagrantfile. After setting options as appropriate, you must call Run() or
// Start() followed by Wait() to execute. Errors will be recorded in Error.
func (client *VagrantClient) Reload() *ReloadCommand {
	return &ReloadCommand{
		BaseCommand:   newBaseCommand(client),
		ErrorResponse: newErrorResponse(),
	}
}

func (cmd *ReloadCommand) init() error {
	args := cmd.appendMachineName(cmd.buildArguments())
	return cmd.BaseCommand.init(&cmd.ErrorResponse, "reload", args...)
}

// Run the command
func (cmd *ReloadCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *ReloadCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

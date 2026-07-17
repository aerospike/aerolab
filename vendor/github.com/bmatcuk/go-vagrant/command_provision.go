package vagrant

// ProvisionCommand specifies the options and output of vagrant provision
type ProvisionCommand struct {
	BaseCommand
	MachineNameArgument
	ErrorResponse
	ProvisionersArgument
}

// Provision will run the provisioners in a Vagrantfile. After setting options
// as appropriate, you must call Run() or Start() followed by Wait() to
// execute. Errors will be recorded in Error.
func (client *VagrantClient) Provision() *ProvisionCommand {
	return &ProvisionCommand{
		BaseCommand:   newBaseCommand(client),
		ErrorResponse: newErrorResponse(),
	}
}

func (cmd *ProvisionCommand) init() error {
	args := cmd.appendMachineName(cmd.buildArguments())
	return cmd.BaseCommand.init(&cmd.ErrorResponse, "provision", args...)
}

// Run the command
func (cmd *ProvisionCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *ProvisionCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

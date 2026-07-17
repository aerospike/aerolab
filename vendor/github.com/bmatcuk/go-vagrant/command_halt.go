package vagrant

// A HaltCommand specifies the options and output of vagrant halt.
type HaltCommand struct {
	BaseCommand
	MachineNameArgument
	ErrorResponse

	// Force shutdown (equivalent to pulling the power from the machine, default:
	// false)
	Force bool
}

// Halt will shutdown the vagrant machine. After setting options as appropriate,
// you must call Run() or Start() followed by Wait() to execute. Errors will be
// recorded in Error.
func (client *VagrantClient) Halt() *HaltCommand {
	return &HaltCommand{
		BaseCommand:   newBaseCommand(client),
		ErrorResponse: newErrorResponse(),
	}
}

func (cmd *HaltCommand) buildArguments() []string {
	args := []string{}
	if cmd.Force {
		args = append(args, "--force")
	}
	return cmd.appendMachineName(args)
}

func (cmd *HaltCommand) init() error {
	args := cmd.buildArguments()
	return cmd.BaseCommand.init(&cmd.ErrorResponse, "halt", args...)
}

// Run the command
func (cmd *HaltCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *HaltCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

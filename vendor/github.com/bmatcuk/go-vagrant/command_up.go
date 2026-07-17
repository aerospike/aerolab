package vagrant

// UpCommand specifies options and output from vagrant up.
type UpCommand struct {
	BaseCommand
	MachineNameArgument
	UpResponse
	ProvisioningArgument

	// Destroy on error (default: true)
	DestroyOnError bool

	// Enable parallel execution if the provider supports it (default: true)
	Parallel bool

	// Provider to use (default: blank which means vagrant will use the default
	// provider)
	Provider string

	// Install the provider if it isn't installed, if possible (default: false)
	InstallProvider bool
}

// Up creates, if necessary, and brings up the vagrant machines. After setting
// options as appropriate, you must call Run() or Start() followed by Wait()
// to execute. Output will be in VMInfo or Error.
func (client *VagrantClient) Up() *UpCommand {
	return &UpCommand{
		BaseCommand:     newBaseCommand(client),
		UpResponse:      newUpResponse(),
		DestroyOnError:  true,
		Parallel:        true,
		InstallProvider: true,
	}
}

func (cmd *UpCommand) buildArguments() []string {
	args := cmd.ProvisioningArgument.buildArguments()
	if !cmd.DestroyOnError {
		args = append(args, "--no-destroy-on-error")
	}
	if !cmd.Parallel {
		args = append(args, "--no-parallel")
	}
	if len(cmd.Provider) > 0 {
		args = append(args, "--provider", cmd.Provider)
	}
	if !cmd.InstallProvider {
		args = append(args, "--no-install-provider")
	}
	return cmd.appendMachineName(args)
}

func (cmd *UpCommand) init() error {
	args := cmd.buildArguments()
	return cmd.BaseCommand.init(&cmd.UpResponse, "up", args...)
}

// Run the command
func (cmd *UpCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *UpCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

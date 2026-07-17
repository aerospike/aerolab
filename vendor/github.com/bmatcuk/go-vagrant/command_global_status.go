package vagrant

// GlobalStatusCommand specifies options and output from vagrant global-status
type GlobalStatusCommand struct {
	BaseCommand
	GlobalStatusResponse

	// Prune will remove invalid entries.
	Prune bool
}

// GlobalStatus will return the status of all vagrant machines regardless of
// directory. After setting options as appropriate, you must call Run() or
// Start() followed by Wait() to execute. Output will be in Status and any
// error will be in Error.
func (client *VagrantClient) GlobalStatus() *GlobalStatusCommand {
	return &GlobalStatusCommand{
		BaseCommand:          newBaseCommand(client),
		GlobalStatusResponse: newGlobalStatusResponse(),
	}
}

func (cmd *GlobalStatusCommand) buildArguments() []string {
	args := []string{}
	if cmd.Prune {
		args = append(args, "--prune")
	}
	return args
}

func (cmd *GlobalStatusCommand) init() error {
	args := cmd.buildArguments()
	return cmd.BaseCommand.init(&cmd.GlobalStatusResponse, "global-status", args...)
}

// Run the command
func (cmd *GlobalStatusCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *GlobalStatusCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

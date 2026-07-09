package vagrant

// VersionCommand specifies options and output from vagrant version
type VersionCommand struct {
	BaseCommand
	VersionResponse
}

// Version returns the current and latest version of vagrant. After setting
// options as appropriate, you must call Run() or Start() followed by Wait()
// to execute. Output will be in InstalledVersion and LatestVersion and any
// error will be in Error.
func (client *VagrantClient) Version() *VersionCommand {
	return &VersionCommand{
		BaseCommand:     newBaseCommand(client),
		VersionResponse: newVersionResponse(),
	}
}

func (cmd *VersionCommand) init() error {
	return cmd.BaseCommand.init(&cmd.VersionResponse, "version")
}

// Run the command
func (cmd *VersionCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *VersionCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

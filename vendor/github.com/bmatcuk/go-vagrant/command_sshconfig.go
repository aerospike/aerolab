package vagrant

// SSHConfigCommand specifies options and output from vagrant ssh-config
type SSHConfigCommand struct {
	BaseCommand
	MachineNameArgument
	SSHConfigResponse

	// Name of a specific host to get SSH config info for. If not set, info for
	// all VMs will be pulled.
	Host string
}

// SSHConfig will return connection information for connecting to a vagrant
// machine via ssh. After setting options as appropriate, you must call
// Run() or Start() followed by Wait() to execute. Output will be in Configs
// and any error will be in Error.
func (client *VagrantClient) SSHConfig() *SSHConfigCommand {
	return &SSHConfigCommand{
		BaseCommand:       newBaseCommand(client),
		SSHConfigResponse: newSSHConfigResponse(),
	}
}

func (cmd *SSHConfigCommand) buildArguments() []string {
	args := []string{}
	if cmd.Host != "" {
		args = append(args, "--host", cmd.Host)
	}
	return cmd.appendMachineName(args)
}

func (cmd *SSHConfigCommand) init() error {
	args := cmd.buildArguments()
	return cmd.BaseCommand.init(&cmd.SSHConfigResponse, "ssh-config", args...)
}

// Run the command
func (cmd *SSHConfigCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *SSHConfigCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

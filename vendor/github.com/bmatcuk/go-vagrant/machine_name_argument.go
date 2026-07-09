package vagrant

type MachineNameArgument struct {
	// MachineName is the name or ID of a vagrant VM to act on. If unspecified
	// (the default), vagrant will act upon all VMs in the current directory.
	MachineName string
}

func (cmd *MachineNameArgument) appendMachineName(args []string) []string {
	if cmd.MachineName == "" {
		return args
	}
	return append(args, cmd.MachineName)
}

package main

type attachAsadmCmd struct {
	attachShellCmd
}

func (c *attachAsadmCmd) Execute(args []string) error {
	command := append([]string{"asadm"}, args...)
	return c.run(command)
}

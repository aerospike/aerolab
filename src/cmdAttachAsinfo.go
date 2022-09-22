package main

type attachAsinfoCmd struct {
	attachShellCmd
}

func (c *attachAsinfoCmd) Execute(args []string) error {
	command := append([]string{"asinfo"}, args...)
	return c.run(command)
}

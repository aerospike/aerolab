package main

type attachAqlCmd struct {
	attachShellCmd
}

func (c *attachAqlCmd) Execute(args []string) error {
	command := append([]string{"aql"}, args...)
	return c.run(command)
}

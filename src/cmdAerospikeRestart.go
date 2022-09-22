package main

type aerospikeRestartCmd struct {
	aerospikeStartCmd
}

func (c *aerospikeRestartCmd) Execute(args []string) error {
	return c.run(args, "restart")
}

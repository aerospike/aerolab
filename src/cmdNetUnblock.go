package main

type netUnblockCmd struct {
	netBlockCmd
}

func (c *netUnblockCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.run("-D")
}

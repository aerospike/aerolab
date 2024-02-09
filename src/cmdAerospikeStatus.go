package main

import "os"

type aerospikeStatusCmd struct {
	aerospikeStartCmd
}

func (c *aerospikeStatusCmd) Execute(args []string) error {
	return c.run(args, "status", os.Stdout)
}

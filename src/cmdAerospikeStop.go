package main

import "os"

type aerospikeStopCmd struct {
	aerospikeStartCmd
}

func (c *aerospikeStopCmd) Execute(args []string) error {
	return c.run(args, "stop", os.Stdout)

}

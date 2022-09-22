package main

import (
	"fmt"
	"os"
)

type versionCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *versionCmd) Execute(args []string) error {
	fmt.Println(version)
	os.Exit(0)
	return nil
}

package main

import "fmt"

type templateListCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *templateListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	l, err := b.TemplateListFull()
	if err != nil {
		return err
	}
	fmt.Print(l)
	return nil
}

package main

import "fmt"

type templateListCmd struct {
	Json    bool    `short:"j" long:"json" description:"Provide output in json format"`
	NoPager bool    `long:"no-pager" description:"set to disable vertical and horizontal pager"`
	Help    helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *templateListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	l, err := b.TemplateListFull(c.Json, c.NoPager)
	if err != nil {
		return err
	}
	fmt.Print(l)
	return nil
}

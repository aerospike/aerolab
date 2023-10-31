package main

import "fmt"

type templateListCmd struct {
	SortBy     []string `long:"sort-by" description:"sort by field name; must match exact header name; can be specified multiple times; format: asc:name dsc:name ascnum:name dscnum:name"`
	Json       bool     `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty bool     `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	Pager      bool     `long:"pager" description:"set to enable vertical and horizontal pager"`
	Help       helpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *templateListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	l, err := b.TemplateListFull(c.Json, c.Pager, c.JsonPretty, c.SortBy)
	if err != nil {
		return err
	}
	fmt.Print(l)
	return nil
}

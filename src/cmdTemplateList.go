package main

import "fmt"

type templateListCmd struct {
	SortBy     []string `long:"sort-by" description:"sort by field name; must match exact header name; can be specified multiple times; format: asc:name dsc:name ascnum:name dscnum:name"`
	Theme      string   `long:"theme" description:"for standard output, pick a theme: default|nocolor|frame|box"`
	NoNotes    bool     `long:"no-notes" description:"for standard output, do not print extra notes below the tables"`
	Json       bool     `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty bool     `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	Pager      bool     `long:"pager" description:"set to enable vertical and horizontal pager"`
	RenderType string   `long:"render" description:"different output rendering; supported: text,csv,tsv,html,markdown" default:"text"`
	Help       helpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *templateListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	l, err := b.TemplateListFull(c.Json, c.Pager, c.JsonPretty, c.SortBy, c.RenderType, c.Theme, c.NoNotes)
	if err != nil {
		return err
	}
	fmt.Print(l)
	return nil
}

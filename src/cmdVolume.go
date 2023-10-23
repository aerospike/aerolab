package main

import (
	"os"
)

type volumeCmd struct {
	Create volumeCreateCmd `command:"create" subcommands-optional:"true" description:"Create a volume"`
	List   volumeListCmd   `command:"create" subcommands-optional:"true" description:"List volumes"`
	Mount  volumeMountCmd  `command:"create" subcommands-optional:"true" description:"Mount a volume on a node"`
	Delete volumeDeleteCmd `command:"create" subcommands-optional:"true" description:"Delete a volume"`
	Help   helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type volumeListCmd struct {
	Json    bool    `short:"j" long:"json" description:"Provide output in json format"`
	Owner   string  `long:"owner" description:"filter by owner tag/label"`
	NoPager bool    `long:"no-pager" description:"set to disable vertical and horizontal pager"`
	Help    helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Inventory.List.Json = c.Json
	a.opts.Inventory.List.Owner = c.Owner
	a.opts.Inventory.List.NoPager = c.NoPager
	return a.opts.Inventory.List.run(false, false, false, false, false, inventoryShowVolumes)
}

type volumeCreateCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeCreateCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type volumeMountCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeMountCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type volumeDeleteCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeDeleteCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

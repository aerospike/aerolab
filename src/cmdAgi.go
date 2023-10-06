package main

import "os"

type agiCmd struct {
	List      agiListCmd      `command:"list" subcommands-optional:"true" description:"List AGI instances"`
	Create    agiCreateCmd    `command:"create" subcommands-optional:"true" description:"Create AGI instance"`
	Destroy   agiDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy AGI instance"`
	Delete    agiDeleteCmd    `command:"delete" hidden:"true" subcommands-optional:"true" description:"Delete AGI volume"`
	Relabel   agiRelabelCmd   `command:"change-label" subcommands-optional:"true" description:"Change instance name label"`
	Details   agiDetailsCmd   `command:"details" subcommands-optional:"true" description:"Show details of an AGI instance"`
	Retrigger agiRetriggerCmd `command:"run-ingest" subcommands-optional:"true" description:"Retrigger log ingest again (will only do bits that have not been done before)"`
	Attach    agiAttachCmd    `command:"attach" subcommands-optional:"true" description:"Attach to an AGI Instance"`
	AddToken  agiAddTokenCmd  `command:"add-auth-token" subcommands-optional:"true" description:"Add an auth token to AGI Proxy - only valid if token auth type was selected"`
	Exec      agiExecCmd      `command:"exec" hidden:"true" subcommands-optional:"true" description:"Run an AGI subsystem"`
	Help      helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type agiListCmd struct {
	Owner string  `long:"owner" description:"Only show resources tagged with this owner"`
	Json  bool    `short:"j" long:"json" description:"Provide output in json format"`
	Help  helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Inventory.List.Json = c.Json
	a.opts.Inventory.List.Owner = c.Owner
	return a.opts.Inventory.List.run(false, false, false, false, false, inventoryShowAGI)
}

type agiCreateCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiCreateCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type agiAddTokenCmd struct {
	Token string  `short:"t" long:"token" description:"A 64+ character long token to use; if not specified, a random token will be generated"`
	Help  helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiAddTokenCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type agiDestroyCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiDestroyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type agiDeleteCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiDeleteCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type agiRelabelCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiRelabelCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type agiDetailsCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiDetailsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type agiRetriggerCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiRetriggerCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

type agiAttachCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiAttachCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return nil
}

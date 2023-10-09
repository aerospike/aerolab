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

// TODO: addToken, relabel, retrigger, details, delete

func (c *agiCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type agiListCmd struct {
	Owner   string  `long:"owner" description:"Only show resources tagged with this owner"`
	Json    bool    `short:"j" long:"json" description:"Provide output in json format"`
	NoPager bool    `long:"no-pager" description:"set to disable vertical and horizontal pager"`
	Help    helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Inventory.List.Json = c.Json
	a.opts.Inventory.List.Owner = c.Owner
	a.opts.Inventory.List.NoPager = c.NoPager
	return a.opts.Inventory.List.run(false, false, false, false, false, inventoryShowAGI)
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
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	Force       bool            `short:"f" long:"force" description:"force stop before destroy"`
	Parallel    bool            `short:"p" long:"parallel" description:"if destroying many AGI at once, set this to destroy in parallel"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiDestroyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Cluster.Destroy.ClusterName = c.ClusterName
	a.opts.Cluster.Destroy.Force = c.Force
	a.opts.Cluster.Destroy.Parallel = c.Parallel
	return a.opts.Cluster.Destroy.doDestroy("agi", args)
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
	ClusterName TypeClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	Detach      bool            `long:"detach" description:"detach the process stdin - will not kill process on CTRL+C"`
	Tail        []string        `description:"List containing command parameters to execute, ex: [\"ls\",\"/opt\"]"`
	Help        attachCmdHelp   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiAttachCmd) Execute(args []string) error {
	a.opts.Attach.Shell.Node = "1"
	a.opts.Attach.Shell.ClusterName = c.ClusterName
	a.opts.Attach.Shell.Detach = c.Detach
	a.opts.Attach.Shell.Tail = c.Tail
	return a.opts.Attach.Shell.run(args)
}

package main

import (
	"errors"

	"github.com/jessevdk/go-flags"
)

type clientCreateToolsCmd struct {
	clientCreateBaseCmd
	aerospikeVersionCmd
	chDirCmd
}

type clientAddToolsCmd struct {
	ClientName  TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines    TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	StartScript flags.Filename `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	Help        helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCreateToolsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	machines, err := c.createBase(args)
	if err != nil {
		return err
	}
	a.opts.Client.Add.Tools.ClientName = c.ClientName
	a.opts.Client.Add.Tools.StartScript = c.StartScript
	a.opts.Client.Add.Tools.Machines = TypeMachines(intSliceToString(machines, ","))
	return a.opts.Client.Add.Tools.addTools(args)
}

func (c *clientAddToolsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.addTools(args)
}

func (c *clientAddToolsCmd) addTools(args []string) error {
	b.WorkOnClients()
	// TODO CODE HERE
	return errors.New("NOT IMPLEMENTED YET")
}

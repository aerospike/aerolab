package main

import (
	"log"
)

type clientConfigureTrinoCmd struct {
	ClientName     TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines       TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	ConnectCluster string         `short:"s" long:"seed" description:"seed IP:PORT (can be changed later using client configure command)" default:"127.0.0.1:3000"`
	Help           helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureTrinoCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running client.configure.trino")
	b.WorkOnClients()
	a.opts.Attach.Client.ClientName = c.ClientName
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	a.opts.Attach.Client.Machine = c.Machines
	defer backendRestoreTerminal()
	nargs := []string{"/bin/bash", "/opt/trino.sh", "reconfigure", c.ConnectCluster}
	err := a.opts.Attach.Client.run(nargs)
	if err != nil {
		return err
	}
	backendRestoreTerminal()
	log.Printf("To access Trino, use the `attach trino` command.")
	log.Print("Done")
	return nil
}

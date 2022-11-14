package main

import (
	"log"
)

type clientConfigureJupyterCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	Kernels    string         `short:"k" long:"kernels" description:"comma-separated list; options: go,python,java,node,dotnet; default: all kernels"`
	Help       helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureJupyterCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running client.configure.jupyter")
	b.WorkOnClients()
	a.opts.Attach.Client.ClientName = c.ClientName
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	a.opts.Attach.Client.Machine = c.Machines
	switches, err := c.parseKernelsToSwitches(c.Kernels)
	if err != nil {
		return err
	}
	nargs := append([]string{"/bin/bash", "/install.sh"}, switches...)
	err = a.opts.Attach.Client.run(nargs)
	if err != nil {
		return err
	}
	log.Printf("To access jupyter, visit the client IP on port 8888 from your browser. Do `aerolab client list` to get IPs.")
	log.Print("Done")
	return nil
}

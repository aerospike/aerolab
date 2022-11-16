package main

import (
	"log"
)

type clientConfigureVSCodeCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	Kernels    string         `short:"k" long:"kernels" description:"comma-separated list; options: go,python,java,dotnet; default: all kernels"`
	Help       helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureVSCodeCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running client.configure.VSCode")
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
	log.Print("Done, to access vscode, run `aerolab client list` to get the IP, and then visit http://IP:8080 in your browser")
	return nil
}

package main

import (
	"log"

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
	aerospikeVersionCmd
	osSelectorCmd
	chDirCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
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
	a.opts.Client.Add.Tools.aerospikeVersionCmd = c.aerospikeVersionCmd
	a.opts.Client.Add.Tools.osSelectorCmd = c.osSelectorCmd
	a.opts.Client.Add.Tools.chDirCmd = c.chDirCmd
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
	a.opts.Installer.Download.AerospikeVersion = c.AerospikeVersion
	a.opts.Installer.Download.ChDir = c.ChDir
	a.opts.Installer.Download.DistroName = c.DistroName
	a.opts.Installer.Download.DistroVersion = c.DistroVersion
	a.opts.Installer.Download.Password = c.Password
	a.opts.Installer.Download.Username = c.Username
	fn, err := a.opts.Installer.Download.runDownload(args)
	if err != nil {
		return err
	}
	a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
	a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
	a.opts.Files.Upload.Files.Source = flags.Filename(fn)
	a.opts.Files.Upload.Files.Destination = flags.Filename("/opt/installer.tgz")
	a.opts.Files.Upload.IsClient = true
	err = a.opts.Files.Upload.runUpload(args)
	if err != nil {
		return err
	}
	a.opts.Attach.Client.ClientName = c.ClientName
	a.opts.Attach.Client.Machine = c.Machines
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "cd /opt && tar -zxvf installer.tgz && cd aerospike-server-* && ./asinstall"})
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}

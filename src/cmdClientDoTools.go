package main

import (
	"fmt"
	"log"
	"strings"

	flags "github.com/rglonek/jeddevdk-goflags"
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
	Aws  clientAddToolsAwsCmd `no-flag:"true"`
	Help helpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientAddToolsAwsCmd struct {
	IsArm bool `long:"arm" description:"indicate installing on an arm instance"`
}

func init() {
	addBackendSwitch("client.add.tools", "aws", &a.opts.Client.Add.Tools.Aws)
}

func (c *clientCreateToolsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	bv := &backendVersion{c.DistroName.String(), c.DistroVersion.String(), c.AerospikeVersion.String(), c.Aws.IsArm}
	if strings.HasPrefix(c.AerospikeVersion.String(), "latest") || strings.HasSuffix(c.AerospikeVersion.String(), "*") || strings.HasPrefix(c.DistroVersion.String(), "latest") {
		_, err := aerospikeGetUrl(bv, c.Username, c.Password)
		if err != nil {
			return fmt.Errorf("aerospike Version not found: %s", err)
		}
		c.AerospikeVersion = TypeAerospikeVersion(bv.aerospikeVersion)
		c.DistroName = TypeDistro(bv.distroName)
		c.DistroVersion = TypeDistroVersion(bv.distroVersion)
	}

	machines, err := c.createBase(args, "tools")
	if err != nil {
		return err
	}
	a.opts.Client.Add.Tools.ClientName = c.ClientName
	a.opts.Client.Add.Tools.StartScript = c.StartScript
	a.opts.Client.Add.Tools.Machines = TypeMachines(intSliceToString(machines, ","))
	a.opts.Client.Add.Tools.Username = c.Username
	a.opts.Client.Add.Tools.Password = c.Password
	a.opts.Client.Add.Tools.AerospikeVersion = c.AerospikeVersion
	a.opts.Client.Add.Tools.DistroName = c.DistroName
	a.opts.Client.Add.Tools.DistroVersion = c.DistroVersion
	a.opts.Client.Add.Tools.ChDir = c.ChDir
	a.opts.Client.Add.Tools.Aws.IsArm = c.Aws.IsArm
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
	a.opts.Installer.Download.IsArm = c.Aws.IsArm
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
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	a.opts.Attach.Client.Machine = c.Machines
	err = a.opts.Attach.Client.run([]string{"/bin/bash", "-c", "cd /opt && tar -zxvf installer.tgz && cd aerospike-server-* ; ./asinstall"})
	if err != nil {
		return err
	}
	// install early/late scripts
	if string(c.StartScript) != "" {
		a.opts.Files.Upload.ClusterName = TypeClusterName(c.ClientName)
		a.opts.Files.Upload.Nodes = TypeNodes(c.Machines)
		a.opts.Files.Upload.Files.Source = flags.Filename(c.StartScript)
		a.opts.Files.Upload.Files.Destination = flags.Filename("/usr/local/bin/start.sh")
		a.opts.Files.Upload.IsClient = true
		err = a.opts.Files.Upload.runUpload(args)
		if err != nil {
			return err
		}
	}
	log.Print("Done")
	return nil
}

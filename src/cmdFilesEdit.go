package main

import (
	"strconv"

	flags "github.com/rglonek/jeddevdk-goflags"
)

type filesEditCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Client group/Cluster name" default:"mydc"`
	IsClient    bool            `short:"c" long:"client" description:"set this to run the command against client groups instead of clusters"`
	Node        TypeNode        `short:"l" long:"node" description:"Node number" default:"1"`
	Editor      string          `short:"e" long:"editor" description:"Editor command; must be present on the node" default:"vi"`
	Path        filesSingleCmd  `positional-args:"true"`
}

type filesSingleCmd struct {
	Path flags.Filename
}

func (c *filesEditCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
	if c.Path.Path == "" || c.Path.Path == "help" {
		return printHelp("")
	}
	if b == nil {
		return logFatal("Invalid backend")
	}
	err := b.Init()
	if err != nil {
		return logFatal("Could not init backend: %s", err)
	}
	if c.IsClient {
		b.WorkOnClients()
		a.opts.Attach.Client.ClientName = TypeClientName(c.ClusterName)
		a.opts.Attach.Client.Machine = TypeMachines(strconv.Itoa(c.Node.Int()))
		return a.opts.Attach.Client.Execute([]string{c.Editor, string(c.Path.Path)})
	}
	a.opts.Attach.Shell.ClusterName = c.ClusterName
	a.opts.Attach.Shell.Node = TypeNodesPlusAllOption(strconv.Itoa(c.Node.Int()))
	return a.opts.Attach.Shell.Execute([]string{c.Editor, string(c.Path.Path)})
}

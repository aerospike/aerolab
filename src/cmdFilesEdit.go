package main

import (
	"strconv"

	"github.com/jessevdk/go-flags"
)

type filesEditCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
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
		logFatal("Invalid backend")
	}
	err := b.Init()
	if err != nil {
		logFatal("Could not init backend: %s", err)
	}
	a.opts.Attach.Shell.ClusterName = c.ClusterName
	a.opts.Attach.Shell.Node = TypeNodesPlusAllOption(strconv.Itoa(c.Node.Int()))
	return a.opts.Attach.Shell.Execute([]string{c.Editor, string(c.Path.Path)})
}

package main

import "strconv"

type filesEditCmd struct {
	ClusterName string         `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Node        int            `short:"l" long:"node" description:"Node number" default:"1"`
	Editor      string         `short:"e" long:"editor" description:"Editor command; must be present on the node" default:"vi"`
	Path        filesSingleCmd `positional-args:"true"`
}

type filesSingleCmd struct {
	Path string
}

func (c *filesEditCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.Path.Path == "" || c.Path.Path == "help" {
		return printHelp("")
	}
	a.opts.Attach.Shell.ClusterName = c.ClusterName
	a.opts.Attach.Shell.Node = strconv.Itoa(c.Node)
	return a.opts.Attach.Shell.Execute([]string{c.Editor, c.Path.Path})
}

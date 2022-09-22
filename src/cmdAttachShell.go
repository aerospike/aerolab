package main

import (
	"fmt"
	"strconv"
	"strings"
)

type attachShellCmd struct {
	ClusterName string        `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Node        string        `short:"l" long:"node" description:"Node to attach to (or comma-separated list, when using '-- ...'). Example: 'attach shell --node=all -- /some/command' will execute command on all nodes" default:"1"`
	Help        attachCmdHelp `command:"help" subcommands-optional:"true" description:"Print help"`
}

type attachCmdHelp struct{}

func (c *attachCmdHelp) Execute(args []string) error {
	return printHelp("The 'attach' commands support inline execution. Ex:\n $ aerolab attach shell -- ls\n $ aerolab attach asadm -- -e info\n\n")
}

func (c *attachShellCmd) Execute(args []string) error {
	return c.run(args)
}

func (c *attachShellCmd) run(args []string) (err error) {
	if earlyProcess(args) {
		return nil
	}
	var nodes []int
	if c.Node == "all" {
		nodes, err = b.NodeListInCluster(c.ClusterName)
		if err != nil {
			return err
		}
	} else {
		for _, node := range strings.Split(c.Node, ",") {
			nodeInt, err := strconv.Atoi(node)
			if err != nil {
				return err
			}
			nodes = append(nodes, nodeInt)
		}
	}
	if len(nodes) > 1 && len(args) == 0 {
		return fmt.Errorf("%s", "When using more than 1 node in node-attach, you must specify the command to run. For example: 'node-attach -l 1,2,3 -- /command/to/run'")
	}
	for _, node := range nodes {
		if len(nodes) > 1 {
			fmt.Printf(" ======== %s:%d ========\n", c.ClusterName, node)
		}
		erra := b.AttachAndRun(c.ClusterName, node, args)
		if erra != nil {
			if err == nil {
				err = erra
			} else {
				err = fmt.Errorf("%s\n%s", err.Error(), erra.Error())
			}
		}
	}
	return err
}

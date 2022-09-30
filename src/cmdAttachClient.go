package main

import (
	"fmt"
	"strconv"
	"strings"
)

type attachClientCmd struct {
	ClientName TypeClientName `short:"n" long:"name" description:"Client group name" default:"client"`
	Machine    TypeMachines   `short:"l" long:"node" description:"Machine to attach to (or comma-separated list, when using '-- ...'). Example: 'attach shell --node=all -- /some/command' will execute command on all nodes" default:"1"`
	Help       attachCmdHelp  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *attachClientCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.run(args)
}

func (c *attachClientCmd) run(args []string) (err error) {
	b.WorkOnClients()
	var nodes []int
	err = c.Machine.ExpandNodes(string(c.ClientName))
	if err != nil {
		return err
	}
	if c.Machine == "all" {
		nodes, err = b.NodeListInCluster(string(c.ClientName))
		if err != nil {
			return err
		}
	} else {
		for _, node := range strings.Split(c.Machine.String(), ",") {
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
			fmt.Printf(" ======== %s:%d ========\n", string(c.ClientName), node)
		}
		erra := b.AttachAndRun(string(c.ClientName), node, args)
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

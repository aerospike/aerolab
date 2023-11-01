package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

type attachCmdTrino struct {
	ClientName TypeClientName `short:"n" long:"name" description:"Client group name" default:"client"`
	Machine    TypeMachines   `short:"l" long:"node" description:"Machine to attach to (or comma-separated list, when using '-- ...'). Example: 'attach shell --node=all -- /some/command' will execute command on all nodes" default:"1"`
	Namespace  string         `short:"m" long:"namespace" description:"Namespace to use" default:"test"`
	Tail       []string       `description:"List containing command parameters to execute, ex: [\"ls\",\"/opt\"]"`
	Help       attachCmdHelp  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *attachCmdTrino) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.run(args)
}

func (c *attachCmdTrino) run(args []string) (err error) {
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
	if len(args) == 0 {
		b.WorkOnClients()
		inv, _ := b.Inventory("", []int{InventoryItemClients})
		b.WorkOnClients()
		expiry := time.Time{}
		for _, v := range inv.Clusters {
			if v.ClusterName == c.ClientName.String() {
				expires, err := time.Parse(time.RFC3339, v.Expires)
				if err == nil && (expiry.IsZero() || expires.Before(expiry)) {
					expiry = expires
				}
			}
		}
		if !expiry.IsZero() && time.Now().Add(24*time.Hour).After(expiry) {
			log.Printf("WARNING: client expires in %s (%s)", time.Until(expiry), expiry.Format(time.RFC850))
		}
	}
	isInteractive := true
	if len(nodes) > 1 {
		isInteractive = false
	}
	for _, node := range nodes {
		if len(nodes) > 1 {
			fmt.Printf(" ======== %s:%d ========\n", string(c.ClientName), node)
		}
		nargs := []string{"su", "-", "trino", "-c", fmt.Sprintf("bash ./trino --server 127.0.0.1:8080 --catalog aerospike --schema %s", c.Namespace)}
		if len(args) > 0 {
			nargs = append(nargs, args...)
		}
		erra := b.AttachAndRun(string(c.ClientName), node, nargs, isInteractive)
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

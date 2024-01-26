package main

import (
	"errors"
	"fmt"

	"github.com/bestmethod/inslice"
)

type logsShowCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Node        TypeNode        `short:"l" long:"node" description:"Node number" default:"1"`
	Journal     bool            `short:"j" long:"journal" description:"Attempt to get logs from journald instead of log files"`
	LogLocation string          `short:"p" long:"path" description:"Aerospike log file path" default:"/var/log/aerospike.log"`
	Follow      bool            `short:"f" long:"follow" description:"Follow logs instead of displaying full log" webdisable:"true"`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *logsShowCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, string(c.ClusterName)) {
		err = fmt.Errorf("cluster does not exist: %s", string(c.ClusterName))
		return err
	}

	nodes, err := b.NodeListInCluster(string(c.ClusterName))
	if err != nil {
		return err
	}
	if !inslice.HasInt(nodes, c.Node.Int()) {
		return errors.New("node in cluter doesn't exist")
	}

	if c.Journal {
		command := []string{"journalctl", "-u", "aerospike"}
		if c.Follow {
			command = append(command, "-f")
		} else {
			command = append(command, "--no-pager")
		}
		err = b.AttachAndRun(string(c.ClusterName), c.Node.Int(), command, true)
		if err != nil {
			return fmt.Errorf("journalctl error: %s", err)
		}
		return nil
	}

	var command []string
	if c.Follow {
		command = []string{"tail", "-f", c.LogLocation}
	} else {
		command = []string{"cat", c.LogLocation}
	}
	err = b.AttachAndRun(string(c.ClusterName), c.Node.Int(), command, true)
	if err != nil {
		return fmt.Errorf("log cat error: %s", err)
	}
	return nil
}

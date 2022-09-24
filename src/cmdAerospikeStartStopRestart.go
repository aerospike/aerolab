package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

func (c *aerospikeStartCmd) run(args []string, command string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Printf("Running aerospike.%s", command)

	// check cluster exists
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, string(c.ClusterName)) {
		err = fmt.Errorf("cluster does not exist: %s", string(c.ClusterName))
		return err
	}
	var nodes []int
	if c.Nodes == "" {
		nodes, err = b.NodeListInCluster(string(c.ClusterName))
		if err != nil {
			return err
		}
	} else {
		for _, nodeString := range strings.Split(c.Nodes.String(), ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				return err
			}
			nodes = append(nodes, nodeInt)
		}
	}
	if len(nodes) == 0 {
		err = errors.New("found 0 nodes in cluster")
		return err
	}

	var out [][]byte
	if command == "start" {
		var commands [][]string
		commands = append(commands, []string{"service", "aerospike", "start"})
		out, err = b.RunCommands(string(c.ClusterName), commands, nodes)
	} else if command == "stop" {
		var commands [][]string
		commands = append(commands, []string{"service", "aerospike", "stop"})
		out, err = b.RunCommands(string(c.ClusterName), commands, nodes)
	} else if command == "restart" {
		var commands [][]string
		commands = append(commands, []string{"service", "aerospike", "stop"})
		commands = append(commands, []string{"sleep", "2"})
		commands = append(commands, []string{"service", "aerospike", "start"})
		out, err = b.RunCommands(string(c.ClusterName), commands, nodes)
	} else if command == "status" {
		var commands [][]string
		commands = append(commands, []string{"bash", "-c", "ps -ef |grep asd |grep -v grep || exit 0"})
		for _, node := range nodes {
			out, err = b.RunCommands(string(c.ClusterName), commands, []int{node})
			fmt.Printf("--- %s:%d ---\n", string(c.ClusterName), node)
			if err != nil {
				fmt.Println(err, " :: ", string(out[0]))
			} else {
				if len(out[0]) == 0 {
					fmt.Println("stopped")
				} else {
					fmt.Print(string(out[0]))
				}
			}
		}
		return nil
	}
	if err != nil {
		err = fmt.Errorf("%s\n%s", err, out)
		return err
	}

	log.Print("Done")
	return nil
}

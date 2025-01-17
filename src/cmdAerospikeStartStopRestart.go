package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
)

func (c *aerospikeStartCmd) run(args []string, command string, stdout *os.File) error {
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
		err = c.Nodes.ExpandNodes(string(c.ClusterName))
		if err != nil {
			return err
		}
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

	if c.ParallelThreads == 1 || len(nodes) == 1 {
		var out [][]byte
		if command == "cold-start" {
			var commands [][]string
			commands = append(commands, []string{"ipcrm", "--all"})
			commands = append(commands, []string{"service", "aerospike", "start"})
			out, err = b.RunCommands(string(c.ClusterName), commands, nodes)
		} else if command == "start" {
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
				fmt.Fprintf(stdout, "--- %s:%d ---\n", string(c.ClusterName), node)
				if err != nil {
					fmt.Fprintln(stdout, err, " :: ", string(out[0]))
				} else {
					if len(out[0]) == 0 {
						fmt.Fprintln(stdout, "stopped")
					} else {
						fmt.Fprint(stdout, string(out[0]))
					}
				}
				stdout.Sync()
			}
			return nil
		}
		if err != nil {
			err = fmt.Errorf("%s\n%s", err, out)
			return err
		}
	} else {
		parallel := make(chan int, c.ParallelThreads)
		hasError := make(chan bool, len(nodes))
		wait := new(sync.WaitGroup)
		for _, node := range nodes {
			parallel <- 1
			wait.Add(1)
			go c.aerospikeParallel(command, node, parallel, wait, hasError, stdout)
		}
		wait.Wait()
		if len(hasError) > 0 {
			return fmt.Errorf("failed to get logs from %d nodes", len(hasError))
		}
	}

	log.Print("Done")
	return nil
}

func (c *aerospikeStartCmd) aerospikeParallel(command string, node int, parallel chan int, wait *sync.WaitGroup, hasError chan bool, stdout *os.File) {
	var err error
	defer func() {
		<-parallel
		wait.Done()
	}()
	nodes := []int{node}
	var out [][]byte
	if command == "cold-start" {
		var commands [][]string
		commands = append(commands, []string{"ipcrm", "--all"})
		commands = append(commands, []string{"service", "aerospike", "start"})
		out, err = b.RunCommands(string(c.ClusterName), commands, nodes)
	} else if command == "start" {
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
			fmt.Fprintf(stdout, "--- %s:%d ---\n", string(c.ClusterName), node)
			if err != nil {
				fmt.Fprintln(stdout, err, " :: ", string(out[0]))
			} else {
				if len(out[0]) == 0 {
					fmt.Fprintln(stdout, "stopped")
				} else {
					fmt.Fprint(stdout, string(out[0]))
				}
			}
		}
		return
	}
	if err != nil {
		outs := ""
		for _, out1 := range out {
			outs = outs + " ;; " + string(out1)
		}
		err = fmt.Errorf("%s\n%s", err, out)
		log.Printf("Node %d error: %s output: %s", node, err, outs)
		hasError <- true
	}
}

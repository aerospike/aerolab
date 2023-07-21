package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type logsGetCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Journal         bool            `short:"j" long:"journal" description:"Attempt to get logs from journald instead of log files"`
	LogLocation     string          `short:"p" long:"path" description:"Aerospike log file path" default:"/var/log/aerospike.log"`
	Destination     flags.Filename  `short:"d" long:"destination" description:"Destination directory (will be created if doesn't exist)" default:"./logs/"`
	ParallelThreads int             `short:"t" long:"threads" description:"Download logs from this many nodes in parallel" default:"50"`
	Help            helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *logsGetCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.ParallelThreads < 1 {
		return errors.New("thread count must be 1+")
	}
	log.Print("Running logs.get")
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, string(c.ClusterName)) {
		err = fmt.Errorf("cluster does not exist: %s", string(c.ClusterName))
		return err
	}

	var nodes []int
	err = c.Nodes.ExpandNodes(string(c.ClusterName))
	if err != nil {
		return err
	}
	nodesList, err := b.NodeListInCluster(string(c.ClusterName))
	if err != nil {
		return err
	}
	if c.Nodes == "" {
		nodes = nodesList
	} else {
		for _, nodeString := range strings.Split(c.Nodes.String(), ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				return err
			}
			if !inslice.HasInt(nodesList, nodeInt) {
				return fmt.Errorf("node %d does not exist in cluster", nodeInt)
			}
			nodes = append(nodes, nodeInt)
		}
	}
	if len(nodes) == 0 {
		err = errors.New("found 0 nodes in cluster")
		return err
	}

	if _, err := os.Stat(string(c.Destination)); err != nil {
		err = os.MkdirAll(string(c.Destination), 0755)
		if err != nil {
			return err
		}
	}

	c.Destination = flags.Filename(path.Join(string(c.Destination), string(c.ClusterName)))

	if c.ParallelThreads == 1 {
		for _, node := range nodes {
			err = c.get(node)
			if err != nil {
				return err
			}
		}
	} else {
		parallel := make(chan int, c.ParallelThreads)
		hasError := make(chan bool, len(nodes))
		wait := new(sync.WaitGroup)
		for _, node := range nodes {
			parallel <- 1
			wait.Add(1)
			go c.getParallel(node, parallel, wait, hasError)
		}
		wait.Wait()
		if len(hasError) > 0 {
			return fmt.Errorf("failed to get logs from %d nodes", len(hasError))
		}
	}
	log.Print("Done")
	return nil
}

func (c *logsGetCmd) getParallel(node int, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	err := c.get(node)
	if err != nil {
		log.Printf("ERROR getting logs from node %d: %s", node, err)
		hasError <- true
	}
}

func (c *logsGetCmd) get(node int) error {
	fn := string(c.Destination) + "-" + strconv.Itoa(node)
	f, err := os.OpenFile(fn, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if c.Journal {
		command := []string{"journalctl", "-u", "aerospike", "--no-pager"}
		err = b.RunCustomOut(string(c.ClusterName), node, command, os.Stdin, f, f)
		if err != nil {
			return fmt.Errorf("journalctl error: %s", err)
		}
		return nil
	}

	command := []string{"cat", c.LogLocation}
	err = b.RunCustomOut(string(c.ClusterName), node, command, os.Stdin, f, f)
	if err != nil {
		return fmt.Errorf("log cat error: %s", err)
	}
	return nil
}

package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type rosterShowCmd struct {
	aerospikeStartCmd
	Namespace string `short:"m" long:"namespace" description:"Namespace name" default:"test"`
}

func (c *rosterShowCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running roster.show")
	err := c.show(args)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}

func (c *rosterShowCmd) show(args []string) error {
	clist, err := b.ClusterList()
	if err != nil {
		return err
	}

	if !inslice.HasString(clist, string(c.ClusterName)) {
		return errors.New("cluster does not exist")
	}

	nodes, err := b.NodeListInCluster(string(c.ClusterName))
	if err != nil {
		return err
	}

	nodesList := []int{}
	if c.Nodes == "" {
		nodesList = nodes
	} else {
		for _, nn := range strings.Split(c.Nodes.String(), ",") {
			n, err := strconv.Atoi(nn)
			if err != nil {
				return fmt.Errorf("%s is not a number: %s", nn, err)
			}
			if !inslice.HasInt(nodes, n) {
				return fmt.Errorf("node %d does not exist in cluster", n)
			}
			nodesList = append(nodesList, n)
		}
	}

	for _, n := range nodesList {
		out, err := b.RunCommands(string(c.ClusterName), [][]string{[]string{"asinfo", "-v", "roster:namespace=" + c.Namespace}}, []int{n})
		if err != nil {
			fmt.Printf("%s:%d ERROR %s: %s\n", string(c.ClusterName), n, err, strings.Trim(strings.ReplaceAll(string(out[0]), "\n", "; "), "\t\r\n "))
		} else {
			fmt.Printf("%s:%d ROSTER %s\n", string(c.ClusterName), n, strings.Trim(string(out[0]), "\t\r\n "))
		}
	}
	return nil
}

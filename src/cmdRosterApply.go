package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type rosterApplyCmd struct {
	rosterShowCmd
	Roster      string `short:"r" long:"roster" description:"set this to specify customer roster; leave empty to apply observed nodes automatically" default:""`
	NoRecluster bool   `short:"c" long:"no-recluster" description:"if set, will not apply recluster command after roster-set"`
}

func (c *rosterApplyCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running roster.apply")
	err := c.runApply(args)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}

func (c *rosterApplyCmd) runApply(args []string) error {
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

	newRoster := c.Roster

	if newRoster == "" {
		foundNodes := []string{}
		for _, n := range nodesList {
			out, err := b.RunCommands(string(c.ClusterName), [][]string{[]string{"asinfo", "-v", "roster:namespace=" + c.Namespace}}, []int{n})
			if err != nil {
				continue
			}
			observedNodes := strings.Split(strings.Split(strings.Trim(string(out[0]), "\t\r\n "), ":observed_nodes=")[1], ",")
			for _, on := range observedNodes {
				if !inslice.HasString(foundNodes, on) {
					foundNodes = append(foundNodes, on)
				}
			}
		}
		if len(foundNodes) == 0 || inslice.HasString(foundNodes, "null") {
			return errors.New("found at least one node which thinks the observed list is 'null' or failed to find any nodes in roster")
		}
		newRoster = strings.Join(foundNodes, ",")
	}

	rosterCmd := []string{"asinfo", "-v", "roster-set:namespace=" + c.Namespace + ";nodes=" + newRoster}
	if a.opts.Config.Backend.Type == "aws" {
		rosterCmd = []string{"asinfo", "-v", "roster-set:namespace=" + c.Namespace + "\\;nodes=" + newRoster}
	}
	out, err := b.RunCommands(string(c.ClusterName), [][]string{rosterCmd}, nodesList)
	for _, out1 := range out {
		if strings.Contains(string(out1), "ERROR") {
			log.Print(string(out1))
		}
	}
	if err != nil {
		outn := ""
		for _, i := range out {
			outn = outn + string(i) + "\n"
		}
		log.Printf("WARNING: could not apply roster to all the nodes: %s: %s", err, outn)
	}

	if c.NoRecluster {
		log.Print("Done. Roster applied, did not recluster!")
		return nil
	}

	out, err = b.RunCommands(string(c.ClusterName), [][]string{[]string{"asinfo", "-v", "recluster:namespace=" + c.Namespace}}, nodesList)
	if err != nil {
		outn := ""
		for _, i := range out {
			outn = outn + string(i) + "\n"
		}
		log.Printf("WARNING: could not send recluster to all the nodes: %s: %s", err, outn)
	}
	err = c.show(args)
	if err != nil {
		return err
	}
	return nil
}

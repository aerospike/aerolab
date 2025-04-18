package main

import (
	"errors"
	"log"
)

type clusterStopCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Help        helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
	clusterStartStopDestroyCmd
}

func (c *clusterStopCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.doStop("cluster")
}

func (c *clusterStopCmd) doStop(nType string) error {
	log.Println("Running " + nType + ".stop")
	err := c.Nodes.ExpandNodes(string(c.ClusterName))
	if err != nil {
		return err
	}
	cList, nodes, err := c.getBasicData(string(c.ClusterName), c.Nodes.String(), nType)
	if err != nil {
		return err
	}
	var nerr error
	for _, ClusterName := range cList {
		err = b.ClusterStop(ClusterName, nodes[ClusterName])
		if err != nil {
			if nerr == nil {
				nerr = err
			} else {
				nerr = errors.New(nerr.Error() + "\n" + err.Error())
			}
		}
	}
	if nerr != nil {
		return nerr
	}
	log.Println("Done")
	return nil
}

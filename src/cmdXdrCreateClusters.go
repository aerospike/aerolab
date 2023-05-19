package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/bestmethod/inslice"
)

type xdrCreateClustersCmd struct {
	DestinationClusterNames TypeClusterName `short:"N" long:"destinations" description:"Comma-separate list of destination cluster names" default:"destdc"`
	DestinationNodeCount    int             `short:"C" long:"destination-count" description:"Number of nodes per destination cluster" default:"1"`
	clusterCreateCmd
	xdrConnectRealCmd
}

func init() {
	addBackendSwitch("xdr.create-clusters", "aws", &a.opts.XDR.CreateClusters.Aws)
	addBackendSwitch("xdr.create-clusters", "gcp", &a.opts.XDR.CreateClusters.Gcp)
	addBackendSwitch("xdr.create-clusters", "docker", &a.opts.XDR.CreateClusters.Docker)
}

func (c *xdrCreateClustersCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running xdr.create-clusters")
	dst := strings.Split(string(c.DestinationClusterNames), ",")

	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}

	for _, d := range dst {
		if inslice.HasString(clusterList, d) {
			return fmt.Errorf("cluster %s already exists", d)
		}
	}

	err = c.realExecute(args, false)
	if err != nil {
		log.Printf("Failed to create source cluster: %s", err)
	}

	src := c.ClusterName
	srcCount := c.NodeCount
	c.NodeCount = c.DestinationNodeCount
	for _, d := range dst {
		c.ClusterName = TypeClusterName(d)
		err = c.realExecute(args, false)
		if err != nil {
			return fmt.Errorf("failed to create cluster %s: %s", d, err)
		}
	}

	c.ClusterName = src
	c.NodeCount = srcCount

	c.sourceClusterName = c.ClusterName
	c.destinationClusterNames = c.DestinationClusterNames
	err = c.runXdrConnect(args)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}

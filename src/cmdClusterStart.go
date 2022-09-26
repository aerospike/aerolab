package main

import (
	"errors"
	"log"
)

type clusterStartCmd struct {
	clusterStopCmd
	NoFixMesh bool `short:"f" long:"no-fix-mesh" description:"Set to avoid running conf-fix-mesh"`
	NoStart   bool `short:"s" long:"no-start" description:"Set to prevent Aerospike from starting on cluster-start"`
	clusterStartStopDestroyCmd
}

func (c *clusterStartCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running cluster.start")
	cList, nodes, err := c.getBasicData(string(c.ClusterName), c.Nodes.String())
	if err != nil {
		return err
	}
	var nerr error
	for _, ClusterName := range cList {
		err = b.ClusterStart(ClusterName, nodes[ClusterName])
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

	if !c.NoFixMesh {
		a.opts.Conf.FixMesh.ClusterName = c.ClusterName
		a.opts.Conf.FixMesh.Nodes = c.Nodes
		e := a.opts.Conf.FixMesh.Execute(nil)
		if e != nil {
			return e
		}
	}
	if !c.NoStart {
		a.opts.Aerospike.Start.ClusterName = c.ClusterName
		a.opts.Aerospike.Start.Nodes = c.Nodes
		e := a.opts.Aerospike.Start.Execute(nil)
		if e != nil {
			return e
		}
	}

	log.Println("Done")
	return nil
}

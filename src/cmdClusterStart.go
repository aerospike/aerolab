package main

import (
	"errors"
	"fmt"
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
	err := c.Nodes.ExpandNodes(string(c.ClusterName))
	if err != nil {
		return err
	}
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
		for _, ClusterName := range cList {
			a.opts.Conf.FixMesh.ClusterName = TypeClusterName(ClusterName)
			a.opts.Conf.FixMesh.Nodes = TypeNodes(intSliceToString(nodes[ClusterName], ","))
			e := a.opts.Conf.FixMesh.Execute(nil)
			if e != nil {
				return e
			}
		}
	}
	if !c.NoStart {
		for _, ClusterName := range cList {
			a.opts.Aerospike.Start.ClusterName = TypeClusterName(ClusterName)
			a.opts.Aerospike.Start.Nodes = TypeNodes(intSliceToString(nodes[ClusterName], ","))
			e := a.opts.Aerospike.Start.Execute(nil)
			if e != nil {
				return e
			}
		}
	}

	for _, ClusterName := range cList {
		out, err := b.RunCommands(c.ClusterName.String(), [][]string{[]string{"/bin/bash", "-c", "[ ! -d /opt/autoload ] && exit 0; ls /opt/autoload |sort -n |while read f; do /bin/bash /opt/autoload/${f}; done"}}, nodes[ClusterName])
		if err != nil {
			nout := ""
			for _, n := range out {
				nout = nout + "\n" + string(n)
			}
			return fmt.Errorf("could not run autoload scripts: %s: %s", err, nout)
		}
	}
	log.Println("Done")
	return nil
}

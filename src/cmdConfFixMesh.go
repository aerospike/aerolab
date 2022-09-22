package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/bestmethod/inslice"
)

type confFixMeshCmd struct {
	aerospikeStartCmd
}

func (c *confFixMeshCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running conf.fixMesh")

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clusterList, c.ClusterName) {
		err = fmt.Errorf("cluster does not exist: %s", c.ClusterName)
		return err
	}

	// get cluster IPs and node list
	clusterIps, err := b.GetClusterNodeIps(c.ClusterName)
	if err != nil {
		return err
	}
	nodeList, err := b.NodeListInCluster(c.ClusterName)
	if err != nil {
		return err
	}

	nip, err := b.GetNodeIpMap(c.ClusterName, false)
	if err != nil {
		return err
	}
	// fix config if needed, read custom config file path if needed
	for _, i := range nodeList {
		if _, ok := nip[i]; !ok {
			continue
		}
		if nip[i] == "" {
			continue
		}
		files := []fileList{}
		var r [][]string
		r = append(r, []string{"cat", "/etc/aerospike/aerospike.conf"})
		var nr [][]byte
		nr, err = b.RunCommands(c.ClusterName, r, []int{i})
		if err != nil {
			return err
		}
		// nr has contents of aerospike.conf
		newconf, err := fixAerospikeConfig(string(nr[0]), "", "mesh", clusterIps, nodeList)
		if err != nil {
			return err
		}
		files = append(files, fileList{"/etc/aerospike/aerospike.conf", strings.NewReader(newconf), len(newconf)})
		if len(files) > 0 {
			err := b.CopyFilesToCluster(c.ClusterName, files, []int{i})
			if err != nil {
				return err
			}
		}
	}

	log.Print("Done")
	return nil
}

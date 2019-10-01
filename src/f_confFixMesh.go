package main

import (
	"errors"
	"fmt"
)

func (c *config) F_confFixMesh() (err error, ret int64) {

	c.log.Info(INFO_SANITY)
	// check cluster name
	if len(c.ConfFixMesh.ClusterName) == 0 || len(c.ConfFixMesh.ClusterName) > 20 {
		err = errors.New(ERR_CLUSTER_NAME_SIZE)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// get backend
	b, err := getBackend(c.ConfFixMesh.DeployOn, c.ConfFixMesh.RemoteHost, c.ConfFixMesh.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}
	if inArray(clusterList, c.ConfFixMesh.ClusterName) == -1 {
		err = errors.New(fmt.Sprintf("Cluster does not exist: %s", c.ConfFixMesh.ClusterName))
		ret = E_BACKEND_ERROR
		return err, ret
	}

	// get cluster IPs and node list
	clusterIps, err := b.GetClusterNodeIps(c.ConfFixMesh.ClusterName)
	if err != nil {
		ret = E_MAKECLUSTER_NODEIPS
		return err, ret
	}
	nodeList, err := b.NodeListInCluster(c.ConfFixMesh.ClusterName)
	if err != nil {
		ret = E_MAKECLUSTER_NODELIST
		return err, ret
	}

	// fix config if needed, read custom config file path if needed
	for _, i := range nodeList {
		files := []fileList{}
		var r [][]string
		r = append(r, []string{"cat", "/etc/aerospike/aerospike.conf"})
		var nr [][]byte
		nr, err = b.RunCommand(c.ConfFixMesh.ClusterName, r, []int{i})
		if err != nil {
			ret = E_MAKECLUSTER_FIXCONF
			return err, ret
		}
		// nr has contents of aerospike.conf
		newconf, err := fixAerospikeConfig(string(nr[0]), "", "mesh", clusterIps, nodeList)
		if err != nil {
			ret = E_MAKECLUSTER_FIXCONF
			return err, ret
		}
		files = append(files, fileList{"/etc/aerospike/aerospike.conf", []byte(newconf)})
		if len(files) > 0 {
			err := b.CopyFilesToCluster(c.ConfFixMesh.ClusterName, files, []int{i})
			if err != nil {
				ret = E_MAKECLUSTER_COPYFILES
				return err, ret
			}
		}
	}

	// done
	c.log.Info(INFO_DONE)
	return
}

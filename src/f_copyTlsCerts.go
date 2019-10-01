package main

import (
	"errors"
	"path"
	"strconv"
	"strings"
)

func (c *config) F_copyTlsCerts() (err error, ret int64) {

	c.log.Info(INFO_SANITY)

	// get backend
	b, err := getBackend(c.CopyTlsCerts.DeployOn, c.CopyTlsCerts.RemoteHost, c.CopyTlsCerts.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	clusterList, err := b.ClusterList()
	if err != nil {
		return err, 1
	}

	if inArray(clusterList, c.CopyTlsCerts.SourceClusterName) == -1 {
		return errors.New("Source Cluster not found"), 1
	}
	if inArray(clusterList, c.CopyTlsCerts.DestinationClusterName) == -1 {
		return errors.New("Destination Cluster not found"), 1
	}

	sourceClusterNodes, err := b.NodeListInCluster(c.CopyTlsCerts.SourceClusterName)
	if err != nil {
		return err, 1
	}
	destClusterNodes, err := b.NodeListInCluster(c.CopyTlsCerts.DestinationClusterName)
	if err != nil {
		return err, 1
	}

	nodesList := []int{}
	if c.CopyTlsCerts.DestinationNodeList == "" {
		nodesList = destClusterNodes
	} else {
		nodes := strings.Split(c.CopyTlsCerts.DestinationNodeList, ",")
		for _, node := range nodes {
			nodeInt, err := strconv.Atoi(node)
			if err != nil {
				return err, 1
			}
			nodesList = append(nodesList, nodeInt)
			if inArray(destClusterNodes, nodeInt) == -1 {
				return errors.New("Destination Node does not exist in cluster"), 1
			}
		}
	}

	if inArray(sourceClusterNodes, c.CopyTlsCerts.SourceNode) == -1 {
		return errors.New("Source Node does not exist in cluster"), 1
	}

	// nodesList has list of nodes to copy TLS cert to
	// we have: sourceClusterNodes, destClusterNodes, nodesList, and everything in conf struct

	var fl []fileList
	files := []string{"cert.pem", "cacert.pem", "key.pem"}
	for _, file := range files {
		out, err := b.RunCommand(c.CopyTlsCerts.SourceClusterName, [][]string{[]string{"cat", path.Join("/etc/aerospike/ssl/", c.CopyTlsCerts.TlsName, file)}}, []int{c.CopyTlsCerts.SourceNode})
		if err != nil {
			ret = E_BACKEND_ERROR
			return err, ret
		}
		fl = append(fl, fileList{path.Join("/etc/aerospike/ssl/", c.CopyTlsCerts.TlsName, file), out[0]})
	}

	_, err = b.RunCommand(c.CopyTlsCerts.DestinationClusterName, [][]string{[]string{"rm", "-rf", path.Join("/etc/aerospike/ssl/", c.CopyTlsCerts.TlsName)}, []string{"mkdir", "-p", path.Join("/etc/aerospike/ssl/", c.CopyTlsCerts.TlsName)}}, nodesList)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	err = b.CopyFilesToCluster(c.CopyTlsCerts.DestinationClusterName, fl, nodesList)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}
	return
}

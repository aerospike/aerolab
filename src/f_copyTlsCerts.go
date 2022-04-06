package main

import (
	"errors"
	"path"
	"strconv"
	"strings"
)

func (c *config) F_copyTlsCerts() (ret int64, err error) {

	c.log.Info(INFO_SANITY)

	// get backend
	b, err := getBackend(c.CopyTlsCerts.DeployOn, c.CopyTlsCerts.RemoteHost, c.CopyTlsCerts.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	clusterList, err := b.ClusterList()
	if err != nil {
		return 1, err
	}

	if inArray(clusterList, c.CopyTlsCerts.SourceClusterName) == -1 {
		return 1, errors.New("Source Cluster not found")
	}
	if inArray(clusterList, c.CopyTlsCerts.DestinationClusterName) == -1 {
		return 1, errors.New("Destination Cluster not found")
	}

	sourceClusterNodes, err := b.NodeListInCluster(c.CopyTlsCerts.SourceClusterName)
	if err != nil {
		return 1, err
	}
	destClusterNodes, err := b.NodeListInCluster(c.CopyTlsCerts.DestinationClusterName)
	if err != nil {
		return 1, err
	}

	nodesList := []int{}
	if c.CopyTlsCerts.DestinationNodeList == "" {
		nodesList = destClusterNodes
	} else {
		nodes := strings.Split(c.CopyTlsCerts.DestinationNodeList, ",")
		for _, node := range nodes {
			nodeInt, err := strconv.Atoi(node)
			if err != nil {
				return 1, err
			}
			nodesList = append(nodesList, nodeInt)
			if inArray(destClusterNodes, nodeInt) == -1 {
				return 1, errors.New("Destination Node does not exist in cluster")
			}
		}
	}

	if inArray(sourceClusterNodes, c.CopyTlsCerts.SourceNode) == -1 {
		return 1, errors.New("Source Node does not exist in cluster")
	}

	// nodesList has list of nodes to copy TLS cert to
	// we have: sourceClusterNodes, destClusterNodes, nodesList, and everything in conf struct

	var fl []fileList
	files := []string{"cert.pem", "cacert.pem", "key.pem"}
	for _, file := range files {
		out, err := b.RunCommand(c.CopyTlsCerts.SourceClusterName, [][]string{[]string{"cat", path.Join("/etc/aerospike/ssl/", c.CopyTlsCerts.TlsName, file)}}, []int{c.CopyTlsCerts.SourceNode})
		if err != nil {
			ret = E_BACKEND_ERROR
			return ret, err
		}
		fl = append(fl, fileList{path.Join("/etc/aerospike/ssl/", c.CopyTlsCerts.TlsName, file), out[0]})
	}

	_, err = b.RunCommand(c.CopyTlsCerts.DestinationClusterName, [][]string{[]string{"rm", "-rf", path.Join("/etc/aerospike/ssl/", c.CopyTlsCerts.TlsName)}, []string{"mkdir", "-p", path.Join("/etc/aerospike/ssl/", c.CopyTlsCerts.TlsName)}}, nodesList)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	err = b.CopyFilesToCluster(c.CopyTlsCerts.DestinationClusterName, fl, nodesList)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	return
}

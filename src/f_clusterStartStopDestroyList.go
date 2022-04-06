package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func (c *config) clusterStartStopDestroy(DeployOn string, RemoteHost string, AccessPublicKeyFilePath string, ClusterName string, Nodes string, command string) (ret int64, err error) {
	// get backend
	b, err := getBackend(DeployOn, RemoteHost, AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	// check cluster exists
	clusterList, err := b.ClusterList()
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	var cList []string
	if ClusterName != "all" {
		cList = strings.Split(ClusterName, ",")
	} else {
		cList = clusterList
	}
	for _, ClusterName = range cList {
		if inArray(clusterList, ClusterName) == -1 {
			err = fmt.Errorf("Cluster does not exist: %s", ClusterName)
			ret = E_BACKEND_ERROR
			return ret, err
		}
	}
	nodes := make(map[string][]int)
	var nodesC []int
	if Nodes == "" {
		for _, ClusterName = range cList {
			nodesC, err = b.NodeListInCluster(ClusterName)
			if err != nil {
				ret = E_BACKEND_ERROR
				return ret, err
			}
			nodes[ClusterName] = nodesC
		}
	} else {
		for _, nodeString := range strings.Split(Nodes, ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				ret = E_BACKEND_ERROR
				return ret, err
			}
			nodesC = append(nodesC, nodeInt)
		}
		for _, ClusterName = range cList {
			nodes[ClusterName] = nodesC
		}
	}
	for _, ClusterName = range cList {
		if len(nodes[ClusterName]) == 0 {
			err = errors.New("found 0 nodes in cluster")
			ret = E_BACKEND_ERROR
			return ret, err
		}
	}

	var nerr error
	for _, ClusterName = range cList {
		if command == "start" {
			err = b.ClusterStart(ClusterName, nodes[ClusterName])
		} else if command == "stop" {
			err = b.ClusterStop(ClusterName, nodes[ClusterName])
		} else if command == "destroy-force" {
			_ = b.ClusterStop(ClusterName, nodes[ClusterName])
			err = b.ClusterDestroy(ClusterName, nodes[ClusterName])
		} else if command == "destroy" {
			err = b.ClusterDestroy(ClusterName, nodes[ClusterName])
		}
		if err != nil {
			ret = E_BACKEND_ERROR
			nerr = err
		}
	}
	return ret, nerr
}

func (c *config) F_clusterStart() (ret int64, err error) {
	return c.clusterStartStopDestroy(c.ClusterStart.DeployOn, c.ClusterStart.RemoteHost, c.ClusterStart.AccessPublicKeyFilePath, c.ClusterStart.ClusterName, c.ClusterStart.Nodes, "start")
}

func (c *config) F_clusterStop() (ret int64, err error) {
	return c.clusterStartStopDestroy(c.ClusterStop.DeployOn, c.ClusterStop.RemoteHost, c.ClusterStop.AccessPublicKeyFilePath, c.ClusterStop.ClusterName, c.ClusterStop.Nodes, "stop")
}

func (c *config) F_clusterDestroy() (ret int64, err error) {
	if c.ClusterDestroy.Force == 1 {
		return c.clusterStartStopDestroy(c.ClusterDestroy.DeployOn, c.ClusterDestroy.RemoteHost, c.ClusterDestroy.AccessPublicKeyFilePath, c.ClusterDestroy.ClusterName, c.ClusterDestroy.Nodes, "destroy-force")
	} else {
		return c.clusterStartStopDestroy(c.ClusterDestroy.DeployOn, c.ClusterDestroy.RemoteHost, c.ClusterDestroy.AccessPublicKeyFilePath, c.ClusterDestroy.ClusterName, c.ClusterDestroy.Nodes, "destroy")
	}
}

func (c *config) F_clusterList() (ret int64, err error) {
	// get backend
	b, err := getBackend(c.ClusterList.DeployOn, c.ClusterList.RemoteHost, c.ClusterList.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	// check cluster exists
	clusterList, err := b.ClusterListFull()
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	fmt.Println(clusterList)
	return ret, err
}

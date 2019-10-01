package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
)

func (c *config) F_getLogs() (err error, ret int64) {

	c.log.Info(INFO_SANITY)

	// get backend
	b, err := getBackend(c.GetLogs.DeployOn, c.GetLogs.RemoteHost, c.GetLogs.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	clusterList, err := b.ClusterList()
	if err != nil {
		return err, 1
	}

	if inArray(clusterList, c.GetLogs.ClusterName) == -1 {
		return errors.New("Cluster not found"), 1
	}

	clusterNodes, err := b.NodeListInCluster(c.GetLogs.ClusterName)
	if err != nil {
		return err, 1
	}

	nodesList := []int{}
	if c.GetLogs.Nodes == "" {
		nodesList = clusterNodes
	} else {
		nodes := strings.Split(c.GetLogs.Nodes, ",")
		for _, node := range nodes {
			nodeInt, err := strconv.Atoi(node)
			if err != nil {
				return err, 1
			}
			nodesList = append(nodesList, nodeInt)
			if inArray(clusterNodes, nodeInt) == -1 {
				return errors.New("Node does not exist in cluster"), 1
			}
		}
	}

	if _, err := os.Stat(c.GetLogs.OutputDir); os.IsNotExist(err) {
		err = os.MkdirAll(c.GetLogs.OutputDir, 0755)
		if err != nil {
			return err, 1
		}
	}

	for _, node := range nodesList {
		out, err := b.RunCommand(c.GetLogs.ClusterName, [][]string{[]string{"cat", c.GetLogs.InputFile}}, []int{node})
		if err != nil {
			ret = E_BACKEND_ERROR
			return err, ret
		}
		err = ioutil.WriteFile(path.Join(c.GetLogs.OutputDir, fmt.Sprintf("%s_%d.log", c.GetLogs.ClusterName, node)), out[0], 0644)
		if err != nil {
			ret = E_BACKEND_ERROR
			return err, ret
		}
	}
	return
}

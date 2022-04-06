package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func (c *config) aerospikeStartStopRestart(DeployOn string, RemoteHost string, AccessPublicKeyFilePath string, ClusterName string, Nodes string, command string) (ret int64, err error) {
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
	if inArray(clusterList, ClusterName) == -1 {
		err = fmt.Errorf("Cluster does not exist: %s", ClusterName)
		ret = E_BACKEND_ERROR
		return ret, err
	}
	var nodes []int
	if Nodes == "" {
		nodes, err = b.NodeListInCluster(ClusterName)
		if err != nil {
			ret = E_BACKEND_ERROR
			return ret, err
		}
	} else {
		for _, nodeString := range strings.Split(Nodes, ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				ret = E_BACKEND_ERROR
				return ret, err
			}
			nodes = append(nodes, nodeInt)
		}
	}
	if len(nodes) == 0 {
		err = errors.New("found 0 nodes in cluster")
		ret = E_BACKEND_ERROR
		return ret, err
	}

	var out [][]byte
	if command == "start" {
		var commands [][]string
		commands = append(commands, []string{"service", "aerospike", "start"})
		out, err = b.RunCommand(ClusterName, commands, nodes)
	} else if command == "stop" {
		var commands [][]string
		commands = append(commands, []string{"service", "aerospike", "stop"})
		out, err = b.RunCommand(ClusterName, commands, nodes)
	} else if command == "restart" {
		var commands [][]string
		commands = append(commands, []string{"service", "aerospike", "stop"})
		commands = append(commands, []string{"sleep", "1"})
		commands = append(commands, []string{"service", "aerospike", "start"})
		out, err = b.RunCommand(ClusterName, commands, nodes)
	}
	if err != nil {
		err = fmt.Errorf("%s\n%s", err, out)
		ret = E_BACKEND_ERROR
	}
	return ret, err
}

func (c *config) F_startAerospike() (ret int64, err error) {
	return c.aerospikeStartStopRestart(c.StartAerospike.DeployOn, c.StartAerospike.RemoteHost, c.StartAerospike.AccessPublicKeyFilePath, c.StartAerospike.ClusterName, c.StartAerospike.Nodes, "start")
}

func (c *config) F_stopAerospike() (ret int64, err error) {
	return c.aerospikeStartStopRestart(c.StopAerospike.DeployOn, c.StopAerospike.RemoteHost, c.StopAerospike.AccessPublicKeyFilePath, c.StopAerospike.ClusterName, c.StopAerospike.Nodes, "stop")
}

func (c *config) F_restartAerospike() (ret int64, err error) {
	return c.aerospikeStartStopRestart(c.RestartAerospike.DeployOn, c.RestartAerospike.RemoteHost, c.RestartAerospike.AccessPublicKeyFilePath, c.RestartAerospike.ClusterName, c.RestartAerospike.Nodes, "restart")
}

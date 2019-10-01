package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func (c *config) aerospikeStartStopRestart(DeployOn string, RemoteHost string, AccessPublicKeyFilePath string, ClusterName string, Nodes string, command string) (err error, ret int64) {
	// get backend
	b, err := getBackend(DeployOn, RemoteHost, AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	// check cluster exists
	clusterList, err := b.ClusterList()
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}
	if inArray(clusterList, ClusterName) == -1 {
		err = errors.New(fmt.Sprintf("Cluster does not exist: %s", ClusterName))
		ret = E_BACKEND_ERROR
		return err, ret
	}
	var nodes []int
	if Nodes == "" {
		nodes, err = b.NodeListInCluster(ClusterName)
		if err != nil {
			ret = E_BACKEND_ERROR
			return err, ret
		}
	} else {
		for _, nodeString := range strings.Split(Nodes, ",") {
			nodeInt, err := strconv.Atoi(nodeString)
			if err != nil {
				ret = E_BACKEND_ERROR
				return err, ret
			}
			nodes = append(nodes, nodeInt)
		}
	}
	if len(nodes) == 0 {
		err = errors.New("found 0 nodes in cluster")
		ret = E_BACKEND_ERROR
		return err, ret
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
		commands = append(commands, []string{"service", "aerospike", "start"})
		out, err = b.RunCommand(ClusterName, commands, nodes)
	}
	if err != nil {
		err = errors.New(fmt.Sprintf("%s\n%s", err, out))
		ret = E_BACKEND_ERROR
	}
	return err, ret
}

func (c *config) F_startAerospike() (err error, ret int64) {
	return c.aerospikeStartStopRestart(c.StartAerospike.DeployOn, c.StartAerospike.RemoteHost, c.StartAerospike.AccessPublicKeyFilePath, c.StartAerospike.ClusterName, c.StartAerospike.Nodes, "start")
}

func (c *config) F_stopAerospike() (err error, ret int64) {
	return c.aerospikeStartStopRestart(c.StopAerospike.DeployOn, c.StopAerospike.RemoteHost, c.StopAerospike.AccessPublicKeyFilePath, c.StopAerospike.ClusterName, c.StopAerospike.Nodes, "stop")
}

func (c *config) F_restartAerospike() (err error, ret int64) {
	return c.aerospikeStartStopRestart(c.RestartAerospike.DeployOn, c.RestartAerospike.RemoteHost, c.RestartAerospike.AccessPublicKeyFilePath, c.RestartAerospike.ClusterName, c.RestartAerospike.Nodes, "restart")
}

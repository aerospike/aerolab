package main

import (
	"fmt"
	"strconv"
	"strings"
)

func (c *config) node_attach_run(DeployOn string, RemoteHost string, AccessPublicKeyFilePath string, Node string, ClusterName string) (ret int64, err error) {
	// get backend
	b, err := getBackend(DeployOn, RemoteHost, AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	var nodes []int
	if Node == "all" {
		nodes, err = b.NodeListInCluster(ClusterName)
		if err != nil {
			return E_BACKEND_ERROR, err
		}
	} else {
		for _, node := range strings.Split(Node, ",") {
			nodeInt, err := strconv.Atoi(node)
			if err != nil {
				return E_BACKEND_ERROR, err
			}
			nodes = append(nodes, nodeInt)
		}
	}
	if len(nodes) > 1 && len(c.tail) == 0 {
		return E_BACKEND_ERROR, fmt.Errorf("%s", "When using more than 1 node in node-attach, you must specify the command to run. For example: 'node-attach -l 1,2,3 -- /command/to/run'")
	}
	for _, node := range nodes {
		if len(nodes) > 1 {
			fmt.Printf(" ======== %s:%d ========\n", ClusterName, node)
		}
		erra := b.AttachAndRun(ClusterName, node, c.tail)
		if erra != nil {
			if err == nil {
				err = erra
			} else {
				err = fmt.Errorf("%s\n%s", err.Error(), erra.Error())
			}
			ret = E_BACKEND_ERROR
		}
	}
	return
}

func (c *config) F_nodeAttach() (ret int64, err error) {
	return c.node_attach_run(c.NodeAttach.DeployOn, c.NodeAttach.RemoteHost, c.NodeAttach.AccessPublicKeyFilePath, c.NodeAttach.Node, c.NodeAttach.ClusterName)
}

func (c *config) F_aql() (ret int64, err error) {
	command := []string{"aql"}
	c.tail = append(command, c.tail...)
	return c.node_attach_run(c.Aql.DeployOn, c.Aql.RemoteHost, c.Aql.AccessPublicKeyFilePath, c.Aql.Node, c.Aql.ClusterName)
}

func (c *config) F_asinfo() (ret int64, err error) {
	command := []string{"asinfo"}
	c.tail = append(command, c.tail...)
	return c.node_attach_run(c.Asinfo.DeployOn, c.Asinfo.RemoteHost, c.Asinfo.AccessPublicKeyFilePath, c.Asinfo.Node, c.Asinfo.ClusterName)
}

func (c *config) F_asadm() (ret int64, err error) {
	command := []string{"asadm"}
	c.tail = append(command, c.tail...)
	return c.node_attach_run(c.Asadm.DeployOn, c.Asadm.RemoteHost, c.Asadm.AccessPublicKeyFilePath, c.Asadm.Node, c.Asadm.ClusterName)
}

func (c *config) F_logs() (ret int64, err error) {
	c.tail = []string{"journalctl", "-u", "aerospike"}
	return c.node_attach_run(c.Logs.DeployOn, c.Logs.RemoteHost, c.Logs.AccessPublicKeyFilePath, c.Logs.Node, c.Logs.ClusterName)
}

package main

import (
	"fmt"
	"strconv"
	"strings"
)

func (c *config) node_attach_run(DeployOn string, RemoteHost string, AccessPublicKeyFilePath string, Node string, ClusterName string) (err error, ret int64) {
	// get backend
	b, err := getBackend(DeployOn, RemoteHost, AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}
	var nodes []int
	if Node == "all" {
		nodes, err = b.NodeListInCluster(ClusterName)
		if err != nil {
			return err, E_BACKEND_ERROR
		}
	} else {
		for _, node := range strings.Split(Node, ",") {
			nodeInt, err := strconv.Atoi(node)
			if err != nil {
				return err, E_BACKEND_ERROR
			}
			nodes = append(nodes, nodeInt)
		}
	}
	if len(nodes) > 1 && len(c.tail) == 0 {
		return makeError("%s", "When using more than 1 node in node-attach, you must specify the command to run. For example: 'node-attach -l 1,2,3 -- /command/to/run'"), E_BACKEND_ERROR
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
				err = makeError("%s\n%s", err.Error(), erra.Error())
			}
			ret = E_BACKEND_ERROR
		}
	}
	return
}

func (c *config) F_nodeAttach() (err error, ret int64) {
	return c.node_attach_run(c.NodeAttach.DeployOn, c.NodeAttach.RemoteHost, c.NodeAttach.AccessPublicKeyFilePath, c.NodeAttach.Node, c.NodeAttach.ClusterName)
}

func (c *config) F_aql() (err error, ret int64) {
	command := []string{"aql"}
	c.tail = append(command, c.tail...)
	return c.node_attach_run(c.Aql.DeployOn, c.Aql.RemoteHost, c.Aql.AccessPublicKeyFilePath, c.Aql.Node, c.Aql.ClusterName)
}

func (c *config) F_asinfo() (err error, ret int64) {
	command := []string{"asinfo"}
	c.tail = append(command, c.tail...)
	return c.node_attach_run(c.Asinfo.DeployOn, c.Asinfo.RemoteHost, c.Asinfo.AccessPublicKeyFilePath, c.Asinfo.Node, c.Asinfo.ClusterName)
}

func (c *config) F_asadm() (err error, ret int64) {
	command := []string{"asadm"}
	c.tail = append(command, c.tail...)
	return c.node_attach_run(c.Asadm.DeployOn, c.Asadm.RemoteHost, c.Asadm.AccessPublicKeyFilePath, c.Asadm.Node, c.Asadm.ClusterName)
}

func (c *config) F_logs() (err error, ret int64) {
	c.tail = []string{"journalctl", "-u", "aerospike"}
	return c.node_attach_run(c.Logs.DeployOn, c.Logs.RemoteHost, c.Logs.AccessPublicKeyFilePath, c.Logs.Node, c.Logs.ClusterName)
}

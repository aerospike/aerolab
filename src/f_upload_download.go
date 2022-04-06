package main

import (
	"io/ioutil"
	"strconv"
	"strings"
)

func (c *config) F_upload() (ret int64, err error) {
	b, err := getBackend(c.Upload.DeployOn, c.Upload.RemoteHost, c.Upload.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	nodes := strings.Split(c.Upload.Nodes, ",")
	var nodesInt []int
	if len(nodes) == 0 || (len(nodes) == 1 && nodes[0] == "") {
		nodesInt, err = b.NodeListInCluster(c.Upload.ClusterName)
		if err != nil {
			ret = E_BACKEND_ERROR
			return ret, err
		}
	} else {
		for _, node := range nodes {
			nodeInt, err := strconv.Atoi(node)
			if err != nil {
				ret = E_BACKEND_ERROR
				return ret, err
			}
			nodesInt = append(nodesInt, nodeInt)
		}
	}
	contents, err := ioutil.ReadFile(c.Upload.InputFile)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	err = b.CopyFilesToCluster(c.Upload.ClusterName, []fileList{fileList{c.Upload.OutputFile, contents}}, nodesInt)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	return
}

func (c *config) F_download() (ret int64, err error) {
	b, err := getBackend(c.Download.DeployOn, c.Download.RemoteHost, c.Download.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	out, err := b.RunCommand(c.Download.ClusterName, [][]string{[]string{"cat", c.Download.InputFile}}, []int{c.Download.Node})
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	err = ioutil.WriteFile(c.Download.OutputFile, out[0], 0644)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	return
}

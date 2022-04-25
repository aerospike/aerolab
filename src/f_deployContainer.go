package main

import (
	"errors"
	"fmt"
	"strings"
)

func (c *config) F_deployContainer() (ret int64, err error) {
	//docker run -it -p 8081:8081 ubuntu:18.04 /bin/bash

	ret, err = chDir(c.DeployContainer.ChDir)
	if err != nil {
		return ret, err
	}

	c.log.Info(INFO_SANITY)
	// check cluster name
	if len(c.DeployContainer.ContainerName) == 0 || len(c.DeployContainer.ContainerName) > 20 {
		err = errors.New(ERR_CLUSTER_NAME_SIZE)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// get backend
	b, err := getBackend(c.DeployContainer.DeployOn, c.DeployContainer.RemoteHost, c.DeployContainer.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	if inArray(clusterList, c.DeployContainer.ContainerName) != -1 {
		err = fmt.Errorf(ERR_CLUSTER_EXISTS, c.DeployContainer.ContainerName)
		ret = E_BACKEND_ERROR
		return ret, err
	}

	// check if template exists before we check if file exists. if template does, no need for file
	c.log.Info(INFO_CHECK_TEMPLATE)
	templates, err := b.ListTemplates()
	if err != nil {
		return E_BACKEND_ERROR, err
	}

	if inArray(templates, version{"ubuntu", "20.04", "empty"}) == -1 {
		// make template here
		c.log.Info(INFO_MAKETEMPLATE)
		err = b.DeployTemplate(version{"ubuntu", "20.04", "empty"}, "apt-get update; DEBIAN_FRONTEND=noninteractive apt-get -y install tcpdump dnsutils binutils wget net-tools curl vim less man-db telnet netcat iproute2 iptables", []fileList{})
		if err != nil {
			ret = E_MAKECLUSTER_MAKETEMPLATE
			return ret, err
		}
	}

	c.log.Info(INFO_STARTMAKE)
	// deploy template onto aerospike cluster, with changes as requested
	ep := []string{}
	if c.DeployContainer.ExposePorts != "" {
		ep = strings.Split(c.DeployContainer.ExposePorts, ",")
	}
	var privileged bool
	if c.DeployContainer.Privileged == 1 {
		privileged = true
	} else {
		privileged = false
	}
	err = b.DeployClusterWithLimits(version{"ubuntu", "20.04", "empty"}, c.DeployContainer.ContainerName, 1, ep, "", "", "", privileged)
	if err != nil {
		ret = E_MAKECLUSTER_MAKECLUSTER
		return ret, err
	}

	// done
	c.log.Info("Done")
	return 0, nil
}

package main

import (
	"errors"
	"fmt"
)

func (c *config) F_makeClient() (err error, ret int64) {
	// check cluster name
	if len(c.MakeClient.ClientName) == 0 || len(c.MakeClient.ClientName) > 20 {
		err = errors.New(ERR_CLUSTER_NAME_SIZE)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// get backend
	b, err := getBackend(c.MakeClient.DeployOn, c.MakeClient.RemoteHost, c.MakeClient.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}
	if inArray(clusterList, c.MakeClient.ClientName) != -1 {
		err = errors.New(fmt.Sprintf(ERR_CLUSTER_EXISTS, c.MakeClient.ClientName))
		ret = E_BACKEND_ERROR
		return err, ret
	}

	// deploy template onto aerospike cluster, with changes as requested
	c.log.Info(INFO_CHECK_TEMPLATE)
	templates, err := b.ListTemplates()
	if err != nil {
		return err, E_BACKEND_ERROR
	}
	if inArray(templates, version{"ubuntu", "xenial", "none"}) == -1 {
		err = b.DeployTemplate(version{"ubuntu", "xenial", "none"}, "", []fileList{})
		if err != nil {
			ret = E_MAKECLUSTER_MAKECLUSTER
			return err, ret
		}
	}

	var privileged bool
	if c.MakeClient.Privileged == 1 {
		privileged = true
	} else {
		privileged = false
	}
	err = b.DeployClusterWithLimits(version{"ubuntu", "xenial", "none"}, c.MakeClient.ClientName, 1, []string{}, "", "", "", privileged)
	if err != nil {
		ret = E_MAKECLUSTER_MAKECLUSTER
		return err, ret
	}

	if c.MakeClient.Language != "all" {
		err = b.CopyFilesToCluster(c.MakeClient.ClientName, []fileList{fileList{"/root/installer.sh", []byte(fmt.Sprintf("%s\n%s\n%s", clientInstallPre, clientInstallScript[c.MakeClient.Language], clientInstallPost))}}, []int{1})
	} else {
		cis := clientInstallPre
		for _, v := range clientInstallScript {
			cis = fmt.Sprintf("%s\n%s", cis, v)
		}
		cis = fmt.Sprintf("%s\n%s", cis, clientInstallPost)
		err = b.CopyFilesToCluster(c.MakeClient.ClientName, []fileList{fileList{"/root/installer.sh", []byte(cis)}}, []int{1})
	}
	if err != nil {
		ret = E_MAKECLUSTER_MAKECLUSTER
		return err, ret
	}

	err = b.ClusterStart(c.MakeClient.ClientName, nil)
	if err != nil {
		ret = E_MAKECLUSTER_FIXCONF
		return err, ret
	}

	err = b.AttachAndRun(c.MakeClient.ClientName, 1, []string{"/bin/bash", "/root/installer.sh"})
	if err != nil {
		ret = E_MAKECLUSTER_MAKETEMPLATE
	}
	return err, ret
}

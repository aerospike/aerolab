package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func (c *config) F_deployAmc() (err error, ret int64) {
	return errors.New("NO LONGER SUPPORTED!"), E_MAKECLUSTER_VALIDATION
	//docker run -it -p 8081:8081 ubuntu:18.04 /bin/bash
	//lxc = iptables forward rule

	err, ret = chDir(c.DeployAmc.ChDir)
	if err != nil {
		return err, ret
	}

	c.log.Info(INFO_SANITY)
	// check cluster name
	if len(c.DeployAmc.AmcName) == 0 || len(c.DeployAmc.AmcName) > 20 {
		err = errors.New(ERR_CLUSTER_NAME_SIZE)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// check autostart
	if inArray([]string{"y", "n", "YES", "NO", "yes", "no", "Y", "N"}, c.DeployAmc.AutoStart) == -1 {
		err = errors.New(ERR_MUSTBE_YN)
		ret = E_MAKECLUSTER_VALIDATION
		return err, ret
	}

	// get backend
	b, err := getBackend(c.DeployAmc.DeployOn, c.DeployAmc.RemoteHost, c.DeployAmc.AccessPublicKeyFilePath)
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
	if inArray(clusterList, c.DeployAmc.AmcName) != -1 {
		err = errors.New(fmt.Sprintf(ERR_CLUSTER_EXISTS, c.DeployAmc.AmcName))
		ret = E_BACKEND_ERROR
		return err, ret
	}

	// check if template exists before we check if file exists. if template does, no need for file
	c.log.Info(INFO_CHECK_TEMPLATE)
	templates, err := b.ListTemplates()
	if err != nil {
		return err, E_BACKEND_ERROR
	}

	// check latest version early if needed to find out if template does not exist
	var url string
	if (len(c.DeployAmc.AmcVersion) > 5 && c.DeployAmc.AmcVersion[:6] == "latest") || (len(c.DeployAmc.AmcVersion) > 6 && c.DeployAmc.AmcVersion[:7] == "latestc") {
		url, err, ret = c.getUrlAmc()
		if err != nil {
			return err, ret
		}
	}

	var edition string
	if c.DeployAmc.AmcVersion[len(c.DeployAmc.AmcVersion)-1] == 'c' {
		edition = "aerospike-amc-community"
	} else {
		edition = "aerospike-amc-enterprise"
	}

	if inArray(templates, version{"ubuntu", "18.04", "amc-" + c.DeployAmc.AmcVersion}) == -1 {
		// check aerospike version - only required if not downloaded, not checked already above
		if url == "" {
			if _, err := os.Stat(edition + "-" + c.DeployAmc.AmcVersion + "-ubuntu1804.deb"); os.IsNotExist(err) {
				url, err, ret = c.getUrlAmc()
				if err != nil {
					return err, ret
				}
			}
		}

		if c.DeployAmc.AmcVersion[len(c.DeployAmc.AmcVersion)-1] == 'c' {
			c.DeployAmc.AmcVersion = c.DeployAmc.AmcVersion[:len(c.DeployAmc.AmcVersion)-1]
		}

		// download file if not exists
		if _, err := os.Stat(edition + "-" + c.DeployAmc.AmcVersion + "-ubuntu1804.deb"); os.IsNotExist(err) {
			c.log.Info(INFO_DOWNLOAD)
			if strings.HasPrefix(c.DeployAmc.AmcVersion, "3") {
				url = url + edition + "-" + c.DeployAmc.AmcVersion + ".all.x86_64.deb"
			} else {
				url = url + edition + "-" + c.DeployAmc.AmcVersion + "_amd64.deb"
			}
			err = downloadFile(url, edition+"-"+c.DeployAmc.AmcVersion+"-ubuntu1804.deb", c.DeployAmc.Username, c.DeployAmc.Password)
			if err != nil {
				ret = E_MAKECLUSTER_VALIDATION
				return err, ret
			}
		}

		// make template here
		c.log.Info(INFO_MAKETEMPLATE)
		packagefile, err := ioutil.ReadFile(edition + "-" + c.DeployAmc.AmcVersion + "-ubuntu1804.deb")
		if err != nil {
			ret = E_MAKECLUSTER_READFILE
			return err, ret
		}
		nFiles := []fileList{}
		nFiles = append(nFiles, fileList{"/root/installer.deb", packagefile})
		err = b.DeployTemplate(version{"ubuntu", "18.04", "amc-" + c.DeployAmc.AmcVersion}, "apt-get update; apt-get -y install wget net-tools; dpkg -i /root/installer.deb; apt-get -f install", nFiles)
		if err != nil {
			ret = E_MAKECLUSTER_MAKETEMPLATE
			return err, ret
		}
	}

	c.log.Info(INFO_STARTMAKE)
	// deploy template onto aerospike cluster, with changes as requested
	ep := strings.Split(c.DeployAmc.ExposePorts, ",")
	var privileged bool
	if c.DeployAmc.Privileged == 1 {
		privileged = true
	} else {
		privileged = false
	}
	err = b.DeployClusterWithLimits(version{"ubuntu", "18.04", "amc-" + c.DeployAmc.AmcVersion}, c.DeployAmc.AmcName, 1, ep, "", "", "", privileged)
	if err != nil {
		ret = E_MAKECLUSTER_MAKECLUSTER
		return err, ret
	}

	// start cluster
	if c.DeployAmc.AutoStart == "y" {
		var comm [][]string
		comm = append(comm, []string{"service", "amc", "start"})
		_, err = b.RunCommand(c.DeployAmc.AmcName, comm, []int{1})
		if err != nil {
			ret = E_MAKECLUSTER_START
			return err, ret
		}
	}

	// done
	c.log.Info("Done, to access amc console, visit http://localhost:%s", strings.Split(ep[0], ":")[0])
	return nil, 0
}

// because we don't want to repeat this code in 2 places
func (c *config) getUrlAmc() (url string, err error, ret int64) {
	c.log.Info(INFO_CHECK_VERSION)
	if url, c.DeployAmc.AmcVersion, err = aeroFindUrlAmc(c.DeployAmc.AmcVersion, c.DeployAmc.Username, c.DeployAmc.Password); err != nil {
		ret = E_MAKECLUSTER_VALIDATION
		return url, err, ret
	}
	return url, err, ret
}

func (c *config) F_deployContainer() (err error, ret int64) {
	//docker run -it -p 8081:8081 ubuntu:18.04 /bin/bash
	//lxc = iptables forward rule

	err, ret = chDir(c.DeployContainer.ChDir)
	if err != nil {
		return err, ret
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
		return err, ret
	}

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}
	if inArray(clusterList, c.DeployContainer.ContainerName) != -1 {
		err = errors.New(fmt.Sprintf(ERR_CLUSTER_EXISTS, c.DeployContainer.ContainerName))
		ret = E_BACKEND_ERROR
		return err, ret
	}

	// check if template exists before we check if file exists. if template does, no need for file
	c.log.Info(INFO_CHECK_TEMPLATE)
	templates, err := b.ListTemplates()
	if err != nil {
		return err, E_BACKEND_ERROR
	}

	if inArray(templates, version{"ubuntu", "18.04", "empty"}) == -1 {
		// make template here
		c.log.Info(INFO_MAKETEMPLATE)
		err = b.DeployTemplate(version{"ubuntu", "18.04", "empty"}, "apt-get update; apt-get -y install tcpdump dnsutils binutils wget net-tools curl vim less man-db telnet netcat iproute2 iptables", []fileList{})
		if err != nil {
			ret = E_MAKECLUSTER_MAKETEMPLATE
			return err, ret
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
	err = b.DeployClusterWithLimits(version{"ubuntu", "18.04", "empty"}, c.DeployContainer.ContainerName, 1, ep, "", "", "", privileged)
	if err != nil {
		ret = E_MAKECLUSTER_MAKECLUSTER
		return err, ret
	}

	// done
	c.log.Info("Done")
	return nil, 0
}

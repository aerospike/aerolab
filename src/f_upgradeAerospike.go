package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

func (c *config) F_upgradeAerospike() (err error, ret int64) {

	err, ret = chDir(c.UpgradeAerospike.ChDir)
	if err != nil {
		return err, ret
	}

	c.log.Info(INFO_SANITY)
	// check cluster name
	if len(c.UpgradeAerospike.ClusterName) == 0 || len(c.UpgradeAerospike.ClusterName) > 20 {
		err = errors.New(ERR_CLUSTER_NAME_SIZE)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// check OS versions
	if c.UpgradeAerospike.DistroName == "rhel" {
		c.UpgradeAerospike.DistroName = "el"
	}
	if ((c.UpgradeAerospike.DistroName == "el" && (c.UpgradeAerospike.DistroVersion == "8" || c.UpgradeAerospike.DistroVersion == "6" || c.UpgradeAerospike.DistroVersion == "7")) || (c.UpgradeAerospike.DistroName == "ubuntu" && (c.UpgradeAerospike.DistroVersion == "20.04" || c.UpgradeAerospike.DistroVersion == "18.04" || c.UpgradeAerospike.DistroVersion == "16.04" || c.UpgradeAerospike.DistroVersion == "14.04" || c.UpgradeAerospike.DistroVersion == "12.04" || c.UpgradeAerospike.DistroVersion == "best"))) == false {
		err = errors.New(ERR_UNSUPPORTED_OS)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// check files exist
	for _, p := range []string{c.UpgradeAerospike.AccessPublicKeyFilePath} {
		if p != "" {
			if _, err := os.Stat(p); os.IsNotExist(err) {
				err = errors.New(fmt.Sprintf(ERR_FILE_NOT_FOUND, p))
				ret = E_MAKECLUSTER_VALIDATION
				return err, ret
			}
		}
	}

	// check autostart
	if inArray([]string{"y", "n", "YES", "NO", "yes", "no", "Y", "N"}, c.UpgradeAerospike.AutoStartAerospike) == -1 {
		err = errors.New(ERR_MUSTBE_YN)
		ret = E_MAKECLUSTER_VALIDATION
		return err, ret
	}

	// get backend
	b, err := getBackend(c.UpgradeAerospike.DeployOn, c.UpgradeAerospike.RemoteHost, c.UpgradeAerospike.AccessPublicKeyFilePath)
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
	if inArray(clusterList, c.UpgradeAerospike.ClusterName) == -1 {
		err = errors.New(fmt.Sprintf("Error, cluster does not exist: %s", c.UpgradeAerospike.ClusterName))
		ret = E_BACKEND_ERROR
		return err, ret
	}

	// check latest version early if needed to find out if template does not exist
	var url string
	if (len(c.UpgradeAerospike.AerospikeVersion) > 5 && c.UpgradeAerospike.AerospikeVersion[:6] == "latest") || (len(c.UpgradeAerospike.AerospikeVersion) > 6 && c.UpgradeAerospike.AerospikeVersion[:7] == "latestc") {
		url, err, ret = c.getUrlUpgrade()
		if err != nil {
			return err, ret
		}
	}

	if c.UpgradeAerospike.DistroName == "ubuntu" {
		var osVers []string
		if c.UpgradeAerospike.AerospikeVersion[len(c.UpgradeAerospike.AerospikeVersion)-1] == 'c' {
			osVers = checkUbuntuAerospikeVersion(c.UpgradeAerospike.AerospikeVersion[:len(c.UpgradeAerospike.AerospikeVersion)-1])
		} else {
			osVers = checkUbuntuAerospikeVersion(c.UpgradeAerospike.AerospikeVersion)
		}
		if len(osVers) == 0 {
			return errors.New("Could not determine ubuntu version required for this aerospike version."), E_BACKEND_ERROR
		}
		if c.UpgradeAerospike.DistroVersion == "best" {
			c.UpgradeAerospike.DistroVersion = osVers[0]
		} else {
			if inArray(osVers, c.UpgradeAerospike.DistroVersion) == -1 {
				return errors.New(fmt.Sprint("Ubuntu version not supported. This aerospike version supports only: ", osVers)), E_BACKEND_ERROR
			}
		}
	}

	var edition string
	if c.UpgradeAerospike.AerospikeVersion[len(c.UpgradeAerospike.AerospikeVersion)-1] == 'c' {
		edition = "aerospike-server-community"
	} else {
		edition = "aerospike-server-enterprise"
	}

	// check aerospike version - only required if not downloaded, not checked already above
	if url == "" {
		if _, err := os.Stat(edition + "-" + c.UpgradeAerospike.AerospikeVersion + "-" + c.UpgradeAerospike.DistroName + c.UpgradeAerospike.DistroVersion + ".tgz"); os.IsNotExist(err) {
			url, err, ret = c.getUrlUpgrade()
			if err != nil {
				return err, ret
			}
		}
	}

	if c.UpgradeAerospike.AerospikeVersion[len(c.UpgradeAerospike.AerospikeVersion)-1] == 'c' {
		c.UpgradeAerospike.AerospikeVersion = c.UpgradeAerospike.AerospikeVersion[:len(c.UpgradeAerospike.AerospikeVersion)-1]
	}

	// download file if not exists
	fn := edition + "-" + c.UpgradeAerospike.AerospikeVersion + "-" + c.UpgradeAerospike.DistroName + c.UpgradeAerospike.DistroVersion + ".tgz"
	if _, err := os.Stat(edition + "-" + c.UpgradeAerospike.AerospikeVersion + "-" + c.UpgradeAerospike.DistroName + c.UpgradeAerospike.DistroVersion + ".tgz"); os.IsNotExist(err) {
		c.log.Info(INFO_DOWNLOAD)
		url = url + edition + "-" + c.UpgradeAerospike.AerospikeVersion + "-" + c.UpgradeAerospike.DistroName + c.UpgradeAerospike.DistroVersion + ".tgz"
		err = downloadFile(url, edition+"-"+c.UpgradeAerospike.AerospikeVersion+"-"+c.UpgradeAerospike.DistroName+c.UpgradeAerospike.DistroVersion+".tgz", c.UpgradeAerospike.Username, c.UpgradeAerospike.Password)
		if err != nil {
			ret = E_MAKECLUSTER_VALIDATION
			return err, ret
		}
	}

	nodes, err := b.NodeListInCluster(c.UpgradeAerospike.ClusterName)
	if err != nil {
		ret = E_MAKECLUSTER_VALIDATION
		return err, ret
	}

	nodeList := []int{}
	if c.UpgradeAerospike.Nodes == "" {
		nodeList = nodes
	} else {
		nNodes := strings.Split(c.UpgradeAerospike.Nodes, ",")
		for _, nNode := range nNodes {
			nNodeInt, err := strconv.Atoi(nNode)
			if err != nil {
				ret = E_MAKECLUSTER_VALIDATION
				return err, ret
			}
			if inArray(nodes, nNodeInt) == -1 {
				return fmt.Errorf("Node %d does not exist in cluster", nNodeInt), E_MAKECLUSTER_VALIDATION
			}
			nodeList = append(nodeList, nNodeInt)
		}
	}

	fnContents, err := ioutil.ReadFile(fn)
	if err != nil {
		ret = E_MAKECLUSTER_VALIDATION
		return err, ret
	}
	err = b.CopyFilesToCluster(c.UpgradeAerospike.ClusterName, []fileList{fileList{"/root/upgrade.tgz", fnContents}}, nodeList)
	if err != nil {
		ret = E_MAKECLUSTER_VALIDATION
		return err, ret
	}

	c.StopAerospike.ClusterName = c.UpgradeAerospike.ClusterName
	c.StopAerospike.AccessPublicKeyFilePath = c.UpgradeAerospike.AccessPublicKeyFilePath
	c.StopAerospike.DeployOn = c.UpgradeAerospike.DeployOn
	c.StopAerospike.RemoteHost = c.UpgradeAerospike.RemoteHost
	c.StopAerospike.Nodes = c.UpgradeAerospike.Nodes
	err, ret64 := c.F_stopAerospike()
	if err != nil {
		return err, ret64
	}

	for _, i := range nodeList {
		// backup aerospike.conf
		nret, err := b.RunCommand(c.UpgradeAerospike.ClusterName, [][]string{[]string{"cat", "/etc/aerospike/aerospike.conf"}}, []int{i})
		if err != nil {
			ret = E_MAKECLUSTER_VALIDATION
			return err, ret
		}
		nfile := nret[0]
		out, err := b.RunCommand(c.UpgradeAerospike.ClusterName, [][]string{[]string{"tar", "-zxvf", "/root/upgrade.tgz"}}, []int{i})
		if err != nil {
			ret = E_MAKECLUSTER_VALIDATION
			return fmt.Errorf("%s : %s", string(out[0]), err), ret
		}
		// TODO upgrade
		out, err = b.RunCommand(c.UpgradeAerospike.ClusterName, [][]string{[]string{"/bin/bash", "-c", "cd aerospike-server* && ./asinstall"}}, []int{i})
		if err != nil {
			ret = E_MAKECLUSTER_VALIDATION
			return fmt.Errorf("%s : %s", string(out[0]), err), ret
		}
		// recover aerospike.conf backup
		err = b.CopyFilesToCluster(c.UpgradeAerospike.ClusterName, []fileList{fileList{"/etc/aerospike/aerospike.conf", nfile}}, []int{i})
		if err != nil {
			ret = E_MAKECLUSTER_VALIDATION
			return err, ret
		}
	}

	// start cluster
	if c.UpgradeAerospike.AutoStartAerospike == "y" {
		var comm [][]string
		comm = append(comm, []string{"service", "aerospike", "start"})
		_, err = b.RunCommand(c.UpgradeAerospike.ClusterName, comm, nodeList)
		if err != nil {
			ret = E_MAKECLUSTER_START
			return err, ret
		}
	}

	// done
	c.log.Info(INFO_DONE)
	c.log.Info("If you deployed HB differently (i.e. mesh vs multicast, etc), you can run 'conf-fix-mesh' to force and fix mesh in the whole cluster")
	return
}

func (c *config) getUrlUpgrade() (url string, err error, ret int64) {
	c.log.Info(INFO_CHECK_VERSION)
	if url, c.UpgradeAerospike.AerospikeVersion, err = aeroFindUrl(c.UpgradeAerospike.AerospikeVersion, c.UpgradeAerospike.Username, c.UpgradeAerospike.Password); err != nil {
		ret = E_MAKECLUSTER_VALIDATION
		if strings.Contains(fmt.Sprintf("%s", err), "401") {
			err = makeError("%s, Unauthorized access, check enterprise download username and password", err)
		}
		return url, err, ret
	}
	return url, err, ret
}

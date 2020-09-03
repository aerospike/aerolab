package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
)

func (c *config) F_clusterGrow() (err error, ret int64) {

	err, ret = chDir(c.ClusterGrow.ChDir)
	if err != nil {
		return err, ret
	}

	c.log.Info(INFO_SANITY)
	// check cluster name
	if len(c.ClusterGrow.ClusterName) == 0 || len(c.ClusterGrow.ClusterName) > 20 {
		err = errors.New(ERR_CLUSTER_NAME_SIZE)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// check node count
	if c.ClusterGrow.NodeCount > 128 {
		err = errors.New(ERR_MAX_NODE_COUNT)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// check OS versions
	if c.ClusterGrow.DistroName == "rhel" {
		c.ClusterGrow.DistroName = "el"
	}
	if ((c.ClusterGrow.DistroName == "el" && (c.ClusterGrow.DistroVersion == "8" || c.ClusterGrow.DistroVersion == "6" || c.ClusterGrow.DistroVersion == "7")) || (c.ClusterGrow.DistroName == "ubuntu" && (c.ClusterGrow.DistroVersion == "20.04" || c.ClusterGrow.DistroVersion == "18.04" || c.ClusterGrow.DistroVersion == "16.04" || c.ClusterGrow.DistroVersion == "14.04" || c.ClusterGrow.DistroVersion == "12.04" || c.ClusterGrow.DistroVersion == "best"))) == false {
		err = errors.New(ERR_UNSUPPORTED_OS)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// check files exist
	for _, p := range []string{c.ClusterGrow.CustomConfigFilePath, c.ClusterGrow.FeaturesFilePath, c.ClusterGrow.AccessPublicKeyFilePath} {
		if p != "" {
			if _, err := os.Stat(p); os.IsNotExist(err) {
				err = errors.New(fmt.Sprintf(ERR_FILE_NOT_FOUND, p))
				ret = E_MAKECLUSTER_VALIDATION
				return err, ret
			}
		}
	}

	// check heartbeat mode
	if c.ClusterGrow.HeartbeatMode == "mcast" || c.ClusterGrow.HeartbeatMode == "multicast" {
		if c.ClusterGrow.MulticastAddress == "" || c.ClusterGrow.MulticastPort == "" {
			err = errors.New(ERRT_MCAST_ADDR_PORT_EMPTY)
			ret = E_MAKECLUSTER_VALIDATION
			return err, ret
		}
	} else if c.ClusterGrow.HeartbeatMode != "mesh" && c.ClusterGrow.HeartbeatMode != "default" {
		err = errors.New(fmt.Sprintf(ERR_HEARTBEAT_MODE, c.ClusterGrow.HeartbeatMode))
		ret = E_MAKECLUSTER_VALIDATION
		return err, ret
	}

	// check autostart
	if inArray([]string{"y", "n", "YES", "NO", "yes", "no", "Y", "N"}, c.ClusterGrow.AutoStartAerospike) == -1 {
		err = errors.New(ERR_MUSTBE_YN)
		ret = E_MAKECLUSTER_VALIDATION
		return err, ret
	}

	// get backend
	b, err := getBackend(c.ClusterGrow.DeployOn, c.ClusterGrow.RemoteHost, c.ClusterGrow.AccessPublicKeyFilePath)
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
	if inArray(clusterList, c.ClusterGrow.ClusterName) == -1 {
		err = errors.New(fmt.Sprintf("Error, cluster does not exist: %s", c.ClusterGrow.ClusterName))
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
	if (len(c.ClusterGrow.AerospikeVersion) > 5 && c.ClusterGrow.AerospikeVersion[:6] == "latest") || (len(c.ClusterGrow.AerospikeVersion) > 6 && c.ClusterGrow.AerospikeVersion[:7] == "latestc") {
		url, err, ret = c.getUrlGrow()
		if err != nil {
			return err, ret
		}
	}

	if c.ClusterGrow.DistroName == "ubuntu" {
		var osVers []string
		if c.ClusterGrow.AerospikeVersion[len(c.ClusterGrow.AerospikeVersion)-1] == 'c' {
			osVers = checkUbuntuAerospikeVersion(c.ClusterGrow.AerospikeVersion[:len(c.ClusterGrow.AerospikeVersion)-1])
		} else {
			osVers = checkUbuntuAerospikeVersion(c.ClusterGrow.AerospikeVersion)
		}
		if len(osVers) == 0 {
			return errors.New("Could not determine ubuntu version required for this aerospike version."), E_BACKEND_ERROR
		}
		if c.ClusterGrow.DistroVersion == "best" {
			c.ClusterGrow.DistroVersion = osVers[0]
		} else {
			if inArray(osVers, c.ClusterGrow.DistroVersion) == -1 {
				return errors.New(fmt.Sprint("Ubuntu version not supported. This aerospike version supports only: ", osVers)), E_BACKEND_ERROR
			}
		}
	}

	var edition string
	if c.ClusterGrow.AerospikeVersion[len(c.ClusterGrow.AerospikeVersion)-1] == 'c' {
		edition = "aerospike-server-community"
	} else {
		edition = "aerospike-server-enterprise"
	}

	nVer := "centos"
	if inArray(templates, version{c.ClusterGrow.DistroName, c.ClusterGrow.DistroVersion, c.ClusterGrow.AerospikeVersion}) == -1 {
		if c.ClusterGrow.DistroName != "el" || inArray(templates, version{nVer, c.ClusterGrow.DistroVersion, c.ClusterGrow.AerospikeVersion}) == -1 {
			// check aerospike version - only required if not downloaded, not checked already above
			if url == "" {
				if _, err := os.Stat(edition + "-" + c.ClusterGrow.AerospikeVersion + "-" + c.ClusterGrow.DistroName + c.ClusterGrow.DistroVersion + ".tgz"); os.IsNotExist(err) {
					url, err, ret = c.getUrlGrow()
					if err != nil {
						return err, ret
					}
				}
			}

			if c.ClusterGrow.AerospikeVersion[len(c.ClusterGrow.AerospikeVersion)-1] == 'c' {
				c.ClusterGrow.AerospikeVersion = c.ClusterGrow.AerospikeVersion[:len(c.ClusterGrow.AerospikeVersion)-1]
			}

			// download file if not exists
			if _, err := os.Stat(edition + "-" + c.ClusterGrow.AerospikeVersion + "-" + c.ClusterGrow.DistroName + c.ClusterGrow.DistroVersion + ".tgz"); os.IsNotExist(err) {
				c.log.Info(INFO_DOWNLOAD)
				url = url + edition + "-" + c.ClusterGrow.AerospikeVersion + "-" + c.ClusterGrow.DistroName + c.ClusterGrow.DistroVersion + ".tgz"
				err = downloadFile(url, edition+"-"+c.ClusterGrow.AerospikeVersion+"-"+c.ClusterGrow.DistroName+c.ClusterGrow.DistroVersion+".tgz", c.ClusterGrow.Username, c.ClusterGrow.Password)
				if err != nil {
					ret = E_MAKECLUSTER_VALIDATION
					return err, ret
				}
			}

			// make template here
			c.log.Info(INFO_MAKETEMPLATE)
			packagefile, err := ioutil.ReadFile(edition + "-" + c.ClusterGrow.AerospikeVersion + "-" + c.ClusterGrow.DistroName + c.ClusterGrow.DistroVersion + ".tgz")
			if err != nil {
				ret = E_MAKECLUSTER_READFILE
				return err, ret
			}
			nFiles := []fileList{}
			nFiles = append(nFiles, fileList{"/root/installer.tgz", packagefile})
			err = b.DeployTemplate(version{c.ClusterGrow.DistroName, c.ClusterGrow.DistroVersion, c.ClusterGrow.AerospikeVersion}, aerospikeInstallScript[c.ClusterGrow.DistroName], nFiles)
			if err != nil {
				ret = E_MAKECLUSTER_MAKETEMPLATE
				return err, ret
			}
		}
	}

	// version 4.6+ warning check
	aver := strings.Split(c.ClusterGrow.AerospikeVersion, ".")
	aver_major, averr := strconv.Atoi(aver[0])
	if averr != nil {
		return errors.New("Aerospike Version is not an int.int.*"), E_MAKECLUSTER_MAKECLUSTER
	}
	aver_minor, averr := strconv.Atoi(aver[1])
	if averr != nil {
		return errors.New("Aerospike Version is not an int.int.*"), E_MAKECLUSTER_MAKECLUSTER
	}
	if c.ClusterGrow.FeaturesFilePath == "" && (aver_major > 4 || (aver_major == 4 && aver_minor > 5)) {
		c.log.Warn("WARNING: you are attempting to install version 4.6+ and did not provide feature.conf file. This will not work. You can either provide a feature file by using the '-f' switch, or inside your ~/aero-lab-common.conf as:\n\n[MakeCluster]\nFeaturesFilePath=/path/to/features.conf\n\nPress ENTER if you still wish to proceed")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	c.log.Info(INFO_STARTMAKE)
	// deploy template onto aerospike cluster, with changes as requested
	nlic, err := b.NodeListInCluster(c.ClusterGrow.ClusterName)
	if err != nil {
		ret = E_MAKECLUSTER_MAKECLUSTER
		return err, ret
	}
	if len(nlic)+c.ClusterGrow.NodeCount > 128 {
		err = errors.New(ERR_MAX_NODE_COUNT)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}
	if c.ClusterGrow.CpuLimit == "" && c.ClusterGrow.RamLimit == "" && c.ClusterGrow.SwapLimit == "" && c.ClusterGrow.Privileged != 1 {
		err = b.DeployCluster(version{c.ClusterGrow.DistroName, c.ClusterGrow.DistroVersion, c.ClusterGrow.AerospikeVersion}, c.ClusterGrow.ClusterName, c.ClusterGrow.NodeCount, []string{})
	} else {
		var privileged bool
		if c.ClusterGrow.Privileged == 1 {
			privileged = true
		} else {
			privileged = false
		}
		err = b.DeployClusterWithLimits(version{c.ClusterGrow.DistroName, c.ClusterGrow.DistroVersion, c.ClusterGrow.AerospikeVersion}, c.ClusterGrow.ClusterName, c.ClusterGrow.NodeCount, []string{}, c.ClusterGrow.CpuLimit, c.ClusterGrow.RamLimit, c.ClusterGrow.SwapLimit, privileged)
	}
	if err != nil {
		ret = E_MAKECLUSTER_MAKECLUSTER
		return err, ret
	}

	files := []fileList{}

	err = b.ClusterStart(c.ClusterGrow.ClusterName, nil)
	if err != nil {
		ret = E_MAKECLUSTER_FIXCONF
		return err, ret
	}
	// get cluster IPs and node list
	clusterIps, err := b.GetClusterNodeIps(c.ClusterGrow.ClusterName)
	if err != nil {
		ret = E_MAKECLUSTER_NODEIPS
		return err, ret
	}
	nodeList, err := b.NodeListInCluster(c.ClusterGrow.ClusterName)
	if err != nil {
		ret = E_MAKECLUSTER_NODELIST
		return err, ret
	}

	newconf := ""
	// fix config if needed, read custom config file path if needed
	if c.ClusterGrow.CustomConfigFilePath != "" {
		conf, err := ioutil.ReadFile(c.ClusterGrow.CustomConfigFilePath)
		if err != nil {
			ret = E_MAKECLUSTER_READCONF
			return err, ret
		}
		newconf, err = fixAerospikeConfig(string(conf), c.ClusterGrow.MulticastAddress, c.ClusterGrow.HeartbeatMode, clusterIps, nodeList)
		if err != nil {
			ret = E_MAKECLUSTER_FIXCONF
			return err, ret
		}

	} else {
		if c.ClusterGrow.HeartbeatMode == "mesh" || c.ClusterGrow.HeartbeatMode == "mcast" {
			var r [][]string
			r = append(r, []string{"cat", "/etc/aerospike/aerospike.conf"})
			var nr [][]byte
			nr, err = b.RunCommand(c.ClusterGrow.ClusterName, r, []int{nodeList[0]})
			if err != nil {
				ret = E_MAKECLUSTER_FIXCONF
				return err, ret
			}
			// nr has contents of aerospike.conf
			newconf, err = fixAerospikeConfig(string(nr[0]), c.ClusterGrow.MulticastAddress, c.ClusterGrow.HeartbeatMode, clusterIps, nodeList)
			if err != nil {
				ret = E_MAKECLUSTER_FIXCONF
				return err, ret
			}
		}
	}

	// add cluster name

	newconf2 := newconf
	if c.ClusterGrow.OverrideASClusterName == 1 {
		newconf2, err = fixClusteNameConfig(string(newconf), c.ClusterGrow.ClusterName)
		if err != nil {
			ret = E_MAKECLUSTER_FIXCONF_CLUSTER_NAME
			return err, ret
		}
	}

	files = append(files, fileList{"/etc/aerospike/aerospike.conf", []byte(newconf2)})

	// load features file path if needed
	if c.ClusterGrow.FeaturesFilePath != "" {
		conf, err := ioutil.ReadFile(c.ClusterGrow.FeaturesFilePath)
		if err != nil {
			ret = E_MAKECLUSTER_READFEATURES
			return err, ret
		}
		files = append(files, fileList{"/etc/aerospike/features.conf", conf})
	}

	nodeListNew := []int{}
	for _, i := range nodeList {
		if inArray(nlic, i) == -1 {
			nodeListNew = append(nodeListNew, i)
		}
	}

	// actually save files to nodes in cluster if needed
	if len(files) > 0 {
		// copy to those in nodeList which are not in nlic
		err := b.CopyFilesToCluster(c.ClusterGrow.ClusterName, files, nodeListNew)
		if err != nil {
			ret = E_MAKECLUSTER_COPYFILES
			return err, ret
		}
	}

	// if docker fix logging location
	var out [][]byte
	out, err = b.RunCommand(c.ClusterGrow.ClusterName, [][]string{[]string{"cat", "/etc/aerospike/aerospike.conf"}}, nodeListNew)
	if err != nil {
		ret = E_MAKECLUSTER_FIXCONF
		return err, ret
	}
	if b.GetBackendName() == "docker" {
		var in [][]byte
		for _, out1 := range out {
			in1 := strings.Replace(string(out1), "console {", "file /var/log/aerospike.log {", 1)
			in = append(in, []byte(in1))
		}
		for i, node := range nodeListNew {
			err = b.CopyFilesToCluster(c.ClusterGrow.ClusterName, []fileList{fileList{"/etc/aerospike/aerospike.conf", in[i]}}, []int{node})
			if err != nil {
				ret = E_MAKECLUSTER_FIXCONF
				return err, ret
			}
		}
	}

	// also create locations if not exist
	for i, node := range nodeListNew {
		log := string(out[i])
		scanner := bufio.NewScanner(strings.NewReader(log))
		for scanner.Scan() {
			t := scanner.Text()
			if strings.Contains(t, "/var") || strings.Contains(t, "/opt") || strings.Contains(t, "/etc") || strings.Contains(t, "/tmp") {
				tStart := strings.Index(t, " /") + 1
				var nLoc string
				if strings.Contains(t[tStart:], " ") {
					tEnd := strings.Index(t[tStart:], " ")
					nLoc = t[tStart:(tEnd + tStart)]
				} else {
					nLoc = t[tStart:]
				}
				var nDir string
				if strings.Contains(t, "file /") || strings.Contains(t, "xdr-digestlog-path /") || strings.Contains(t, "file:/") {
					nDir = path.Dir(nLoc)
				} else {
					nDir = nLoc
				}
				// create dir
				nout, err := b.RunCommand(c.ClusterGrow.ClusterName, [][]string{[]string{"mkdir", "-p", nDir}}, []int{node})
				if err != nil {
					return errors.New(fmt.Sprintf("Could not create directory in container: %s\n%s\n%s", nDir, err, string(nout[0]))), 1
				}
			}
		}
	}

	// aws-public-ip
	if c.ClusterGrow.PublicIP == 1 {
		var systemdScript fileList
		var accessAddressScript fileList
		systemdScript.filePath = "/etc/systemd/system/aerospike-access-address.service"
		systemdScript.fileContents = []byte(`[Unit]
		Description=Fix Aerospike access-address and alternate-access-address
		RequiredBy=aerospike.service
		Before=aerospike.service
				
		[Service]
		Type=oneshot
		ExecStart=/bin/bash /usr/local/bin/aerospike-access-address.sh
				
		[Install]
		WantedBy=multi-user.target`)
		accessAddressScript.filePath = "/usr/local/bin/aerospike-access-address.sh"
		accessAddressScript.fileContents = []byte(`grep 'alternate-access-address' /etc/aerospike/aerospike.conf
if [ $? -ne 0 ]
then
sed -i 's/address any/address any\naccess-address\nalternate-access-address\n/g' /etc/aerospike/aerospike.conf
fi
sed -e "s/access-address.*/access-address $(curl http://169.254.169.254/latest/meta-data/local-ipv4)/g" -e "s/alternate-access-address.*/alternate-access-address $(curl http://169.254.169.254/latest/meta-data/public-ipv4)/g"  /etc/aerospike/aerospike.conf > ~/aerospike.conf.new && cp /etc/aerospike/aerospike.conf /etc/aerospike/aerospike.conf.bck && cp ~/aerospike.conf.new /etc/aerospike/aerospike.conf
`)
		err = b.CopyFilesToCluster(c.ClusterGrow.ClusterName, []fileList{systemdScript, accessAddressScript}, nodeList)
		if err != nil {
			ret = E_MAKECLUSTER_START
			return makeError("Could not make access-address script in aws: %s", err), ret
		}
		bouta, err := b.RunCommand(c.ClusterGrow.ClusterName, [][]string{[]string{"chmod", "755", "/usr/local/bin/aerospike-access-address.sh"}, []string{"chmod", "755", "/etc/systemd/system/aerospike-access-address.service"}, []string{"systemctl", "daemon-reload"}, []string{"systemctl", "enable", "aerospike-access-address.service"}, []string{"service", "aerospike-access-address", "start"}}, nodeList)
		if err != nil {
			ret = E_MAKECLUSTER_START
			nstr := ""
			for _, bout := range bouta {
				nstr = fmt.Sprintf("%s\n%s", nstr, string(bout))
			}
			return makeError("Could not register access-address script in aws: %s\n%s", err, nstr), ret
		}
	}

	// start cluster
	if c.ClusterGrow.AutoStartAerospike == "y" {
		var comm [][]string
		comm = append(comm, []string{"service", "aerospike", "start"})
		_, err = b.RunCommand(c.ClusterGrow.ClusterName, comm, nodeListNew)
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

func (c *config) getUrlGrow() (url string, err error, ret int64) {
	c.log.Info(INFO_CHECK_VERSION)
	if url, c.ClusterGrow.AerospikeVersion, err = aeroFindUrl(c.ClusterGrow.AerospikeVersion, c.ClusterGrow.Username, c.ClusterGrow.Password); err != nil {
		ret = E_MAKECLUSTER_VALIDATION
		if strings.Contains(fmt.Sprintf("%s", err), "401") {
			err = makeError("%s, Unauthorized access, check enterprise download username and password", err)
		}
		return url, err, ret
	}
	return url, err, ret
}

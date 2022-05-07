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

func (c *config) F_makeCluster() (ret int64, err error) {

	ret, err = chDir(c.MakeCluster.ChDir)
	if err != nil {
		return ret, err
	}

	c.log.Info(INFO_SANITY)

	if c.MakeCluster.DistroName != "ubuntu" && c.MakeCluster.DistroVersion == "best" {
		err = errors.New("Please specify OS/Distro version (-i switch) when specifying non-ubuntu distro")
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// check cluster name
	if len(c.MakeCluster.ClusterName) == 0 || len(c.MakeCluster.ClusterName) > 20 {
		err = errors.New(ERR_CLUSTER_NAME_SIZE)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// check node count
	if c.MakeCluster.NodeCount > 128 {
		err = errors.New(ERR_MAX_NODE_COUNT)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	if c.MakeCluster.NodeCount > 1 && c.MakeCluster.ExposePortsToHost != "" {
		err = errors.New("ExportPorts feature can only be used if node-count is 1")
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// check OS versions
	if c.MakeCluster.DistroName == "rhel" {
		c.MakeCluster.DistroName = "el"
	}
	if !((c.MakeCluster.DistroName == "el" && (c.MakeCluster.DistroVersion == "8" || c.MakeCluster.DistroVersion == "6" || c.MakeCluster.DistroVersion == "7")) || (c.MakeCluster.DistroName == "ubuntu" && (c.MakeCluster.DistroVersion == "20.04" || c.MakeCluster.DistroVersion == "18.04" || c.MakeCluster.DistroVersion == "16.04" || c.MakeCluster.DistroVersion == "14.04" || c.MakeCluster.DistroVersion == "12.04" || c.MakeCluster.DistroVersion == "best"))) {
		err = errors.New(ERR_UNSUPPORTED_OS)
		ret = E_MAKECLUSTER_VALIDATION
		return
	}

	// check files exist
	for _, p := range []string{c.MakeCluster.CustomConfigFilePath, c.MakeCluster.FeaturesFilePath, c.MakeCluster.AccessPublicKeyFilePath} {
		if p != "" {
			if _, err := os.Stat(p); os.IsNotExist(err) {
				err = fmt.Errorf(ERR_FILE_NOT_FOUND, p)
				ret = E_MAKECLUSTER_VALIDATION
				return ret, err
			}
		}
	}

	// check heartbeat mode
	if c.MakeCluster.HeartbeatMode == "mcast" || c.MakeCluster.HeartbeatMode == "multicast" {
		if c.MakeCluster.MulticastAddress == "" || c.MakeCluster.MulticastPort == "" {
			err = errors.New(ERRT_MCAST_ADDR_PORT_EMPTY)
			ret = E_MAKECLUSTER_VALIDATION
			return ret, err
		}
	} else if c.MakeCluster.HeartbeatMode != "mesh" && c.MakeCluster.HeartbeatMode != "default" {
		err = fmt.Errorf(ERR_HEARTBEAT_MODE, c.MakeCluster.HeartbeatMode)
		ret = E_MAKECLUSTER_VALIDATION
		return ret, err
	}

	// check autostart
	if inArray([]string{"y", "n", "YES", "NO", "yes", "no", "Y", "N"}, c.MakeCluster.AutoStartAerospike) == -1 {
		err = errors.New(ERR_MUSTBE_YN)
		ret = E_MAKECLUSTER_VALIDATION
		return ret, err
	}

	// get backend
	b, err := getBackend(c.MakeCluster.DeployOn, c.MakeCluster.RemoteHost, c.MakeCluster.AccessPublicKeyFilePath)
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

	if inArray(clusterList, c.MakeCluster.ClusterName) != -1 {
		err = fmt.Errorf(ERR_CLUSTER_EXISTS, c.MakeCluster.ClusterName)
		ret = E_BACKEND_ERROR
		return ret, err
	}

	// check if template exists before we check if file exists. if template does, no need for file
	c.log.Info(INFO_CHECK_TEMPLATE)
	templates, err := b.ListTemplates()
	if err != nil {
		return E_BACKEND_ERROR, err
	}

	// check latest version early if needed to find out if template does not exist
	var url string
	if (len(c.MakeCluster.AerospikeVersion) > 5 && c.MakeCluster.AerospikeVersion[:6] == "latest") || (len(c.MakeCluster.AerospikeVersion) > 6 && c.MakeCluster.AerospikeVersion[:7] == "latestc") || strings.HasSuffix(c.MakeCluster.AerospikeVersion, "*") {
		url, ret, err = c.getUrl()
		if err != nil {
			return ret, err
		}
	} else {
		if inArray(templates, version{c.MakeCluster.DistroName, c.MakeCluster.DistroVersion, c.MakeCluster.AerospikeVersion}) == -1 {
			if c.MakeCluster.DistroName != "el" || inArray(templates, version{"centos", c.MakeCluster.DistroVersion, c.MakeCluster.AerospikeVersion}) == -1 {
				url, ret, err = c.getUrl()
				if err != nil {
					return ret, fmt.Errorf("Version not found: %s", err)
				}
			}
		}
	}

	if c.MakeCluster.DistroName == "ubuntu" {
		var osVers []string
		if c.MakeCluster.AerospikeVersion[len(c.MakeCluster.AerospikeVersion)-1] == 'c' {
			osVers = checkUbuntuAerospikeVersion(c.MakeCluster.AerospikeVersion[:len(c.MakeCluster.AerospikeVersion)-1])
		} else {
			osVers = checkUbuntuAerospikeVersion(c.MakeCluster.AerospikeVersion)
		}
		if len(osVers) == 0 {
			return E_BACKEND_ERROR, errors.New("Could not determine ubuntu version required for this aerospike version.")
		}
		if c.MakeCluster.DistroVersion == "best" {
			c.MakeCluster.DistroVersion = osVers[0]
		} else {
			if inArray(osVers, c.MakeCluster.DistroVersion) == -1 {
				return E_BACKEND_ERROR, errors.New(fmt.Sprint("Ubuntu version not supported. This aerospike version supports only: ", osVers))
			}
		}
	}

	var edition string
	if c.MakeCluster.AerospikeVersion[len(c.MakeCluster.AerospikeVersion)-1] == 'c' {
		edition = "aerospike-server-community"
	} else {
		edition = "aerospike-server-enterprise"
	}

	nVer := "centos"
	c.log.Info("Distro = %s:%s ; AerospikeVersion = %s", c.MakeCluster.DistroName, c.MakeCluster.DistroVersion, c.MakeCluster.AerospikeVersion)
	if inArray(templates, version{c.MakeCluster.DistroName, c.MakeCluster.DistroVersion, c.MakeCluster.AerospikeVersion}) == -1 {
		if c.MakeCluster.DistroName != "el" || inArray(templates, version{nVer, c.MakeCluster.DistroVersion, c.MakeCluster.AerospikeVersion}) == -1 {
			// check aerospike version - only required if not downloaded, not checked already above
			if url == "" {
				if _, err := os.Stat(edition + "-" + c.MakeCluster.AerospikeVersion + "-" + c.MakeCluster.DistroName + c.MakeCluster.DistroVersion + ".tgz"); os.IsNotExist(err) {
					url, ret, err = c.getUrl()
					if err != nil {
						return ret, err
					}
				}
			}

			if c.MakeCluster.AerospikeVersion[len(c.MakeCluster.AerospikeVersion)-1] == 'c' {
				c.MakeCluster.AerospikeVersion = c.MakeCluster.AerospikeVersion[:len(c.MakeCluster.AerospikeVersion)-1]
			}

			// download file if not exists
			if _, err := os.Stat(edition + "-" + c.MakeCluster.AerospikeVersion + "-" + c.MakeCluster.DistroName + c.MakeCluster.DistroVersion + ".tgz"); os.IsNotExist(err) {
				c.log.Info(INFO_DOWNLOAD)
				url = url + edition + "-" + c.MakeCluster.AerospikeVersion + "-" + c.MakeCluster.DistroName + c.MakeCluster.DistroVersion + ".tgz"
				err = downloadFile(url, edition+"-"+c.MakeCluster.AerospikeVersion+"-"+c.MakeCluster.DistroName+c.MakeCluster.DistroVersion+".tgz", c.MakeCluster.Username, c.MakeCluster.Password)
				if err != nil {
					ret = E_MAKECLUSTER_VALIDATION
					return ret, err
				}
			}

			// make template here
			c.log.Info(INFO_MAKETEMPLATE)
			packagefile, err := ioutil.ReadFile(edition + "-" + c.MakeCluster.AerospikeVersion + "-" + c.MakeCluster.DistroName + c.MakeCluster.DistroVersion + ".tgz")
			if err != nil {
				ret = E_MAKECLUSTER_READFILE
				return ret, err
			}
			nFiles := []fileList{}
			nFiles = append(nFiles, fileList{"/root/installer.tgz", packagefile})
			var nscript string
			if b.GetBackendName() != "docker" {
				nscript = aerospikeInstallScript[c.MakeCluster.DistroName]
			} else {
				nscript = aerospikeInstallScriptDocker[c.MakeCluster.DistroName]
			}
			err = b.DeployTemplate(version{c.MakeCluster.DistroName, c.MakeCluster.DistroVersion, c.MakeCluster.AerospikeVersion}, nscript, nFiles)
			if err != nil {
				ret = E_MAKECLUSTER_MAKETEMPLATE
				return ret, err
			}
		}
	}

	// version 4.6+ warning check
	aver := strings.Split(c.MakeCluster.AerospikeVersion, ".")
	aver_major, averr := strconv.Atoi(aver[0])
	if averr != nil {
		return E_MAKECLUSTER_MAKECLUSTER, errors.New("Aerospike Version is not an int.int.*")
	}
	aver_minor, averr := strconv.Atoi(aver[1])
	if averr != nil {
		return E_MAKECLUSTER_MAKECLUSTER, errors.New("Aerospike Version is not an int.int.*")
	}
	if c.MakeCluster.FeaturesFilePath == "" && (aver_major > 4 || (aver_major == 4 && aver_minor > 5)) {
		c.log.Warn("WARNING: you are attempting to install version 4.6+ and did not provide feature.conf file. This will not work. You can either provide a feature file by using the '-f' switch, or inside your ~/aero-lab-common.conf as:\n\n[MakeCluster]\nFeaturesFilePath=/path/to/features.conf\n\nPress ENTER if you still wish to proceed")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	c.log.Info(INFO_STARTMAKE)
	// deploy template onto aerospike cluster, with changes as requested
	var ep []string
	if c.MakeCluster.ExposePortsToHost != "" {
		ep = strings.Split(c.MakeCluster.ExposePortsToHost, ",")
	}
	var privileged bool
	if c.MakeCluster.Privileged == 1 {
		privileged = true
	} else {
		privileged = false
	}
	if c.MakeCluster.RamLimit != "" || c.MakeCluster.CpuLimit != "" || c.MakeCluster.SwapLimit != "" || privileged {
		err = b.DeployClusterWithLimits(version{c.MakeCluster.DistroName, c.MakeCluster.DistroVersion, c.MakeCluster.AerospikeVersion}, c.MakeCluster.ClusterName, c.MakeCluster.NodeCount, ep, c.MakeCluster.CpuLimit, c.MakeCluster.RamLimit, c.MakeCluster.SwapLimit, privileged)
	} else {
		err = b.DeployCluster(version{c.MakeCluster.DistroName, c.MakeCluster.DistroVersion, c.MakeCluster.AerospikeVersion}, c.MakeCluster.ClusterName, c.MakeCluster.NodeCount, ep)
	}
	if err != nil {
		ret = E_MAKECLUSTER_MAKECLUSTER
		return ret, err
	}

	files := []fileList{}

	err = b.ClusterStart(c.MakeCluster.ClusterName, nil)
	if err != nil {
		ret = E_MAKECLUSTER_FIXCONF
		return ret, err
	}
	// get cluster IPs and node list
	clusterIps, err := b.GetClusterNodeIps(c.MakeCluster.ClusterName)
	if err != nil {
		ret = E_MAKECLUSTER_NODEIPS
		return ret, err
	}
	nodeList, err := b.NodeListInCluster(c.MakeCluster.ClusterName)
	if err != nil {
		ret = E_MAKECLUSTER_NODELIST
		return ret, err
	}

	newconf := ""
	// fix config if needed, read custom config file path if needed
	if c.MakeCluster.CustomConfigFilePath != "" {
		conf, err := ioutil.ReadFile(c.MakeCluster.CustomConfigFilePath)
		if err != nil {
			ret = E_MAKECLUSTER_READCONF
			return ret, err
		}
		newconf, err = fixAerospikeConfig(string(conf), c.MakeCluster.MulticastAddress, c.MakeCluster.HeartbeatMode, clusterIps, nodeList)
		if err != nil {
			ret = E_MAKECLUSTER_FIXCONF
			return ret, err
		}
	} else {
		if c.MakeCluster.HeartbeatMode == "mesh" || c.MakeCluster.HeartbeatMode == "mcast" || c.MakeCluster.OverrideASClusterName == 1 {
			var r [][]string
			r = append(r, []string{"cat", "/etc/aerospike/aerospike.conf"})
			var nr [][]byte
			nr, err = b.RunCommand(c.MakeCluster.ClusterName, r, []int{nodeList[0]})
			if err != nil {
				ret = E_MAKECLUSTER_FIXCONF
				return ret, err
			}
			newconf = string(nr[0])
			if c.MakeCluster.HeartbeatMode == "mesh" || c.MakeCluster.HeartbeatMode == "mcast" {
				// nr has contents of aerospike.conf
				newconf, err = fixAerospikeConfig(string(nr[0]), c.MakeCluster.MulticastAddress, c.MakeCluster.HeartbeatMode, clusterIps, nodeList)
				if err != nil {
					ret = E_MAKECLUSTER_FIXCONF
					return ret, err
				}
			}
		}
	}

	// add cluster name
	newconf2 := newconf
	if c.MakeCluster.OverrideASClusterName == 1 {
		newconf2, err = fixClusteNameConfig(string(newconf), c.MakeCluster.ClusterName)
		if err != nil {
			ret = E_MAKECLUSTER_FIXCONF_CLUSTER_NAME
			return ret, err
		}
	}

	if c.MakeCluster.HeartbeatMode == "mesh" || c.MakeCluster.HeartbeatMode == "mcast" || c.MakeCluster.OverrideASClusterName == 1 || c.MakeCluster.CustomConfigFilePath != "" {
		files = append(files, fileList{"/etc/aerospike/aerospike.conf", []byte(newconf2)})
	}

	// load features file path if needed
	if c.MakeCluster.FeaturesFilePath != "" {
		conf, err := ioutil.ReadFile(c.MakeCluster.FeaturesFilePath)
		if err != nil {
			ret = E_MAKECLUSTER_READFEATURES
			return ret, err
		}
		files = append(files, fileList{"/etc/aerospike/features.conf", conf})
	}

	// actually save files to nodes in cluster if needed
	if len(files) > 0 {
		err := b.CopyFilesToCluster(c.MakeCluster.ClusterName, files, nodeList)
		if err != nil {
			ret = E_MAKECLUSTER_COPYFILES
			return ret, err
		}
	}

	// if docker fix logging location
	var out [][]byte
	out, err = b.RunCommand(c.MakeCluster.ClusterName, [][]string{[]string{"cat", "/etc/aerospike/aerospike.conf"}}, nodeList)
	if err != nil {
		ret = E_MAKECLUSTER_FIXCONF
		return ret, err
	}
	if b.GetBackendName() == "docker" {
		var in [][]byte
		for _, out1 := range out {
			in1 := strings.Replace(string(out1), "console {", "file /var/log/aerospike.log {", 1)
			in = append(in, []byte(in1))
		}
		for i, node := range nodeList {
			err = b.CopyFilesToCluster(c.MakeCluster.ClusterName, []fileList{fileList{"/etc/aerospike/aerospike.conf", in[i]}}, []int{node})
			if err != nil {
				ret = E_MAKECLUSTER_FIXCONF
				return ret, err
			}
		}
	}
	// also create locations if not exist
	for i, node := range nodeList {
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
				nout, err := b.RunCommand(c.MakeCluster.ClusterName, [][]string{[]string{"mkdir", "-p", nDir}}, []int{node})
				if err != nil {
					return 1, fmt.Errorf("Could not create directory in container: %s\n%s\n%s", nDir, err, string(nout[0]))
				}
			}
		}
	}

	// aws-public-ip
	if c.MakeCluster.PublicIP == 1 {
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
		err = b.CopyFilesToCluster(c.MakeCluster.ClusterName, []fileList{systemdScript, accessAddressScript}, nodeList)
		if err != nil {
			ret = E_MAKECLUSTER_START
			return ret, fmt.Errorf("Could not make access-address script in aws: %s", err)
		}
		bouta, err := b.RunCommand(c.MakeCluster.ClusterName, [][]string{[]string{"chmod", "755", "/usr/local/bin/aerospike-access-address.sh"}, []string{"chmod", "755", "/etc/systemd/system/aerospike-access-address.service"}, []string{"systemctl", "daemon-reload"}, []string{"systemctl", "enable", "aerospike-access-address.service"}, []string{"service", "aerospike-access-address", "start"}}, nodeList)
		if err != nil {
			ret = E_MAKECLUSTER_START
			nstr := ""
			for _, bout := range bouta {
				nstr = fmt.Sprintf("%s\n%s", nstr, string(bout))
			}
			return ret, fmt.Errorf("Could not register access-address script in aws: %s\n%s", err, nstr)
		}
	}

	// start cluster
	if c.MakeCluster.AutoStartAerospike == "y" {
		var comm [][]string
		comm = append(comm, []string{"service", "aerospike", "start"})
		_, err = b.RunCommand(c.MakeCluster.ClusterName, comm, nodeList)
		if err != nil {
			ret = E_MAKECLUSTER_START
			return ret, err
		}
	}

	// done
	c.log.Info(INFO_DONE)
	return
}

// because we don't want to repeat this code in 2 places
func (c *config) getUrl() (url string, ret int64, err error) {
	c.log.Info(INFO_CHECK_VERSION)
	if url, c.MakeCluster.AerospikeVersion, err = aeroFindUrl(c.MakeCluster.AerospikeVersion, c.MakeCluster.Username, c.MakeCluster.Password); err != nil {
		ret = E_MAKECLUSTER_VALIDATION
		if strings.Contains(fmt.Sprintf("%s", err), "401") {
			err = fmt.Errorf("%s, Unauthorized access, check enterprise download username and password", err)
		}
		return url, ret, err
	}
	return url, ret, err
}

func checkUbuntuAerospikeVersion(aeroVer string) []string {
	aver := strings.Split(aeroVer, ".")
	if len(aver) < 3 {
		return []string{}
	}
	a, err := strconv.Atoi(aver[0])
	if err != nil {
		return []string{}
	}
	b, err := strconv.Atoi(aver[1])
	if err != nil {
		return []string{}
	}
	c, err := strconv.Atoi(aver[2])
	if err != nil {
		return []string{}
	}
	/*var d int
	if len(aver) > 3 {
		d, err = strconv.Atoi(aver[3])
		if err != nil {
			return []string{}
		}
	}*/
	if a == 3 && b < 6 {
		return []string{"12.04"}
	}
	if a == 3 && b >= 6 && b < 9 && !(b == 8 && c >= 4) {
		return []string{"14.04", "12.04"}
	}
	if (a == 3 && (b > 8 || (b == 8 && c >= 4))) || (a == 4 && b <= 1) {
		return []string{"16.04", "14.04", "12.04"}
	}
	if a == 4 && b >= 2 {
		return []string{"18.04", "16.04", "14.04"}
	}
	if a > 5 || (a == 5 && b >= 1) {
		return []string{"20.04", "18.04", "16.04", "14.04"}
	}
	if a > 4 {
		return []string{"18.04", "16.04", "14.04"}
	}
	return []string{}
}

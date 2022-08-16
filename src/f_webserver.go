package main

import "errors"

func (c *config) F_webserver() (ret int64, err error) {
	/*
			err, ret = chDir(c.UpgradeAerospike.ChDir)
			if err != nil {
				return err,ret
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
			if ((c.UpgradeAerospike.DistroName == "el" && (c.UpgradeAerospike.DistroVersion == "6" || c.UpgradeAerospike.DistroVersion == "7")) || (c.UpgradeAerospike.DistroName == "ubuntu" && (c.UpgradeAerospike.DistroVersion == "18.04" || c.UpgradeAerospike.DistroVersion == "16.04" || c.UpgradeAerospike.DistroVersion == "14.04" || c.UpgradeAerospike.DistroVersion == "12.04"  || c.UpgradeAerospike.DistroVersion == "best"))) == false {
				err = errors.New(ERR_UNSUPPORTED_OS)
				ret = E_MAKECLUSTER_VALIDATION
				return
			}

			// check files exist
			for _, p := range []string{c.UpgradeAerospike.CustomConfigFilePath, c.UpgradeAerospike.FeaturesFilePath, c.UpgradeAerospike.AccessPublicKeyFilePath} {
				if p != "" {
					if _, err := os.Stat(p); os.IsNotExist(err) {
						err = fmt.Errorf(ERR_FILE_NOT_FOUND, p)
						ret = E_MAKECLUSTER_VALIDATION
						return ret, err
					}
				}
			}

			// check heartbeat mode
			if c.UpgradeAerospike.HeartbeatMode == "mcast" || c.UpgradeAerospike.HeartbeatMode == "multicast" {
				if c.UpgradeAerospike.MulticastAddress == "" || c.UpgradeAerospike.MulticastPort == "" {
					err = errors.New(ERRT_MCAST_ADDR_PORT_EMPTY)
					ret = E_MAKECLUSTER_VALIDATION
					return ret, err
				}
			} else if c.UpgradeAerospike.HeartbeatMode != "mesh" && c.UpgradeAerospike.HeartbeatMode != "default" {
				err = fmt.Errorf(ERR_HEARTBEAT_MODE, c.UpgradeAerospike.HeartbeatMode)
				ret = E_MAKECLUSTER_VALIDATION
				return ret, err
			}

			// check autostart
			if inArray([]string{"y", "n", "YES", "NO", "yes", "no", "Y", "N"}, c.UpgradeAerospike.AutoStartAerospike) == -1 {
				err = errors.New(ERR_MUSTBE_YN)
				ret = E_MAKECLUSTER_VALIDATION
				return ret, err
			}

			// get backend
			b, err := getBackend(c.UpgradeAerospike.DeployOn, c.UpgradeAerospike.RemoteHost, c.UpgradeAerospike.AccessPublicKeyFilePath)
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
			if inArray(clusterList, c.UpgradeAerospike.ClusterName) == -1 {
				err = fmt.Errorf("Error, cluster does not exist: %s", c.UpgradeAerospike.ClusterName)
				ret = E_BACKEND_ERROR
				return ret, err
			}

			// check if template exists before we check if file exists. if template does, no need for file
			c.log.Info(INFO_CHECK_TEMPLATE)
			templates, err := b.ListTemplates()
			if err != nil {
				return err, E_BACKEND_ERROR
			}

			// check latest version early if needed to find out if template does not exist
			var url string
			if c.UpgradeAerospike.AerospikeVersion[:6] == "latest" || c.UpgradeAerospike.AerospikeVersion[:7] == "latestc" {
				url, err, ret = c.getUrlGrow()
				if err != nil {
					return ret, err
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
					if inArray(osVers,c.UpgradeAerospike.DistroVersion) == -1 {
						return errors.New(fmt.Sprint("Ubuntu version not supported. This aerospike version supports only: ",osVers)), E_BACKEND_ERROR
					}
				}
			}

			var edition string
			if c.UpgradeAerospike.AerospikeVersion[len(c.UpgradeAerospike.AerospikeVersion)-1] == 'c' {
				edition = "aerospike-server-community"
			} else {
				edition = "aerospike-server-enterprise"
			}

			if inArray(templates, version{c.UpgradeAerospike.DistroName, c.UpgradeAerospike.DistroVersion, c.UpgradeAerospike.AerospikeVersion}) == -1 {
				// check aerospike version - only required if not downloaded, not checked already above
				if url == "" {
					if _, err := os.Stat(edition + "-" + c.UpgradeAerospike.AerospikeVersion + "-" + c.UpgradeAerospike.DistroName + c.UpgradeAerospike.DistroVersion + ".tgz"); os.IsNotExist(err) {
						url, err, ret = c.getUrlGrow()
						if err != nil {
							return ret, err
						}
					}
				}

				if c.UpgradeAerospike.AerospikeVersion[len(c.UpgradeAerospike.AerospikeVersion)-1] == 'c' {
					c.UpgradeAerospike.AerospikeVersion = c.UpgradeAerospike.AerospikeVersion[:len(c.UpgradeAerospike.AerospikeVersion)-1]
				}

				// download file if not exists
				if _, err := os.Stat(edition + "-" + c.UpgradeAerospike.AerospikeVersion + "-" + c.UpgradeAerospike.DistroName + c.UpgradeAerospike.DistroVersion + ".tgz"); os.IsNotExist(err) {
					c.log.Info(INFO_DOWNLOAD)
					url = url + edition + "-" + c.UpgradeAerospike.AerospikeVersion + "-" + c.UpgradeAerospike.DistroName + c.UpgradeAerospike.DistroVersion + ".tgz"
					err = downloadFile(url, edition+"-"+c.UpgradeAerospike.AerospikeVersion+"-"+c.UpgradeAerospike.DistroName+c.UpgradeAerospike.DistroVersion+".tgz", c.UpgradeAerospike.Username, c.UpgradeAerospike.Password)
					if err != nil {
						ret = E_MAKECLUSTER_VALIDATION
						return ret, err
					}
				}

				// make template here
				c.log.Info(INFO_MAKETEMPLATE)
				packagefile, err := os.ReadFile(edition + "-" + c.UpgradeAerospike.AerospikeVersion + "-" + c.UpgradeAerospike.DistroName + c.UpgradeAerospike.DistroVersion + ".tgz")
				if err != nil {
					ret = E_MAKECLUSTER_READFILE
					return ret, err
				}
				nFiles := []fileList{}
				nFiles = append(nFiles, fileList{"/root/installer.tgz", packagefile})
				err = b.DeployTemplate(version{c.UpgradeAerospike.DistroName, c.UpgradeAerospike.DistroVersion, c.UpgradeAerospike.AerospikeVersion}, aerospikeInstallScript[c.UpgradeAerospike.DistroName], nFiles)
				if err != nil {
					ret = E_MAKECLUSTER_MAKETEMPLATE
					return ret, err
				}
			}

			c.log.Info(INFO_STARTMAKE)
			// deploy template onto aerospike cluster, with changes as requested
			nlic, err := b.NodeListInCluster(c.UpgradeAerospike.ClusterName)
			if err != nil {
				ret = E_MAKECLUSTER_MAKECLUSTER
				return ret, err
			}
			if len(nlic) + c.UpgradeAerospike.NodeCount > 128 {
				err = errors.New(ERR_MAX_NODE_COUNT)
				ret = E_MAKECLUSTER_VALIDATION
				return
			}
			if c.UpgradeAerospike.CpuLimit == "" && c.UpgradeAerospike.RamLimit == "" && c.UpgradeAerospike.SwapLimit == "" {
				err = b.DeployCluster(version{c.UpgradeAerospike.DistroName, c.UpgradeAerospike.DistroVersion, c.UpgradeAerospike.AerospikeVersion}, c.UpgradeAerospike.ClusterName, c.UpgradeAerospike.NodeCount, []string{})
			} else {
				err = b.DeployClusterWithLimits(version{c.UpgradeAerospike.DistroName, c.UpgradeAerospike.DistroVersion, c.UpgradeAerospike.AerospikeVersion}, c.UpgradeAerospike.ClusterName, c.UpgradeAerospike.NodeCount, []string{},c.UpgradeAerospike.CpuLimit,c.UpgradeAerospike.RamLimit,c.UpgradeAerospike.SwapLimit)
			}
			if err != nil {
				ret = E_MAKECLUSTER_MAKECLUSTER
				return ret, err
			}

			files := []fileList{}

			err = b.ClusterStart(c.UpgradeAerospike.ClusterName, nil)
			if err != nil {
				ret = E_MAKECLUSTER_FIXCONF
				return ret, err
			}
			// get cluster IPs and node list
			clusterIps, err := b.GetClusterNodeIps(c.UpgradeAerospike.ClusterName)
			if err != nil {
				ret = E_MAKECLUSTER_NODEIPS
				return ret, err
			}
			nodeList, err := b.NodeListInCluster(c.UpgradeAerospike.ClusterName)
			if err != nil {
				ret = E_MAKECLUSTER_NODELIST
				return ret, err
			}

			// fix config if needed, read custom config file path if needed

			if c.UpgradeAerospike.CustomConfigFilePath != "" {
				conf, err := os.ReadFile(c.UpgradeAerospike.CustomConfigFilePath)
				if err != nil {
					ret = E_MAKECLUSTER_READCONF
					return ret, err
				}
				newconf, err := fixAerospikeConfig(string(conf), c.UpgradeAerospike.MulticastAddress, c.UpgradeAerospike.HeartbeatMode, clusterIps, nodeList)
				if err != nil {
					ret = E_MAKECLUSTER_FIXCONF
					return ret, err
				}

				files = append(files, fileList{"/etc/aerospike/aerospike.conf", []byte(newconf)})
			} else {
				if c.UpgradeAerospike.HeartbeatMode == "mesh" || c.UpgradeAerospike.HeartbeatMode == "mcast" {
					var r [][]string
					r = append(r, []string{"cat", "/etc/aerospike/aerospike.conf"})
					var nr [][]byte
					nr, err = b.RunCommand(c.UpgradeAerospike.ClusterName, r, []int{nodeList[0]})
					if err != nil {
						ret = E_MAKECLUSTER_FIXCONF
						return ret, err
					}
					// nr has contents of aerospike.conf
					newconf, err := fixAerospikeConfig(string(nr[0]), c.UpgradeAerospike.MulticastAddress, c.UpgradeAerospike.HeartbeatMode, clusterIps, nodeList)
					if err != nil {
						ret = E_MAKECLUSTER_FIXCONF
						return ret, err
					}
					files = append(files, fileList{"/etc/aerospike/aerospike.conf", []byte(newconf)})
				}
			}

			// load features file path if needed
			if c.UpgradeAerospike.FeaturesFilePath != "" {
				conf, err := os.ReadFile(c.UpgradeAerospike.FeaturesFilePath)
				if err != nil {
					ret = E_MAKECLUSTER_READFEATURES
					return ret, err
				}
				files = append(files, fileList{"/etc/aerospike/features.conf", conf})
			}

			nodeListNew := []int{}
			for _,i := range nodeList {
				if inArray(nlic,i) == -1 {
					nodeListNew = append(nodeListNew,i)
				}
			}

			// actually save files to nodes in cluster if needed
			if len(files) > 0 {
				// copy to those in nodeList which are not in nlic
				err := b.CopyFilesToCluster(c.UpgradeAerospike.ClusterName, files, nodeListNew)
				if err != nil {
					ret = E_MAKECLUSTER_COPYFILES
					return ret, err
				}
			}

			// if docker fix logging location
			var out [][]byte
			out, err = b.RunCommand(c.UpgradeAerospike.ClusterName, [][]string{[]string{"cat","/etc/aerospike/aerospike.conf"}}, nodeListNew)
			if err != nil {
				ret = E_MAKECLUSTER_FIXCONF
				return ret, err
			}
			if b.GetBackendName() == "docker" {
				var in [][]byte
				for _,out1 := range out {
					in1 := strings.Replace(string(out1),"console {","file /var/log/aerospike.log {",1)
					in = append(in,[]byte(in1))
				}
				for i,node := range nodeListNew {
					err = b.CopyFilesToCluster(c.UpgradeAerospike.ClusterName,[]fileList{fileList{"/etc/aerospike/aerospike.conf",in[i]}},[]int{node})
					if err != nil {
						ret = E_MAKECLUSTER_FIXCONF
						return ret, err
					}
				}
			}

			// also create locations if not exist
			for i,node := range nodeListNew {
				log := string(out[i])
				scanner := bufio.NewScanner(strings.NewReader(log))
				for scanner.Scan() {
					t := scanner.Text()
					if strings.Contains(t,"/var") || strings.Contains(t,"/opt") || strings.Contains(t,"/etc") || strings.Contains(t,"/tmp") {
						tStart := strings.Index(t," /")+1
						var nLoc string
						if strings.Contains(t[tStart:]," ") {
							tEnd := strings.Index(t[tStart:], " ")
							nLoc = t[tStart:(tEnd+tStart)]
						} else {
							nLoc = t[tStart:]
						}
						var nDir string
						if strings.Contains(t,"file /") {
							nDir = path.Dir(nLoc)
						} else {
							nDir = nLoc
						}
						// create dir
						nout, err := b.RunCommand(c.UpgradeAerospike.ClusterName, [][]string{[]string{"mkdir","-p",nDir}}, []int{node})
						if err != nil {
							return fmt.Errorf("Could not create directory in container: %s\n%s\n%s",nDir,err,string(nout[0])),1
						}
					}
				}
			}

			// start cluster
			if c.UpgradeAerospike.AutoStartAerospike == "y" {
				var comm [][]string
				comm = append(comm, []string{"service", "aerospike", "start"})
				_, err = b.RunCommand(c.UpgradeAerospike.ClusterName, comm, nodeListNew)
				if err != nil {
					ret = E_MAKECLUSTER_START
					return ret, err
				}
			}

			// done
			c.log.Info(INFO_DONE)
			c.log.Info("If you deployed HB differently (i.e. mesh vs multicast, etc), you can run 'conf-fix-mesh' to force and fix mesh in the whole cluster")
			return
		}

		func (c *config) getUrlGrow() (url string, err error, ret int64) {
			c.log.Info(INFO_CHECK_VERSION)
			if url, c.UpgradeAerospike.AerospikeVersion, err = aeroFindUrl(c.UpgradeAerospike.AerospikeVersion, c.UpgradeAerospike.Username, c.UpgradeAerospike.Password); err != nil {
				ret = E_MAKECLUSTER_VALIDATION
				if strings.Contains(fmt.Sprintf("%s",err), "401") {
					err = fmt.Errorf("%s, Unauthorized access, check enterprise download username and password",err)
				}
				return url, err, ret
			}
			return url, err, ret
		}
	*/
	return 1, errors.New("NOT IMPLEMENTED YET")
}

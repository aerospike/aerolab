package main

import (
	"errors"
	"fmt"
	"strings"
)

func (c *config) F_xdrConnect() (err error, ret int64) {
	c.log.Info("XdrConnect running")
	namespaces := strings.Split(c.XdrConnect.Namespaces, ",")
	destinations := strings.Split(c.XdrConnect.DestinationClusterNames, ",")

	// get backend
	b, err := getBackend(c.XdrConnect.DeployOn, c.XdrConnect.RemoteHost, c.XdrConnect.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	clusterList, err := b.ClusterList()
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	if inArray(clusterList, c.XdrConnect.SourceClusterName) == -1 {
		err = errors.New(fmt.Sprintf("Cluster does not exist: %s", c.XdrConnect.SourceClusterName))
		ret = E_BACKEND_ERROR
		return err, ret
	}

	sourceNodeList, err := b.NodeListInCluster(c.XdrConnect.SourceClusterName)
	if err != nil {
		ret = E_BACKEND_ERROR
		return err, ret
	}

	destIpList := make(map[string][]string)
	for _, destination := range destinations {
		if inArray(clusterList, destination) == -1 {
			err = errors.New(fmt.Sprintf("Cluster does not exist: %s", destination))
			ret = E_BACKEND_ERROR
			return err, ret
		}
		destNodes, err := b.NodeListInCluster(destination)
		if err != nil {
			ret = E_BACKEND_ERROR
			return err, ret
		}
		destIps, err := b.GetClusterNodeIps(destination)
		if err != nil {
			ret = E_BACKEND_ERROR
			return err, ret
		}
		if len(destNodes) != len(destIps) {
			return errors.New(fmt.Sprintf("Cluster %s is not on or IP allocation failed. Run cluster-list.", destination)), E_MAKECLUSTER_NODEIPS
		}
		destIpList[destination] = destIps
	}

	// we have c.XdrConnect.SourceClusterName, sourceNodeList, destinations, destIpList, namespaces

	//build empty basic xdr stanza
	xdr_stanza := `
xdr {
	enable-xdr true
	xdr-digestlog-path /opt/aerospike/xdr/digestlog 1G

}
`
	if c.XdrConnect.Xdr5 == 1 {
		xdr_stanza = `
xdr {

}
`
	}

	_, err = b.RunCommand(c.XdrConnect.SourceClusterName, [][]string{[]string{"mkdir", "-p", "/opt/aerospike/xdr"}}, sourceNodeList)
	if err != nil {
		return errors.New(fmt.Sprintf("Failed running mkdir /opt/aerospike/xdr: %s", err)), 999
	}
	//for each source node
	for _, snode := range sourceNodeList {

		//read file
		out, err := b.RunCommand(c.XdrConnect.SourceClusterName, [][]string{[]string{"cat", "/etc/aerospike/aerospike.conf"}}, []int{snode})
		if err != nil {
			return errors.New(fmt.Sprintf("Failed running cat /etc/aerospike/aerospike.conf: %s", err)), 999
		}
		conf := string(out[0])

		//add xdr stanza if not found
		if strings.Contains(conf, "xdr {\n") == false {
			conf = conf + xdr_stanza
		}

		//split conf to slice
		confs := strings.Split(conf, "\n")
		for i, _ := range confs {
			confs[i] = strings.Trim(confs[i], "\r")
		}

		//find start and end of xdr stanza, find configured DCs
		dcStanzaName := "datacenter"
		nodeAddressPort := "dc-node-address-port"
		if c.XdrConnect.Xdr5 == 1 {
			dcStanzaName = "dc"
			nodeAddressPort = "node-address-port"
		}
		xdr_start := -1
		xdr_end := -1
		lvl := 0
		var xdr_dcs []string
		for i := 0; i < len(confs); i++ {
			if strings.Contains(confs[i], "xdr {") {
				xdr_start = i
				lvl = 1
			} else if strings.Contains(confs[i], "{") && xdr_start != -1 {
				lvl = lvl + strings.Count(confs[i], "{")
			} else if strings.Contains(confs[i], "}") && xdr_start != -1 {
				lvl = lvl - strings.Count(confs[i], "}")
			}
			if strings.Contains(confs[i], dcStanzaName+" ") && xdr_start != -1 && strings.HasSuffix(confs[i], "{") {
				tmp := strings.Split(confs[i], " ")
				for j := 0; j < len(tmp); j++ {
					if strings.Contains(tmp[j], dcStanzaName) {
						xdr_dcs = append(xdr_dcs, tmp[j+1])
						break
					}
				}
			}
			if lvl == 0 && xdr_start != -1 {
				xdr_end = i
				break
			}
		}

		//filter to find which DCs we need to add only, build DC "add string"
		dc_to_add := ""
		dc2namespace := make(map[string][]string)
		for i := 0; i < len(destinations); i++ {
			found := destinations[i]
			for _, k := range xdr_dcs {
				if destinations[i] == k {
					found = ""
					break
				}
			}
			if found != "" {
				dc_to_add = dc_to_add + fmt.Sprintf("\n\t%s %s {\n", dcStanzaName, found)
				dst_cluster_ips := destIpList[found]
				for j := 0; j < len(dst_cluster_ips); j++ {
					dc_to_add = dc_to_add + fmt.Sprintf("\t\t%s %s 3000\n", nodeAddressPort, dst_cluster_ips[j])
					if c.XdrConnect.Xdr5 == 1 {
						if _, ok := dc2namespace[found]; !ok {
							dc2namespace[found] = []string{}
						}
						for _, nspace := range namespaces {
							if inArray(dc2namespace[found], nspace) == -1 {
								dc2namespace[found] = append(dc2namespace[found], nspace)
								dc_to_add = dc_to_add + fmt.Sprintf("\t\tnamespace %s {\n\t\t}\n", nspace)
							}
						}
					}
				}
				dc_to_add = dc_to_add + "\t}\n"
			}
		}

		//add DCs to XDR stanza
		//copy dc_to_add just between confs[xdr_end-1] and confs[xdr_end]
		confsx := confs[:xdr_end]
		confsy := confs[xdr_end:]
		if len(dc_to_add) > 0 {
			confsx = append(confsx, strings.Split(dc_to_add, "\n")...)
		}
		confsx = append(confsx, confsy...)

		if c.XdrConnect.Xdr5 == 0 {
			//now latest config is in confsx
			//update namespaces to enable XDR in them
			for i := 0; i < len(namespaces); i++ {
				nsname := ""
				nsloc := -1
				lvl := 0
				has_enable_xdr := false
				var has_dc_list []string
				for j := 0; j < len(confsx); j++ {
					if strings.HasPrefix(confsx[j], "namespace ") {
						nsloc = j
						atmp := strings.Split(confsx[j], " ")
						nsname = atmp[1]
						lvl = 1
					} else if strings.Contains(confsx[j], "{") && nsloc != -1 {
						lvl = lvl + strings.Count(confsx[j], "{")
					} else if strings.Contains(confsx[j], "}") && nsloc != -1 {
						lvl = lvl - strings.Count(confsx[j], "}")
					} else if (strings.Contains(confsx[j], "enable-xdr true") && strings.Contains(confsx[j], "-enable-xdr true") == false) && nsloc != -1 && namespaces[i] == nsname {
						has_enable_xdr = true
					} else if strings.Contains(confsx[j], "xdr-remote-datacenter ") && nsloc != -1 && namespaces[i] == nsname {
						tmp := strings.Split(confsx[j], " ")
						for k := 0; k < len(tmp); k++ {
							if strings.Contains(tmp[k], "xdr-remote-datacenter") {
								has_dc_list = append(has_dc_list, tmp[k+1])
								break
							}
						}
					}
					if lvl == 0 && nsloc != -1 && nsname == namespaces[i] {
						//if has_enable_xdr is false, add that after confsx[nsloc]
						if has_enable_xdr == false {
							confsx[nsloc] = confsx[nsloc] + "\nenable-xdr true"
						}
						//for each dc, if not found in has_dc_list, add like above, as remote-datacenter
						for k := 0; k < len(destinations); k++ {
							found := false
							for _, l := range has_dc_list {
								if destinations[k] == l {
									found = true
								}
							}
							if found == false {
								confsx[nsloc] = confsx[nsloc] + fmt.Sprintf("\nxdr-remote-datacenter %s", destinations[k])
							}
						}
						has_dc_list = has_dc_list[:0]
						nsloc = -1
						nsname = ""
						has_enable_xdr = false
					}
				}
			}
		}

		err = b.CopyFilesToCluster(c.XdrConnect.SourceClusterName, []fileList{fileList{"/etc/aerospike/aerospike.conf", []byte(strings.Join(confsx, "\n"))}}, []int{snode})
		if err != nil {
			return errors.New(fmt.Sprintf("ERROR trying to modify config file while configuring xdr: %s\n", err)), 999
		}
	}

	c.log.Info("Done, now restart the source cluster for changes to take effect.")
	return
}

package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func (c *config) F_netLoss() (ret int64, err error) {
	// get backend
	c.log.Info("Starting net loss/delay")
	var b backend
	b, err = getBackend(c.NetLoss.DeployOn, c.NetLoss.RemoteHost, c.NetLoss.AccessPublicKeyFilePath)
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

	fullIpMap := make(map[string]string)
	if c.NetLoss.Action == "show" {
		for _, cluster := range clusterList {
			ips, err := b.GetNodeIpMap(cluster)
			if err != nil {
				ret = E_BACKEND_ERROR
				return ret, err
			}
			for n, v := range ips {
				fullIpMap[v] = fmt.Sprintf("CLUSTER=%s NODE=%d", cluster, n)
			}
		}
	}

	if inArray(clusterList, c.NetLoss.SourceClusterName) == -1 {
		err = fmt.Errorf("Error, cluster does not exist: %s", c.GenTlsCerts.ClusterName)
		ret = E_BACKEND_ERROR
		return ret, err
	}

	if c.NetLoss.Action != "show" && c.NetLoss.Action != "delall" {
		if inArray(clusterList, c.NetLoss.DestinationClusterName) == -1 {
			err = fmt.Errorf("Error, cluster does not exist: %s", c.GenTlsCerts.ClusterName)
			ret = E_BACKEND_ERROR
			return ret, err
		}
	}

	sourceNodeList := []int{}
	var sourceNodeIpMap map[int]string
	var sourceNodeIpMapInternal map[int]string
	if c.NetLoss.SourceNodeList == "" {
		sourceNodeList, err = b.NodeListInCluster(c.NetLoss.SourceClusterName)
		if err != nil {
			ret = E_BACKEND_ERROR
			return ret, err
		}
	} else {
		snl, err := b.NodeListInCluster(c.NetLoss.SourceClusterName)
		if err != nil {
			ret = E_BACKEND_ERROR
			return ret, err
		}
		sn := strings.Split(c.NetLoss.SourceNodeList, ",")
		for _, i := range sn {
			snInt, err := strconv.Atoi(i)
			if err != nil {
				ret = E_BACKEND_ERROR
				return ret, err
			}
			if inArray(snl, snInt) == -1 {
				if err != nil {
					ret = E_BACKEND_ERROR
					return ret, err
				}
			}
			sourceNodeList = append(sourceNodeList, snInt)
		}
	}

	sourceNodeIpMap, err = b.GetNodeIpMap(c.NetLoss.SourceClusterName)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	sourceNodeIpMapInternal, err = b.GetNodeIpMapInternal(c.NetLoss.SourceClusterName)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	destNodeList := []int{}
	var destNodeIpMap map[int]string
	var destNodeIpMapInternal map[int]string
	if c.NetLoss.DestinationNodeList == "" {
		destNodeList, err = b.NodeListInCluster(c.NetLoss.DestinationClusterName)
		if err != nil {
			ret = E_BACKEND_ERROR
			return ret, err
		}
	} else {
		dnl, err := b.NodeListInCluster(c.NetLoss.DestinationClusterName)
		if err != nil {
			ret = E_BACKEND_ERROR
			return ret, err
		}
		dn := strings.Split(c.NetLoss.DestinationNodeList, ",")
		for _, i := range dn {
			dnInt, err := strconv.Atoi(i)
			if err != nil {
				ret = E_BACKEND_ERROR
				return ret, err
			}
			if inArray(dnl, dnInt) == -1 {
				if err != nil {
					ret = E_BACKEND_ERROR
					return ret, err
				}
			}
			destNodeList = append(destNodeList, dnInt)
		}
	}

	destNodeIpMap, err = b.GetNodeIpMap(c.NetLoss.DestinationClusterName)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	destNodeIpMapInternal, err = b.GetNodeIpMapInternal(c.NetLoss.DestinationClusterName)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}

	rest := ""
	if c.NetLoss.Action == "set" {
		rest = "tcset eth0 --change"
	} else if c.NetLoss.Action == "del" {
		rest = "tcdel eth0"
	} else if c.NetLoss.Action == "delall" {
		rest = "tcdel eth0 --all"
	} else {
		rest = "tcshow eth0"
	}
	if c.NetLoss.Rate != "" {
		rest = rest + " --rate " + c.NetLoss.Rate
	}
	if c.NetLoss.Delay != "" {
		rest = rest + " --delay " + c.NetLoss.Delay
	}
	if c.NetLoss.Loss != "" {
		rest = rest + " --loss " + c.NetLoss.Loss
	}

	sysRunOnClusterName := c.NetLoss.SourceClusterName
	sysLogTheOther := c.NetLoss.DestinationClusterName
	sysRunOnNodeList := sourceNodeList
	sysRunOnDestNodeList := destNodeList
	sysRunOnDestIpMap := destNodeIpMap
	sysRunOnDestIpMapInternal := destNodeIpMapInternal
	if c.NetLoss.RunOnDestination != 0 {
		sysRunOnClusterName = c.NetLoss.DestinationClusterName
		sysLogTheOther = c.NetLoss.SourceClusterName
		sysRunOnNodeList = destNodeList
		sysRunOnDestNodeList = sourceNodeList
		sysRunOnDestIpMap = sourceNodeIpMap
		sysRunOnDestIpMapInternal = sourceNodeIpMapInternal
	}
	c.log.Info("Run on '%s' nodes '%v', implement loss/delay against '%s' nodes '%v' with IPs '%v' and optional IPs '%v'", sysRunOnClusterName, sysRunOnNodeList, sysLogTheOther, sysRunOnDestNodeList, sysRunOnDestIpMap, sysRunOnDestIpMapInternal)
	for _, sourceNode := range sysRunOnNodeList {
		container := fmt.Sprintf("cluster %s node %d", sysRunOnClusterName, sourceNode)
		if c.NetLoss.Action != "show" && c.NetLoss.Action != "delall" {
			for _, destNode := range sysRunOnDestNodeList {
				destNodeIp := sysRunOnDestIpMap[destNode]
				command := []string{"/bin/bash", "-c", fmt.Sprintf("source /tcconfig/bin/activate; %s --network %s", rest, destNodeIp)}
				out, err := b.RunCommand(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
				if err != nil {
					c.log.Error("%s %s %s", container, err, string(out[0]))
				}
				if sysRunOnDestIpMapInternal != nil {
					destNodeIpInternal := sysRunOnDestIpMapInternal[destNode]
					command := []string{"/bin/bash", "-c", fmt.Sprintf("source /tcconfig/bin/activate; %s --network %s", rest, destNodeIpInternal)}
					out, err = b.RunCommand(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
					if err != nil {
						c.log.Error("%s %s %s", container, err, string(out[0]))
					}
				}
			}
		} else {
			command := []string{"/bin/bash", "-c", fmt.Sprintf("source /tcconfig/bin/activate; %s", rest)}
			out, err := b.RunCommand(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
			if err != nil {
				c.log.Error("%s %s %s", container, err, string(out[0]))
			} else if c.NetLoss.Action == "show" {
				fmt.Printf("========== %s ==========\n", container)
				prt := string(out[0])
				if c.NetLoss.ShowNames == 1 {
					for _, ip := range findIP(prt) {
						if fullIpMap[ip] != "" {
							prt = strings.Replace(prt, fmt.Sprintf("%s/32", ip), fullIpMap[ip], -1)
							prt = strings.Replace(prt, ip, fullIpMap[ip], -1)
						}
					}
				}
				fmt.Println(prt)
			}
		}
	}
	c.log.Info("Done")
	return
}

func findIP(input string) []string {
	numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock

	regEx := regexp.MustCompile(regexPattern)
	return regEx.FindAllString(input, -1)
}

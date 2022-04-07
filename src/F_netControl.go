package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func (c *config) F_netBlockUnblock(call string) (ret int64, err error) {
	// get backend
	c.log.Info("Starting net blocker")
	var b backend
	var sc string
	var dc string
	var sn []string
	var dn []string
	var t string
	var ports []string
	var loc string
	var statisticMode string
	var statisticRandom string
	var statisticEvery string
	if call == "block" {
		b, err = getBackend(c.NetBlock.DeployOn, c.NetBlock.RemoteHost, c.NetBlock.AccessPublicKeyFilePath)
		sc = c.NetBlock.SourceClusterName
		dc = c.NetBlock.DestinationClusterName
		sn = strings.Split(c.NetBlock.SourceNodeList, ",")
		dn = strings.Split(c.NetBlock.DestinationNodeList, ",")
		t = c.NetBlock.Type
		ports = strings.Split(c.NetBlock.Ports, ",")
		loc = c.NetBlock.BlockOn
		statisticMode = c.NetBlock.StatisticMode
		statisticRandom = c.NetBlock.StatisticProbability
		statisticEvery = c.NetBlock.StatisticEvery
	} else {
		b, err = getBackend(c.NetUnblock.DeployOn, c.NetUnblock.RemoteHost, c.NetUnblock.AccessPublicKeyFilePath)
		sc = c.NetUnblock.SourceClusterName
		dc = c.NetUnblock.DestinationClusterName
		sn = strings.Split(c.NetUnblock.SourceNodeList, ",")
		dn = strings.Split(c.NetUnblock.DestinationNodeList, ",")
		t = c.NetUnblock.Type
		ports = strings.Split(c.NetUnblock.Ports, ",")
		loc = c.NetUnblock.BlockOn
		statisticMode = c.NetUnblock.StatisticMode
		statisticRandom = c.NetUnblock.StatisticProbability
		statisticEvery = c.NetUnblock.StatisticEvery
	}
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

	if inArray(clusterList, sc) == -1 {
		err = fmt.Errorf("Error, source cluster does not exist: %s", c.GenTlsCerts.ClusterName)
		ret = E_BACKEND_ERROR
		return ret, err
	}
	if inArray(clusterList, dc) == -1 {
		err = fmt.Errorf("Error, destination cluster does not exist: %s", c.GenTlsCerts.ClusterName)
		ret = E_BACKEND_ERROR
		return ret, err
	}
	wherec := sc
	wheren := sn
	towherec := dc
	towheren := dn
	blockon := "--destination"
	r := "-D"
	if call == "block" {
		r = "-I"
	}
	if loc == "input" {
		wherec = dc
		wheren = dn
		towherec = sc
		towheren = sn
		blockon = "--source"
	}

	if len(wheren) == 1 && wheren[0] == "" {
		asdf, _ := b.NodeListInCluster(wherec)
		wheren = []string{}
		for _, asd := range asdf {
			wheren = append(wheren, strconv.Itoa(asd))
		}
	}
	if len(towheren) == 1 && towheren[0] == "" {
		asdf, _ := b.NodeListInCluster(towherec)
		towheren = []string{}
		for _, asd := range asdf {
			towheren = append(towheren, strconv.Itoa(asd))
		}
	}

	nodeIps, err := b.GetNodeIpMap(towherec)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	nodeIpsInternal, err := b.GetNodeIpMapInternal(towherec)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	for _, nodes := range wheren {
		node, err := strconv.Atoi(nodes)
		if err != nil {
			return 999, err
		}
		container := fmt.Sprintf("cluster %s node %d", wherec, node)
		for _, port := range ports {
			for _, bbb := range towheren {
				bb, err := strconv.Atoi(bbb)
				if err != nil {
					return 999, err
				}
				ip := nodeIps[bb]
				nComm := []string{"/sbin/iptables", r, strings.ToUpper(loc), "-p", "tcp", "--dport", port, blockon, ip}
				if statisticMode != "" {
					nComm = append(nComm, "-m", "statistic", "--mode", statisticMode)
					if statisticMode == "random" {
						nComm = append(nComm, "--probability", statisticRandom)
					} else {
						nComm = append(nComm, "--every", statisticEvery)
					}
				}
				nComm = append(nComm, "-j", strings.ToUpper(t))
				out, err := b.RunCommand(wherec, [][]string{nComm}, []int{node})
				if err != nil {
					c.log.Error("WARNING: ERROR adding iptables rule on %s to block %s with IP %s\n%s\n", container, fmt.Sprintf("aero-%s_%s", towherec, b), ip, string(out[0]))
					c.log.Error("RAN: %s %s %s %s %s %s %s %s %s %s %s\n", "iptables", r, strings.ToUpper(loc), "-p", "tcp", "--dport", port, blockon, ip, "-j", strings.ToUpper(t))
					return 999, err
				}
				if nodeIpsInternal != nil {
					ip = nodeIpsInternal[bb]
					nComm := []string{"/sbin/iptables", r, strings.ToUpper(loc), "-p", "tcp", "--dport", port, blockon, ip}
					if statisticMode != "" {
						nComm = append(nComm, "-m", "statistic", "--mode", statisticMode)
						if statisticMode == "random" {
							nComm = append(nComm, "--probability", statisticRandom)
						} else {
							nComm = append(nComm, "--every", statisticEvery)
						}
					}
					nComm = append(nComm, "-j", strings.ToUpper(t))
					out, err = b.RunCommand(wherec, [][]string{nComm}, []int{node})
					if err != nil {
						c.log.Error("WARNING: ERROR adding iptables rule on %s to block %s with IP %s\n%s\n", container, fmt.Sprintf("aero-%s_%s", towherec, b), ip, string(out[0]))
						c.log.Error("RAN: %s %s %s %s %s %s %s %s %s %s %s\n", "iptables", r, strings.ToUpper(loc), "-p", "tcp", "--dport", port, blockon, ip, "-j", strings.ToUpper(t))
						return 999, err
					}
				}
			}
		}
	}
	c.log.Info("Done")
	return
}

func (c *config) F_netBlock() (ret int64, err error) {
	return c.F_netBlockUnblock("block")
}

func (c *config) F_netUnblock() (ret int64, err error) {
	return c.F_netBlockUnblock("unblock")
}

func (c *config) F_netList() (ret int64, err error) {
	//go through all DCs and all nodes and list iptables, display in a nice (this -> that) format
	check_out := ""
	b, err := getBackend(c.NetList.DeployOn, c.NetList.RemoteHost, c.NetList.AccessPublicKeyFilePath)
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	clusters, err := b.ClusterList()
	if err != nil {
		ret = E_BACKEND_ERROR
		return ret, err
	}
	nodes := make(map[string]map[int]string)
	for _, cluster := range clusters {
		tmpnodes, err := b.GetNodeIpMapInternal(cluster)
		if err != nil {
			ret = E_BACKEND_ERROR
			return ret, err
		}
		if tmpnodes == nil {
			tmpnodes, err = b.GetNodeIpMap(cluster)
			if err != nil {
				ret = E_BACKEND_ERROR
				return ret, err
			}
		}
		nodes[cluster] = tmpnodes
	}
	// nodes[cluster string][node int] = ip
	for _, cluster := range clusters {
		for node := range nodes[cluster] {
			outs, err := b.RunCommand(cluster, [][]string{[]string{"/sbin/iptables", "-L", "INPUT", "-vn"}}, []int{node})
			out := outs[0]
			if err != nil {
				c.log.Warn("WARNING: Could not check: %s, got:\n---\n%s\n---\n", cluster, string(out))
			} else {
				for _, line := range strings.Split(string(out), "\n") {
					if strings.Contains(line, "REJECT") || strings.Contains(line, "DROP") {
						t := ""
						if strings.Contains(line, "REJECT") {
							t = "REJECT"
						} else {
							t = "DROP"
						}
						srcIp := cut(line, 8, " ")
						srcC, srcN := find_node_by_ip(nodes, srcIp)
						src := fmt.Sprintf("%s_%d", srcC, srcN)
						dport := strings.Split(cut(line, 11, " "), ":")[1]
						dport = strings.Trim(dport, "\n\r")
						suffix := cutSuffix(line, 12, " ")
						check_out = check_out + fmt.Sprintf("%s => %s:%s %s (rule:INPUT  on:%s)%s\n", src, fmt.Sprintf("%s_%d", cluster, node), dport, t, fmt.Sprintf("%s_%d", cluster, node), suffix)
					}
				}
			}
			outs, err = b.RunCommand(cluster, [][]string{[]string{"/sbin/iptables", "-L", "OUTPUT", "-vn"}}, []int{node})
			out = outs[0]
			if err != nil {
				c.log.Warn("WARNING: Could not check: %s, got:\n---\n%s\n---\n", cluster, string(out))
			} else {
				for _, line := range strings.Split(string(out), "\n") {
					if strings.Contains(line, "REJECT") || strings.Contains(line, "DROP") {
						t := ""
						if strings.Contains(line, "REJECT") {
							t = "REJECT"
						} else {
							t = "DROP"
						}
						dstIp := cut(line, 9, " ")
						dstC, dstN := find_node_by_ip(nodes, dstIp)
						dst := fmt.Sprintf("%s_%d", dstC, dstN)
						dport := strings.Split(cut(line, 11, " "), ":")[1]
						suffix := cutSuffix(line, 12, " ")
						check_out = check_out + fmt.Sprintf("%s => %s:%s %s (rule:OUTPUT on:%s)%s\n", fmt.Sprintf("%s_%d", cluster, node), dst, dport, t, fmt.Sprintf("%s_%d", cluster, node), suffix)
					}
				}
			}
		}
	}
	ss := strings.Split(check_out, "\n")
	sort.Strings(ss)
	check_out = strings.Join(ss, "\n") + "\n"
	check_out = strings.TrimPrefix(check_out, "\n")
	fmt.Println("RULES:")
	fmt.Println(check_out)
	return
}

func find_node_by_ip(nodes map[string]map[int]string, ip string) (string, int) {
	for cluster := range nodes {
		for node, nodeIp := range nodes[cluster] {
			if nodeIp == ip {
				return cluster, node
			}
		}
	}
	return "none", 0
}

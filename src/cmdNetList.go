package main

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/bestmethod/inslice"
)

type netListCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *netListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	//go through all DCs and all nodes and list iptables, display in a nice (this -> that) format
	check_out := ""
	clusters, err := b.ClusterList()
	if err != nil {
		return err
	}
	nodes := make(map[string]map[int][]string)
	for _, cluster := range clusters {
		tmpnodes, err := b.GetNodeIpMap(cluster, true)
		if err != nil {
			return err
		}
		for i, j := range tmpnodes {
			if _, ok := nodes[cluster]; !ok {
				nodes[cluster] = make(map[int][]string)
			}
			if _, ok := nodes[cluster][i]; !ok {
				nodes[cluster][i] = []string{j}
			} else {
				nodes[cluster][i] = append(nodes[cluster][i], j)
			}
		}
		tmpnodes, err = b.GetNodeIpMap(cluster, false)
		if err != nil {
			return err
		}
		for i, j := range tmpnodes {
			if _, ok := nodes[cluster]; !ok {
				nodes[cluster] = make(map[int][]string)
			}
			if _, ok := nodes[cluster][i]; !ok {
				nodes[cluster][i] = []string{j}
			} else {
				nodes[cluster][i] = append(nodes[cluster][i], j)
			}
		}
	}
	// nodes[cluster string][node int] = ip
	for _, cluster := range clusters {
		for node := range nodes[cluster] {
			outs, err := b.RunCommands(cluster, [][]string{[]string{"/sbin/iptables", "-L", "INPUT", "-vn"}}, []int{node})
			out := outs[0]
			if err != nil {
				log.Printf("WARNING: Could not check: %s, got:\n---\n%s\n---\n", cluster, string(out))
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
			outs, err = b.RunCommands(cluster, [][]string{[]string{"/sbin/iptables", "-L", "OUTPUT", "-vn"}}, []int{node})
			out = outs[0]
			if err != nil {
				log.Printf("WARNING: Could not check: %s, got:\n---\n%s\n---\n", cluster, string(out))
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
	return nil
}

func find_node_by_ip(nodes map[string]map[int][]string, ip string) (string, int) {
	for cluster := range nodes {
		for node, nodeIp := range nodes[cluster] {
			if inslice.HasString(nodeIp, ip) {
				return cluster, node
			}
		}
	}
	return "none", 0
}

func cut(line string, pos int, split string) string {
	p := 0
	for _, v := range strings.Split(line, split) {
		if v != "" {
			p = p + 1
		}
		if p == pos {
			return v
		}
	}
	return ""
}

func cutSuffix(line string, pos int, split string) string {
	p := 0
	ret := ""
	for _, v := range strings.Split(line, split) {
		if v != "" {
			p = p + 1
		}
		if p >= pos {
			ret = ret + " " + v
		}
	}
	return ret
}

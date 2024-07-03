package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/bestmethod/inslice"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	isatty "github.com/mattn/go-isatty"
	"golang.org/x/term"
)

type netListCmd struct {
	Json       bool    `short:"j" long:"json" description:"set to display output in json format"`
	PrettyJson bool    `short:"p" long:"pretty" description:"set to indent json and pretty-print"`
	SortBy     string  `short:"s" long:"sort" description:"sort-by comma-separated fields, must match column names exactly" default:"Source,Destination,Port"`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *netListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	// pretty tables, Jim's fault, now I want them everywhere
	tb := table.NewWriter()
	isTerminal := false
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		isTerminal = true
	}
	isColor := true
	if _, ok := os.LookupEnv("NO_COLOR"); ok || os.Getenv("CLICOLOR") == "0" || !isTerminal {
		isColor = false
	}
	colorHiWhite := colorPrint{c: text.Colors{text.FgHiWhite}, enable: true}
	//warnExp := colorPrint{c: text.Colors{text.BgHiYellow, text.FgBlack}, enable: true}
	//errExp := colorPrint{c: text.Colors{text.BgHiRed, text.FgWhite}, enable: true}
	if !isColor {
		tb.SetStyle(table.StyleDefault)
		colorHiWhite.enable = false
		//warnExp.enable = false
		//errExp.enable = false
		tstyle := tb.Style()
		tstyle.Options.DrawBorder = false
		tstyle.Options.SeparateColumns = false
	} else {
		tb.SetStyle(table.StyleColoredBlackOnCyanWhite)
	}
	if isTerminal {
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || width < 1 {
			fmt.Fprintf(os.Stderr, "Couldn't get terminal width (int:%v): %v", width, err)
		} else {
			if width < 40 {
				width = 40
			}
			tb.SetAllowedRowLength(width)
		}
	}
	tstyle := tb.Style()
	tstyle.Format.Header = text.FormatDefault
	tstyle.Format.Footer = text.FormatDefault
	tb.SetTitle(colorHiWhite.Sprint("RULES"))
	tb.AppendHeader(table.Row{"Source", "Destination", "Port", "Type", "Chain", "RuleOn", "Behaviour"})
	sortBy := strings.Split(c.SortBy, ",")
	sortByTable := []table.SortBy{}
	for _, sortitem := range sortBy {
		sortByTable = append(sortByTable, table.SortBy{Mode: table.Asc, Name: sortitem})
	}
	tb.SortBy(sortByTable)
	type rule struct {
		SourceCluster      string
		SourceNode         int
		SourceClient       bool
		DestinationCluster string
		DestinationNode    int
		DestinationClient  bool
		Port               string
		Type               string
		Chain              string
		RuleAppliedOn      string
		Behaviour          string
	}
	jout := []*rule{}

	//go through all DCs and all nodes and list iptables, display in a nice (this -> that) format
	clusters := make(map[bool][]string) // map[isClient][]names
	clusters[true] = []string{}
	clusters[false] = []string{}
	clustersList, err := b.ClusterList()
	if err != nil {
		return err
	}
	clusters[false] = clustersList
	b.WorkOnClients()
	clustersList, err = b.ClusterList()
	if err != nil {
		return err
	}
	b.WorkOnServers()
	clusters[true] = clustersList
	nodes := make(map[bool]map[string]map[int][]string) // map[isClient][cluster][node][]ips
	nodes[false] = make(map[string]map[int][]string)
	nodes[true] = make(map[string]map[int][]string)
	for isClient, clusterEnum := range clusters {
		for _, cluster := range clusterEnum {
			if isClient {
				b.WorkOnClients()
			}
			tmpnodes, err := b.GetNodeIpMap(cluster, true)
			b.WorkOnServers()
			if err != nil {
				return err
			}
			for i, j := range tmpnodes {
				if _, ok := nodes[isClient][cluster]; !ok {
					nodes[isClient][cluster] = make(map[int][]string)
				}
				if _, ok := nodes[isClient][cluster][i]; !ok {
					nodes[isClient][cluster][i] = []string{j}
				} else {
					nodes[isClient][cluster][i] = append(nodes[isClient][cluster][i], j)
				}
			}
			if isClient {
				b.WorkOnClients()
			}
			tmpnodes, err = b.GetNodeIpMap(cluster, false)
			b.WorkOnServers()
			if err != nil {
				return err
			}
			for i, j := range tmpnodes {
				if _, ok := nodes[isClient][cluster]; !ok {
					nodes[isClient][cluster] = make(map[int][]string)
				}
				if _, ok := nodes[isClient][cluster][i]; !ok {
					nodes[isClient][cluster][i] = []string{j}
				} else {
					nodes[isClient][cluster][i] = append(nodes[isClient][cluster][i], j)
				}
			}
		}
	}
	// nodes[cluster string][node int] = ip
	for isClient, clusterEnum := range clusters {
		for _, cluster := range clusterEnum {
			for node := range nodes[isClient][cluster] {
				if isClient {
					b.WorkOnClients()
				}
				outs, err := b.RunCommands(cluster, [][]string{{"/sbin/iptables", "-L", "INPUT", "-vn"}}, []int{node})
				b.WorkOnServers()
				out := outs[0]
				if err != nil {
					if !strings.Contains(err.Error(), "/sbin/iptables: no such file or directory") {
						log.Printf("WARNING: Could not check INPUT: %s, got: %s", cluster, string(out))
					}
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
							srcclient, srcC, srcN := find_node_by_ip(nodes, srcIp)
							srcclienttext := ""
							dstclienttext := ""
							if srcclient {
								srcclienttext = " (client)"
							}
							if isClient {
								dstclienttext = " (client)"
							}
							dport := strings.Split(cut(line, 11, " "), ":")[1]
							dport = strings.Trim(dport, "\n\r")
							suffix := cutSuffix(line, 12, " ")
							tb.AppendRow(table.Row{fmt.Sprintf("%s-%d%s", srcC, srcN, srcclienttext), fmt.Sprintf("%s-%d%s", cluster, node, dstclienttext), dport, t, "INPUT", "DestNode", suffix})
							jout = append(jout, &rule{
								SourceCluster:      srcC,
								SourceNode:         srcN,
								SourceClient:       srcclient,
								DestinationCluster: cluster,
								DestinationNode:    node,
								DestinationClient:  isClient,
								Port:               dport,
								Type:               t,
								Chain:              "INPUT",
								RuleAppliedOn:      "Destination",
								Behaviour:          suffix,
							})
						}
					}
				}
				if isClient {
					b.WorkOnClients()
				}
				outs, err = b.RunCommands(cluster, [][]string{{"/sbin/iptables", "-L", "OUTPUT", "-vn"}}, []int{node})
				b.WorkOnServers()
				out = outs[0]
				if err != nil {
					if !strings.Contains(err.Error(), "/sbin/iptables: no such file or directory") {
						log.Printf("WARNING: Could not check OUTPUT: %s, got: %s", cluster, string(out))
					}
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
							dstclient, dstC, dstN := find_node_by_ip(nodes, dstIp)
							srcclienttext := ""
							dstclienttext := ""
							if isClient {
								srcclienttext = " (client)"
							}
							if dstclient {
								dstclienttext = " (client)"
							}

							dport := strings.Split(cut(line, 11, " "), ":")[1]
							suffix := cutSuffix(line, 12, " ")
							tb.AppendRow(table.Row{fmt.Sprintf("%s-%d%s", cluster, node, srcclienttext), fmt.Sprintf("%s-%d%s", dstC, dstN, dstclienttext), dport, t, "OUTPUT", "SrcNode", suffix})
							jout = append(jout, &rule{
								SourceCluster:      cluster,
								SourceNode:         node,
								SourceClient:       isClient,
								DestinationCluster: dstC,
								DestinationNode:    dstN,
								DestinationClient:  dstclient,
								Port:               dport,
								Type:               t,
								Chain:              "OUTPUT",
								RuleAppliedOn:      "Source",
								Behaviour:          suffix,
							})
						}
					}
				}
			}
		}
	}
	if !c.Json && !c.PrettyJson {
		fmt.Println(tb.Render())
		return nil
	}
	//sort json: {"Source", "Destination", "Port", "Type", "Chain", "RuleOn", "Behaviour"}
	sort.Slice(jout, func(i, j int) bool {
		for _, sortItem := range sortBy {
			sortItem = strings.ToLower(sortItem)
			switch sortItem {
			case "source":
				if jout[i].SourceCluster < jout[j].SourceCluster {
					return true
				}
				if jout[i].SourceCluster > jout[j].SourceCluster {
					return false
				}
				if jout[i].SourceNode < jout[j].SourceNode {
					return true
				}
				if jout[i].SourceNode > jout[j].SourceNode {
					return false
				}
				if !jout[i].SourceClient && jout[j].SourceClient {
					return true
				}
				if jout[i].SourceClient && !jout[j].SourceClient {
					return false
				}
				continue
			case "destination":
				if jout[i].DestinationCluster < jout[j].DestinationCluster {
					return true
				}
				if jout[i].DestinationCluster > jout[j].DestinationCluster {
					return false
				}
				if jout[i].DestinationNode < jout[j].DestinationNode {
					return true
				}
				if jout[i].DestinationNode > jout[j].DestinationNode {
					return false
				}
				if !jout[i].DestinationClient && jout[j].DestinationClient {
					return true
				}
				if jout[i].DestinationClient && !jout[j].DestinationClient {
					return false
				}
				continue
			case "port":
				if jout[i].Port < jout[j].Port {
					return true
				}
				if jout[i].Port > jout[j].Port {
					return false
				}
				continue
			case "type":
				if jout[i].Type < jout[j].Type {
					return true
				}
				if jout[i].Type > jout[j].Type {
					return false
				}
				continue
			case "chain":
				if jout[i].Chain < jout[j].Chain {
					return true
				}
				if jout[i].Chain > jout[j].Chain {
					return false
				}
				continue
			case "ruleon":
				if jout[i].RuleAppliedOn < jout[j].RuleAppliedOn {
					return true
				}
				if jout[i].RuleAppliedOn > jout[j].RuleAppliedOn {
					return false
				}
				continue
			case "behaviour":
				if jout[i].Behaviour < jout[j].Behaviour {
					return true
				}
				if jout[i].Behaviour > jout[j].Behaviour {
					return false
				}
				continue
			}
		}
		return false
	})
	j := json.NewEncoder(os.Stdout)
	if c.PrettyJson {
		j.SetIndent("", "  ")
	}
	j.Encode(jout)
	return nil
}

func find_node_by_ip(nodes map[bool]map[string]map[int][]string, ip string) (isClient bool, cluster string, node int) {
	for isClient, clusters := range nodes {
		for cluster := range clusters {
			for node, nodeIp := range nodes[isClient][cluster] {
				if inslice.HasString(nodeIp, ip) {
					return isClient, cluster, node
				}
			}
		}
	}
	return false, "UNDEF", 0
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

package main

import (
	"fmt"
	"sort"

	"github.com/bestmethod/inslice"
)

type clusterListCmd struct {
	Owner      string   `long:"owner" description:"Only show resources tagged with this owner"`
	SortBy     []string `long:"sort-by" description:"sort by field name; must match exact header name; can be specified multiple times; format: asc:name dsc:name ascnum:name dscnum:name"`
	Theme      string   `long:"theme" description:"for standard output, pick a theme: default|nocolor|frame|box"`
	NoNotes    bool     `long:"no-notes" description:"for standard output, do not print extra notes below the tables"`
	Json       bool     `short:"j" long:"json" description:"Provide output in json format"`
	JsonPretty bool     `short:"p" long:"pretty" description:"Provide json output with line-feeds and indentations"`
	Pager      bool     `long:"pager" description:"set to enable vertical and horizontal pager" simplemode:"false"`
	IP         bool     `short:"i" long:"ip" description:"print only the IP of the client machines (disables JSON output)"`
	RenderType string   `long:"render" description:"different output rendering; supported: text,csv,tsv,html,markdown" default:"text"`
	Help       helpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if c.IP {
		clusters, err := b.ClusterList()
		if err != nil {
			return err
		}
		sort.Strings(clusters)
		for _, cluster := range clusters {
			nodesI, err := b.GetNodeIpMap(cluster, true)
			if err != nil {
				return err
			}
			nodesE, err := b.GetNodeIpMap(cluster, false)
			if err != nil {
				return err
			}
			nodesIS := make([]int, 0, len(nodesI))
			for k := range nodesI {
				nodesIS = append(nodesIS, k)
			}
			sort.Ints(nodesIS)
			nodesES := make([]int, 0, len(nodesE))
			for k := range nodesE {
				if !inslice.HasInt(nodesIS, k) {
					nodesES = append(nodesES, k)
				}
			}
			sort.Ints(nodesES)
			for _, no := range nodesIS {
				ip := nodesI[no]
				extIP := nodesE[no]
				fmt.Printf("cluster=%s node=%d int_ip=%s ext_ip=%s\n", cluster, no, ip, extIP)
			}
			for _, no := range nodesES {
				ip := nodesE[no]
				if nodesI[no] == "" {
					fmt.Printf("cluster=%s node=%d int_ip= ext_ip=%s\n", cluster, no, ip)
				}
			}
		}
		return nil
	}
	f, e := b.ClusterListFull(c.Json, c.Owner, c.Pager, c.JsonPretty, c.SortBy, c.RenderType, c.Theme, c.NoNotes)
	if e != nil {
		return e
	}
	fmt.Println(f)
	return nil
}

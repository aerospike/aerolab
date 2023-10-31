package main

import (
	"fmt"
	"sort"
)

type clientListCmd struct {
	Owner string  `long:"owner" description:"Only show resources tagged with this owner"`
	Json  bool    `short:"j" long:"json" description:"Provide output in json format"`
	Pager bool    `long:"pager" description:"set to enable vertical and horizontal pager"`
	IP    bool    `short:"i" long:"ip" description:"print only the IP of the client machines (disables JSON output)"`
	Help  helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	b.WorkOnClients()
	defer b.WorkOnServers()
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
				nodesES = append(nodesES, k)
			}
			sort.Ints(nodesES)
			for _, no := range nodesIS {
				ip := nodesI[no]
				extIP := nodesE[no]
				fmt.Printf("client=%s machine=%d int_ip=%s ext_ip=%s\n", cluster, no, ip, extIP)
			}
			for _, no := range nodesES {
				ip := nodesE[no]
				if nodesI[no] == "" {
					fmt.Printf("client=%s machine=%d int_ip= ext_ip=%s\n", cluster, no, ip)
				}
			}
		}
		return nil
	}
	f, e := b.ClusterListFull(c.Json, c.Owner, c.Pager)
	if e != nil {
		return e
	}
	fmt.Println(f)
	return nil
}

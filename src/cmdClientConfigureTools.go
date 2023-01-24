package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bestmethod/inslice"
)

type clientConfigureToolsCmd struct {
	ClientName TypeClientName  `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines   TypeMachines    `short:"l" long:"machines" description:"Comma separated list of machines, empty=all" default:""`
	ConnectAMS TypeClusterName `short:"m" long:"ams" default:"ams" description:"AMS client machine name"`
	Help       helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureToolsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Print("Running client.configure.tools")
	a.opts.Attach.Client.ClientName = c.ClientName
	if c.Machines == "" {
		c.Machines = "ALL"
	}
	a.opts.Attach.Client.Machine = c.Machines
	nodeList, err := c.checkClustersExist(c.ConnectAMS.String())
	if err != nil {
		return err
	}
	b.WorkOnClients()
	allnodes := []string{}
	for _, nodes := range nodeList {
		for _, node := range nodes {
			allnodes = append(allnodes, node+"3100")
		}
	}
	if len(allnodes) == 0 {
		return errors.New("found 0 AMS machines")
	}
	if len(allnodes) > 1 {
		log.Printf("Found more than 1 AMS machine, will point log consolidator at the first one: %s", allnodes[0])
	}
	ip := allnodes[0] // this will have ip:3100
	_ = ip
	// TODO HERE - store contents of `ip` to /opt/asbench-grafana.ip
	// TODO HERE - install promtail if not found
	// TODO HERE - add startup script for promtail
	// TODO HERE - (re)configure promtail to new IP and (re) start promtail (kill it first)
	log.Print("Done")
	return nil
}

// return map[clusterName][]nodeIPs
func (c *clientConfigureToolsCmd) checkClustersExist(clusters string) (map[string][]string, error) {
	cnames := []string{}
	clusters = strings.Trim(clusters, "\r\n\t ")
	if len(clusters) > 0 {
		cnames = strings.Split(clusters, ",")
	}
	ret := make(map[string][]string)
	clist, err := b.ClusterList()
	if err != nil {
		return nil, err
	}
	// first pass check clusters exist
	for _, cname := range cnames {
		if !inslice.HasString(clist, cname) {
			return nil, fmt.Errorf("cluster %s does not exist", cname)
		}
	}
	// 2nd pass enumerate node IPs
	for _, cname := range cnames {
		ips, err := b.GetClusterNodeIps(cname)
		if err != nil {
			return nil, err
		}
		ret[cname] = ips
	}
	return ret, nil
}

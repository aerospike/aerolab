package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bestmethod/inslice"
)

// TODO change vector port, port 5000 is used by too many services
const vectorAccessPort = "5000"
const vectorAccessProtocol = "http"

type clientCreateVectorCmd struct {
	clientCreateNoneCmd
	ClusterName TypeClusterName `short:"C" long:"cluster-name" description:"cluster name to seed from" default:"mydc"`
	JustDoIt    bool            `long:"confirm" description:"set this parameter to confirm any warning questions without being asked to press ENTER to continue" webdisable:"true" webset:"true"`
	seedip      string
	seedport    string
	chDirCmd
}

func (c *clientCreateVectorCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type == "docker" && !strings.Contains(c.Docker.ExposePortsToHost, ":"+vectorAccessPort) {
		if c.Docker.NoAutoExpose {
			fmt.Printf("Docker backend is in use, but vector access port is not being forwarded. If using Docker Desktop, use '-e %s:%s' parameter in order to forward port %s. Press ENTER to continue regardless.\n", vectorAccessPort, vectorAccessPort, vectorAccessPort)
			if !c.JustDoIt {
				var ignoreMe string
				fmt.Scanln(&ignoreMe)
			}
		} else {
			c.Docker.ExposePortsToHost = strings.Trim(vectorAccessPort+":"+vectorAccessPort+","+c.Docker.ExposePortsToHost, ",")
		}
	}
	fmt.Println("Getting cluster list")
	b.WorkOnServers()
	clist, err := b.ClusterList()
	if err != nil {
		return err
	}
	if !inslice.HasString(clist, string(c.ClusterName)) {
		return errors.New("cluster not found")
	}
	ips, err := b.GetNodeIpMap(string(c.ClusterName), true)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		ips, err = b.GetNodeIpMap(string(c.ClusterName), false)
		if err != nil {
			return err
		}
		if len(ips) == 0 {
			return errors.New("node IPs not found")
		}
	}
	for _, ip := range ips {
		if ip != "" {
			c.seedip = ip
			break
		}
	}
	c.seedport = "3000"
	if a.opts.Config.Backend.Type == "docker" {
		inv, err := b.Inventory("", []int{InventoryItemClusters})
		if err != nil {
			return err
		}
		for _, item := range inv.Clusters {
			if item.ClusterName == c.ClusterName.String() {
				if item.PrivateIp != "" && item.DockerExposePorts != "" {
					c.seedport = item.DockerExposePorts
					c.seedip = item.PrivateIp
				}
			}
		}
	}
	b.WorkOnClients()
	if c.seedip == "" {
		return errors.New("could not find an IP for a node in the given cluster - are all the nodes down?")
	}

	// TODO future this whole thing
	return errors.New("not implemented yet")
}

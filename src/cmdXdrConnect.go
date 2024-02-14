package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/parallelize"
	"github.com/bestmethod/inslice"
)

type TypeAwsRegion string

func (t *TypeAwsRegion) Set(defaultRegion string) (changed bool, err error) {
	if a.opts.Config.Backend.Type != "aws" {
		// not aws, ignore this call
		return false, nil
	}
	if *t == "" {
		// reset to default region
		if a.opts.Config.Backend.Region == defaultRegion {
			// already at default
			return false, nil
		}
		// reset and init
		a.opts.Config.Backend.Region = defaultRegion
		if err := b.Init(); err != nil {
			return false, err
		}
		return true, nil
	}
	if string(*t) == a.opts.Config.Backend.Region {
		// region is already set accordingly
		return false, nil
	}
	// set new region
	a.opts.Config.Backend.Region = string(*t)
	if err := b.Init(); err != nil {
		return false, err
	}
	return true, nil
}

type xdrConnectCmd struct {
	SourceClusterName       TypeClusterName `short:"S" long:"source" description:"Source Cluster name" default:"mydc"`
	DestinationClusterNames TypeClusterName `short:"D" long:"destinations" description:"Destination Cluster names, comma separated." default:"destdc"`
	IsConnector             bool            `short:"c" long:"connector" description:"set to indicate that the destination is a client connector, not a cluster"`
	parallelThreadsCmd
	xdrConnectRealCmd
	Aws xdrConnectAws `no-flag:"true"`
}

type xdrConnectRealCmd struct {
	sourceClusterName       TypeClusterName
	destinationClusterNames TypeClusterName
	aws                     xdrConnectAws
	prevAwsRegion           string
	isConnector             bool
	parallelLimit           int
	Version                 TypeXDRVersion `short:"V" long:"xdr-version" description:"specify aerospike xdr configuration version (4|5|auto)" default:"auto" webchoice:"auto,5,4"`
	Restart                 TypeYesNo      `short:"T" long:"restart-source" description:"restart source nodes after connecting (y/n)" default:"y" webchoice:"y,n"`
	Namespaces              string         `short:"M" long:"namespaces" description:"Comma-separated list of namespaces to connect." default:"test"`
	xDestinations           []string
	xNamespaces             []string
	xDestIpList             map[string][]string
}

type xdrConnectAws struct {
	SourceRegion      TypeAwsRegion `short:"s" long:"source-region" description:"if set, will override the default configured backend region"`
	DestinationRegion TypeAwsRegion `short:"d" long:"destination-region" description:"if set, will override the default configured backend region"`
}

func init() {
	addBackendSwitch("xdr.connect", "aws", &a.opts.XDR.Connect.Aws)
}

func (c *xdrConnectCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	c.sourceClusterName = c.SourceClusterName
	c.destinationClusterNames = c.DestinationClusterNames
	c.aws = c.Aws
	c.isConnector = c.IsConnector
	c.parallelLimit = c.ParallelThreads
	return c.runXdrConnect(args)
}

func (c *xdrConnectRealCmd) runXdrConnect(args []string) error {
	log.Print("Running xdr.connect")
	c.prevAwsRegion = a.opts.Config.Backend.Region
	namespaces := strings.Split(c.Namespaces, ",")
	destinations := strings.Split(string(c.destinationClusterNames), ",")

	if c.Restart != "n" && c.Restart != "y" {
		return errors.New("restart-source option only accepts 'y' or 'n'")
	}

	_, err := c.aws.SourceRegion.Set(c.prevAwsRegion)
	if err != nil {
		return err
	}
	sourceClusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	destClusterList := sourceClusterList
	isChanged, err := c.aws.DestinationRegion.Set(c.prevAwsRegion)
	if err != nil {
		return err
	}
	if isChanged || c.isConnector {
		if c.isConnector {
			b.WorkOnClients()
		}
		destClusterList, err = b.ClusterList()
		if err != nil {
			return err
		}
		b.WorkOnServers()
	}

	if !inslice.HasString(sourceClusterList, string(c.sourceClusterName)) {
		err = fmt.Errorf("cluster does not exist: %s", c.sourceClusterName)
		return err
	}

	_, err = c.aws.SourceRegion.Set(c.prevAwsRegion)
	if err != nil {
		return err
	}
	sourceNodeList, err := b.NodeListInCluster(string(c.sourceClusterName))
	if err != nil {
		return err
	}

	destIpList := make(map[string][]string)
	_, err = c.aws.DestinationRegion.Set(c.prevAwsRegion)
	if err != nil {
		return err
	}
	if c.isConnector {
		b.WorkOnClients()
	}
	var inv inventoryJson
	if a.opts.Config.Backend.Type == "docker" {
		inv, err = b.Inventory("", []int{InventoryItemClusters})
		if err != nil {
			return err
		}
	}
	for _, destination := range destinations {
		if !inslice.HasString(destClusterList, destination) {
			err = fmt.Errorf("cluster does not exist: %s", destination)
			return err
		}
		destNodes, err := b.NodeListInCluster(destination)
		if err != nil {
			return err
		}
		var destIps []string
		if a.opts.Config.Backend.Type == "docker" {
			for _, item := range inv.Clusters {
				if item.ClusterName == destination {
					if item.DockerExposePorts == "" {
						destIps = append(destIps, item.PrivateIp)
					} else {
						destIps = append(destIps, item.PrivateIp+" "+item.DockerExposePorts)
					}
				}
			}
		} else {
			destIps, err = b.GetClusterNodeIps(destination)
			if err != nil {
				return err
			}
		}
		if len(destNodes) != len(destIps) {
			return fmt.Errorf("cluster %s is not on or IP allocation failed. Run: cluster list", destination)
		}
		destIpList[destination] = destIps
	}
	b.WorkOnServers()

	// we have c.SourceClusterName, sourceNodeList, destinations, destIpList, namespaces
	_, err = c.aws.SourceRegion.Set(c.prevAwsRegion)
	if err != nil {
		return err
	}
	returns := parallelize.MapLimit(sourceNodeList, c.parallelLimit, func(node int) error {
		_, err = b.RunCommands(string(c.sourceClusterName), [][]string{{"mkdir", "-p", "/opt/aerospike/xdr"}}, []int{node})
		if err != nil {
			return fmt.Errorf("failed running mkdir /opt/aerospike/xdr: %s", err)
		}
		return nil
	})
	isError := false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %d returned %s", sourceNodeList[i], ret)
			isError = true
		}
	}
	if isError {
		return errors.New("some nodes returned errors")
	}
	//for each source node
	c.xDestIpList = destIpList
	c.xDestinations = destinations
	c.xNamespaces = namespaces
	returns = parallelize.MapLimit(sourceNodeList, c.parallelLimit, c.doItXdrConnect)
	isError = false
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %d returned %s", sourceNodeList[i], ret)
			isError = true
		}
	}
	if isError {
		return errors.New("some nodes returned errors")
	}

	if c.Restart == "n" {
		log.Print("Done, Aerospike on source has NOT been restarted, changes not yet in effect")
		return nil
	}
	log.Print("Restarting source cluster nodes")
	snl := []string{}
	for _, sn := range sourceNodeList {
		snl = append(snl, strconv.Itoa(sn))
	}
	a.opts.Aerospike.Restart.ClusterName = c.sourceClusterName
	a.opts.Aerospike.Restart.Nodes = TypeNodes(strings.Join(snl, ","))
	a.opts.Aerospike.Restart.ParallelThreads = c.parallelLimit
	e := a.opts.Aerospike.Restart.Execute(args)
	if e != nil {
		return e
	}
	log.Print("Done")
	return nil
}

func (c *xdrConnectRealCmd) doItXdrConnect(snode int) error {
	xdrVersion := ""
	switch c.Version {
	case "5":
		xdrVersion = "5"
	case "4":
		xdrVersion = "4"
	case "auto":
		// perform discovery
		xdrVersion = "5"
		out, err := b.RunCommands(string(c.sourceClusterName), [][]string{{"cat", "/opt/aerolab.aerospike.version"}}, []int{snode})
		if err != nil {
			return fmt.Errorf("failed running cat /opt/aerolab.aerospike.version, cannot auto-discover: %s", err)
		}
		if strings.HasPrefix(string(out[0]), "4.") || strings.HasPrefix(string(out[0]), "3.") {
			xdrVersion = "4"
		}
	default:
		return fmt.Errorf("unrecognised xdr-config version: %s", c.Version)
	}
	if c.isConnector && xdrVersion == "4" {
		return errors.New("for connector setup, only use server versions 5+")
	}
	//build empty basic xdr stanza
	xdr_stanza := "\nxdr {\n    enable-xdr true\n    xdr-digestlog-path /opt/aerospike/xdr/digestlog 1G\n}\n"
	if xdrVersion == "5" {
		xdr_stanza = "\nxdr {\n\n}\n"
	}

	//read file
	out, err := b.RunCommands(string(c.sourceClusterName), [][]string{{"cat", "/etc/aerospike/aerospike.conf"}}, []int{snode})
	if err != nil {
		return fmt.Errorf("failed running cat /etc/aerospike/aerospike.conf: %s", err)
	}
	conf := string(out[0])

	//add xdr stanza if not found
	if !strings.Contains(conf, "xdr {\n") {
		conf = conf + xdr_stanza
	}

	//split conf to slice
	confs := strings.Split(conf, "\n")
	for i := range confs {
		confs[i] = strings.Trim(confs[i], "\r")
	}

	//find start and end of xdr stanza, find configured DCs
	dcStanzaName := "datacenter"
	nodeAddressPort := "dc-node-address-port"
	if xdrVersion == "5" {
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
	for i := 0; i < len(c.xDestinations); i++ {
		found := c.xDestinations[i]
		for _, k := range xdr_dcs {
			if c.xDestinations[i] == k {
				found = ""
				break
			}
		}
		if found != "" {
			useport := "3000"
			if c.isConnector {
				useport = "8901"
			}
			dc_to_add = dc_to_add + fmt.Sprintf("\n\t%s %s {\n", dcStanzaName, found)
			if c.isConnector {
				dc_to_add = dc_to_add + "\t\tconnector true\n"
			}
			dst_cluster_ips := c.xDestIpList[found]
			for j := 0; j < len(dst_cluster_ips); j++ {
				if strings.Contains(dst_cluster_ips[j], " ") {
					dc_to_add = dc_to_add + fmt.Sprintf("\t\t%s %s\n", nodeAddressPort, dst_cluster_ips[j])
				} else {
					dc_to_add = dc_to_add + fmt.Sprintf("\t\t%s %s %s\n", nodeAddressPort, dst_cluster_ips[j], useport)
				}
				if xdrVersion == "5" {
					if _, ok := dc2namespace[found]; !ok {
						dc2namespace[found] = []string{}
					}
					for _, nspace := range c.xNamespaces {
						if !inslice.HasString(dc2namespace[found], nspace) {
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

	if xdrVersion == "4" {
		//now latest config is in confsx
		//update namespaces to enable XDR in them
		for i := 0; i < len(c.xNamespaces); i++ {
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
				} else if (strings.Contains(confsx[j], "enable-xdr true") && !strings.Contains(confsx[j], "-enable-xdr true")) && nsloc != -1 && c.xNamespaces[i] == nsname {
					has_enable_xdr = true
				} else if strings.Contains(confsx[j], "xdr-remote-datacenter ") && nsloc != -1 && c.xNamespaces[i] == nsname {
					tmp := strings.Split(confsx[j], " ")
					for k := 0; k < len(tmp); k++ {
						if strings.Contains(tmp[k], "xdr-remote-datacenter") {
							has_dc_list = append(has_dc_list, tmp[k+1])
							break
						}
					}
				}
				if lvl == 0 && nsloc != -1 && nsname == c.xNamespaces[i] {
					//if has_enable_xdr is false, add that after confsx[nsloc]
					if !has_enable_xdr {
						confsx[nsloc] = confsx[nsloc] + "\nenable-xdr true"
					}
					//for each dc, if not found in has_dc_list, add like above, as remote-datacenter
					for k := 0; k < len(c.xDestinations); k++ {
						found := false
						for _, l := range has_dc_list {
							if c.xDestinations[k] == l {
								found = true
							}
						}
						if !found {
							confsx[nsloc] = confsx[nsloc] + fmt.Sprintf("\nxdr-remote-datacenter %s", c.xDestinations[k])
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

	finalConf := strings.Join(confsx, "\n")
	err = b.CopyFilesToCluster(string(c.sourceClusterName), []fileList{{"/etc/aerospike/aerospike.conf", finalConf, len(finalConf)}}, []int{snode})
	if err != nil {
		return fmt.Errorf("error trying to modify config file while configuring xdr: %s", err)
	}
	return nil
}

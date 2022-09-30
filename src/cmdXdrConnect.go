package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type TypeAwsRegion string

func (t *TypeAwsRegion) Set(defaultRegion string) (changed bool) {
	if a.opts.Config.Backend.Type != "aws" {
		// not aws, ignore this call
		return false
	}
	if *t == "" {
		// reset to default region
		if a.opts.Config.Backend.Region == defaultRegion {
			// already at default
			return false
		}
		// reset and init
		a.opts.Config.Backend.Region = defaultRegion
		if err := b.Init(); err != nil {
			log.Fatal(err)
		}
		return true
	}
	if string(*t) == a.opts.Config.Backend.Region {
		// region is already set accordingly
		return false
	}
	// set new region
	a.opts.Config.Backend.Region = string(*t)
	if err := b.Init(); err != nil {
		log.Fatal(err)
	}
	return true
}

type xdrConnectCmd struct {
	SourceClusterName       TypeClusterName `short:"S" long:"source" description:"Source Cluster name" default:"mydc"`
	DestinationClusterNames TypeClusterName `short:"D" long:"destinations" description:"Destination Cluster names, comma separated." default:"destdc"`
	Aws                     xdrConnectAws   `no-flag:"true"`
	xdrConnectRealCmd
}

type xdrConnectRealCmd struct {
	sourceClusterName       TypeClusterName
	destinationClusterNames TypeClusterName
	aws                     xdrConnectAws
	prevAwsRegion           string
	Version                 TypeXDRVersion `short:"V" long:"xdr-version" description:"specify aerospike xdr configuration version (4|5|auto)" default:"auto"`
	Restart                 TypeYesNo      `short:"T" long:"restart-source" description:"restart source nodes after connecting (y/n)" default:"y"`
	Namespaces              string         `short:"M" long:"namespaces" description:"Comma-separated list of namespaces to connect." default:"test"`
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

	c.aws.SourceRegion.Set(c.prevAwsRegion)
	sourceClusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	destClusterList := sourceClusterList
	if c.aws.DestinationRegion.Set(c.prevAwsRegion) {
		destClusterList, err = b.ClusterList()
		if err != nil {
			return err
		}
	}

	if !inslice.HasString(sourceClusterList, string(c.sourceClusterName)) {
		err = fmt.Errorf("cluster does not exist: %s", c.sourceClusterName)
		return err
	}

	c.aws.SourceRegion.Set(c.prevAwsRegion)
	sourceNodeList, err := b.NodeListInCluster(string(c.sourceClusterName))
	if err != nil {
		return err
	}

	destIpList := make(map[string][]string)
	c.aws.DestinationRegion.Set(c.prevAwsRegion)
	for _, destination := range destinations {
		if !inslice.HasString(destClusterList, destination) {
			err = fmt.Errorf("cluster does not exist: %s", destination)
			return err
		}
		destNodes, err := b.NodeListInCluster(destination)
		if err != nil {
			return err
		}
		destIps, err := b.GetClusterNodeIps(destination)
		if err != nil {
			return err
		}
		if len(destNodes) != len(destIps) {
			return fmt.Errorf("cluster %s is not on or IP allocation failed. Run: cluster list", destination)
		}
		destIpList[destination] = destIps
	}

	// we have c.SourceClusterName, sourceNodeList, destinations, destIpList, namespaces
	c.aws.SourceRegion.Set(c.prevAwsRegion)
	_, err = b.RunCommands(string(c.sourceClusterName), [][]string{[]string{"mkdir", "-p", "/opt/aerospike/xdr"}}, sourceNodeList)
	if err != nil {
		return fmt.Errorf("failed running mkdir /opt/aerospike/xdr: %s", err)
	}
	//for each source node
	for _, snode := range sourceNodeList {
		xdrVersion := ""
		switch c.Version {
		case "5":
			xdrVersion = "5"
		case "4":
			xdrVersion = "4"
		case "auto":
			// perform discovery
			xdrVersion = "5"
			out, err := b.RunCommands(string(c.sourceClusterName), [][]string{[]string{"cat", "/opt/aerolab.aerospike.version"}}, []int{snode})
			if err != nil {
				return fmt.Errorf("failed running cat /opt/aerolab.aerospike.version, cannot auto-discover: %s", err)
			}
			if strings.HasPrefix(string(out[0]), "4.") || strings.HasPrefix(string(out[0]), "3.") {
				xdrVersion = "4"
			}
		default:
			return fmt.Errorf("unrecognised xdr-config version: %s", c.Version)
		}
		//build empty basic xdr stanza
		xdr_stanza := "\nxdr {\n    enable-xdr true\n    xdr-digestlog-path /opt/aerospike/xdr/digestlog 1G\n}\n"
		if xdrVersion == "5" {
			xdr_stanza = "\nxdr {\n\n}\n"
		}

		//read file
		out, err := b.RunCommands(string(c.sourceClusterName), [][]string{[]string{"cat", "/etc/aerospike/aerospike.conf"}}, []int{snode})
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
					if xdrVersion == "5" {
						if _, ok := dc2namespace[found]; !ok {
							dc2namespace[found] = []string{}
						}
						for _, nspace := range namespaces {
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
					} else if (strings.Contains(confsx[j], "enable-xdr true") && !strings.Contains(confsx[j], "-enable-xdr true")) && nsloc != -1 && namespaces[i] == nsname {
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
						if !has_enable_xdr {
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
							if !found {
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

		finalConf := strings.Join(confsx, "\n")
		err = b.CopyFilesToCluster(string(c.sourceClusterName), []fileList{fileList{"/etc/aerospike/aerospike.conf", strings.NewReader(finalConf), len(finalConf)}}, []int{snode})
		if err != nil {
			return fmt.Errorf("error trying to modify config file while configuring xdr: %s", err)
		}
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
	e := a.opts.Aerospike.Restart.Execute(args)
	if e != nil {
		return e
	}
	log.Print("Done")
	return nil
}

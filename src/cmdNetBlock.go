package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type netBlockCmd struct {
	SourceClusterName      TypeClusterName      `short:"s" long:"source" description:"Source Cluster name" default:"mydc"`
	SourceNodeList         TypeNodes            `short:"l" long:"source-node-list" description:"List of source nodes. Empty=ALL." default:""`
	DestinationClusterName TypeClusterName      `short:"d" long:"destination" description:"Destination Cluster name" default:"mydc-xdr"`
	DestinationNodeList    TypeNodes            `short:"i" long:"destination-node-list" description:"List of destination nodes. Empty=ALL." default:""`
	Type                   TypeNetType          `short:"t" long:"type" description:"Block type (reject|drop)." default:"reject"`
	Ports                  string               `short:"p" long:"ports" description:"Comma separated list of ports to block." default:"3000"`
	BlockOn                TypeNetBlockOn       `short:"b" long:"block-on" description:"Block where (input|output). Input=on destination, output=on source." default:"input"`
	StatisticMode          TypeNetStatisticMode `short:"M" long:"statistic-mode" description:"for partial packet loss, supported are: random | nth. Not set: drop all packets." default:""`
	StatisticProbability   string               `short:"P" long:"probability" description:"for partial packet loss mode random. Supported values are between 0.0 and 1.0 (0% to 100%)" default:"0.5"`
	StatisticEvery         string               `short:"E" long:"every" description:"for partial packet loss mode nth. Match one every nth packet. Default: 2 (50% loss)" default:"2"`
	Help                   helpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *netBlockCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.run(args, "-I")
}

func (c *netBlockCmd) run(args []string, blockString string) error {
	if blockString == "-I" {
		log.Print("Running net.block")
	} else {
		log.Print("Running net.unblock")
	}
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
	sc = string(c.SourceClusterName)
	dc = string(c.DestinationClusterName)
	sn = strings.Split(c.SourceNodeList.String(), ",")
	dn = strings.Split(c.DestinationNodeList.String(), ",")
	t = c.Type.String()
	ports = strings.Split(c.Ports, ",")
	loc = c.BlockOn.String()
	statisticMode = c.StatisticMode.String()
	statisticRandom = c.StatisticProbability
	statisticEvery = c.StatisticEvery

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}

	if !inslice.HasString(clusterList, sc) {
		err = fmt.Errorf("error, source cluster does not exist: %s", sc)
		return err
	}
	if !inslice.HasString(clusterList, dc) {
		err = fmt.Errorf("error, destination cluster does not exist: %s", dc)
		return err
	}
	wherec := sc
	wheren := sn
	towherec := dc
	towheren := dn
	blockon := "--destination"
	r := blockString
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

	nodeIps, err := b.GetNodeIpMap(towherec, false)
	if err != nil {
		return err
	}
	nodeIpsInternal, err := b.GetNodeIpMap(towherec, true)
	if err != nil {
		return err
	}
	for _, nodes := range wheren {
		node, err := strconv.Atoi(nodes)
		if err != nil {
			return err
		}
		container := fmt.Sprintf("cluster %s node %d", wherec, node)
		for _, port := range ports {
			for _, bbb := range towheren {
				bb, err := strconv.Atoi(bbb)
				if err != nil {
					return err
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
				log.Printf("Running: %v", nComm)
				out, err := b.RunCommands(wherec, [][]string{nComm}, []int{node})
				if err != nil {
					log.Printf("WARNING: ERROR adding iptables rule on %s to block %s with IP %s\n%s\n", container, fmt.Sprintf("aero-%s_%s", towherec, b), ip, string(out[0]))
					log.Printf("RAN: %s %s %s %s %s %s %s %s %s %s %s\n", "iptables", r, strings.ToUpper(loc), "-p", "tcp", "--dport", port, blockon, ip, "-j", strings.ToUpper(t))
					return err
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
					log.Printf("Running: %v", nComm)
					out, err = b.RunCommands(wherec, [][]string{nComm}, []int{node})
					if err != nil {
						log.Printf("WARNING: ERROR adding iptables rule on %s to block %s with IP %s\n%s\n", container, fmt.Sprintf("aero-%s_%s", towherec, b), ip, string(out[0]))
						log.Printf("RAN: %s %s %s %s %s %s %s %s %s %s %s\n", "iptables", r, strings.ToUpper(loc), "-p", "tcp", "--dport", port, blockon, ip, "-j", strings.ToUpper(t))
						return err
					}
				}
			}
		}
	}
	log.Print("Done")
	return nil
}

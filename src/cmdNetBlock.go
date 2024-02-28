package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
)

type netBlockCmd struct {
	SourceClusterName      TypeClusterName      `short:"s" long:"source" description:"Source Cluster name/Client group" default:"mydc"`
	SourceNodeList         TypeNodes            `short:"l" long:"source-node-list" description:"List of source nodes. Empty=ALL." default:""`
	IsSourceClient         bool                 `short:"c" long:"source-client" description:"set to indicate the source is a client group"`
	DestinationClusterName TypeClusterName      `short:"d" long:"destination" description:"Destination Cluster name/Client group" default:"mydc-xdr"`
	DestinationNodeList    TypeNodes            `short:"i" long:"destination-node-list" description:"List of destination nodes. Empty=ALL." default:""`
	IsDestinationClient    bool                 `short:"C" long:"destination-client" description:"set to indicate the destination is a client group"`
	Type                   TypeNetType          `short:"t" long:"type" description:"Block type (reject|drop)." default:"reject" webchoice:"reject,drop"`
	Ports                  string               `short:"p" long:"ports" description:"Comma separated list of ports to block." default:"3000"`
	BlockOn                TypeNetBlockOn       `short:"b" long:"block-on" description:"Block where (input|output). Input=on destination, output=on source." default:"input" webchoice:"input,output"`
	StatisticMode          TypeNetStatisticMode `short:"M" long:"statistic-mode" description:"for partial packet loss, supported are: random | nth. Not set: drop all packets." default:""`
	StatisticProbability   string               `short:"P" long:"probability" description:"for partial packet loss mode random. Supported values are between 0.0 and 1.0 (0% to 100%)" default:"0.5"`
	StatisticEvery         string               `short:"E" long:"every" description:"for partial packet loss mode nth. Match one every nth packet. Default: 2 (50% loss)" default:"2"`
	Help                   helpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *netBlockCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.run("-I")
}

func (c *netBlockCmd) run(blockString string) error {
	if blockString == "-I" {
		log.Print("Running net.block")
	} else {
		log.Print("Running net.unblock")
	}
	log.Print("Gathering cluster information")
	if c.IsSourceClient {
		b.WorkOnClients()
	}
	err := c.SourceNodeList.ExpandNodes(string(c.SourceClusterName))
	b.WorkOnServers()
	if err != nil {
		return err
	}
	if c.IsDestinationClient {
		b.WorkOnClients()
	}
	err = c.DestinationNodeList.ExpandNodes(string(c.DestinationClusterName))
	b.WorkOnServers()
	if err != nil {
		return err
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
	clientList := []string{}
	if c.IsDestinationClient || c.IsSourceClient {
		b.WorkOnClients()
		clientList, err = b.ClusterList()
		b.WorkOnServers()
		if err != nil {
			return err
		}
	}

	if (c.IsSourceClient && !inslice.HasString(clientList, sc)) || (!c.IsSourceClient && !inslice.HasString(clusterList, sc)) {
		err = fmt.Errorf("error, source does not exist: %s", sc)
		return err
	}
	if (c.IsDestinationClient && !inslice.HasString(clientList, dc)) || (!c.IsDestinationClient && !inslice.HasString(clusterList, dc)) {
		err = fmt.Errorf("error, destination does not exist: %s", dc)
		return err
	}
	wherecClient := c.IsSourceClient
	towherecClient := c.IsDestinationClient
	wherec := sc
	wheren := sn
	towherec := dc
	towheren := dn
	blockon := "--destination"
	r := blockString
	if loc == "input" {
		wherecClient = c.IsDestinationClient
		towherecClient = c.IsSourceClient
		wherec = dc
		wheren = dn
		towherec = sc
		towheren = sn
		blockon = "--source"
	}

	if len(wheren) == 1 && wheren[0] == "" {
		var asdf []int
		if wherecClient {
			b.WorkOnClients()
		}
		asdf, _ = b.NodeListInCluster(wherec)
		b.WorkOnServers()
		wheren = []string{}
		for _, asd := range asdf {
			wheren = append(wheren, strconv.Itoa(asd))
		}
	}
	if len(towheren) == 1 && towheren[0] == "" {
		if towherecClient {
			b.WorkOnClients()
		}
		asdf, _ := b.NodeListInCluster(towherec)
		b.WorkOnServers()
		towheren = []string{}
		for _, asd := range asdf {
			towheren = append(towheren, strconv.Itoa(asd))
		}
	}

	var nodeIps map[int]string
	var nodeIpsInternal map[int]string
	if towherecClient {
		b.WorkOnClients()
	}
	nodeIps, err = b.GetNodeIpMap(towherec, false)
	if err != nil {
		return err
	}
	nodeIpsInternal, err = b.GetNodeIpMap(towherec, true)
	if err != nil {
		return err
	}
	b.WorkOnServers()
	if wherecClient {
		b.WorkOnClients()
	}
	log.Print("Compiling command list")
	commandList := make(map[int][]string) // map[node][]command
	for _, nodes := range wheren {
		node, err := strconv.Atoi(nodes)
		if err != nil {
			return err
		}
		//container := fmt.Sprintf("cluster %s node %d", wherec, node)
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
				if _, ok := commandList[node]; !ok {
					commandList[node] = []string{
						strings.Join(nComm, " "),
					}
				} else {
					commandList[node] = append(commandList[node], strings.Join(nComm, " "))
				}
				/*
					log.Printf("Running: %v", nComm)
					out, err := b.RunCommands(wherec, [][]string{nComm}, []int{node})
					if err != nil {
						log.Printf("WARNING: ERROR adding iptables rule on %s to block %s with IP %s\n%s\n", container, fmt.Sprintf("aero-%s_%s", towherec, b), ip, string(out[0]))
						log.Printf("RAN: %s %s %s %s %s %s %s %s %s %s %s\n", "iptables", r, strings.ToUpper(loc), "-p", "tcp", "--dport", port, blockon, ip, "-j", strings.ToUpper(t))
						return err
					}
				*/
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
					if _, ok := commandList[node]; !ok {
						commandList[node] = []string{
							strings.Join(nComm, " "),
						}
					} else {
						commandList[node] = append(commandList[node], strings.Join(nComm, " "))
					}
					/*
						log.Printf("Running: %v", nComm)
						out, err = b.RunCommands(wherec, [][]string{nComm}, []int{node})
						if err != nil {
							log.Printf("WARNING: ERROR adding iptables rule on %s to block %s with IP %s\n%s\n", container, fmt.Sprintf("aero-%s_%s", towherec, b), ip, string(out[0]))
							log.Printf("RAN: %s %s %s %s %s %s %s %s %s %s %s\n", "iptables", r, strings.ToUpper(loc), "-p", "tcp", "--dport", port, blockon, ip, "-j", strings.ToUpper(t))
							return err
						}
					*/
				}
			}
		}
	}
	log.Print("Executing commands on nodes")
	isErr := false
	lock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for node, commands := range commandList {
		wg.Add(1)
		go func(node int, commands []string) {
			defer wg.Done()
			out, err := b.RunCommands(wherec, [][]string{{"/bin/bash", "-c", strings.Join(commands, ";")}}, []int{node})
			if err != nil {
				log.Printf("ERROR running iptables on cluster %s node %v: %s: %s", wherec, node, err, string(out[0]))
				lock.Lock()
				isErr = true
				lock.Unlock()
			} else {
				log.Printf("Executed iptables on cluster %s node %v", wherec, node)
			}
		}(node, commands)
	}
	wg.Wait()
	if isErr {
		return errors.New("errors were encountered")
	}
	log.Print("Done")
	return nil
}

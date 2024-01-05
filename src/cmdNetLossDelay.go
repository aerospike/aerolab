package main

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type netLossDelayCmd struct {
	SourceClusterName      TypeClusterName   `short:"s" long:"source" description:"Source Cluster name/Client group" default:"mydc"`
	SourceNodeList         TypeNodes         `short:"l" long:"source-node-list" description:"List of source nodes. Empty=ALL." default:""`
	IsSourceClient         bool              `short:"c" long:"source-client" description:"set to indicate the source is a client group"`
	DestinationClusterName TypeClusterName   `short:"d" long:"destination" description:"Destination Cluster name/Client group" default:"mydc-xdr"`
	DestinationNodeList    TypeNodes         `short:"i" long:"destination-node-list" description:"List of destination nodes. Empty=ALL." default:""`
	IsDestinationClient    bool              `short:"C" long:"destination-client" description:"set to indicate the destination is a client group"`
	Action                 TypeNetLossAction `short:"a" long:"action" description:"One of: set|del|delall|show. delall does not require dest dc, as it removes all rules" default:"show"`
	ShowNames              bool              `short:"n" long:"show-names" description:"if action is show, this will cause IPs to resolve to names in output"`
	Delay                  string            `short:"D" long:"delay" description:"Delay (packet latency), e.g. 100ms or 0.5sec" default:""`
	Loss                   string            `short:"L" long:"loss" description:"Network loss in % packets. E.g. 0.1% or 20%" default:""`
	Rate                   string            `short:"R" long:"rate" description:"Max link speed, e.g. 100Kbps" default:""`
	RunOnDestination       bool              `short:"o" long:"on-destination" description:"if set, the rules will be created on destination nodes (avoid EPERM on source, true simulation)"`
	DstPort                int               `short:"p" long:"dst-port" description:"only apply the rule to a specific destination port"`
	SrcPort                int               `short:"P" long:"src-port" description:"only apply the rule to a specific source port"`
}

func (c *netLossDelayCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running net.loss-delay")

	// check cluster exists already
	clusterList := make(map[string]bool)
	ccClusters, err := b.ClusterList()
	if err != nil {
		return err
	}
	for _, c := range ccClusters {
		clusterList[c] = false
	}
	b.WorkOnClients()
	ccClients, err := b.ClusterList()
	b.WorkOnServers()
	if err != nil {
		return err
	}
	for _, c := range ccClients {
		clusterList[c] = true
	}

	if c.IsSourceClient {
		b.WorkOnClients()
	}
	err = c.SourceNodeList.ExpandNodes(string(c.SourceClusterName))
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

	fullIpMap := make(map[string]string)
	if c.Action == "show" {
		for cluster, isClient := range clusterList {
			if isClient {
				b.WorkOnClients()
			}
			ips, err := b.GetNodeIpMap(cluster, false)
			b.WorkOnServers()
			if err != nil {
				return err
			}
			for n, v := range ips {
				fullIpMap[v] = fmt.Sprintf("CLUSTER=%s NODE=%d", cluster, n)
			}
		}
	}

	if (c.IsSourceClient && !inslice.HasString(ccClients, string(c.SourceClusterName))) || (!c.IsSourceClient && !inslice.HasString(ccClusters, string(c.SourceClusterName))) {
		err = fmt.Errorf("error, does not exist: %s", string(c.SourceClusterName))
		return err
	}

	if c.Action != "show" && c.Action != "delall" {
		if (c.IsDestinationClient && !inslice.HasString(ccClients, string(c.DestinationClusterName))) || (!c.IsDestinationClient && !inslice.HasString(ccClusters, string(c.DestinationClusterName))) {
			err = fmt.Errorf("error, does not exist: %s", string(c.DestinationClusterName))
			return err
		}
	}

	sourceNodeList := []int{}
	var sourceNodeIpMap map[int]string
	var sourceNodeIpMapInternal map[int]string
	if c.SourceNodeList == "" {
		if c.IsSourceClient {
			b.WorkOnClients()
		}
		sourceNodeList, err = b.NodeListInCluster(string(c.SourceClusterName))
		b.WorkOnServers()
		if err != nil {
			return err
		}
	} else {
		if c.IsSourceClient {
			b.WorkOnClients()
		}
		snl, err := b.NodeListInCluster(string(c.SourceClusterName))
		b.WorkOnServers()
		if err != nil {
			return err
		}
		sn := strings.Split(c.SourceNodeList.String(), ",")
		for _, i := range sn {
			snInt, err := strconv.Atoi(i)
			if err != nil {
				return err
			}
			if !inslice.HasInt(snl, snInt) {
				if err != nil {
					return err
				}
			}
			sourceNodeList = append(sourceNodeList, snInt)
		}
	}

	if c.IsSourceClient {
		b.WorkOnClients()
	}
	sourceNodeIpMap, err = b.GetNodeIpMap(string(c.SourceClusterName), false)
	if err != nil {
		return err
	}

	sourceNodeIpMapInternal, err = b.GetNodeIpMap(string(c.SourceClusterName), true)
	if err != nil {
		return err
	}
	b.WorkOnServers()

	destNodeList := []int{}
	var destNodeIpMap map[int]string
	var destNodeIpMapInternal map[int]string
	if c.DestinationNodeList == "" {
		if c.IsDestinationClient {
			b.WorkOnClients()
		}
		destNodeList, err = b.NodeListInCluster(string(c.DestinationClusterName))
		b.WorkOnServers()
		if err != nil {
			return err
		}
	} else {
		if c.IsDestinationClient {
			b.WorkOnClients()
		}
		dnl, err := b.NodeListInCluster(string(c.DestinationClusterName))
		b.WorkOnServers()
		if err != nil {
			return err
		}
		dn := strings.Split(c.DestinationNodeList.String(), ",")
		for _, i := range dn {
			dnInt, err := strconv.Atoi(i)
			if err != nil {
				return err
			}
			if !inslice.HasInt(dnl, dnInt) {
				if err != nil {
					return err
				}
			}
			destNodeList = append(destNodeList, dnInt)
		}
	}

	if c.IsDestinationClient {
		b.WorkOnClients()
	}
	destNodeIpMap, err = b.GetNodeIpMap(string(c.DestinationClusterName), false)
	if err != nil {
		return err
	}

	destNodeIpMapInternal, err = b.GetNodeIpMap(string(c.DestinationClusterName), true)
	if err != nil {
		return err
	}
	b.WorkOnServers()

	sysRunOnClient := c.IsSourceClient
	sysRunOnClusterName := string(c.SourceClusterName)
	sysLogTheOther := string(c.DestinationClusterName)
	sysRunOnNodeList := sourceNodeList
	sysRunOnDestNodeList := destNodeList
	sysRunOnDestIpMap := destNodeIpMap
	sysRunOnDestIpMapInternal := destNodeIpMapInternal
	rule := "--direction=outgoing --network"
	if c.RunOnDestination {
		rule = "--direction=incoming --src-network"
		sysRunOnClient = c.IsDestinationClient
		sysRunOnClusterName = string(c.DestinationClusterName)
		sysLogTheOther = string(c.SourceClusterName)
		sysRunOnNodeList = destNodeList
		sysRunOnDestNodeList = sourceNodeList
		sysRunOnDestIpMap = sourceNodeIpMap
		sysRunOnDestIpMapInternal = sourceNodeIpMapInternal
	}
	if c.DstPort != 0 {
		rule = fmt.Sprintf("--port %d %s", c.DstPort, rule)
	}
	if c.SrcPort != 0 {
		rule = fmt.Sprintf("--src-port %d %s", c.SrcPort, rule)
	}

	iface := "eth0"
	found := false
	for _, sourceNode := range sysRunOnNodeList {
		command := []string{"ip", "route", "ls"}
		if sysRunOnClient {
			b.WorkOnClients()
		}
		out, err := b.RunCommands(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
		b.WorkOnServers()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out[0]), "\n") {
			line = strings.Trim(line, "\r\n\t ")
			if !strings.HasPrefix(line, "default via") {
				continue
			}
			lines := strings.Split(line, " ")
			for i, item := range lines {
				if item == "dev" {
					iface = lines[i+1]
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			break
		}
	}
	rest := ""
	if c.Action == "set" {
		rest = "tcset " + iface + " --change"
	} else if c.Action == "del" {
		rest = "tcdel " + iface
	} else if c.Action == "delall" {
		rest = "tcdel " + iface + " --all"
	} else {
		rest = "tcshow " + iface
	}
	if c.Rate != "" {
		rest = rest + " --rate " + c.Rate
	}
	if c.Delay != "" {
		rest = rest + " --delay " + c.Delay
	}
	if c.Loss != "" {
		rest = rest + " --loss " + c.Loss
	}

	log.Printf("Run on '%s' nodes '%v', implement loss/delay against '%s' nodes '%v' with IPs '%v' and optional IPs '%v'", sysRunOnClusterName, sysRunOnNodeList, sysLogTheOther, sysRunOnDestNodeList, sysRunOnDestIpMap, sysRunOnDestIpMapInternal)
	for _, sourceNode := range sysRunOnNodeList {
		container := fmt.Sprintf("cluster %s node %d", sysRunOnClusterName, sourceNode)
		if c.Action != "show" && c.Action != "delall" {
			for _, destNode := range sysRunOnDestNodeList {
				destNodeIp := sysRunOnDestIpMap[destNode]
				command := []string{"/bin/bash", "-c", fmt.Sprintf("%s %s %s", rest, rule, destNodeIp)}
				if sysRunOnClient {
					b.WorkOnClients()
				}
				out, err := b.RunCommands(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
				b.WorkOnServers()
				if err != nil {
					log.Printf("ERROR: %s %s %s", container, err, string(out[0]))
				}
				if sysRunOnDestIpMapInternal != nil {
					destNodeIpInternal := sysRunOnDestIpMapInternal[destNode]
					command := []string{"/bin/bash", "-c", fmt.Sprintf("%s %s %s", rest, rule, destNodeIpInternal)}
					if sysRunOnClient {
						b.WorkOnClients()
					}
					out, err = b.RunCommands(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
					b.WorkOnServers()
					if err != nil {
						log.Printf("ERROR: %s %s %s", container, err, string(out[0]))
					}
				}
			}
		} else {
			command := []string{"/bin/bash", "-c", rest}
			if sysRunOnClient {
				b.WorkOnClients()
			}
			out, err := b.RunCommands(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
			b.WorkOnServers()
			if err != nil {
				log.Printf("ERROR: %s %s %s", container, err, string(out[0]))
			} else if c.Action == "show" {
				fmt.Printf("========== %s ==========\n", container)
				prt := string(out[0])
				if c.ShowNames {
					for _, ip := range findIP(prt) {
						if fullIpMap[ip] != "" {
							prt = strings.Replace(prt, fmt.Sprintf("%s/32", ip), fullIpMap[ip], -1)
							prt = strings.Replace(prt, ip, fullIpMap[ip], -1)
						}
					}
				}
				fmt.Println(prt)
			}
		}
	}
	log.Print("Done")
	return nil
}

func findIP(input string) []string {
	numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock

	regEx := regexp.MustCompile(regexPattern)
	return regEx.FindAllString(input, -1)
}

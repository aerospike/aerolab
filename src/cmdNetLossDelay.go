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
	SourceClusterName      TypeClusterName   `short:"s" long:"source" description:"Source Cluster name" default:"mydc"`
	SourceNodeList         TypeNodes         `short:"l" long:"source-node-list" description:"List of source nodes. Empty=ALL." default:""`
	DestinationClusterName TypeClusterName   `short:"d" long:"destination" description:"Destination Cluster name" default:"mydc-xdr"`
	DestinationNodeList    TypeNodes         `short:"i" long:"destination-node-list" description:"List of destination nodes. Empty=ALL." default:""`
	Action                 TypeNetLossAction `short:"a" long:"action" description:"One of: set|del|delall|show. delall does not require dest dc, as it removes all rules" default:"show"`
	ShowNames              bool              `short:"n" long:"show-names" description:"if action is show, this will cause IPs to resolve to names in output"`
	Delay                  string            `short:"p" long:"delay" description:"Delay (packet latency), e.g. 100ms or 0.5sec" default:""`
	Loss                   string            `short:"L" long:"loss" description:"Network loss in % packets. E.g. 0.1% or 20%" default:""`
	RunOnDestination       bool              `short:"D" long:"on-destination" description:"if set, the rules will be created on destination nodes (avoid EPERM on source, true simulation)"`
	Rate                   string            `short:"R" long:"rate" description:"Max link speed, e.g. 100Kbps" default:""`
}

func (c *netLossDelayCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running net.loss-delay")

	// check cluster exists already
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	err = c.SourceNodeList.ExpandNodes(string(c.SourceClusterName))
	if err != nil {
		return err
	}
	err = c.DestinationNodeList.ExpandNodes(string(c.DestinationClusterName))
	if err != nil {
		return err
	}

	fullIpMap := make(map[string]string)
	if c.Action == "show" {
		for _, cluster := range clusterList {
			ips, err := b.GetNodeIpMap(cluster, false)
			if err != nil {
				return err
			}
			for n, v := range ips {
				fullIpMap[v] = fmt.Sprintf("CLUSTER=%s NODE=%d", cluster, n)
			}
		}
	}

	if !inslice.HasString(clusterList, string(c.SourceClusterName)) {
		err = fmt.Errorf("error, cluster does not exist: %s", string(c.SourceClusterName))
		return err
	}

	if c.Action != "show" && c.Action != "delall" {
		if !inslice.HasString(clusterList, string(c.DestinationClusterName)) {
			err = fmt.Errorf("error, cluster does not exist: %s", string(c.DestinationClusterName))
			return err
		}
	}

	sourceNodeList := []int{}
	var sourceNodeIpMap map[int]string
	var sourceNodeIpMapInternal map[int]string
	if c.SourceNodeList == "" {
		sourceNodeList, err = b.NodeListInCluster(string(c.SourceClusterName))
		if err != nil {
			return err
		}
	} else {
		snl, err := b.NodeListInCluster(string(c.SourceClusterName))
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

	sourceNodeIpMap, err = b.GetNodeIpMap(string(c.SourceClusterName), false)
	if err != nil {
		return err
	}

	sourceNodeIpMapInternal, err = b.GetNodeIpMap(string(c.SourceClusterName), true)
	if err != nil {
		return err
	}

	destNodeList := []int{}
	var destNodeIpMap map[int]string
	var destNodeIpMapInternal map[int]string
	if c.DestinationNodeList == "" {
		destNodeList, err = b.NodeListInCluster(string(c.DestinationClusterName))
		if err != nil {
			return err
		}
	} else {
		dnl, err := b.NodeListInCluster(string(c.DestinationClusterName))
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

	destNodeIpMap, err = b.GetNodeIpMap(string(c.DestinationClusterName), false)
	if err != nil {
		return err
	}

	destNodeIpMapInternal, err = b.GetNodeIpMap(string(c.DestinationClusterName), true)
	if err != nil {
		return err
	}

	sysRunOnClusterName := string(c.SourceClusterName)
	sysLogTheOther := string(c.DestinationClusterName)
	sysRunOnNodeList := sourceNodeList
	sysRunOnDestNodeList := destNodeList
	sysRunOnDestIpMap := destNodeIpMap
	sysRunOnDestIpMapInternal := destNodeIpMapInternal
	if c.RunOnDestination {
		sysRunOnClusterName = string(c.DestinationClusterName)
		sysLogTheOther = string(c.SourceClusterName)
		sysRunOnNodeList = destNodeList
		sysRunOnDestNodeList = sourceNodeList
		sysRunOnDestIpMap = sourceNodeIpMap
		sysRunOnDestIpMapInternal = sourceNodeIpMapInternal
	}

	iface := "eth0"
	found := false
	for _, sourceNode := range sysRunOnNodeList {
		command := []string{"ip", "route", "ls"}
		out, err := b.RunCommands(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
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
				command := []string{"/bin/bash", "-c", fmt.Sprintf("source /tcconfig/bin/activate; %s --network %s", rest, destNodeIp)}
				out, err := b.RunCommands(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
				if err != nil {
					log.Printf("ERROR: %s %s %s", container, err, string(out[0]))
				}
				if sysRunOnDestIpMapInternal != nil {
					destNodeIpInternal := sysRunOnDestIpMapInternal[destNode]
					command := []string{"/bin/bash", "-c", fmt.Sprintf("source /tcconfig/bin/activate; %s --network %s", rest, destNodeIpInternal)}
					out, err = b.RunCommands(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
					if err != nil {
						log.Printf("ERROR: %s %s %s", container, err, string(out[0]))
					}
				}
			}
		} else {
			command := []string{"/bin/bash", "-c", fmt.Sprintf("source /tcconfig/bin/activate; %s", rest)}
			out, err := b.RunCommands(sysRunOnClusterName, [][]string{command}, []int{sourceNode})
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

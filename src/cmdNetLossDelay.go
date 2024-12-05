package main

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/parallelize"
	"github.com/aerospike/aerolab/scripts"

	"github.com/bestmethod/inslice"
)

type netLossDelayCmd struct {
	SourceClusterName      TypeClusterName   `short:"s" long:"source" description:"Source Cluster name/Client group" default:"mydc"`
	SourceNodeList         TypeNodes         `short:"l" long:"source-node-list" description:"List of source nodes. Empty=ALL." default:""`
	IsSourceClient         bool              `short:"c" long:"source-client" description:"set to indicate the source is a client group"`
	DestinationClusterName TypeClusterName   `short:"d" long:"destination" description:"Destination Cluster name/Client group" default:"mydc-xdr"`
	DestinationNodeList    TypeNodes         `short:"i" long:"destination-node-list" description:"List of destination nodes. Empty=ALL." default:""`
	IsDestinationClient    bool              `short:"C" long:"destination-client" description:"set to indicate the destination is a client group"`
	Action                 TypeNetLossAction `short:"a" long:"action" description:"One of: set|del|delall|show. delall does not require dest dc, as it removes all rules" default:"show" webchoice:"show,set,del,reset"`
	LatencyMs              string            `short:"D" long:"latency-ms" description:"optional: specify latency (number) of milliseconds"`
	PacketLossPct          string            `short:"L" long:"loss-pct" description:"optional: specify packet loss percentage"`
	LinkSpeedRateBytes     string            `short:"E" long:"rate-bytes" description:"optional: specify link speed rate, in bytes"`
	CorruptPct             string            `short:"O" long:"corrupt-pct" description:"optional: currupt packets (percentage)"`
	RunOnDestination       bool              `short:"o" long:"on-destination" description:"if set, the rules will be created on destination nodes (avoid EPERM on source, true simulation)"`
	DstPort                int               `short:"p" long:"dst-port" description:"only apply the rule to a specific destination port"`
	SrcPort                int               `short:"P" long:"src-port" description:"only apply the rule to a specific source port"`
	Verbose                bool              `short:"v" long:"verbose" description:"run easytc in verbose mode"`
	parallelThreadsCmd
}

func expandNodeList(nodes string) ([]int, error) {
	if nodes == "" {
		return []int{}, nil
	}
	ret := []int{}
	for _, i := range strings.Split(nodes, ",") {
		if !strings.Contains(i, "-") {
			n, err := strconv.Atoi(i)
			if err != nil {
				return nil, err
			}
			if !inslice.HasInt(ret, n) {
				ret = append(ret, n)
			}
			continue
		}
		ii := strings.Split(i, "-")
		if len(ii) != 2 {
			return nil, errors.New("double-minus in node range list")
		}
		start, err := strconv.Atoi(ii[0])
		if err != nil {
			return nil, err
		}
		end, err := strconv.Atoi(ii[1])
		if err != nil {
			return nil, err
		}
		for x := start; x <= end; x++ {
			if !inslice.HasInt(ret, x) {
				ret = append(ret, x)
			}
		}
	}
	sort.Ints(ret)
	return ret, nil
}

func (c *netLossDelayCmd) findClient(inv inventoryJson, name string, nodes string, optional bool) ([]inventoryClient, error) {
	n, err := expandNodeList(nodes)
	if err != nil {
		return nil, err
	}
	ret := []inventoryClient{}
	for _, i := range inv.Clients {
		if i.ClientName != name {
			continue
		}
		if len(n) == 0 {
			ret = append(ret, i)
			continue
		}
		nno, err := strconv.Atoi(i.NodeNo)
		if err != nil {
			return nil, err
		}
		if inslice.HasInt(n, nno) {
			ret = append(ret, i)
		}
	}
	if optional {
		return ret, nil
	}
	if len(ret) == 0 {
		err = errors.New("client group not found")
		return ret, err
	}
	if len(ret) < len(n) {
		err = errors.New("not all nodes selected exist")
	}
	return ret, err
}

func (c *netLossDelayCmd) findCluster(inv inventoryJson, name string, nodes string, optional bool) ([]inventoryCluster, error) {
	n, err := expandNodeList(nodes)
	if err != nil {
		return nil, err
	}
	ret := []inventoryCluster{}
	for _, i := range inv.Clusters {
		if i.ClusterName != name {
			continue
		}
		if len(n) == 0 {
			ret = append(ret, i)
			continue
		}
		nno, err := strconv.Atoi(i.NodeNo)
		if err != nil {
			return nil, err
		}
		if inslice.HasInt(n, nno) {
			ret = append(ret, i)
		}
	}
	if optional {
		return ret, nil
	}
	if len(ret) == 0 {
		err = errors.New("cluster not found")
		return ret, err
	}
	if len(ret) < len(n) {
		err = errors.New("not all nodes selected exist")
	}
	return ret, err
}

func (c *netLossDelayCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running net.loss-delay")

	// inventory, get items
	var srcCluster, dstCluster []inventoryCluster
	var srcClient, dstClient []inventoryClient
	inv, err := b.Inventory("", []int{InventoryItemClusters, InventoryItemClients})
	if err != nil {
		return err
	}
	isSrcOptional := false
	isDstOptional := false
	if c.Action != "set" && c.Action != "del" {
		if c.RunOnDestination {
			isSrcOptional = true
		} else {
			isDstOptional = true
		}
	}
	if c.IsSourceClient {
		srcClient, err = c.findClient(inv, string(c.SourceClusterName), string(c.SourceNodeList), isSrcOptional)
		if err != nil {
			return err
		}
	} else {
		srcCluster, err = c.findCluster(inv, string(c.SourceClusterName), string(c.SourceNodeList), isSrcOptional)
		if err != nil {
			return err
		}
	}
	if c.IsDestinationClient {
		dstClient, err = c.findClient(inv, string(c.DestinationClusterName), string(c.DestinationNodeList), isDstOptional)
		if err != nil {
			return err
		}
	} else {
		dstCluster, err = c.findCluster(inv, string(c.DestinationClusterName), string(c.DestinationNodeList), isDstOptional)
		if err != nil {
			return err
		}
	}
	ips := make(map[string]string)
	for _, a := range inv.Clients {
		if a.PrivateIp != "" {
			ips[a.PrivateIp] = fmt.Sprintf("client:%s:%s", a.ClientName, a.NodeNo)
		}
		if a.PublicIp != "" && a.PublicIp != a.PrivateIp {
			ips[a.PublicIp] = fmt.Sprintf("client:%s:%s", a.ClientName, a.NodeNo)
		}
	}
	for _, a := range inv.Clusters {
		if a.PrivateIp != "" {
			ips[a.PrivateIp] = fmt.Sprintf("cluster:%s:%s", a.ClusterName, a.NodeNo)
		}
		if a.PublicIp != "" && a.PublicIp != a.PrivateIp {
			ips[a.PublicIp] = fmt.Sprintf("cluster:%s:%s", a.ClusterName, a.NodeNo)
		}
	}

	comm := []string{"easytc"}
	switch c.Action {
	case "reset", "delall":
		comm = append(comm, "reset")
		if a.opts.Config.Backend.Type == "docker" {
			comm = append(comm, "--no-check-module")
		}
	case "show":
		comm = append(comm, "show", "rules", "--quiet")
	case "set", "del":
		comm = append(comm, c.Action.String())
		if c.SrcPort != 0 {
			comm = append(comm, "-S", strconv.Itoa(c.SrcPort))
		}
		if c.DstPort != 0 {
			comm = append(comm, "-D", strconv.Itoa(c.DstPort))
		}
		if c.Action == "set" {
			if c.CorruptPct != "" {
				comm = append(comm, "-c", c.CorruptPct)
			}
			if c.LinkSpeedRateBytes != "" {
				comm = append(comm, "-e", c.LinkSpeedRateBytes)
			}
			if c.PacketLossPct != "" {
				comm = append(comm, "-p", c.PacketLossPct)
			}
			if c.LatencyMs != "" {
				comm = append(comm, "-l", c.LatencyMs)
			}
		}
		if a.opts.Config.Backend.Type == "docker" {
			comm = append(comm, "--no-check-module")
		}
	}
	if c.Verbose {
		comm = append(comm, "--verbose")
	}

	comms := [][]string{}
	if c.Action == "set" || c.Action == "del" {
		if c.RunOnDestination {
			// add sourceIPs to list of commands
			for _, node := range srcClient {
				if node.PrivateIp != "" {
					new := append(slices.Clone(comm), "-s", node.PrivateIp)
					comms = append(comms, new)
				}
				if node.PublicIp != "" && node.PublicIp != node.PrivateIp {
					new := append(slices.Clone(comm), "-s", node.PublicIp)
					comms = append(comms, new)
				}
			}
			for _, node := range srcCluster {
				if node.PrivateIp != "" {
					new := append(slices.Clone(comm), "-s", node.PrivateIp)
					comms = append(comms, new)
				}
				if node.PublicIp != "" && node.PublicIp != node.PrivateIp {
					new := append(slices.Clone(comm), "-s", node.PublicIp)
					comms = append(comms, new)
				}
			}
		} else {
			// add destIPs to list of commands
			for _, node := range dstClient {
				if node.PrivateIp != "" {
					new := append(slices.Clone(comm), "-d", node.PrivateIp)
					comms = append(comms, new)
				}
				if node.PublicIp != "" && node.PublicIp != node.PrivateIp {
					new := append(slices.Clone(comm), "-d", node.PublicIp)
					comms = append(comms, new)
				}
			}
			for _, node := range dstCluster {
				if node.PrivateIp != "" {
					new := append(slices.Clone(comm), "-d", node.PrivateIp)
					comms = append(comms, new)
				}
				if node.PublicIp != "" && node.PublicIp != node.PrivateIp {
					new := append(slices.Clone(comm), "-d", node.PublicIp)
					comms = append(comms, new)
				}
			}
		}
	} else {
		comms = append(comms, comm)
	}

	// comms is a full list of commands to execute on either source or destination
	script := scripts.GetNetLossDelay()
	for _, c := range comms {
		script = script + "\n" + strings.Join(c, " ")
	}

	isError := false
	runListClient := srcClient
	runListCluster := srcCluster
	if c.RunOnDestination {
		runListClient = dstClient
		runListCluster = dstCluster
	}
	// parallel run on each client/cluster from the list
	b.WorkOnClients()
	returns := parallelize.MapLimit(runListClient, c.ParallelThreads, func(node inventoryClient) error {
		return netLossRun(node.ClientName, node.NodeNo, script, ips)
	})
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %s returned %s", runListClient[i].NodeNo, ret)
			isError = true
		}
	}
	b.WorkOnServers()
	returns = parallelize.MapLimit(runListCluster, c.ParallelThreads, func(node inventoryCluster) error {
		return netLossRun(node.ClusterName, node.NodeNo, script, ips)
	})
	for i, ret := range returns {
		if ret != nil {
			log.Printf("Node %s returned %s", runListCluster[i].NodeNo, ret)
			isError = true
		}
	}

	if isError {
		return errors.New("some nodes returned errors")
	}
	log.Print("Done")
	return nil
}

func netLossRun(name string, node string, script string, ipmap map[string]string) error {
	nno, _ := strconv.Atoi(node)
	err := b.CopyFilesToCluster(name, []fileList{
		{
			filePath:     "/tmp/runtc.sh",
			fileContents: script,
			fileSize:     len(script),
		},
	}, []int{nno})
	if err != nil {
		return err
	}
	out, err := b.RunCommands(name, [][]string{{"/bin/bash", "/tmp/runtc.sh"}}, []int{nno})
	if err != nil {
		if len(out) == 0 {
			out = [][]byte{{'-'}}
		}
		return fmt.Errorf("(%s:%d) %s: %s", name, nno, err, string(out[0]))
	}
	lines := strings.Split(string(out[0]), "\n")
	ret := fmt.Sprintf("================== %s:%d ==================", name, nno)
	for _, line := range lines {
		if strings.Contains(line, "TcFilterHandle") {
			line = line + " IPNode "
		} else if strings.Contains(line, "-----------") {
			line = line + "--------"
		} else {
			ips := findIP(line)
			if len(ips) > 0 {
				if val, ok := ipmap[ips[0]]; ok {
					line = line + " " + val
				}
			}
		}
		ret = ret + "\n" + line
	}
	fmt.Print(ret)
	return nil
}

func findIP(input string) []string {
	numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock

	regEx := regexp.MustCompile(regexPattern)
	return regEx.FindAllString(input, -1)
}

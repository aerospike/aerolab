package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/bestmethod/inslice"
)

type clusterPartitionListCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes            TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks      TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterPartitions TypeFilterRange `short:"p" long:"filter-partitions" description:"Select partitions on each disk by number, empty or 0 = don't show partitions, ex: 1,2,4-8" default:"ALL"`
	FilterType       string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme/local or ebs/persistent" default:"ALL"`
	parallelThreadsLongCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterPartitionListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	_, err := c.run(true)
	if err != nil {
		return err
	}
	return nil
}

type disks map[int]map[int]map[int]blockDevices // map[node]map[diskNo]map[partNo]blockDevice

func (f TypeFilterRange) Expand() ([]int, error) {
	if f == "ALL" || f == "" {
		return nil, nil
	}
	list := []int{}
	for _, item := range strings.Split(string(f), ",") {
		if strings.HasPrefix(item, "-") {
			itemNo, err := strconv.Atoi(strings.TrimPrefix(item, "-"))
			if err != nil {
				return nil, err
			}
			ind := inslice.Int(list, itemNo, 1)
			if len(ind) == 0 {
				continue
			}
			list = append(list[:ind[0]], list[ind[0]+1:]...)
		} else if strings.Contains(item, "-") {
			itemRange := strings.Split(item, "-")
			if len(itemRange) != 2 {
				return nil, errors.New("badly formatted range")
			}
			start, err := strconv.Atoi(itemRange[0])
			if err != nil {
				return nil, err
			}
			end, err := strconv.Atoi(itemRange[1])
			if err != nil {
				return nil, err
			}
			if start < 1 || end < start {
				return nil, errors.New("range is incorrect")
			}
			for start <= end {
				list = append(list, start)
				start++
			}
		} else {
			itemNo, err := strconv.Atoi(item)
			if err != nil {
				return nil, err
			}
			list = append(list, itemNo)
		}
	}
	sort.Ints(list)
	return list, nil
}

func (c *clusterPartitionListCmd) fixPartOut(bd []blockDevices, blkFormat int) []blockDevices {
	for i := range bd {
		bd[i].FsSize = strings.Trim(bd[i].FsSize, "\t\r\n ")
		bd[i].FsType = strings.Trim(bd[i].FsType, "\t\r\n ")
		bd[i].Model = strings.Trim(bd[i].Model, "\t\r\n ")
		bd[i].MountPoint = strings.Trim(bd[i].MountPoint, "\t\r\n ")
		bd[i].Name = strings.Trim(bd[i].Name, "\t\r\n ")
		bd[i].Path = strings.Trim(bd[i].Path, "\t\r\n ")
		bd[i].Size = strings.Trim(bd[i].Size, "\t\r\n ")
		bd[i].Type = strings.Trim(bd[i].Type, "\t\r\n ")
		if blkFormat == 2 {
			bd[i].Path = bd[i].Name
			bd[i].FsSize = "N/A"
		}
		if len(bd[i].Children) > 0 {
			bd[i].Children = c.fixPartOut(bd[i].Children, blkFormat)
		}
	}
	return bd
}

func (c *clusterPartitionListCmd) run(printable bool) (disks, error) {
	if c.FilterType == "local" {
		c.FilterType = "nvme"
	} else if c.FilterType == "persistent" {
		c.FilterType = "ebs"
	}
	if !inslice.HasString([]string{"nvme", "ebs", "ALL"}, c.FilterType) {
		return nil, fmt.Errorf("disk type has to be one of: nvme,ebs")
	}
	filterPartitions, err := c.FilterPartitions.Expand()
	if err != nil {
		return nil, err
	}
	filterDisks, err := c.FilterDisks.Expand()
	if err != nil {
		return nil, err
	}
	dout := make(disks)
	err = c.Nodes.ExpandNodes(c.ClusterName.String())
	if err != nil {
		return nil, err
	}
	nodes, err := c.Nodes.Translate(c.ClusterName.String())
	if err != nil {
		return nil, err
	}
	sort.Ints(nodes)
	headerPrinted := NewSafeBool()
	if len(nodes) == 1 || c.ParallelThreads == 1 {
		for _, node := range nodes {
			err = c.runList(dout, node, printable, filterDisks, filterPartitions, headerPrinted)
			if err != nil {
				return dout, err
			}
		}
	} else {
		parallel := make(chan int, c.ParallelThreads)
		hasError := make(chan bool, len(nodes))
		wait := new(sync.WaitGroup)
		for _, node := range nodes {
			parallel <- 1
			wait.Add(1)
			go c.runListParallel(dout, node, printable, filterDisks, filterPartitions, headerPrinted, parallel, wait, hasError)
		}
		wait.Wait()
		if len(hasError) > 0 {
			return dout, fmt.Errorf("failed to get logs from %d nodes", len(hasError))
		}
	}
	if printable && !headerPrinted.Get() {
		fmt.Println("NO NON-OS DISKS FOUND MATCHING SEARCH CRITERIA")
	}
	return dout, nil
}

func (c *clusterPartitionListCmd) runListParallel(dout disks, node int, printable bool, filterDisks []int, filterPartitions []int, headerPrinted *SafeBool, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	err := c.runList(dout, node, printable, filterDisks, filterPartitions, headerPrinted)
	if err != nil {
		log.Printf("ERROR getting logs from node %d: %s", node, err)
		hasError <- true
	}
}

func (c *clusterPartitionListCmd) runList(dout disks, node int, printable bool, filterDisks []int, filterPartitions []int, headerPrinted *SafeBool) error {
	blkFormat := 1
	ret, err := b.RunCommands(c.ClusterName.String(), [][]string{{"lsblk", "-a", "-f", "-J", "-o", "NAME,PATH,FSTYPE,FSSIZE,MOUNTPOINT,MODEL,SIZE,TYPE"}}, []int{node})
	if err != nil {
		blkFormat = 2
		ret, err = b.RunCommands(c.ClusterName.String(), [][]string{{"lsblk", "-a", "-f", "-J", "-p", "-o", "NAME,FSTYPE,MOUNTPOINT,MODEL,SIZE,TYPE"}}, []int{node})
		if err != nil {
			return err
		}
	}
	disks := &blockDeviceInformation{}
	err = json.Unmarshal(ret[0], disks)
	if err != nil {
		return err
	}
	// fix centos weirdnesses ...
	disks.BlockDevices = c.fixPartOut(disks.BlockDevices, blkFormat)
	// centos fix end
	out := []string{}
	sort.Slice(disks.BlockDevices, func(x, y int) bool {
		return disks.BlockDevices[x].Name < disks.BlockDevices[y].Name
	})
	diskNo := 0
	for _, disk := range disks.BlockDevices {
		if disk.Type != "disk" {
			continue
		}
		diskType := ""
		if (a.opts.Config.Backend.Type == "aws" && disk.Model == "Amazon Elastic Block Store") || (a.opts.Config.Backend.Type == "gcp" && strings.HasPrefix(disk.Model, "PersistentDisk")) {
			diskType = "ebs"
			if c.FilterType != "" && c.FilterType != "ALL" && c.FilterType != "ebs" {
				continue
			}
		} else if (a.opts.Config.Backend.Type == "aws" && disk.Model == "Amazon EC2 NVMe Instance Storage") || (a.opts.Config.Backend.Type == "gcp" && disk.Model != "") {
			diskType = "nvme"
			if c.FilterType != "" && c.FilterType != "ALL" && c.FilterType != "nvme" {
				continue
			}
		} else {
			continue
		}
		if disk.MountPoint == "/" || strings.HasPrefix(disk.MountPoint, "/boot") {
			continue
		}
		isRestricted := false
		for _, part := range disk.Children {
			if part.MountPoint == "/" || strings.HasPrefix(part.MountPoint, "/boot") {
				isRestricted = true
				break
			}
		}
		if isRestricted {
			continue
		}
		diskNo++
		if c.FilterDisks != "ALL" && c.FilterDisks != "" && !inslice.HasInt(filterDisks, diskNo) {
			continue
		}
		disk.diskNo = diskNo
		disk.partNo = 0
		disk.nodeNo = node
		if _, ok := dout[node]; !ok {
			dout[node] = make(map[int]map[int]blockDevices)
		}
		if _, ok := dout[node][diskNo]; !ok {
			dout[node][diskNo] = make(map[int]blockDevices)
		}
		dout[node][diskNo][0] = disk
		diskTypePrint := diskType
		if a.opts.Config.Backend.Type == "gcp" {
			if diskType == "ebs" {
				diskTypePrint = "persistent"
			} else {
				diskTypePrint = "local"
			}
		}
		out = append(out, fmt.Sprintf("%4d %-10s %4d    - %-12s %8s %6s %8s %s", node, diskTypePrint, diskNo, disk.Name, disk.Size, disk.FsType, disk.FsSize, disk.MountPoint))
		if len(disk.Children) != 0 && c.FilterPartitions != "0" && c.FilterPartitions != "" {
			sort.Slice(disk.Children, func(x, y int) bool {
				return disk.Children[x].Name < disk.Children[y].Name
			})
			partNo := 0
			for _, part := range disk.Children {
				if part.Type != "part" {
					continue
				}
				partNo++
				if c.FilterPartitions == "ALL" || inslice.HasInt(filterPartitions, partNo) {
					part.diskNo = diskNo
					part.nodeNo = node
					part.partNo = partNo
					dout[node][diskNo][partNo] = part
					diskTypePrint := diskType
					if a.opts.Config.Backend.Type == "gcp" {
						if diskType == "ebs" {
							diskTypePrint = "persistent"
						} else {
							diskTypePrint = "local"
						}
					}
					out = append(out, fmt.Sprintf("%4d %-10s %4d %4d %-12s %8s %6s %8s %s", node, diskTypePrint, diskNo, partNo, part.Name, part.Size, part.FsType, part.FsSize, part.MountPoint))
				}
			}
		}
	}
	if printable && len(out) > 0 {
		headerPrinted.lock.Lock()
		if !headerPrinted.val {
			fmt.Println(strings.ToUpper("node type       disk part name             size fstype     fssize mountpoint"))
			fmt.Println("----------------------------------------------------------------------")
			headerPrinted.val = true
		}
		headerPrinted.lock.Unlock()
		fmt.Println(strings.Join(out, "\n"))
	}
	return nil
}

type SafeBool struct {
	val  bool
	lock *sync.RWMutex
}

func NewSafeBool() *SafeBool {
	return &SafeBool{
		lock: new(sync.RWMutex),
	}
}

func (i *SafeBool) Get() bool {
	i.lock.RLock()
	defer i.lock.RUnlock()
	return i.val
}

func (i *SafeBool) Set(p bool) {
	i.lock.Lock()
	i.val = p
	i.lock.Unlock()
}

func (i *SafeBool) GetAndSet(p bool) bool {
	i.lock.Lock()
	defer i.lock.Unlock()
	a := i.val
	i.val = p
	return a
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type clusterPartitionListCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes            TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks      TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterPartitions TypeFilterRange `short:"p" long:"filter-partitions" description:"Select partitions on each disk by number, empty or 0 = don't show partitions, ex: 1,2,4-8" default:"ALL"`
	FilterType       string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme|ebs" default:"ALL"`
	Help             helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
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

func (c *clusterPartitionListCmd) run(printable bool) (disks, error) {
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
	headerPrinted := false
	for _, node := range nodes {
		ret, err := b.RunCommands(c.ClusterName.String(), [][]string{[]string{"lsblk", "-a", "-f", "-J", "-o", "NAME,PATH,FSTYPE,FSSIZE,MOUNTPOINT,MODEL,SIZE,TYPE"}}, []int{node})
		if err != nil {
			return nil, err
		}
		disks := &blockDeviceInformation{}
		err = json.Unmarshal(ret[0], disks)
		if err != nil {
			return nil, err
		}
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
			if disk.Model == "Amazon Elastic Block Store" {
				diskType = "ebs"
				if c.FilterType != "" && c.FilterType != "ALL" && c.FilterType != "ebs" {
					continue
				}
			} else if disk.Model == "Amazon EC2 NVMe Instance Storage" {
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
			out = append(out, fmt.Sprintf("%4d %4s %4d    - %-12s %8s %6s %8s %s", node, diskType, diskNo, disk.Name, disk.Size, disk.FsType, disk.FsSize, disk.MountPoint))
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
						out = append(out, fmt.Sprintf("%4d %4s %4d %4d %-12s %8s %6s %8s %s", node, diskType, diskNo, partNo, part.Name, part.Size, part.FsType, part.FsSize, part.MountPoint))
					}
				}
			}
		}
		if printable && len(out) > 0 {
			if !headerPrinted {
				fmt.Println(strings.ToUpper("node type disk part name             size fstype     fssize mountpoint"))
				fmt.Println("----------------------------------------------------------------------")
				headerPrinted = true
			}
			fmt.Println(strings.Join(out, "\n"))
		}
	}
	if printable && !headerPrinted {
		fmt.Println("NO NON-OS DISKS FOUND MATCHING SEARCH CRITERIA")
	}
	return dout, nil
}
package cmd

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/bestmethod/inslice"
)

type ClusterPartitionCmd struct {
	Create ClusterPartitionCreateCmd `command:"create" subcommands-optional:"true" description:"Blkdiscard disks and/or create partitions on disks" webicon:"fas fa-circle-plus"`
	Mkfs   ClusterPartitionMkfsCmd   `command:"mkfs" subcommands-optional:"true" description:"Make filesystems on partitions and mount - for allflash" webicon:"fas fa-folder-tree"`
	Conf   ClusterPartitionConfCmd   `command:"conf" subcommands-optional:"true" description:"Adjust Aerospike configuration files on nodes to use created partitions" webicon:"fas fa-gear"`
	List   ClusterPartitionListCmd   `command:"list" subcommands-optional:"true" description:"List disks and partitions" webicon:"fas fa-list"`
	Help   HelpCmd                   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterPartitionCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type PartitionList []*PartitionInformation

type PartitionInformation struct {
	ClusterName  string      `json:"cluster_name"`
	NodeNo       int         `json:"node_no"`
	Disk         int         `json:"disk"`
	Partition    int         `json:"partition"` // 0 = root disk, no partition
	BlockDevices BlockDevice `json:"block_devices"`
}

type BlockDevice struct {
	Name         string         `json:"name"`
	Path         string         `json:"path"`
	FsType       string         `json:"fstype"`
	FsSize       string         `json:"fssize"`
	MountPoint   string         `json:"mountpoint"`
	Model        string         `json:"model"`      // "Amazon EC2 NVMe Instance Storage" or "Amazon Elastic Block Store"
	ModelType    string         `json:"model_type"` // "nvme" or "ebs"
	Size         string         `json:"size"`
	Type         string         `json:"type"` // loop or disk or part
	PartUUID     string         `json:"partuuid"`
	PartUUIDPath string         `json:"partuuid_path"`
	Children     []*BlockDevice `json:"children"`
}

func (p *PartitionList) Sort() {
	sort.Slice(*p, func(i, j int) bool {
		if (*p)[i].ClusterName < (*p)[j].ClusterName {
			return true
		}
		if (*p)[i].ClusterName > (*p)[j].ClusterName {
			return false
		}
		if (*p)[i].NodeNo < (*p)[j].NodeNo {
			return true
		}
		if (*p)[i].NodeNo > (*p)[j].NodeNo {
			return false
		}
		if (*p)[i].Disk < (*p)[j].Disk {
			return true
		}
		if (*p)[i].Disk > (*p)[j].Disk {
			return false
		}
		return (*p)[i].Partition < (*p)[j].Partition
	})
	for _, p := range *p {
		p.BlockDevices.Sort()
	}
}

func (b *BlockDevice) Sort() {
	sort.Slice(b.Children, func(i, j int) bool {
		return b.Children[i].Name < b.Children[j].Name
	})
	for _, c := range b.Children {
		c.Sort()
	}
}

type TypeFilterRange string

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

type blockDeviceInformation struct {
	BlockDevices []*BlockDevice `json:"blockdevices"`
}

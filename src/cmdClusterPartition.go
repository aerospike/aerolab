package main

import (
	"os"
	"strconv"
	"strings"
)

type clusterPartitionCmd struct {
	Create clusterPartitionCreateCmd `command:"create" subcommands-optional:"true" description:"Blkdiscard disks and/or create partitions on disks" webicon:"fas fa-circle-plus"`
	Mkfs   clusterPartitionMkfsCmd   `command:"mkfs" subcommands-optional:"true" description:"Make filesystems on partitions and mount - for allflash" webicon:"fas fa-folder-tree"`
	Conf   clusterPartitionConfCmd   `command:"conf" subcommands-optional:"true" description:"Adjust Aerospike configuration files on nodes to use created partitions" webicon:"fas fa-gear"`
	List   clusterPartitionListCmd   `command:"list" subcommands-optional:"true" description:"List disks and partitions" webicon:"fas fa-list"`
	Help   helpCmd                   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterPartitionCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type TypeFilterRange string

func (n *TypeNodes) Translate(clusterName string) ([]int, error) {
	if n.String() == "" {
		return b.NodeListInCluster(clusterName)
	}
	nodes := []int{}
	for _, ns := range strings.Split(n.String(), ",") {
		nn, err := strconv.Atoi(ns)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, nn)
	}
	return nodes, nil
}

type blockDeviceInformation struct {
	BlockDevices []blockDevices `json:"blockdevices"`
}

type blockDevices struct {
	Name       string         `json:"name"`
	Path       string         `json:"path"`
	FsType     string         `json:"fstype"`
	FsSize     string         `json:"fssize"`
	MountPoint string         `json:"mountpoint"`
	Model      string         `json:"model"` // "Amazon EC2 NVMe Instance Storage" or "Amazon Elastic Block Store"
	Size       string         `json:"size"`
	Type       string         `json:"type"` // loop or disk or part
	PartUUID   string         `json:"partuuid"`
	Children   []blockDevices `json:"children"`
	diskNo     int
	partNo     int
	nodeNo     int
}

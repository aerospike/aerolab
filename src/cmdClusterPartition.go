package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type clusterPartitionCmd struct {
	Create clusterPartitionCreateCmd `command:"create" subcommands-optional:"true" description:"Blkdiscard disks and/or create partitions on disks"`
	Mkfs   clusterPartitionMkfsCmd   `command:"mkfs" subcommands-optional:"true" description:"Make filesystems on partitions and mount - for allflash"`
	Conf   clusterPartitionConfCmd   `command:"conf" subcommands-optional:"true" description:"Adjust Aerospike configuration files on nodes to use created partitions"`
	List   clusterPartitionListCmd   `command:"list" subcommands-optional:"true" description:"List disks and partitions"`
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
	Children   []blockDevices `json:"children"`
	diskNo     int
	partNo     int
	nodeNo     int
}

type clusterPartitionCreateCmd struct {
	ClusterName  TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes        TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks  TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterType   string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme|ebs" default:"ALL"`
	Partitions   string          `short:"p" long:"partitions" description:"partitions to create, size is in %% of total disk space; ex: 25,25,25,25; default: just remove all partitions"`
	NoBlkdiscard bool            `short:"b" long:"no-blkdiscard" description:"set to prevent aerolab from running blkdiscard on the disks and partitions"`
	Help         helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterPartitionCreateCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running cluster.partition.create")
	partitions := []int{}
	if len(c.Partitions) > 0 {
		parts := strings.Split(c.Partitions, ",")
		total := 0
		for _, i := range parts {
			j, err := strconv.Atoi(i)
			if err != nil {
				return fmt.Errorf("could not translate partitions, must be number,number,number,... :%s", err)
			}
			if j < 1 {
				return fmt.Errorf("cannot create partition of %% size lesser than 1")
			}
			total += j
			if total > 100 {
				return fmt.Errorf("cannot create partitions totalling more than 100%% of the drive")
			}
			partitions = append(partitions, j)
		}
	}
	a.opts.Cluster.Partition.List.ClusterName = c.ClusterName
	a.opts.Cluster.Partition.List.Nodes = c.Nodes
	a.opts.Cluster.Partition.List.FilterDisks = c.FilterDisks
	a.opts.Cluster.Partition.List.FilterType = c.FilterType
	a.opts.Cluster.Partition.List.FilterPartitions = "0"
	d, err := a.opts.Cluster.Partition.List.run(false)
	if err != nil {
		return err
	}
	if len(d) == 0 {
		return errors.New("no matching disks found")
	}
	// TODO create c.Partitions partitions on the selected disks, don't forget to blkdiscard disks and partitions (blkdiscard, and then blkdiscard -z 8MiB each)
	// TODO remember to check for mountpoints, and unmount (modify /etc/fstab also if required) any partitions before removing them
	log.Print("Done")
	return nil
}

type clusterPartitionMkfsCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes            TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks      TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterPartitions TypeFilterRange `short:"p" long:"filter-partitions" description:"Select partitions on each disk by number, ex: 1,2,4-8" default:"ALL"`
	FilterType       string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme|ebs" default:"ALL"`
	FsType           string          `short:"f" long:"fs-type" description:"type of filesystem, ex: xfs" default:"xfs"`
	MountRoot        string          `short:"r" long:"mount-root" description:"path to where all the mounts will be created" default:"/mnt/"`
	MountOpts        string          `short:"o" long:"mount-options" description:"additional mount options to pass, ex: noatime,noexec" default:""`
	Help             helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterPartitionMkfsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running cluster.partition.mkfs")
	log.Print("Done")
	return nil
}

type clusterPartitionConfCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes            TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks      TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterPartitions TypeFilterRange `short:"p" long:"filter-partitions" description:"Select partitions on each disk by number, ex: 1,2,4-8" default:"ALL"`
	FilterType       string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme|ebs" default:"ALL"`
	Namespace        string          `short:"m" long:"namespace" description:"namespace to modify the settings for; default: first found namespace" default:""`
	ConfDest         string          `short:"o" long:"configure" description:"what to configure the selections as; options: device|shadow|allflash" default:""`
	Help             helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterPartitionConfCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running cluster.partition.conf")
	log.Print("Done")
	return nil
}

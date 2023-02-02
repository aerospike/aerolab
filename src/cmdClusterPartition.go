package main

import "log"

type clusterPartitionCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	// TODO: switches for: filters, partitions, etc
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterPartitionCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	log.Print("Running cluster.partition")
	log.Print("Done")
	return nil
}

/*
# partition disks
aerolab cluster partition create -n name -l all --filter-disks=1-3 --filter-partitions=1-3 --filter-type=nvme --partitions="20%,20%,20%,20%,20%"

# mkfs particular partitions, and mount
aerolab cluster partition mkfs   -n name -l all --filter-disks=1-3 --filter-partitions=1-3 --filter-type=nvme --mount-root="/mnt/" --type=xfs --options="noatime"

# clear namespace storage type definitions from the conf file
aerolab cluster partition conf   -n name -l all --clear=storage
aerolab cluster partition conf   -n name -l all --clear=allflash

# add storage type device and setup the devices matching filters
aerolab cluster partition conf   -n name -l all --filter-disks=1-3 --filter-partitions=1-3 --filter-type=nvme --namespace=test --device

# add shadow devices matching filters
aerolab cluster partition conf   -n name -l all --filter-disks=1-3 --filter-partitions=1-3 --filter-type=nvme --namespace=test --shadow

# add allflash definition matching filters
aerolab cluster partition conf   -n name -l all --filter-disks=1-3 --filter-partitions=1-3 --filter-type=nvme --namespace=test --allflash

--- filters ---
TODO
--filter-disks=1-3      // first 3 nvme, or first 3 sdX, that are not used for root '/', leave empty for ALL disks
--filter-partitions=1-3 // first 3 partitions on each selected disk, or don't set to select the disk itself
--filter-type=nvme      // or ebs
*/

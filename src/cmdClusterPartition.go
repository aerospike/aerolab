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
aerolab cluster partition create -n name -l all --filters="" --partitions="20%,20%,20%,20%,20%"

# mkfs particular partitions, and mount
aerolab cluster partition mkfs   -n name -l all --filters="" --mount-root="/mnt/" --type=xfs --options="noatime"

# clear namespace storage type definitions from the conf file
aerolab cluster partition conf   -n name -l all --clear=storage
aerolab cluster partition conf   -n name -l all --clear=allflash

# add storage type device and setup the devices matching filters
aerolab cluster partition conf   -n name -l all --filters="" --namespace=test --device

# add shadow devices matching filters
aerolab cluster partition conf   -n name -l all --filters="" --namespace=test --shadow

# add allflash definition matching filters
aerolab cluster partition conf   -n name -l all --filters="" --namespace=test --allflash

--- filters ---
TODO
*/

package main

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
)

type clusterPartitionCreateCmd struct {
	ClusterName  TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes        TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks  TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterType   string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme|ebs" default:"ALL"`
	Partitions   string          `short:"p" long:"partitions" description:"partitions to create, size is in %% of total disk space; ex: 25,25,25,25; default: just remove all partitions"`
	NoBlkdiscard bool            `short:"b" long:"no-blkdiscard" description:"set to prevent aerolab from running blkdiscard on the disks and partitions"`
	Help         helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

type partition struct {
	start string
	end   string
}

func (c *clusterPartitionCreateCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	if a.opts.Config.Backend.Type == "docker" {
		return fmt.Errorf("partition creation and mkfs are not supported on docker backend")
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
	partitionsSpread := []partition{}
	start := 0
	for _, spreadInt := range partitions {
		end := start + spreadInt
		if end > 100 {
			return errors.New("partition layout would exceed 100%")
		}
		partitionsSpread = append(partitionsSpread, partition{
			start: strconv.Itoa(start),
			end:   strconv.Itoa(end),
		})
		start = start + spreadInt
	}
	a.opts.Cluster.Partition.List.ClusterName = c.ClusterName
	a.opts.Cluster.Partition.List.Nodes = c.Nodes
	a.opts.Cluster.Partition.List.FilterDisks = c.FilterDisks
	a.opts.Cluster.Partition.List.FilterType = c.FilterType
	a.opts.Cluster.Partition.List.FilterPartitions = "ALL"
	d, err := a.opts.Cluster.Partition.List.run(false)
	if err != nil {
		return err
	}
	if len(d) == 0 {
		return errors.New("no matching disks found")
	}
	filterDiskCount := 0
	if c.FilterDisks != "ALL" {
		filterDisks, _ := c.FilterDisks.Expand()
		filterDiskCount = len(filterDisks)
	}
	nodes := []int{}
	for nodeNo, disks := range d {
		script := makePartCommand()
		diskCount := 0
		for _, part := range disks {
			if _, ok := part[0]; !ok {
				continue
			}
			diskCount++
			for _, p := range part {
				if p.MountPoint != "" {
					script.Add("umount -f " + p.Path + " || echo 'not mounted'")
					script.Add("set +e")
					script.Add("RET=0; while [ $RET -eq 0 ]; do mount |egrep '^" + p.Path + "( |\\t)'; RET=$?; sleep 1; done")
					script.Add("set -e")
				}
				script.Add("sed -i.bak -e 's~" + p.Path + ".*~~g' /etc/fstab || echo 'not mounted'")
			}
			if len(part) > 1 {
				script.Add("sleep 1; parted -s " + part[0].Path + " 'mktable gpt'")
			}
			if !c.NoBlkdiscard {
				script.Add(fmt.Sprintf("blkdiscard %s || echo 'blkdiscard not supported'", part[0].Path))
			}
			if c.Partitions == "" {
				if !c.NoBlkdiscard {
					script.Add(fmt.Sprintf("blkdiscard -z --length 8388608 %s", part[0].Path))
				}
			} else {
				script.Add("parted -s " + part[0].Path + " 'mktable gpt'")
				for _, p := range partitionsSpread {
					script.Add("parted -a optimal -s " + part[0].Path + fmt.Sprintf(" mkpart primary %s%% %s%%", p.start, p.end))
				}
				if !c.NoBlkdiscard {
					script.Add("sleep 1; lsblk " + part[0].Path + " -o NAME -l -n |tail -n+2 |while read p; do wipefs -a /dev/$p; blkdiscard -z --length 8388608 /dev/$p; done")
				}
			}
			script.Add("grep \"\\S\" /etc/fstab > /etc/fstab.clean; mv /etc/fstab.clean /etc/fstab")
		}
		if c.FilterDisks != "ALL" && diskCount < filterDiskCount {
			return fmt.Errorf("could not find all the required disks on node %d", nodeNo)
		}
		nodes = append(nodes, nodeNo)
		err := b.CopyFilesToCluster(c.ClusterName.String(), []fileList{{"/opt/partition.disks.sh", strings.NewReader(script.String()), script.Len()}}, []int{nodeNo})
		if err != nil {
			return err
		}
	}
	sort.Ints(nodes)
	out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"/bin/bash", "/opt/partition.disks.sh"}}, nodes)
	if err != nil {
		nout := ""
		for _, o := range out {
			nout = nout + "\n" + string(o)
		}
		return fmt.Errorf("%s: %s", err, nout)
	}
	log.Print("Done")
	return nil
}

type partcommand string

func makePartCommand() partcommand {
	return "#!/bin/bash\nset -x\nset -e\n"
}

func (c *partcommand) Add(new string) {
	*c = *c + "\n" + partcommand(new)
}

func (c *partcommand) String() string {
	return string(*c)
}

func (c *partcommand) Len() int {
	return len(*c)
}

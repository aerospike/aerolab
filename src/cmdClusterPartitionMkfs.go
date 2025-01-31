package main

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

type clusterPartitionMkfsCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes            TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks      TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterPartitions TypeFilterRange `short:"p" long:"filter-partitions" description:"Select partitions on each disk by number, ex: 1,2,4-8" default:"ALL"`
	FilterType       string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme/local or ebs/persistent" default:"ALL"`
	FsType           string          `short:"f" long:"fs-type" description:"type of filesystem, ex: xfs" default:"xfs"`
	MkfsOpts         string          `short:"s" long:"fs-options" description:"filesystem mkfs options" default:""`
	MountRoot        string          `short:"r" long:"mount-root" description:"path to where all the mounts will be created" default:"/mnt/"`
	MountOpts        string          `short:"o" long:"mount-options" description:"additional mount options to pass, ex: noatime,noexec" default:""`
	parallelThreadsLongCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterPartitionMkfsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}

	if a.opts.Config.Backend.Type == "docker" {
		return fmt.Errorf("partition creation and mkfs are not supported on docker backend")
	}
	if c.FilterType == "local" {
		c.FilterType = "nvme"
	} else if c.FilterType == "persistent" {
		c.FilterType = "ebs"
	}
	log.Print("Running cluster.partition.mkfs")
	if c.MountOpts == "" {
		c.MountOpts = "defaults"
	} else {
		c.MountOpts = "defaults," + c.MountOpts
	}
	a.opts.Cluster.Partition.List.ClusterName = c.ClusterName
	a.opts.Cluster.Partition.List.Nodes = c.Nodes
	a.opts.Cluster.Partition.List.FilterDisks = c.FilterDisks
	a.opts.Cluster.Partition.List.FilterType = c.FilterType
	a.opts.Cluster.Partition.List.FilterPartitions = c.FilterPartitions
	a.opts.Cluster.Partition.List.ParallelThreads = c.ParallelThreads
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
	filterPartCount := 0
	if c.FilterPartitions != "ALL" {
		filterPartitions, _ := c.FilterPartitions.Expand()
		filterPartCount = len(filterPartitions)
	}
	nodes := []int{}
	parallel := make(chan int, c.ParallelThreads)
	hasError := make(chan bool, len(nodes))
	wait := new(sync.WaitGroup)
	for nodeNo, disks := range d {
		script := makePartCommand()
		diskCount := 0
		for _, part := range disks {
			if _, ok := part[0]; !ok {
				continue
			}
			diskCount++
			if c.FilterPartitions != "ALL" && len(part)-1 < filterPartCount {
				return fmt.Errorf("could not find all the required partitions on disk %d on node %d", part[0].diskNo, nodeNo)
			}
			if len(part) == 1 {
				return fmt.Errorf("did not find any partitions on disk %d on node %d", part[0].diskNo, nodeNo)
			}
			for pi, p := range part {
				if p.MountPoint != "" {
					script.Add("umount -f " + p.Path + " || echo 'not mounted'")
					script.Add("set +e")
					script.Add("RET=0; while [ $RET -eq 0 ]; do mount |egrep '^" + p.Path + "( |\\t)'; RET=$?; sleep 1; done")
					script.Add("set -e")
					script.Add("sed -i.bak -e 's~.*" + p.MountPoint + ".*~~g' /etc/fstab || echo 'not mounted'")
					script.Add("grep \"\\S\" /etc/fstab > /etc/fstab.clean; mv /etc/fstab.clean /etc/fstab")
					script.Add("rm -rf " + p.MountPoint)
				}
				if pi == 0 {
					continue
				}
				forceFlag := " -f "
				if strings.Contains(c.FsType, "ext") {
					forceFlag = " -F "
				}

				// Obtain the PARTUUID for the partition
				script.Add("PARTUUID=$(blkid -s PARTUUID -o value " + p.Path + ")")
				script.Add("if [ -z \"$PARTUUID\" ]; then")
				script.Add("    echo 'Failed to get PARTUUID for " + p.Path + "'")
				script.Add("    exit 1")
				script.Add("fi")

				script.Add("mkfs -t " + c.FsType + forceFlag + c.MkfsOpts + " " + p.Path)

				mountRoot := strings.TrimRight(c.MountRoot, "/") + "/"
				script.Add("mkdir -p " + mountRoot + `"$PARTUUID"`)
				script.Add(fmt.Sprintf("echo \"PARTUUID=$PARTUUID %s$PARTUUID %s %s 0 9\" >> /etc/fstab", mountRoot, c.FsType, c.MountOpts))
				script.Add("mount -a")
			}
		}
		if c.FilterDisks != "ALL" && diskCount < filterDiskCount {
			return fmt.Errorf("could not find all the required disks on node %d", nodeNo)
		}
		nodes = append(nodes, nodeNo)
		if c.ParallelThreads == 1 {
			err := b.CopyFilesToCluster(c.ClusterName.String(), []fileList{{"/opt/mkfs.disks.sh", script.String(), script.Len()}}, []int{nodeNo})
			if err != nil {
				return err
			}
		} else {
			parallel <- 1
			wait.Add(1)
			go func(script partcommand, nodeNo int, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
				defer func() {
					<-parallel
					wait.Done()
				}()
				err := b.CopyFilesToCluster(c.ClusterName.String(), []fileList{{"/opt/mkfs.disks.sh", script.String(), script.Len()}}, []int{nodeNo})
				if err != nil {
					fmt.Printf("Node %d error: %s", nodeNo, err)
					hasError <- true
				}
			}(script, nodeNo, parallel, wait, hasError)
		}
	}
	wait.Wait()
	if len(hasError) > 0 {
		return fmt.Errorf("failed from %d nodes", len(hasError))
	}
	sort.Ints(nodes)

	if c.ParallelThreads == 1 {
		out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"/bin/bash", "/opt/mkfs.disks.sh"}}, nodes)
		if err != nil {
			nout := ""
			for _, o := range out {
				nout = nout + "\n" + string(o)
			}
			return fmt.Errorf("%s: %s", err, nout)
		}
	} else {
		parallel = make(chan int, c.ParallelThreads)
		hasError = make(chan bool, len(nodes))
		wait = new(sync.WaitGroup)
		for _, node := range nodes {
			parallel <- 1
			wait.Add(1)
			go func(nodeNo int, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
				defer func() {
					<-parallel
					wait.Done()
				}()
				out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"/bin/bash", "/opt/mkfs.disks.sh"}}, []int{nodeNo})
				if err != nil {
					nout := ""
					for _, o := range out {
						nout = nout + "\n" + string(o)
					}
					log.Printf("Node %d: %s: %s", nodeNo, err, nout)
					hasError <- true
				}
			}(node, parallel, wait, hasError)

		}
		wait.Wait()
		if len(hasError) > 0 {
			return fmt.Errorf("failed from %d nodes", len(hasError))
		}
	}
	log.Print("Done")
	return nil
}

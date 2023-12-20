package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	aeroconf "github.com/rglonek/aerospike-config-file-parser"

	"github.com/bestmethod/inslice"
)

type clusterPartitionConfCmd struct {
	ClusterName        TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes              TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks        TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterPartitions   TypeFilterRange `short:"p" long:"filter-partitions" description:"Select partitions on each disk by number; 0=use entire disk itself, ex: 1,2,4-8" default:"ALL"`
	FilterType         string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme/local or ebs/persistent" default:"ALL"`
	Namespace          string          `short:"m" long:"namespace" description:"namespace to modify the settings for" default:""`
	ConfDest           string          `short:"o" long:"configure" description:"what to configure the selections as; options: memory|device|shadow|pi-flash|si-flash" default:""`
	MountsSizeLimitPct float64         `short:"s" long:"mounts-size-limit-pct" description:"specify %% for what the mounts-size-limit should be set to for flash configs" default:"90"`
	parallelThreadsLongCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterPartitionConfCmd) Execute(args []string) error {
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
	if !inslice.HasString([]string{"memory", "device", "shadow", "pi-flash", "si-flash", "allflash"}, c.ConfDest) {
		return fmt.Errorf("configure options must be one of: memory, device, shadow, pi-flash, si-flash, allflash")
	}
	if c.Namespace == "" {
		return fmt.Errorf("namespace name is required")
	}

	log.Print("Running cluster.partition.conf")
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

	if c.ParallelThreads == 1 {
		for nodeNo, disks := range d {
			err := c.do(nodeNo, disks, filterDiskCount, filterPartCount)
			if err != nil {
				return err
			}
		}
	} else {
		parallel := make(chan int, c.ParallelThreads)
		hasError := make(chan bool, len(d))
		wait := new(sync.WaitGroup)
		for nodeNo, disks := range d {
			parallel <- 1
			wait.Add(1)
			go c.doParallel(nodeNo, disks, filterDiskCount, filterPartCount, parallel, wait, hasError)
		}
		wait.Wait()
		if len(hasError) > 0 {
			return fmt.Errorf("failed to get logs from %d nodes", len(hasError))
		}
	}
	log.Print("Done")
	return nil
}

func (c *clusterPartitionConfCmd) doParallel(nodeNo int, disks map[int]map[int]blockDevices, filterDiskCount int, filterPartCount int, parallel chan int, wait *sync.WaitGroup, hasError chan bool) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	err := c.do(nodeNo, disks, filterDiskCount, filterPartCount)
	if err != nil {
		log.Printf("ERROR from node %d: %s", nodeNo, err)
		hasError <- true
	}
}

func (c *clusterPartitionConfCmd) do(nodeNo int, disks map[int]map[int]blockDevices, filterDiskCount int, filterPartCount int) error {
	is7 := "mounts-budget"
	outn, err := b.RunCommands(c.ClusterName.String(), [][]string{{"cat", "/opt/aerolab.aerospike.version"}}, []int{nodeNo})
	if err != nil {
		log.Printf("could not read /opt/aerolab.aerospike.version on node %d, assuming asd version 7: %s", nodeNo, err)
	} else {
		major := strings.Trim(strings.Split(string(outn[0]), ".")[0], "\r\n\t ")
		majorInt, err := strconv.Atoi(major)
		if err != nil {
			log.Printf("could not parse /opt/aerolab.aerospike.version on node %d, assuming asd version 7: %s", nodeNo, err)
		} else {
			if majorInt < 7 {
				is7 = "mounts-size-limit"
			}
		}
	}
	outn, err = b.RunCommands(c.ClusterName.String(), [][]string{{"cat", "/etc/aerospike/aerospike.conf"}}, []int{nodeNo})
	if err != nil {
		return fmt.Errorf("could not read aerospike.conf on node %d: %s", nodeNo, err)
	}
	aconf := outn[0]
	cc, err := aeroconf.Parse(bytes.NewReader(aconf))
	if err != nil {
		return fmt.Errorf("could not parse aerospike.conf on node %d: %s", nodeNo, err)
	}
	if cc.Type("namespace "+c.Namespace) == aeroconf.ValueNil {
		cc.NewStanza("namespace " + c.Namespace)
	}
	switch c.ConfDest {
	case "memory":
		for _, key := range cc.Stanza("namespace " + c.Namespace).ListKeys() {
			if strings.HasPrefix(key, "storage-engine") && (!strings.HasSuffix(key, "memory") || cc.Stanza("namespace "+c.Namespace).Type("storage-engine memory") != aeroconf.ValueStanza) {
				cc.Stanza("namespace " + c.Namespace).Delete(key)
			}
		}
		if cc.Stanza("namespace "+c.Namespace).Type("storage-engine memory") == aeroconf.ValueNil {
			cc.Stanza("namespace " + c.Namespace).NewStanza("storage-engine memory")
		}
		cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine memory").Delete("device")
		cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine memory").Delete("file")
		cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine memory").Delete("filesize")
		cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine memory").Delete("data-size")
	case "device":
		for _, key := range cc.Stanza("namespace " + c.Namespace).ListKeys() {
			if strings.HasPrefix(key, "storage-engine") && (!strings.HasSuffix(key, "device") || cc.Stanza("namespace "+c.Namespace).Type("storage-engine device") != aeroconf.ValueStanza) {
				cc.Stanza("namespace " + c.Namespace).Delete(key)
			}
		}
		if cc.Stanza("namespace "+c.Namespace).Type("storage-engine device") == aeroconf.ValueNil {
			cc.Stanza("namespace " + c.Namespace).NewStanza("storage-engine device")
		}
		cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine device").Delete("device")
		cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine device").Delete("file")
		cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine device").Delete("filesize")
	case "shadow":
		if cc.Stanza("namespace "+c.Namespace).Type("storage-engine device") != aeroconf.ValueStanza && cc.Stanza("namespace "+c.Namespace).Type("storage-engine memory") != aeroconf.ValueStanza {
			return fmt.Errorf("storage-engine device|memory configuration not found for namespace")
		}
		if !inslice.HasString(cc.Stanza("namespace "+c.Namespace).Stanza("storage-engine device").ListKeys(), "device") && !inslice.HasString(cc.Stanza("namespace "+c.Namespace).Stanza("storage-engine memory").ListKeys(), "device") {
			return fmt.Errorf("no device configured for given namespace, cannot add shadow devices")
		}
	case "allflash":
		fallthrough
	case "pi-flash":
		if cc.Stanza("namespace "+c.Namespace).Type("index-type flash") == aeroconf.ValueStanza {
			cc.Stanza("namespace " + c.Namespace).Stanza("index-type flash").Delete("mount")
		}
		for _, key := range cc.Stanza("namespace " + c.Namespace).ListKeys() {
			if strings.HasPrefix(key, "index-type") && (!strings.HasSuffix(key, "flash") || cc.Stanza("namespace "+c.Namespace).Type("index-type flash") != aeroconf.ValueStanza) {
				cc.Stanza("namespace " + c.Namespace).Delete(key)
			}
		}
		if cc.Stanza("namespace "+c.Namespace).Type("index-type flash") == aeroconf.ValueNil {
			cc.Stanza("namespace " + c.Namespace).NewStanza("index-type flash")
		}
	case "si-flash":
		if cc.Stanza("namespace "+c.Namespace).Type("sindex-type flash") == aeroconf.ValueStanza {
			cc.Stanza("namespace " + c.Namespace).Stanza("sindex-type flash").Delete("mount")
		}
		for _, key := range cc.Stanza("namespace " + c.Namespace).ListKeys() {
			if strings.HasPrefix(key, "sindex-type") && (!strings.HasSuffix(key, "flash") || cc.Stanza("namespace "+c.Namespace).Type("sindex-type flash") != aeroconf.ValueStanza) {
				cc.Stanza("namespace " + c.Namespace).Delete(key)
			}
		}
		if cc.Stanza("namespace "+c.Namespace).Type("sindex-type flash") == aeroconf.ValueNil {
			cc.Stanza("namespace " + c.Namespace).NewStanza("sindex-type flash")
		}
	}
	diskCount := 0
	useParts := []blockDevices{}
	for _, parts := range disks {
		if _, ok := parts[0]; !ok {
			continue
		}
		diskCount++
		if c.FilterPartitions != "ALL" && c.FilterPartitions != "0" && len(parts)-1 < filterPartCount {
			return fmt.Errorf("could not find all the required partitions on disk %d on node %d", parts[0].diskNo, nodeNo)
		}
		for _, part := range parts {
			if c.FilterPartitions != "0" && part.partNo == 0 {
				continue
			}
			if part.MountPoint != "" && (c.ConfDest != "pi-flash" && c.ConfDest != "si-flash" && c.ConfDest != "allflash") {
				return fmt.Errorf("partition %d on disk %d on node %d has a filesystem, cannot use for device storage", part.partNo, part.diskNo, part.nodeNo)
			} else if part.MountPoint == "" && (c.ConfDest == "pi-flash" || c.ConfDest == "si-flash" || c.ConfDest == "allflash") {
				return fmt.Errorf("partition %d on disk %d on node %d does not have a filesystem, cannot use for all-flash storage", part.partNo, part.diskNo, part.nodeNo)
			}
			useParts = append(useParts, part)
		}
	}
	if c.FilterDisks != "ALL" && diskCount < filterDiskCount {
		return fmt.Errorf("could not find all the required disks on node %d", nodeNo)
	}
	sort.Slice(useParts, func(x, y int) bool {
		if useParts[x].diskNo < useParts[y].diskNo {
			return true
		} else if useParts[x].diskNo > useParts[y].diskNo {
			return false
		} else {
			return useParts[x].partNo < useParts[y].partNo
		}
	})
	for _, p := range useParts {
		switch c.ConfDest {
		case "memory":
			vals, err := cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine memory").GetValues("device")
			if err != nil {
				return err
			}
			pp := p.Path
			vals = append(vals, &pp)
			cc.Stanza("namespace "+c.Namespace).Stanza("storage-engine memory").SetValues("device", vals)
		case "device":
			vals, err := cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine device").GetValues("device")
			if err != nil {
				return err
			}
			pp := p.Path
			vals = append(vals, &pp)
			cc.Stanza("namespace "+c.Namespace).Stanza("storage-engine device").SetValues("device", vals)
		case "shadow":
			ntype := "memory"
			if cc.Stanza("namespace "+c.Namespace).Stanza("storage-engine device") != nil {
				ntype = "device"
			}
			vals, err := cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine " + ntype).GetValues("device")
			if err != nil {
				return err
			}
			found := false
			for valI, val := range vals {
				if len(strings.Split(*val, " ")) == 2 {
					continue
				}
				found = true
				newval := *val + " " + p.Path
				vals[valI] = &newval
				break
			}
			if !found {
				return errors.New("not enough primary devices for chosen shadow devices")
			}
			cc.Stanza("namespace "+c.Namespace).Stanza("storage-engine "+ntype).SetValues("device", vals)
		case "allflash":
			fallthrough
		case "pi-flash":
			vals, err := cc.Stanza("namespace " + c.Namespace).Stanza("index-type flash").GetValues("mount")
			if err != nil {
				return err
			}
			if p.MountPoint == "" {
				return fmt.Errorf("partition %d on disk %d on node %d does not have a mountpoint", p.partNo, p.diskNo, p.nodeNo)
			}
			pp := p.MountPoint
			vals = append(vals, &pp)
			cc.Stanza("namespace "+c.Namespace).Stanza("index-type flash").SetValues("mount", vals)
			var fsSizeI int
			if strings.Contains(p.FsSize, ".") {
				var fsSizeF float64
				fsSizeF, err = strconv.ParseFloat(strings.TrimRight(p.FsSize, "GMBTgmbti"), 64)
				if err == nil {
					fsSizeI = int(math.Floor(fsSizeF))
				}
			} else {
				fsSizeI, err = strconv.Atoi(strings.TrimRight(p.FsSize, "GMBTgmbti"))
			}
			fsSize := p.FsSize
			if err == nil {
				suffix := ""
				if strings.HasSuffix(strings.ToUpper(p.FsSize), "G") {
					suffix = "G"
				} else if strings.HasSuffix(strings.ToUpper(p.FsSize), "M") {
					suffix = "M"
				} else if strings.HasSuffix(strings.ToUpper(p.FsSize), "K") {
					suffix = "K"
				} else if strings.HasSuffix(strings.ToUpper(p.FsSize), "T") {
					suffix = "T"
				}
				fsSize = strconv.Itoa(int(float64(fsSizeI)*c.MountsSizeLimitPct/100)) + suffix
			}
			cc.Stanza("namespace "+c.Namespace).Stanza("index-type flash").SetValue(is7, fsSize)
		case "si-flash":
			vals, err := cc.Stanza("namespace " + c.Namespace).Stanza("sindex-type flash").GetValues("mount")
			if err != nil {
				return err
			}
			if p.MountPoint == "" {
				return fmt.Errorf("partition %d on disk %d on node %d does not have a mountpoint", p.partNo, p.diskNo, p.nodeNo)
			}
			pp := p.MountPoint
			vals = append(vals, &pp)
			cc.Stanza("namespace "+c.Namespace).Stanza("sindex-type flash").SetValues("mount", vals)
			var fsSizeI int
			if strings.Contains(p.FsSize, ".") {
				var fsSizeF float64
				fsSizeF, err = strconv.ParseFloat(strings.TrimRight(p.FsSize, "GMBTgmbti"), 64)
				if err == nil {
					fsSizeI = int(math.Floor(fsSizeF))
				}
			} else {
				fsSizeI, err = strconv.Atoi(strings.TrimRight(p.FsSize, "GMBTgmbti"))
			}
			fsSize := p.FsSize
			if err == nil {
				suffix := ""
				if strings.HasSuffix(strings.ToUpper(p.FsSize), "G") {
					suffix = "G"
				} else if strings.HasSuffix(strings.ToUpper(p.FsSize), "M") {
					suffix = "M"
				} else if strings.HasSuffix(strings.ToUpper(p.FsSize), "K") {
					suffix = "K"
				} else if strings.HasSuffix(strings.ToUpper(p.FsSize), "T") {
					suffix = "T"
				}
				fsSize = strconv.Itoa(int(float64(fsSizeI)*c.MountsSizeLimitPct/100)) + suffix
			}
			cc.Stanza("namespace "+c.Namespace).Stanza("sindex-type flash").SetValue(is7, fsSize)
		}
	}
	if c.ConfDest == "shadow" {
		ntype := "memory"
		if cc.Stanza("namespace "+c.Namespace).Stanza("storage-engine device") != nil {
			ntype = "device"
		}
		vals, err := cc.Stanza("namespace " + c.Namespace).Stanza("storage-engine " + ntype).GetValues("device")
		if err != nil {
			return err
		}
		for _, val := range vals {
			if len(strings.Split(*val, " ")) != 2 {
				log.Print("WARNING: not all devices have a shadow device, not enough shadow devices")
				break
			}
		}
	}
	var buf bytes.Buffer
	err = cc.Write(&buf, "", "    ", true)
	if err != nil {
		return err
	}
	aconf = buf.Bytes()
	err = b.CopyFilesToCluster(c.ClusterName.String(), []fileList{{"/etc/aerospike/aerospike.conf", string(aconf), len(aconf)}}, []int{nodeNo})
	if err != nil {
		return err
	}
	return nil
}

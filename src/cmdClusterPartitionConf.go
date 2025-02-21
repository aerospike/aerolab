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
	"golang.org/x/text/language"
	"golang.org/x/text/message"

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
	MountsSizeLimitPct float64         `short:"s" long:"mounts-size-limit-pct" description:"specify %% space to use for configurating partition-tree-sprigs for pi-flash; this also sets mounts-budget accordingly" default:"90"`
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
	isv7 := true
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
				isv7 = false
			}
		}
	}

	outn, err = b.RunCommands(c.ClusterName.String(), [][]string{{"cat", "/etc/aerospike/aerospike.conf"}}, []int{nodeNo})
	if err != nil {
		return fmt.Errorf("could not read aerospike.conf on node %d: %s", nodeNo, err)
	}
	aconf := outn[0]
	conf, err := aeroconf.Parse(bytes.NewReader(aconf))
	if err != nil {
		return fmt.Errorf("could not parse aerospike.conf on node %d: %s", nodeNo, err)
	}

	if conf.Type("namespace "+c.Namespace) == aeroconf.ValueNil {
		err := conf.NewStanza("namespace " + c.Namespace)
		if err != nil {
			return logFatal("could not create namespace stanza %s: %v", c.Namespace, err)
		}
	}

	//  Create a namespace if it does not exist
	namespace := conf.Stanza("namespace " + c.Namespace)
	if namespace == nil {
		return logFatal("namespace " + c.Namespace + " does not exist")
	}

	// Clear devices and validate the existing configuration in the namespace
	switch c.ConfDest {
	case "memory":
		err := prepareNamespaceForMemoryStorage(&namespace)
		if err != nil {
			return err
		}
	case "device":
		err := prepareNamespaceForDeviceStorage(&namespace)
		if err != nil {
			return err
		}
	case "shadow":
		if namespace.Type("storage-engine device") != aeroconf.ValueStanza && namespace.Type("storage-engine memory") != aeroconf.ValueStanza {
			return fmt.Errorf("storage-engine device|memory configuration not found for namespace %s", c.Namespace)
		}
		if !inslice.HasString(namespace.Stanza("storage-engine device").ListKeys(), "device") && !inslice.HasString(namespace.Stanza("storage-engine memory").ListKeys(), "device") {
			return fmt.Errorf("no devices are configured for namespace %s, cannot add shadow devices", c.Namespace)
		}
	case "allflash", "pi-flash":
		err := prepareNamespaceForIndexFlash(&namespace, "index-type")
		if err != nil {
			return err
		}
	case "si-flash":
		err := prepareNamespaceForIndexFlash(&namespace, "sindex-type")
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown device configuration type %s", c.ConfDest)
	}

	// Validate disk and partition counts, gather devices for configuration
	diskCount := 0
	var useDevices []blockDevices
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
			useDevices = append(useDevices, part)
		}
	}
	if c.FilterDisks != "ALL" && diskCount < filterDiskCount {
		return fmt.Errorf("could not find all the required disks on node %d", nodeNo)
	}
	sort.Slice(useDevices, func(x, y int) bool {
		if useDevices[x].diskNo < useDevices[y].diskNo {
			return true
		} else if useDevices[x].diskNo > useDevices[y].diskNo {
			return false
		} else {
			return useDevices[x].partNo < useDevices[y].partNo
		}
	})

	assignFunc := func(vals []*string, device blockDevices) ([]*string, error) {
		if device.PartUUIDPath == "" {
			log.Printf("WARNING: the device %s does not have a partition UUID, falling back on the device path\n", device.Name)
			log.Println("   HINT: this path can change after a node restart")
			return append(vals, &device.Path), nil
		}
		return append(vals, &device.PartUUIDPath), nil
	}

	// Assign devices
	totalFsSizeBytes := 0
	for _, device := range useDevices {
		switch c.ConfDest {
		case "memory", "device":
			err = assignStorageEngineDevice(&namespace, c.ConfDest, device, assignFunc)
			if err != nil {
				return err
			}
		case "shadow":
			ntype := "memory"
			if namespace.Stanza("storage-engine device") != nil {
				ntype = "device"
			}

			err = assignStorageEngineDevice(&namespace, ntype, device, func(vals []*string, device blockDevices) ([]*string, error) {
				path := device.PartUUIDPath
				if path == "" {
					log.Printf("WARNING: the device %s does not have a partition UUID, falling back on the device path\n", device.Name)
					log.Println("   HINT: this path can change after a node restart")
					path = device.Path
				}

				found := false
				for valI, val := range vals {
					if len(strings.Split(*val, " ")) == 2 {
						continue
					}
					found = true
					newval := fmt.Sprintf("%s %s", *val, path)
					vals[valI] = &newval
					break
				}
				if !found {
					return nil, errors.New("not enough primary devices for chosen shadow devices")
				}

				return vals, nil
			})
			if err != nil {
				return err
			}
		case "allflash", "pi-flash":
			err = assignFlashIndexDevice(&namespace, "index-type", device, isv7, c.MountsSizeLimitPct, c.ClusterName.String(), nodeNo, &totalFsSizeBytes)
			if err != nil {
				return err
			}
		case "si-flash":
			err = assignFlashIndexDevice(&namespace, "sindex-type", device, isv7, c.MountsSizeLimitPct, c.ClusterName.String(), nodeNo, &totalFsSizeBytes)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown device configuration type %s", c.ConfDest)
		}
	}

	// Validate and finalize configuration
	if c.ConfDest == "shadow" {
		ntype := "memory"
		if namespace.Stanza("storage-engine device") != nil {
			ntype = "device"
		}
		vals, err := namespace.Stanza("storage-engine " + ntype).GetValues("device")
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

	if (c.ConfDest == "allflash" || c.ConfDest == "pi-flash") && totalFsSizeBytes > 0 {
		treeSprigs := "256"
		rf := "2"
		if namespace.Type("partition-tree-sprigs") != aeroconf.ValueNil {
			sprigsconf, err := namespace.GetValues("partition-tree-sprigs")
			if err == nil && len(sprigsconf) > 0 {
				treeSprigs = *sprigsconf[0]
			}
		}
		if namespace.Type("replication-factor") != aeroconf.ValueNil {
			rfconf, err := namespace.GetValues("replication-factor")
			if err == nil && len(rfconf) > 0 {
				rf = *rfconf[0]
			}
		}
		rfInt, _ := strconv.Atoi(rf)
		treeSprigsInt, _ := strconv.Atoi(treeSprigs)
		spaceRequired := 4096 * rfInt * 4096 * treeSprigsInt
		maxUsableBytes := int(float64(totalFsSizeBytes) * c.MountsSizeLimitPct / 100)
		maxSprigs := NextPowOf2(maxUsableBytes / 4096 / rfInt / 4096)
		maxRecords := maxSprigs * 4096 * 64
		p := message.NewPrinter(language.English)
		maxRecordsStr := p.Sprintf("%d", maxRecords)
		if spaceRequired > maxUsableBytes {
			log.Printf("WARNING: node=%d space required for partition-tree-sprigs (%s) exceeds the pi-flash partition_size*%d%% (%s). Changing from %d to %d. At %s master records the fill factor will be 1.", nodeNo, convSize(int64(spaceRequired)), int(c.MountsSizeLimitPct), convSize(int64(maxUsableBytes)), treeSprigsInt, maxSprigs, maxRecordsStr)
			namespace.SetValue("partition-tree-sprigs", strconv.Itoa(maxSprigs))
		} else if maxSprigs > treeSprigsInt {
			log.Printf("WARNING: node=%d partition-tree-sprigs seems to be low for the amount of partition space configured (%s). Changing from %d to %d. At %s master records the fill factor will be 1.", nodeNo, convSize(int64(maxUsableBytes)), treeSprigsInt, maxSprigs, maxRecordsStr)
			namespace.SetValue("partition-tree-sprigs", strconv.Itoa(maxSprigs))
		}
	}

	// Write out the new config file
	var buf bytes.Buffer
	err = conf.Write(&buf, "", "    ", true)
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

func assignStorageEngineDevice(namespace *aeroconf.Stanza, deviceType string, device blockDevices, assign func([]*string, blockDevices) ([]*string, error)) error {
	if namespace == nil {
		return fmt.Errorf("namespace stanza is nil")
	}

	deviceStanza := namespace.Stanza("storage-engine " + deviceType)
	if deviceStanza == nil {
		return fmt.Errorf("namespace does not contain storage-engine %s stanza", deviceType)
	}

	vals, err := deviceStanza.GetValues("device")
	if err != nil {
		return err
	}

	vals, err = assign(vals, device)
	if err != nil {
		return err
	}

	return deviceStanza.SetValues("device", vals)
}

func assignFlashIndexDevice(namespace *aeroconf.Stanza, indexType string, device blockDevices, is7 bool, mountsSizeLimitPct float64, clusterName string, nodeNo int, totalFsSizeBytes *int) error {
	if namespace == nil {
		return fmt.Errorf("namespace stanza is nil")
	}

	indexStanza := namespace.Stanza(indexType + " flash")
	if indexStanza == nil {
		return fmt.Errorf("namespace does not contain %s flash stanza", indexType)
	}

	vals, err := indexStanza.GetValues("mount")
	if err != nil {
		return err
	}

	if device.MountPoint == "" {
		return fmt.Errorf("partition %d on disk %d on node %d does not have a mountpoint", device.partNo, device.diskNo, device.nodeNo)
	}

	pp := device.MountPoint
	vals = append(vals, &pp)
	err = indexStanza.SetValues("mount", vals)
	if err != nil {
		return err
	}

	var fsSizeI int
	if strings.Contains(device.FsSize, ".") {
		var fsSizeF float64
		fsSizeF, err = strconv.ParseFloat(strings.TrimRight(device.FsSize, "GMBTgmbti"), 64)
		if err == nil {
			fsSizeI = int(math.Floor(fsSizeF))
		}
	} else {
		fsSizeI, err = strconv.Atoi(strings.TrimRight(device.FsSize, "GMBTgmbti"))
	}
	if err == nil {
		if strings.HasSuffix(strings.ToUpper(device.FsSize), "T") {
			*totalFsSizeBytes += fsSizeI * 1024 * 1024 * 1024 * 1024
		} else if strings.HasSuffix(strings.ToUpper(device.FsSize), "G") {
			*totalFsSizeBytes += fsSizeI * 1024 * 1024 * 1024
		} else if strings.HasSuffix(strings.ToUpper(device.FsSize), "M") {
			*totalFsSizeBytes += fsSizeI * 1024 * 1024
		} else if strings.HasSuffix(strings.ToUpper(device.FsSize), "K") {
			*totalFsSizeBytes += fsSizeI * 1024
		} else {
			*totalFsSizeBytes += fsSizeI
		}
		// Need to get the device size from the node directly
	} else {
		outn, err := b.RunCommands(clusterName, [][]string{{"blockdev", "--getsize64", device.Path}}, []int{nodeNo})
		if err == nil {
			if len(outn) != 1 {
				err = fmt.Errorf("expected out string from one command, got %d", len(outn))
			} else {
				fsSize := strings.Trim(string(outn[0]), "\r\t\n")
				fsSizeI, err = strconv.Atoi(fsSize)
			}
		}
		if err != nil {
			return fmt.Errorf("could not determine the size of %s on node %d: %v", device.Path, nodeNo, err)
		}
		*totalFsSizeBytes += fsSizeI
	}

	maxUsableBytes := int(float64(*totalFsSizeBytes) * mountsSizeLimitPct / 100)
	fsSize := strconv.Itoa(maxUsableBytes/1024) + "K"

	// This configuration setting has changed as of v7.x to mounts-budget
	mountsConfig := "mounts-size-limit"
	if is7 {
		mountsConfig = "mounts-budget"
	}

	return indexStanza.SetValue(mountsConfig, fsSize)
}

func prepareNamespaceForMemoryStorage(namespace *aeroconf.Stanza) error {
	if namespace == nil {
		return fmt.Errorf("namespace stanza is nil")
	}

	for _, key := range namespace.ListKeys() {
		if strings.HasPrefix(key, "storage-engine") && !strings.HasSuffix(key, "memory") {
			_ = namespace.Delete(key)
		}
	}

	if namespace.Type("storage-engine memory") == aeroconf.ValueNil {
		err := namespace.NewStanza("storage-engine memory")
		if err != nil {
			return fmt.Errorf("could not create stanza for memory storage: %v", err)
		}
	}

	memoryStanza := namespace.Stanza("storage-engine memory")
	_ = memoryStanza.Delete("device")
	_ = memoryStanza.Delete("file")
	_ = memoryStanza.Delete("filesize")
	_ = memoryStanza.Delete("data-size")

	return nil
}

func prepareNamespaceForDeviceStorage(namespace *aeroconf.Stanza) error {
	if namespace == nil {
		return fmt.Errorf("namespace stanza is nil")
	}

	for _, key := range namespace.ListKeys() {
		if strings.HasPrefix(key, "storage-engine") && (!strings.HasSuffix(key, "device") || namespace.Type("storage-engine device") != aeroconf.ValueStanza) {
			_ = namespace.Delete(key)
		}
	}

	if namespace.Type("storage-engine device") == aeroconf.ValueNil {
		err := namespace.NewStanza("storage-engine device")
		if err != nil {
			return fmt.Errorf("could not create stanza for device storage: %v", err)
		}
	}

	deviceStanza := namespace.Stanza("storage-engine device")
	_ = deviceStanza.Delete("device")
	_ = deviceStanza.Delete("file")
	_ = deviceStanza.Delete("filesize")

	return nil
}

func prepareNamespaceForIndexFlash(namespace *aeroconf.Stanza, indexTypePrefix string) error {
	if namespace == nil {
		return fmt.Errorf("namespace stanza is nil")
	}

	for _, key := range namespace.ListKeys() {
		if strings.HasPrefix(key, indexTypePrefix) && (!strings.HasSuffix(key, "flash") || namespace.Type(indexTypePrefix+" flash") != aeroconf.ValueStanza) {
			_ = namespace.Delete(key)
		}
	}

	if namespace.Type(indexTypePrefix+" flash") == aeroconf.ValueNil {
		err := namespace.NewStanza(indexTypePrefix + " flash")
		if err != nil {
			return err
		}
	}

	indexStanza := namespace.Stanza(indexTypePrefix + " flash")
	_ = indexStanza.Delete("mount")

	return nil
}

func NextPowOf2(n int) int {
	k := 1
	l := 1
	for k < n {
		l = k
		k = k << 1
	}
	return l
}

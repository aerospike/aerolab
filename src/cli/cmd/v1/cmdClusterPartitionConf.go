package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/bestmethod/inslice"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
	"github.com/rglonek/logger"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type ClusterPartitionConfCmd struct {
	ClusterName        TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes              TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks        TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterPartitions   TypeFilterRange `short:"p" long:"filter-partitions" description:"Select partitions on each disk by number; 0=use entire disk itself, ex: 1,2,4-8" default:"ALL"`
	FilterType         string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme/local or ebs/persistent" default:"ALL"`
	Namespace          string          `short:"m" long:"namespace" description:"namespace to modify the settings for" default:""`
	ConfDest           string          `short:"o" long:"configure" description:"what to configure the selections as; options: memory|device|shadow|pi-flash|si-flash" default:""`
	MountsSizeLimitPct float64         `short:"s" long:"mounts-size-limit-pct" description:"specify %% space to use for configurating partition-tree-sprigs for pi-flash; this also sets mounts-budget accordingly" default:"90"`
	ConfigPath         string          `short:"c" long:"config-path" description:"path to a custom aerospike config file to use for the configuration" default:"/etc/aerospike/aerospike.conf"`
	ParallelThreads    int             `long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help               HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterPartitionConfCmd) Execute(args []string) error {
	cmd := []string{"cluster", "partition", "conf"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	var stdout, stderr *os.File
	var stdin io.ReadCloser
	if system.logLevel >= 5 {
		stdout = os.Stdout
		stderr = os.Stderr
		stdin = io.NopCloser(os.Stdin)
	}
	_, err = c.PartitionConfCluster(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterPartitionConfCmd) PartitionConfCluster(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) (output []*backends.ExecOutput, err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "partition", "conf"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	if c.ClusterName.String() == "" {
		return nil, fmt.Errorf("cluster name is required")
	}
	if !slices.Contains([]string{"memory", "device", "shadow", "pi-flash", "si-flash", "allflash"}, c.ConfDest) {
		return nil, fmt.Errorf("configure options must be one of: memory, device, shadow, pi-flash, si-flash, allflash")
	}
	if c.Namespace == "" {
		return nil, fmt.Errorf("namespace name is required")
	}

	if c.FilterType == "local" {
		c.FilterType = "nvme"
	} else if c.FilterType == "persistent" {
		c.FilterType = "ebs"
	}
	filterDiskCount := 0
	var disksFilter []int
	if c.FilterDisks != "ALL" {
		disksFilter, err = c.FilterDisks.Expand()
		if err != nil {
			return nil, err
		}
		filterDiskCount = len(disksFilter)
	}
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		var output []*backends.ExecOutput
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			inst, err := c.PartitionConfCluster(system, inventory, args, stdin, stdout, stderr, logger)
			if err != nil {
				return nil, err
			}
			output = append(output, inst...)
		}
		return output, nil
	}
	cluster := inventory.Instances.WithClusterName(c.ClusterName.String())
	if cluster == nil {
		return nil, fmt.Errorf("cluster %s not found", c.ClusterName.String())
	}
	// Filter by Running state first, before checking node numbers
	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if c.Nodes.String() != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return nil, err
		}
		cluster = cluster.WithNodeNo(nodes...)
		if cluster.Count() != len(nodes) {
			return nil, fmt.Errorf("some nodes in %s not found (may be terminated)", c.Nodes.String())
		}
	}
	if cluster.Count() == 0 {
		logger.Info("No nodes to configure partitions on")
		return nil, nil
	}
	logger.Info("Configuring partitions on %d nodes", cluster.Count())
	var hasErr error
	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		nver := inst.Tags["aerolab.soft.version"]
		nverx := strings.Split(nver, ".")
		nvera, _ := strconv.Atoi(nverx[0])
		isv7 := false
		if nvera >= 7 {
			isv7 = true
		}
		// run partitionList on node
		plistCmd := &ClusterPartitionListCmd{
			ClusterName:      TypeClusterName(inst.ClusterName),
			Nodes:            TypeNodes(strconv.Itoa(inst.NodeNo)),
			FilterDisks:      c.FilterDisks,
			FilterPartitions: c.FilterPartitions,
			FilterType:       c.FilterType,
			ParallelThreads:  c.ParallelThreads,
		}
		plist, err := plistCmd.PartitionListClusterDo(system, inventory, args, logger)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		conf, err := inst.GetSftpConfig("root")
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		client, err := sshexec.NewSftp(conf)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		defer client.Close()
		var buf = &bytes.Buffer{}
		err = client.ReadFile(&sshexec.FileReader{
			SourcePath:  c.ConfigPath,
			Destination: buf,
		})
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}

		aconf, err := aeroconf.Parse(buf)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		if aconf.Type("namespace "+c.Namespace) == aeroconf.ValueNil {
			err := aconf.NewStanza("namespace " + c.Namespace)
			if err != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
				return
			}
		}
		namespace := aconf.Stanza("namespace " + c.Namespace)
		switch c.ConfDest {
		case "memory":
			err := prepareNamespaceForMemoryStorage(&namespace)
			if err != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
				return
			}
		case "device":
			err := prepareNamespaceForDeviceStorage(&namespace)
			if err != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
				return
			}
		case "shadow":
			if namespace.Type("storage-engine device") != aeroconf.ValueStanza && namespace.Type("storage-engine memory") != aeroconf.ValueStanza {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, fmt.Errorf("storage-engine device|memory configuration not found for namespace %s", c.Namespace)))
				return
			}
			if !inslice.HasString(namespace.Stanza("storage-engine device").ListKeys(), "device") && !inslice.HasString(namespace.Stanza("storage-engine memory").ListKeys(), "device") {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, fmt.Errorf("no devices are configured for namespace %s, cannot add shadow devices", c.Namespace)))
				return
			}
		case "allflash", "pi-flash":
			err := prepareNamespaceForIndexFlash(&namespace, "index-type")
			if err != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
				return
			}
		case "si-flash":
			err := prepareNamespaceForIndexFlash(&namespace, "sindex-type")
			if err != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
				return
			}
		default:
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, fmt.Errorf("unknown device configuration type %s", c.ConfDest)))
			return
		}

		// Validate disk and partition counts, gather devices for configuration
		diskCount := 0
		var useDevices []BlockDevice
		// Build a map of disk to partition info for quick lookup
		diskMap := make(map[int]*PartitionInformation)
		for _, parts := range plist {
			if parts.Partition == 0 {
				diskMap[parts.Disk] = parts
				diskCount++
			}
		}
		// Handle FilterPartitions == "0" or "" - use entire disk itself
		if c.FilterPartitions == "0" || c.FilterPartitions == "" {
			for _, parts := range plist {
				if parts.Partition == 0 {
					// Use the disk itself
					diskDevice := parts.BlockDevices
					// Validate disk properties
					if diskDevice.MountPoint != "" && (c.ConfDest != "pi-flash" && c.ConfDest != "si-flash" && c.ConfDest != "allflash") {
						hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, fmt.Errorf("disk %d on node %d has a filesystem, cannot use for device storage", parts.Disk, inst.NodeNo)))
						return
					} else if diskDevice.MountPoint == "" && (c.ConfDest == "pi-flash" || c.ConfDest == "si-flash" || c.ConfDest == "allflash") {
						hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, fmt.Errorf("disk %d on node %d does not have a filesystem, cannot use for all-flash storage", parts.Disk, inst.NodeNo)))
						return
					}
					useDevices = append(useDevices, diskDevice)
				}
			}
		} else {
			// Collect filtered partitions from plist (plist already contains filtered results)
			// When parts.Partition > 0, parts.BlockDevices already contains the partition device itself
			for _, parts := range plist {
				if parts.Partition == 0 {
					// This is a disk entry, skip it as we only process partitions
					continue
				}
				// parts.BlockDevices already contains the partition BlockDevice when Partition > 0
				partitionDevice := parts.BlockDevices
				// Validate partition properties
				if partitionDevice.MountPoint != "" && (c.ConfDest != "pi-flash" && c.ConfDest != "si-flash" && c.ConfDest != "allflash") {
					hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, fmt.Errorf("partition %d on disk %d on node %d has a filesystem, cannot use for device storage", parts.Partition, parts.Disk, inst.NodeNo)))
					return
				} else if partitionDevice.MountPoint == "" && (c.ConfDest == "pi-flash" || c.ConfDest == "si-flash" || c.ConfDest == "allflash") {
					hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, fmt.Errorf("partition %d on disk %d on node %d does not have a filesystem, cannot use for all-flash storage", parts.Partition, parts.Disk, inst.NodeNo)))
					return
				}
				useDevices = append(useDevices, partitionDevice)
			}
		}
		// Validate disk count
		if c.FilterDisks != "ALL" && diskCount < filterDiskCount {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, fmt.Errorf("could not find all the required disks on node %d", inst.NodeNo)))
			return
		}
		sort.Slice(useDevices, func(x, y int) bool {
			if useDevices[x].Name < useDevices[y].Name {
				return true
			} else if useDevices[x].Name > useDevices[y].Name {
				return false
			} else {
				return x < y
			}
		})

		assignFunc := func(vals []*string, device BlockDevice) ([]*string, error) {
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
					hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
					return
				}
			case "shadow":
				ntype := "memory"
				if namespace.Stanza("storage-engine device") != nil {
					ntype = "device"
				}

				err = assignStorageEngineDevice(&namespace, ntype, device, func(vals []*string, device BlockDevice) ([]*string, error) {
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
					hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
					return
				}
			case "allflash", "pi-flash":
				err = assignFlashIndexDevice(&namespace, "index-type", device, isv7, c.MountsSizeLimitPct, c.ClusterName.String(), inst, &totalFsSizeBytes)
				if err != nil {
					hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
					return
				}
			case "si-flash":
				err = assignFlashIndexDevice(&namespace, "sindex-type", device, isv7, c.MountsSizeLimitPct, c.ClusterName.String(), inst, &totalFsSizeBytes)
				if err != nil {
					hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
					return
				}
			default:
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, fmt.Errorf("unknown device configuration type %s", c.ConfDest)))
				return
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
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
				return
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
				log.Printf("WARNING: node=%d space required for partition-tree-sprigs (%s) exceeds the pi-flash partition_size*%d%% (%s). Changing from %d to %d. At %s master records the fill factor will be 1.", inst.NodeNo, convSize(int64(spaceRequired)), int(c.MountsSizeLimitPct), convSize(int64(maxUsableBytes)), treeSprigsInt, maxSprigs, maxRecordsStr)
				namespace.SetValue("partition-tree-sprigs", strconv.Itoa(maxSprigs))
			} else if maxSprigs > treeSprigsInt {
				log.Printf("WARNING: node=%d partition-tree-sprigs seems to be low for the amount of partition space configured (%s). Changing from %d to %d. At %s master records the fill factor will be 1.", inst.NodeNo, convSize(int64(maxUsableBytes)), treeSprigsInt, maxSprigs, maxRecordsStr)
				namespace.SetValue("partition-tree-sprigs", strconv.Itoa(maxSprigs))
			}
		}
		buf = &bytes.Buffer{}
		aconf.Write(buf, "", "    ", true)
		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath:    c.ConfigPath,
			Source:      buf,
			Permissions: 0644,
		})
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
	})
	if hasErr != nil {
		return nil, hasErr
	}
	return nil, nil
}

func assignStorageEngineDevice(namespace *aeroconf.Stanza, deviceType string, device BlockDevice, assign func([]*string, BlockDevice) ([]*string, error)) error {
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

func assignFlashIndexDevice(namespace *aeroconf.Stanza, indexType string, device BlockDevice, is7 bool, mountsSizeLimitPct float64, clusterName string, inst *backends.Instance, totalFsSizeBytes *int) error {
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
		return fmt.Errorf("partition %s on disk %s on node %d does not have a mountpoint", device.Name, device.PartUUID, inst.NodeNo)
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
		eout := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"blockdev", "--getsize64", device.Path},
				Stdin:          nil,
				Stdout:         nil,
				Stderr:         nil,
				SessionTimeout: 0,
				Env:            []*sshexec.Env{},
				Terminal:       true,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if eout.Output.Err == nil {
			if len(eout.Output.Stdout) != 1 {
				err = fmt.Errorf("expected out string from one command, got %d", len(eout.Output.Stdout))
			} else {
				fsSize := strings.Trim(string(eout.Output.Stdout[0]), "\r\t\n")
				fsSizeI, err = strconv.Atoi(fsSize)
			}
		}
		if err != nil {
			return fmt.Errorf("could not determine the size of %s on node %d: %v", device.Path, inst.NodeNo, err)
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

func convSize(size int64) string {
	var sizeString string
	if size > 1023 && size < 1024*1024 {
		sizeString = fmt.Sprintf("%.2f KB", float64(size)/1024)
	} else if size < 1024 {
		sizeString = fmt.Sprintf("%v B", size)
	} else if size >= 1024*1024 && size < 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f MB", float64(size)/1024/1024)
	} else if size >= 1024*1024*1024 {
		sizeString = fmt.Sprintf("%.2f GB", float64(size)/1024/1024/1024)
	}
	return sizeString
}

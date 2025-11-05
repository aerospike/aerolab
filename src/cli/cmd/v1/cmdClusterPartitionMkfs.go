package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/logger"
)

type ClusterPartitionMkfsCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes            TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks      TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterPartitions TypeFilterRange `short:"p" long:"filter-partitions" description:"Select partitions on each disk by number, ex: 1,2,4-8" default:"ALL"`
	FilterType       string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme/local or ebs/persistent" default:"ALL"`
	FsType           string          `short:"f" long:"fs-type" description:"type of filesystem, ex: xfs" default:"xfs"`
	MkfsOpts         string          `short:"s" long:"fs-options" description:"filesystem mkfs options" default:""`
	MountRoot        string          `short:"r" long:"mount-root" description:"path to where all the mounts will be created" default:"/mnt/"`
	MountOpts        string          `short:"o" long:"mount-options" description:"additional mount options to pass, ex: noatime,noexec" default:""`
	ParallelThreads  int             `long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help             HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterPartitionMkfsCmd) Execute(args []string) error {
	cmd := []string{"cluster", "partition", "mkfs"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	var stdout, stderr *io.Writer
	var stdoutp, stderrp io.Writer
	var stdin *io.ReadCloser
	if system.logLevel >= 5 {
		stdoutp = os.Stdout
		stdout = &stdoutp
		stderrp = os.Stderr
		stderr = &stderrp
		stdinp := io.NopCloser(os.Stdin)
		stdin = &stdinp
	}
	_, err = c.PartitionMkfsCluster(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterPartitionMkfsCmd) PartitionMkfsCluster(system *System, inventory *backends.Inventory, args []string, stdin *io.ReadCloser, stdout *io.Writer, stderr *io.Writer, logger *logger.Logger) (output []*backends.ExecOutput, err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "partition", "mkfs"}, c, args...)
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
			inst, err := c.PartitionMkfsCluster(system, inventory, args, stdin, stdout, stderr, logger)
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
		logger.Info("No nodes to run partition mkfs")
		return nil, nil
	}
	logger.Info("Running partition mkfs to %d nodes", cluster.Count())
	var hasErr error
	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
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
		// calculate what we need to do and build a script into installScript
		script := makePartCommand()
		diskCount := 0
		// Build a map of disk to partition info for quick lookup
		diskMap := make(map[int]*PartitionInformation)
		for _, parts := range plist {
			if parts.Partition == 0 {
				diskMap[parts.Disk] = parts
				diskCount++
			}
		}

		processPartition := func(p *BlockDevice) {
			if p.MountPoint != "" {
				script.Add("umount -f " + p.Path + " || echo 'not mounted'")
				script.Add("set +e")
				script.Add("RET=0; while [ $RET -eq 0 ]; do mount |egrep '^" + p.Path + "( |\\t)'; RET=$?; sleep 1; done")
				script.Add("set -e")
				script.Add("sed -i.bak -e 's~.*" + p.MountPoint + ".*~~g' /etc/fstab || echo 'not mounted'")
				script.Add("rm -rf " + p.MountPoint)
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
			mountOpts := c.MountOpts
			if mountOpts == "" {
				mountOpts = "defaults"
			}
			script.Add("mkdir -p " + mountRoot + `"$PARTUUID"`)
			script.Add(fmt.Sprintf("echo \"PARTUUID=$PARTUUID %s$PARTUUID %s %s 0 9\" >> /etc/fstab", mountRoot, c.FsType, mountOpts))
		}

		// Handle FilterPartitions == "0" or "" - use entire disk itself
		if c.FilterPartitions == "0" || c.FilterPartitions == "" {
			for _, parts := range plist {
				if parts.Partition == 0 {
					// Use the disk itself
					processPartition(&parts.BlockDevices)
				}
			}
		} else {
			// Process only the filtered partitions from plist (plist already contains filtered results)
			// When parts.Partition > 0, parts.BlockDevices already contains the partition device itself
			for _, parts := range plist {
				if parts.Partition == 0 {
					// This is a disk entry, skip it as we only process partitions
					continue
				}
				// parts.BlockDevices already contains the partition BlockDevice when Partition > 0
				processPartition(&parts.BlockDevices)
			}
		}
		if c.FilterDisks != "ALL" && diskCount < filterDiskCount {
			hasErr = errors.Join(hasErr, fmt.Errorf("could not find all the required disks on node %d", inst.NodeNo))
			return
		}
		// Mount all partitions after all fstab entries have been added
		// Use set +e to allow mount -a to fail without stopping the script
		// (mount -a can return non-zero if other fstab entries fail, even if ours succeed)
		script.Add("mount -a")
		installScript := script.String()
		// execute install script
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
		now := time.Now().Format("20060102150405")
		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/tmp/install.sh." + now,
			Source:      strings.NewReader(string(installScript)),
			Permissions: 0755,
		})
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		detail := sshexec.ExecDetail{
			Command:        []string{"bash", "/tmp/install.sh." + now},
			SessionTimeout: 5 * time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		}
		if stdin != nil {
			detail.Stdin = *stdin
		}
		if stdout != nil {
			detail.Stdout = *stdout
		}
		if stderr != nil {
			detail.Stderr = *stderr
		}
		output := inst.Exec(&backends.ExecInput{
			ExecDetail:      detail,
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: c.ParallelThreads,
		})
		if output.Output.Err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s (%s) (%s)", inst.ClusterName, inst.NodeNo, output.Output.Err, string(output.Output.Stdout), string(output.Output.Stderr)))
		}
	})
	if hasErr != nil {
		return nil, hasErr
	}
	return nil, nil
}

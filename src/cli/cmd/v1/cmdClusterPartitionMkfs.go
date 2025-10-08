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

	var stdout, stderr *os.File
	var stdin io.ReadCloser
	if system.logLevel >= 5 {
		stdout = os.Stdout
		stderr = os.Stderr
		stdin = io.NopCloser(os.Stdin)
	}
	_, err = c.PartitionMkfsCluster(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterPartitionMkfsCmd) PartitionMkfsCluster(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) (output []*backends.ExecOutput, err error) {
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
	if c.Nodes.String() != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return nil, err
		}
		cluster = cluster.WithNodeNo(nodes...)
		if cluster.Count() != len(nodes) {
			return nil, fmt.Errorf("some nodes in %s not found", c.Nodes.String())
		}
	}
	cluster = cluster.WithState(backends.LifeCycleStateRunning)
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
		for _, disk := range plist {
			diskCount++
			for _, p := range disk.BlockDevices.Children {
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
				script.Add("mkdir -p " + mountRoot + `"$PARTUUID"`)
				script.Add(fmt.Sprintf("echo \"PARTUUID=$PARTUUID %s$PARTUUID %s %s 0 9\" >> /etc/fstab", mountRoot, c.FsType, c.MountOpts))
				script.Add("mount -a")
			}
		}
		if c.FilterDisks != "ALL" && diskCount < filterDiskCount {
			hasErr = errors.Join(hasErr, fmt.Errorf("could not find all the required disks on node %d", inst.NodeNo))
			return
		}
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
		output := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "/tmp/install.sh." + now},
				Stdin:          stdin,
				Stdout:         stdout,
				Stderr:         stderr,
				SessionTimeout: 5 * time.Minute,
				Env:            []*sshexec.Env{},
				Terminal:       false,
			},
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

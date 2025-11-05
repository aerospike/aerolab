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

type ClusterPartitionCreateCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks     TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterType      string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme/local or ebs/persistent" default:"ALL"`
	Partitions      string          `short:"p" long:"partitions" description:"partitions to create, size is in %% of total disk space; ex: 25,25,25,25; default: just remove all partitions"`
	NoBlkdiscard    bool            `short:"b" long:"no-blkdiscard" description:"set to prevent aerolab from running blkdiscard on the disks and partitions"`
	ParallelThreads int             `long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help            HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterPartitionCreateCmd) Execute(args []string) error {
	cmd := []string{"cluster", "partition", "create"}
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
	_, err = c.PartitionCreateCluster(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterPartitionCreateCmd) PartitionCreateCluster(system *System, inventory *backends.Inventory, args []string, stdin *io.ReadCloser, stdout *io.Writer, stderr *io.Writer, logger *logger.Logger) (output []*backends.ExecOutput, err error) {
	type partition struct {
		start string
		end   string
	}
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "partition", "create"}, c, args...)
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
	partitions := []int{}
	if len(c.Partitions) > 0 {
		parts := strings.Split(c.Partitions, ",")
		total := 0
		for _, i := range parts {
			j, err := strconv.Atoi(i)
			if err != nil {
				return nil, fmt.Errorf("could not translate partitions, must be number,number,number,... :%s", err)
			}
			if j < 1 {
				return nil, fmt.Errorf("cannot create partition of %% size lesser than 1")
			}
			total += j
			if total > 100 {
				return nil, fmt.Errorf("cannot create partitions totalling more than 100%% of the drive")
			}
			partitions = append(partitions, j)
		}
	}
	partitionsSpread := []partition{}
	start := 0
	for _, spreadInt := range partitions {
		end := start + spreadInt
		if end > 100 {
			return nil, errors.New("partition layout would exceed 100%")
		}
		partitionsSpread = append(partitionsSpread, partition{
			start: strconv.Itoa(start),
			end:   strconv.Itoa(end),
		})
		start = start + spreadInt
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
			inst, err := c.PartitionCreateCluster(system, inventory, args, stdin, stdout, stderr, logger)
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
		logger.Info("No nodes to add partition create")
		return nil, nil
	}
	logger.Info("Adding partition create to %d nodes", cluster.Count())
	var hasErr error
	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		// run partitionList on node
		plistCmd := &ClusterPartitionListCmd{
			ClusterName:     TypeClusterName(inst.ClusterName),
			Nodes:           TypeNodes(strconv.Itoa(inst.NodeNo)),
			FilterDisks:     c.FilterDisks,
			FilterType:      c.FilterType,
			ParallelThreads: c.ParallelThreads,
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
			}
			if len(disk.BlockDevices.Children) > 1 {
				script.Add("sleep 1; parted -s " + disk.BlockDevices.Path + " 'mktable gpt'")
			}
			if !c.NoBlkdiscard {
				script.Add(fmt.Sprintf("blkdiscard %s || echo 'blkdiscard not supported'", disk.BlockDevices.Path))
			}
			if c.Partitions == "" {
				if !c.NoBlkdiscard {
					script.Add(fmt.Sprintf("blkdiscard -z --length 8388608 %s", disk.BlockDevices.Path))
				}
			} else {
				script.Add("parted -s " + disk.BlockDevices.Path + " 'mktable gpt'")
				for _, p := range partitionsSpread {
					script.Add("parted -a optimal -s " + disk.BlockDevices.Path + fmt.Sprintf(" mkpart primary %s%% %s%%", p.start, p.end))
				}
				if !c.NoBlkdiscard {
					script.Add("sleep 1; lsblk " + disk.BlockDevices.Path + " -o NAME -l -n |tail -n+2 |while read p; do wipefs -a /dev/$p; blkdiscard -z --length 8388608 /dev/$p; done")
				}
			}
			script.Add("grep \"\\S\" /etc/fstab > /etc/fstab.clean; mv /etc/fstab.clean /etc/fstab")
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

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/bestmethod/inslice"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rglonek/logger"
)

type ClusterPartitionListCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes            TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	FilterDisks      TypeFilterRange `short:"d" long:"filter-disks" description:"Select disks by number, ex: 1,2,4-8" default:"ALL"`
	FilterPartitions TypeFilterRange `short:"p" long:"filter-partitions" description:"Select partitions on each disk by number, empty or 0 = don't show partitions, ex: 1,2,4-8" default:"ALL"`
	FilterType       string          `short:"t" long:"filter-type" description:"what disk types to select, options: nvme/local or ebs/persistent" default:"ALL"`
	ParallelThreads  int             `long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Output           string          `long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme       string          `long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy           []string        `long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager            bool            `long:"pager" description:"Use a pager to display the output"`
	Help             HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterPartitionListCmd) Execute(args []string) error {
	cmd := []string{"cluster", "partition", "list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.PartitionListCluster(system, system.Backend.GetInventory(), args, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterPartitionListCmd) PartitionListCluster(system *System, inventory *backends.Inventory, args []string, logger *logger.Logger) error {
	output, err := c.PartitionListClusterDo(system, inventory, args, logger)
	if err != nil {
		return err
	}
	output.Sort()

	var page *pager.Pager
	if c.Pager {
		page, err = pager.New(os.Stdout)
		if err != nil {
			return err
		}
		err = page.Start()
		if err != nil {
			return err
		}
		defer page.Close()
	}

	switch c.Output {
	case "jq":
		params := []string{}
		if page != nil && page.HasColors() {
			params = append(params, "-C")
		}
		cmd := exec.Command("jq", params...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout
		w, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		defer w.Close()
		enc := json.NewEncoder(w)
		go func() {
			enc.Encode(output)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(os.Stdout).Encode(output)
	case "json-indent":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(output)
	default:
		writer, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			return err
		}
		out := []table.Row{}
		for _, p := range output {
			out = append(out, table.Row{p.ClusterName, p.NodeNo, p.BlockDevices.ModelType, p.Disk, p.Partition, p.BlockDevices.Name, p.BlockDevices.PartUUID, p.BlockDevices.Size, p.BlockDevices.FsType, p.BlockDevices.FsSize, p.BlockDevices.MountPoint})
		}
		fmt.Println(writer.RenderTable(nil, table.Row{"Cluster", "Node", "Type", "Disk", "Partition", "Name", "UUID", "Size", "FsType", "FsSize", "MountPoint"}, out))
	}
	return nil
}

func (c *ClusterPartitionListCmd) PartitionListClusterDo(system *System, inventory *backends.Inventory, args []string, logger *logger.Logger) (output PartitionList, err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "partition", "list"}, c, args...)
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

	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		var output PartitionList
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			inst, err := c.PartitionListClusterDo(system, inventory, args, logger)
			if err != nil {
				return nil, err
			}
			output = append(output, inst...)
		}
		output.Sort()
		return output, nil
	}
	if c.FilterType == "local" {
		c.FilterType = "nvme"
	} else if c.FilterType == "persistent" {
		c.FilterType = "ebs"
	}
	disksFilter, err := c.FilterDisks.Expand()
	if err != nil {
		return nil, fmt.Errorf("could not expand disks filter: %s", err)
	}
	partitionsFilter, err := c.FilterPartitions.Expand()
	if err != nil {
		return nil, fmt.Errorf("could not expand partitions filter: %s", err)
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
		logger.Info("No nodes to add partition create")
		return nil, nil
	}
	logger.Info("Adding partition create to %d nodes", cluster.Count())
	var hasErr error
	outlock := &sync.Mutex{}
	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		var stdout = &bytes.Buffer{}
		blkFormat := 1
		out := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"lsblk", "-a", "-f", "-J", "-o", "NAME,PATH,FSTYPE,FSSIZE,MOUNTPOINT,MODEL,SIZE,TYPE,PARTUUID"},
				Stdin:          nil,
				Stdout:         stdout,
				Stderr:         nil,
				SessionTimeout: 5 * time.Minute,
				Env:            []*sshexec.Env{},
				Terminal:       true,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: c.ParallelThreads,
		})
		if out.Output.Err != nil {
			blkFormat = 2
			stdout = &bytes.Buffer{}
			out = inst.Exec(&backends.ExecInput{
				ExecDetail: sshexec.ExecDetail{
					Command:        []string{"lsblk", "-a", "-f", "-J", "-o", "NAME,PATH,FSTYPE,FSSIZE,MOUNTPOINT,MODEL,SIZE,TYPE,PARTUUID"},
					Stdin:          nil,
					Stdout:         stdout,
					Stderr:         nil,
					SessionTimeout: 5 * time.Minute,
					Env:            []*sshexec.Env{},
					Terminal:       true,
				},
				Username:        "root",
				ConnectTimeout:  30 * time.Second,
				ParallelThreads: c.ParallelThreads,
			})
			if out.Output.Err != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s (%s) (%s)", inst.ClusterName, inst.NodeNo, out.Output.Err, string(out.Output.Stdout), string(out.Output.Stderr)))
				return
			}
		}
		disks := &blockDeviceInformation{}
		err = json.Unmarshal(stdout.Bytes(), disks)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s (%s) (%s)", inst.ClusterName, inst.NodeNo, err, stdout.String(), string(out.Output.Stderr)))
			return
		}
		disks.BlockDevices = c.fixPartOut(disks.BlockDevices, blkFormat) // centos workaround
		outlock.Lock()
		output = append(output, c.buildPartitionList(system, inst, disks.BlockDevices, disksFilter, partitionsFilter)...)
		outlock.Unlock()
	})
	if hasErr != nil {
		return nil, hasErr
	}
	output.Sort()
	return output, nil
}

func (c *ClusterPartitionListCmd) buildPartitionList(system *System, inst *backends.Instance, bd []*BlockDevice, disksFilter []int, partitionsFilter []int) []*PartitionInformation {
	diskNo := 0
	ret := []*PartitionInformation{}
	for _, disk := range bd {
		if disk.Type != "disk" {
			continue
		}
		diskType := ""
		if (system.Opts.Config.Backend.Type == "aws" && disk.Model == "Amazon Elastic Block Store") || (system.Opts.Config.Backend.Type == "gcp" && strings.HasPrefix(disk.Model, "PersistentDisk")) {
			diskType = "ebs"
			if c.FilterType != "" && c.FilterType != "ALL" && c.FilterType != "ebs" {
				continue
			}
		} else if (system.Opts.Config.Backend.Type == "aws" && disk.Model == "Amazon EC2 NVMe Instance Storage") || (system.Opts.Config.Backend.Type == "gcp" && disk.Model != "") {
			diskType = "nvme"
			if c.FilterType != "" && c.FilterType != "ALL" && c.FilterType != "nvme" {
				continue
			}
		} else {
			continue
		}
		if disk.MountPoint == "/" || strings.HasPrefix(disk.MountPoint, "/boot") {
			continue
		}
		isRestricted := false
		for _, part := range disk.Children {
			if part.MountPoint == "/" || strings.HasPrefix(part.MountPoint, "/boot") {
				isRestricted = true
				break
			}
		}
		if isRestricted {
			continue
		}
		diskNo++
		if c.FilterDisks != "ALL" && c.FilterDisks != "" && !inslice.HasInt(disksFilter, diskNo) {
			continue
		}
		disk.ModelType = diskType
		ret = append(ret, &PartitionInformation{
			ClusterName:  inst.ClusterName,
			NodeNo:       inst.NodeNo,
			Disk:         diskNo,
			Partition:    0,
			BlockDevices: *disk,
		})
		if len(disk.Children) != 0 && c.FilterPartitions != "0" && c.FilterPartitions != "" {
			partNo := 0
			for _, part := range disk.Children {
				if part.Type != "part" {
					continue
				}
				partNo++
				if c.FilterPartitions == "ALL" || inslice.HasInt(partitionsFilter, partNo) {
					ret = append(ret, &PartitionInformation{
						ClusterName:  inst.ClusterName,
						NodeNo:       inst.NodeNo,
						Disk:         diskNo,
						Partition:    partNo,
						BlockDevices: *part,
					})
				}
			}
		}
	}
	return ret
}

func (c *ClusterPartitionListCmd) fixPartOut(bd []*BlockDevice, blkFormat int) []*BlockDevice {
	for i := range bd {
		bd[i].FsSize = strings.Trim(bd[i].FsSize, "\t\r\n ")
		bd[i].FsType = strings.Trim(bd[i].FsType, "\t\r\n ")
		bd[i].Model = strings.Trim(bd[i].Model, "\t\r\n ")
		bd[i].MountPoint = strings.Trim(bd[i].MountPoint, "\t\r\n ")
		bd[i].Name = strings.Trim(bd[i].Name, "\t\r\n ")
		bd[i].Path = strings.Trim(bd[i].Path, "\t\r\n ")
		bd[i].Size = strings.Trim(bd[i].Size, "\t\r\n ")
		bd[i].Type = strings.Trim(bd[i].Type, "\t\r\n ")
		bd[i].PartUUID = strings.Trim(bd[i].PartUUID, "\t\r\n ")
		if bd[i].PartUUID != "" {
			bd[i].PartUUIDPath = path.Join("/dev/disk/by-partuuid", bd[i].PartUUID)
		}
		if blkFormat == 2 {
			bd[i].Path = bd[i].Name
			bd[i].FsSize = "N/A"
		}
		if len(bd[i].Children) > 0 {
			bd[i].Children = c.fixPartOut(bd[i].Children, blkFormat)
		}
	}
	return bd
}

package cmd

import (
	"fmt"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/go-flags"
)

type FilesSyncCmd struct {
	SourceCluster      TypeClusterName `short:"n" long:"source-name" description:"Source cluster name" default:"mydc"`
	SourceNode         TypeNode        `short:"l" long:"source-node" description:"Source node number" default:"1"`
	DestinationCluster TypeClusterName `short:"d" long:"dest-name" description:"Destination cluster name" default:"mydc"`
	DestinationNodes   TypeNodes       `short:"o" long:"destn-nodes" description:"Destination node numbers; default: all except source node" default:""`
	ParallelThreads    int             `short:"t" long:"threads" description:"Run on this many nodes in parallel" default:"10"`
	Path               FilesSingleCmd  `positional-args:"true"`
	Help               HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *FilesSyncCmd) Execute(args []string) error {
	if string(c.Path.Path) == "help" {
		return PrintHelp(false, "Specify a file or directory at the end of the command. Ex: aerolab files sync ... /etc/resolv.conf\n\n")
	}
	if c.Path.Path == "" {
		return PrintHelp(false, "Specify a file or directory at the end of the command. Ex: aerolab files sync ... /etc/resolv.conf\n\n")
	}
	cmd := []string{"files", "sync"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.Sync(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *FilesSyncCmd) Sync(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"files", "sync"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	tmpPath, err := os.MkdirTemp("", "aerolab-files-sync-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpPath)

	// get source node
	source := inventory.Instances.WithClusterName(c.SourceCluster.String())
	if source.Count() == 0 {
		return fmt.Errorf("cluster %s not found", c.SourceCluster.String())
	}
	source = source.WithNodeNo(int(c.SourceNode))
	if source.Count() == 0 {
		return fmt.Errorf("cluster %s node %d not found", c.SourceCluster.String(), c.SourceNode)
	}
	source = source.WithState(backends.LifeCycleStateRunning).Describe()
	if source.Count() == 0 {
		return fmt.Errorf("source instance is not running")
	}
	if source.Count() > 1 {
		return fmt.Errorf("multiple source instances found, specify cluster name and node number")
	}

	// get destination nodes
	dest := inventory.Instances.WithClusterName(c.DestinationCluster.String())
	if dest.Count() == 0 {
		return fmt.Errorf("cluster %s not found", c.DestinationCluster.String())
	}
	if c.DestinationNodes.String() != "" {
		nodes, err := expandNodeNumbers(c.DestinationNodes.String())
		if err != nil {
			return err
		}
		new := dest.WithNodeNo(nodes...).Describe()
		if new.Count() != len(nodes) {
			return fmt.Errorf("some destination nodes not found: %s", c.DestinationNodes.String())
		}
		dest = new
	}
	dest = dest.WithState(backends.LifeCycleStateRunning).Describe()
	if dest.Count() == 0 {
		return fmt.Errorf("no running destination instances found")
	}

	// get new list of nodes we are processing - for filtering
	destInt := []int{}
	for _, node := range dest.Describe() {
		destInt = append(destInt, node.NodeNo)
	}

	// if source cluster name is same as destination, do NOT reupload to source node
	if c.SourceCluster.String() == c.DestinationCluster.String() {
		if len(destInt) > 0 {
			if slices.Contains(destInt, int(c.SourceNode)) {
				destInt = slices.Delete(destInt, slices.Index(destInt, int(c.SourceNode)), slices.Index(destInt, int(c.SourceNode))+1)
				dest = dest.WithNodeNo(destInt...).Describe()
			}
		}
	}

	// get a string version of final node list - for logging
	destNodes := []string{}
	for _, node := range dest.Describe() {
		destNodes = append(destNodes, strconv.Itoa(node.NodeNo))
	}

	system.Logger.Info("Syncing %s from source cluster %s node %d to destination cluster %s nodes %s", string(c.Path.Path), c.SourceCluster.String(), c.SourceNode, c.DestinationCluster.String(), strings.Join(destNodes, ","))

	// download from source
	destPath := path.Join(tmpPath, string(c.Path.Path))
	destDir := path.Dir(destPath)
	err = os.MkdirAll(destDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create destination directory %s: %v", destDir, err)
	}

	dl := &FilesDownloadCmd{
		ClusterName:     c.SourceCluster,
		Nodes:           TypeNodes(strconv.Itoa(int(c.SourceNode))),
		ParallelThreads: 1,
		Files: FilesRestDownloadCmd{
			Source:      string(c.Path.Path),
			Destination: flags.Filename(destPath),
		},
	}
	err = dl.Download(system, inventory, args)
	if err != nil {
		return err
	}

	system.Logger.Info("Downloaded %s from source cluster %s node %d to %s", string(c.Path.Path), c.SourceCluster.String(), c.SourceNode, destPath)

	// upload to destination
	up := &FilesUploadCmd{
		ClusterName:     c.DestinationCluster,
		Nodes:           TypeNodes(strings.Join(destNodes, ",")),
		ParallelThreads: c.ParallelThreads,
		Files: FilesRestUploadCmd{
			Source:      flags.Filename(destPath),
			Destination: string(c.Path.Path),
		},
	}
	err = up.Upload(system, inventory, args)
	if err != nil {
		return err
	}

	system.Logger.Info("Uploaded %s to destination cluster %s nodes %s", string(c.Path.Path), c.DestinationCluster.String(), strings.Join(destNodes, ","))

	return nil
}

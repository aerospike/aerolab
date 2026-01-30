package cmd

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClusterApplyCmd struct {
	ClusterName             TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Count                   int             `short:"c" long:"count" description:"Desired number of nodes in the cluster" default:"1"`
	CustomConfigFilePath    flags.Filename  `short:"o" long:"customconf" description:"Custom aerospike config file path to install"`
	CustomToolsFilePath     flags.Filename  `short:"z" long:"toolsconf" description:"Custom astools config file path to install"`
	FeaturesFilePath        flags.Filename  `short:"f" long:"featurefile" description:"Features file to install, or directory containing feature files"`
	FeaturesFilePrintDetail bool            `long:"featurefile-printdetail" description:"Print details of discovered features files" hidden:"true"`
	HeartbeatMode           TypeHBMode      `short:"m" long:"mode" description:"Heartbeat mode, one of: mcast|mesh|default" default:"mesh" webchoice:"mesh,mcast,default" simplemode:"false"`
	MulticastAddress        string          `short:"a" long:"mcast-address" description:"Multicast address to change to in config file" simplemode:"false"`
	MulticastPort           string          `short:"p" long:"mcast-port" description:"Multicast port to change to in config file" simplemode:"false"`
	aerospikeVersionSelectorCmd
	AutoStartAerospike    TypeYesNo              `short:"s" long:"start" description:"Auto-start aerospike after creation of cluster (y/n)" default:"y" webchoice:"y,n"`
	NoOverrideClusterName bool                   `short:"O" long:"no-override-cluster-name" description:"Aerolab sets cluster-name by default, use this parameter to not set cluster-name" simplemode:"false"`
	NoSetDNS              bool                   `long:"no-set-dns" description:"set to prevent aerolab from updating resolved to use 1.1.1.1/8.8.8.8 DNS"`
	ScriptEarly           flags.Filename         `short:"X" long:"early-script" description:"optionally specify a script to be installed which will run before every aerospike start" simplemode:"false"`
	ScriptLate            flags.Filename         `short:"Z" long:"late-script" description:"optionally specify a script to be installed which will run after every aerospike stop" simplemode:"false"`
	ParallelThreads       int                    `short:"P" long:"parallel-threads" description:"number of threads to use for parallel operations" default:"10" simplemode:"false"`
	NoVacuumOnFail        bool                   `long:"no-vacuum" description:"if set, will not remove the template instance/container should it fail installation" simplemode:"false"`
	Owner                 string                 `long:"owner" description:"AWS/GCP only: create owner tag with this value" simplemode:"false"`
	PriceOnly             bool                   `long:"price" description:"Only display price of ownership; do not actually create the cluster" simplemode:"false"`
	Force                 bool                   `short:"F" long:"force" description:"Force destroy when shrinking"`
	DryRun                bool                   `long:"dry-run" description:"Dry run, print what would be done but don't do it"`
	Aws                   ClusterCreateCmdAws    `group:"AWS" description:"backend-aws"`
	Gcp                   ClusterCreateCmdGcp    `group:"GCP" description:"backend-gcp"`
	Docker                ClusterCreateCmdDocker `group:"Docker" description:"backend-docker"`
	Help                  HelpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterApplyCmd) Execute(args []string) error {
	cmd := []string{"cluster", "apply"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	instances, err := c.ApplyCluster(system, system.Backend.GetInventory(), system.Logger, args, "apply")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Applied cluster changes to %d instances", instances.Count())
	for _, i := range instances.Describe() {
		system.Logger.Debug("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ApplyCluster applies the desired cluster size by creating, growing, or shrinking the cluster as needed.
// This function performs the following operations:
// 1. Determines the current cluster size
// 2. Compares with desired size to determine action (create/grow/shrink/noop)
// 3. Calls the appropriate cluster command (create/grow/destroy)
// 4. Outputs the node numbers that were created or destroyed
//
// Parameters:
//   - system: The system instance for logging and backend operations
//   - inventory: The backend inventory containing cluster information
//   - logger: Logger instance for output
//   - args: Command line arguments
//   - action: The action being performed
//
// Returns:
//   - backends.InstanceList: List of instances that were processed
//   - error: nil on success, or an error describing what failed
func (c *ClusterApplyCmd) ApplyCluster(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", action}, c, args...)
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
		var instances backends.InstanceList
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			inst, err := c.ApplyCluster(system, inventory, logger, args, action)
			if err != nil {
				return nil, err
			}
			instances = append(instances, inst...)
		}
		return instances, nil
	}

	if c.Count < 0 {
		return nil, errors.New("count must be at least 0")
	}

	cluster := inventory.Instances.WithNotState(backends.LifeCycleStateTerminated).WithClusterName(c.ClusterName.String())
	currentCount := cluster.Count()

	// Determine action based on current vs desired count
	actionType := ""
	if currentCount == 0 && c.Count == 0 {
		actionType = "noop"
	} else if currentCount == 0 && c.Count > 0 {
		actionType = "create"
	} else if currentCount < c.Count {
		actionType = "grow"
	} else if currentCount == c.Count {
		actionType = "noop"
	} else {
		actionType = "shrink"
	}

	logger.Info("Current cluster size: %d, desired size: %d, action: %s", currentCount, c.Count, actionType)

	var instances backends.InstanceList
	var err error

	switch actionType {
	case "create":
		logger.Info("Creating cluster %s with %d nodes", c.ClusterName.String(), c.Count)
		instances, err = c.createCluster(system, inventory, logger, args)
		if err != nil {
			return nil, fmt.Errorf("failed to create cluster: %w", err)
		}
		c.outputCreatedNodes(instances)

	case "grow":
		growCount := c.Count - currentCount
		logger.Info("Growing cluster %s by %d nodes", c.ClusterName.String(), growCount)
		instances, err = c.growCluster(system, inventory, logger, args, growCount)
		if err != nil {
			return nil, fmt.Errorf("failed to grow cluster: %w", err)
		}
		c.outputCreatedNodes(instances)

	case "shrink":
		shrinkCount := currentCount - c.Count
		logger.Info("Shrinking cluster %s by %d nodes", c.ClusterName.String(), shrinkCount)
		instances, err = c.shrinkCluster(system, inventory, logger, args, cluster, shrinkCount)
		if err != nil {
			return nil, fmt.Errorf("failed to shrink cluster: %w", err)
		}
		c.outputDestroyedNodes(instances)

	case "noop":
		logger.Info("Cluster %s is already at the desired size of %d nodes", c.ClusterName.String(), c.Count)
		return cluster.Describe(), nil
	}

	return instances, nil
}

// createCluster creates a new cluster with the specified number of nodes.
func (c *ClusterApplyCmd) createCluster(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (backends.InstanceList, error) {
	createCmd := &ClusterCreateCmd{
		ClusterName:                 c.ClusterName,
		NodeCount:                   c.Count,
		CustomConfigFilePath:        c.CustomConfigFilePath,
		CustomToolsFilePath:         c.CustomToolsFilePath,
		FeaturesFilePath:            c.FeaturesFilePath,
		FeaturesFilePrintDetail:     c.FeaturesFilePrintDetail,
		HeartbeatMode:               c.HeartbeatMode,
		MulticastAddress:            c.MulticastAddress,
		MulticastPort:               c.MulticastPort,
		aerospikeVersionSelectorCmd: c.aerospikeVersionSelectorCmd,
		AutoStartAerospike:          c.AutoStartAerospike,
		NoOverrideClusterName:       c.NoOverrideClusterName,
		NoSetDNS:                    c.NoSetDNS,
		ScriptEarly:                 c.ScriptEarly,
		ScriptLate:                  c.ScriptLate,
		ParallelThreads:             c.ParallelThreads,
		NoVacuumOnFail:              c.NoVacuumOnFail,
		Owner:                       c.Owner,
		PriceOnly:                   c.PriceOnly,
		Aws:                         c.Aws,
		Gcp:                         c.Gcp,
		Docker:                      c.Docker,
	}

	instances, err := createCmd.CreateCluster(system, inventory, logger, args, "create")
	if err != nil {
		return nil, err
	}

	return instances, nil
}

// growCluster grows an existing cluster by adding the specified number of nodes.
func (c *ClusterApplyCmd) growCluster(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, growCount int) (backends.InstanceList, error) {
	growCmd := &ClusterGrowCmd{
		ClusterCreateCmd: ClusterCreateCmd{
			ClusterName:                 c.ClusterName,
			NodeCount:                   growCount,
			CustomConfigFilePath:        c.CustomConfigFilePath,
			CustomToolsFilePath:         c.CustomToolsFilePath,
			FeaturesFilePath:            c.FeaturesFilePath,
			FeaturesFilePrintDetail:     c.FeaturesFilePrintDetail,
			HeartbeatMode:               c.HeartbeatMode,
			MulticastAddress:            c.MulticastAddress,
			MulticastPort:               c.MulticastPort,
			aerospikeVersionSelectorCmd: c.aerospikeVersionSelectorCmd,
			AutoStartAerospike:          c.AutoStartAerospike,
			NoOverrideClusterName:       c.NoOverrideClusterName,
			NoSetDNS:                    c.NoSetDNS,
			ScriptEarly:                 c.ScriptEarly,
			ScriptLate:                  c.ScriptLate,
			ParallelThreads:             c.ParallelThreads,
			NoVacuumOnFail:              c.NoVacuumOnFail,
			Owner:                       c.Owner,
			PriceOnly:                   c.PriceOnly,
			Aws:                         c.Aws,
			Gcp:                         c.Gcp,
			Docker:                      c.Docker,
		},
	}

	instances, err := growCmd.CreateCluster(system, inventory, logger, args, "grow")
	if err != nil {
		return nil, err
	}

	return instances, nil
}

// shrinkCluster shrinks an existing cluster by destroying the specified number of nodes.
func (c *ClusterApplyCmd) shrinkCluster(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, cluster backends.Instances, shrinkCount int) (backends.InstanceList, error) {
	// Get the highest numbered nodes to destroy (same logic as instances apply)
	var nodes []int
	for _, node := range cluster.Describe() {
		nodes = append(nodes, node.NodeNo)
	}
	sort.Ints(nodes)

	// Debug logging
	logger.Debug("Current cluster nodes: %v", nodes)
	logger.Debug("Shrinking by %d nodes", shrinkCount)

	if len(nodes) < shrinkCount {
		return nil, fmt.Errorf("cannot shrink by %d nodes: cluster only has %d nodes", shrinkCount, len(nodes))
	}

	nodes = nodes[len(nodes)-shrinkCount:]
	logger.Debug("Nodes to destroy: %v", nodes)

	// Convert to string for the destroy command
	nodesStr := []string{}
	for _, node := range nodes {
		nodesStr = append(nodesStr, strconv.Itoa(node))
	}

	logger.Debug("Nodes to destroy string: %s", strings.Join(nodesStr, ","))

	destroyCmd := &ClusterDestroyCmd{
		ClusterName: c.ClusterName,
		Nodes:       TypeNodes(strings.Join(nodesStr, ",")),
		Force:       c.Force,
	}

	instances, err := destroyCmd.DestroyCluster(system, inventory, logger, args, "destroy")
	if err != nil {
		return nil, err
	}

	return instances, nil
}

// outputCreatedNodes outputs the node numbers of created nodes in the specified format.
func (c *ClusterApplyCmd) outputCreatedNodes(instances backends.InstanceList) {
	if instances.Count() == 0 {
		fmt.Println("created-node-numbers:")
		return
	}

	var nodeNumbers []int
	for _, instance := range instances.Describe() {
		nodeNumbers = append(nodeNumbers, instance.NodeNo)
	}
	sort.Ints(nodeNumbers)

	var nodeStrs []string
	for _, node := range nodeNumbers {
		nodeStrs = append(nodeStrs, strconv.Itoa(node))
	}

	fmt.Printf("created-node-numbers:%s\n", strings.Join(nodeStrs, ","))
}

// outputDestroyedNodes outputs the node numbers of destroyed nodes in the specified format.
func (c *ClusterApplyCmd) outputDestroyedNodes(instances backends.InstanceList) {
	if instances.Count() == 0 {
		fmt.Println("destroyed-node-numbers:")
		return
	}

	var nodeNumbers []int
	for _, instance := range instances.Describe() {
		nodeNumbers = append(nodeNumbers, instance.NodeNo)
	}
	sort.Ints(nodeNumbers)

	var nodeStrs []string
	for _, node := range nodeNumbers {
		nodeStrs = append(nodeStrs, strconv.Itoa(node))
	}

	fmt.Printf("destroyed-node-numbers:%s\n", strings.Join(nodeStrs, ","))
}

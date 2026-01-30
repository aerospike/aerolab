package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type ClusterStopCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Force       bool            `long:"force" description:"Force stop the cluster"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterStopCmd) Execute(args []string) error {
	cmd := []string{"cluster", "stop"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	instances, err := c.StopCluster(system, system.Backend.GetInventory(), system.Logger, args, "stop")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Stopped %d instances", instances.Count())
	for _, i := range instances.Describe() {
		fmt.Printf("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s\n", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterStopCmd) StopCluster(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
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
			inst, err := c.StopCluster(system, inventory, logger, args, action)
			if err != nil {
				return nil, err
			}
			instances = append(instances, inst...)
		}
		return instances, nil
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
		logger.Debug("Requested nodes to stop: %v", nodes)
		logger.Debug("Total cluster nodes before filtering: %d", cluster.Count())

		// Debug: show all nodes in cluster
		allNodeNumbers := []int{}
		for _, node := range cluster.Describe() {
			allNodeNumbers = append(allNodeNumbers, node.NodeNo)
			logger.Debug("Cluster node %d: state=%s", node.NodeNo, node.InstanceState)
		}
		logger.Debug("All nodes in cluster: %v", allNodeNumbers)

		// First check if all requested nodes exist in the cluster (regardless of state)
		allNodes := cluster.WithNodeNo(nodes...).WithState(backends.LifeCycleStateRunning)
		logger.Debug("Found nodes matching requested numbers: %d", allNodes.Count())

		if allNodes.Count() != len(nodes) {
			// Debug: show which nodes were actually found
			foundNodes := []int{}
			for _, node := range allNodes.Describe() {
				foundNodes = append(foundNodes, node.NodeNo)
			}
			logger.Debug("Found nodes: %v, requested nodes: %v", foundNodes, nodes)
			return nil, fmt.Errorf("some nodes in %s not found", c.Nodes.String())
		}
		// Then filter by state to only stop running nodes
		cluster = allNodes.WithState(backends.LifeCycleStateRunning)
		if cluster.Count() == 0 {
			logger.Info("All requested nodes are already stopped")
			return nil, nil
		}
	} else {
		cluster = cluster.WithState(backends.LifeCycleStateRunning)
		if cluster.Count() == 0 {
			logger.Info("No nodes to stop")
			return nil, nil
		}
	}
	logger.Info("Stopping %d nodes", cluster.Count())
	err := cluster.Stop(c.Force, 10*time.Minute)
	return cluster.Describe(), err
}

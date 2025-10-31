package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/rglonek/logger"
)

type ClusterDestroyCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Force       bool            `short:"f" long:"force" description:"Force destroy"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterDestroyCmd) Execute(args []string) error {
	cmd := []string{"cluster", "destroy"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	instances, err := c.DestroyCluster(system, system.Backend.GetInventory(), system.Logger, args, "destroy")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Destroyed %d instances", instances.Count())
	for _, i := range instances.Describe() {
		fmt.Printf("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s\n", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterDestroyCmd) DestroyCluster(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
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
			inst, err := c.DestroyCluster(system, inventory, logger, args, action)
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
		logger.Debug("Requested nodes to destroy: %v", nodes)
		logger.Debug("Total cluster nodes before filtering: %d", cluster.Count())

		// First check if all requested nodes exist in the cluster (regardless of state)
		allNodes := cluster.WithNodeNo(nodes...).WithNotState(backends.LifeCycleStateTerminated, backends.LifeCycleStateTerminating)
		logger.Debug("Found nodes matching requested numbers: %d", allNodes.Count())

		// Debug: show all nodes in cluster with their states
		for _, node := range cluster.Describe() {
			logger.Debug("Cluster node %d: state=%s", node.NodeNo, node.InstanceState)
		}

		if allNodes.Count() != len(nodes) {
			// Debug: show which nodes exist
			existingNodes := []int{}
			for _, node := range allNodes.Describe() {
				existingNodes = append(existingNodes, node.NodeNo)
			}
			logger.Debug("Existing nodes in cluster: %v", existingNodes)
			return nil, fmt.Errorf("some nodes in %s not found", c.Nodes.String())
		}

		// Then filter by state
		cluster = allNodes.WithNotState(backends.LifeCycleStateTerminated, backends.LifeCycleStateTerminating)
		logger.Debug("Nodes after state filtering: %d", cluster.Count())

		if cluster.Count() == 0 {
			logger.Info("All requested nodes are already terminated or terminating")
			return nil, nil
		}
	} else {
		cluster = cluster.WithNotState(backends.LifeCycleStateTerminated, backends.LifeCycleStateTerminating)
		if cluster.Count() == 0 {
			logger.Info("No nodes to destroy")
			return nil, nil
		}
	}
	if !c.Force {
		if IsInteractive() {
			choice, quitting, err := choice.Choice("Are you sure you want to destroy "+strconv.Itoa(cluster.Count())+" nodes?", choice.Items{
				choice.Item("Yes"),
				choice.Item("No"),
			})
			if err != nil {
				return nil, err
			}
			if quitting {
				return nil, errors.New("aborted")
			}
			switch choice {
			case "No":
				return nil, errors.New("aborted")
			}
		}
	}
	logger.Info("Destroying %d nodes", cluster.Count())
	err := cluster.Terminate(10 * time.Minute)
	return cluster.Describe(), err
}

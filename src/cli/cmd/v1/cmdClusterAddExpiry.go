package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type ClusterAddExpiryCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	ExpireIn    TypeExpiry       `short:"e" long:"expiry" description:"Expiry in duration from now; Y/M/W/D/h/m/s, ex 1D12h 2W 1Y6M" default:"30h"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterAddExpiryCmd) Execute(args []string) error {
	cmd := []string{"cluster", "add", "expiry"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.AddExpiryCluster(system, system.Backend.GetInventory(), args, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterAddExpiryCmd) AddExpiryCluster(system *System, inventory *backends.Inventory, args []string, logger *logger.Logger) (err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "add", "expiry"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	if c.ClusterName.String() == "" {
		return fmt.Errorf("cluster name is required")
	}
	var cluster backends.Instances
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		for _, cluster := range clusters {
			if inventory.Instances.WithClusterName(cluster).WithState(backends.LifeCycleStateRunning).Count() == 0 {
				return fmt.Errorf("cluster %s not found", cluster)
			}
		}
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			err := c.AddExpiryCluster(system, inventory, args, logger)
			if err != nil {
				return err
			}
		}
		return nil
	} else {
		var err error
		cluster, err = c.ClusterName.GetInstanceList(inventory, backends.LifeCycleStateRunning)
		if err != nil {
			return err
		}
	}
	if c.Nodes.String() != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return err
		}
		cluster = cluster.WithNodeNo(nodes...)
		if cluster.Count() != len(nodes) {
			return fmt.Errorf("some nodes in %s not found", c.Nodes.String())
		}
	}
	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		logger.Info("No nodes to add expiry")
		return nil
	}
	expiry := time.Time{}
	if c.ExpireIn == 0 {
		logger.Info("Removing expiry from %d nodes", cluster.Count())
	} else {
		logger.Info("Adding expiry to %d nodes", cluster.Count())
		expiry = time.Now().Add(c.ExpireIn.Duration())
	}
	err = cluster.ChangeExpiry(expiry)
	if err != nil {
		return err
	}
	for _, inst := range cluster.Describe() {
		if inst.AttachedVolumes == nil {
			continue
		}
		dotVols := inst.AttachedVolumes.WithDeleteOnTermination(true)
		if dotVols.Count() == 0 {
			continue
		}
		if vErr := dotVols.ChangeExpiry(expiry); vErr != nil {
			logger.Warn("Failed to update volume expiry for %s:%d: %s", inst.ClusterName, inst.NodeNo, vErr)
		}
	}
	return nil
}

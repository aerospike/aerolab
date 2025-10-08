package cmd

import (
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClusterAddFirewallCmd struct {
	ClusterName  TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes        TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	CustomConf   flags.Filename  `short:"o" long:"custom-conf" description:"To deploy a custom ape.toml configuration file, specify it's path here"`
	FirewallName string          `short:"f" long:"firewall" description:"Firewall name to assign to the nodes"`
	Help         HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterAddFirewallCmd) Execute(args []string) error {
	cmd := []string{"cluster", "add", "firewall"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.AddFirewallCluster(system, system.Backend.GetInventory(), args, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterAddFirewallCmd) AddFirewallCluster(system *System, inventory *backends.Inventory, args []string, logger *logger.Logger) (err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "add", "firewall"}, c, args...)
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
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			err := c.AddFirewallCluster(system, inventory, args, logger)
			if err != nil {
				return err
			}
		}
		return nil
	}
	cluster := inventory.Instances.WithClusterName(c.ClusterName.String())
	if cluster == nil {
		return fmt.Errorf("cluster %s not found", c.ClusterName.String())
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
		logger.Info("No nodes to add firewall")
		return nil
	}
	logger.Info("Adding firewall to %d nodes", cluster.Count())
	return cluster.AssignFirewalls(inventory.Firewalls.WithName(c.FirewallName).Describe())
}

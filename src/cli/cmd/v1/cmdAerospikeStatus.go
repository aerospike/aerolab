package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

type AerospikeStatusCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Threads     int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AerospikeStatusCmd) Execute(args []string) error {
	cmd := []string{"aerospike", "status"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	instances, err := c.StatusAerospike(system, system.Backend.GetInventory(), system.Logger, args, "status")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Checked aerospike status on %d instances", instances.Count())
	for _, i := range instances.Describe() {
		system.Logger.Debug("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s\n", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *AerospikeStatusCmd) StatusAerospike(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"aerospike", action}, c, args...)
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
			inst, err := c.StatusAerospike(system, inventory, logger, args, action)
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
		cluster = cluster.WithNodeNo(nodes...)
		if cluster.Count() != len(nodes) {
			return nil, fmt.Errorf("some nodes in %s not found", c.Nodes.String())
		}
	}
	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		logger.Info("No running instances found for cluster %s", c.ClusterName.String())
		return nil, nil
	}
	logger.Info("Checking aerospike status on %d nodes", cluster.Count())

	// Execute systemctl status aerospike on all instances
	out := cluster.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"systemctl", "status", "aerospike"},
			Stdin:          nil,
			Stdout:         os.Stdout,
			Stderr:         os.Stderr,
			SessionTimeout: time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       true,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: c.Threads,
	})

	var errs error
	for _, o := range out {
		if o.Output.Err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s:%d: %s (%s) (%s)", o.Instance.ClusterName, o.Instance.NodeNo, o.Output.Err, o.Output.Stdout, o.Output.Stderr))
		}
	}

	return cluster.Describe(), errs
}

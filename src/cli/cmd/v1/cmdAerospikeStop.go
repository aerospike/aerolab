package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

type AerospikeStopCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Threads     int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	MaxRetries  int             `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep  time.Duration   `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AerospikeStopCmd) Execute(args []string) error {
	cmd := []string{"aerospike", "stop"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	instances, err := c.StopAerospike(system, system.Backend.GetInventory(), system.Logger, args, "stop")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Stopped aerospike on %d instances", instances.Count())
	for _, i := range instances.Describe() {
		system.Logger.Debug("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s\n", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *AerospikeStopCmd) StopAerospike(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
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
	var cluster backends.Instances
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		var instances backends.InstanceList
		for _, cluster := range clusters {
			if inventory.Instances.WithClusterName(cluster).WithState(backends.LifeCycleStateRunning).Count() == 0 {
				return nil, fmt.Errorf("cluster %s not found", cluster)
			}
		}
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			inst, err := c.StopAerospike(system, inventory, logger, args, action)
			if err != nil {
				return nil, err
			}
			instances = append(instances, inst...)
		}
		return instances, nil
	} else {
		var err error
		cluster, err = c.ClusterName.GetInstanceList(inventory, backends.LifeCycleStateRunning)
		if err != nil {
			return nil, err
		}
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
	logger.Info("Stopping aerospike on %d nodes", cluster.Count())

	// Execute systemctl stop aerospike on all instances
	out := cluster.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"systemctl", "stop", "aerospike"},
			SessionTimeout: time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: c.Threads,
		MaxRetries:      c.MaxRetries,
		RetrySleep:      c.RetrySleep,
	})

	var errs error
	for _, o := range out {
		if o.Output.Err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s:%d: %s (%s) (%s)", o.Instance.ClusterName, o.Instance.NodeNo, o.Output.Err, o.Output.Stdout, o.Output.Stderr))
		}
	}

	return cluster.Describe(), errs
}

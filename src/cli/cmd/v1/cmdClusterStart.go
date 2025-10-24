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

type ClusterStartCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	NoFixMesh   bool            `short:"f" long:"no-fix-mesh" description:"Set to avoid running conf-fix-mesh" simplemode:"false"`
	NoStart     bool            `short:"s" long:"no-start" description:"Set to prevent Aerospike from starting on cluster-start"`
	Threads     int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterStartCmd) Execute(args []string) error {
	cmd := []string{"cluster", "start"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	instances, err := c.StartCluster(system, system.Backend.GetInventory(), system.Logger, args, "start")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Started %d instances", instances.Count())
	for _, i := range instances.Describe() {
		fmt.Printf("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s\n", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterStartCmd) StartCluster(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
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
			inst, err := c.StartCluster(system, inventory, logger, args, action)
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
	cluster = cluster.WithState(backends.LifeCycleStateStopped)
	if cluster.Count() == 0 {
		logger.Info("No nodes to start")
		return nil, nil
	}
	logger.Info("Starting %d nodes", cluster.Count())
	err := cluster.Start(10 * time.Minute)
	if err != nil {
		return nil, err
	}
	var errs error
	if !c.NoFixMesh {
		logger.Info("Fixing heartbeat configuration")
		fixMesh := &ConfFixMeshCmd{
			ClusterName:     c.ClusterName,
			Nodes:           c.Nodes,
			ParallelThreads: c.Threads,
		}
		if err := fixMesh.FixMesh(system, inventory, logger, args); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to fix mesh configuration: %w", err))
		}
	}
	if !c.NoStart {
		logger.Info("Starting Aerospike")
		out := cluster.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"systemctl", "start", "aerospike"},
				Stdin:          nil,
				Stdout:         nil,
				Stderr:         nil,
				SessionTimeout: time.Minute,
				Env:            []*sshexec.Env{},
				Terminal:       false,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: c.Threads,
		})
		for _, o := range out {
			if o.Output.Err != nil {
				errs = errors.Join(errs, fmt.Errorf("%s:%d: %s (%s) (%s)", o.Instance.ClusterName, o.Instance.NodeNo, o.Output.Err, o.Output.Stdout, o.Output.Stderr))
			}
		}
	}
	return cluster.Describe(), errs
}

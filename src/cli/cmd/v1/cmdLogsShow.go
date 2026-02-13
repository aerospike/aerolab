package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

type LogsShowCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Node        TypeNode        `short:"l" long:"node" description:"Node number" default:"1"`
	Journal     bool            `short:"j" long:"journal" description:"Attempt to get logs from journald instead of log files"`
	LogLocation string          `short:"p" long:"path" description:"Aerospike log file path" default:"/var/log/aerospike.log"`
	Follow      bool            `short:"f" long:"follow" description:"Follow logs instead of displaying full log" webdisable:"true"`
	MaxRetries  int             `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep  time.Duration   `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *LogsShowCmd) Execute(args []string) error {
	cmd := []string{"logs", "show"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	_, err = c.ShowLogs(system, system.Backend.GetInventory(), system.Logger, args, "show")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *LogsShowCmd) ShowLogs(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"logs", action}, c, args...)
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
		// Pre-validation loop - check ALL clusters exist before processing any
		for _, clusterName := range clusters {
			if inventory.Instances.WithClusterName(clusterName).WithState(backends.LifeCycleStateRunning).Count() == 0 {
				return nil, fmt.Errorf("cluster %s not found", clusterName)
			}
		}
		// Processing loop - actually perform the operation
		for _, clusterName := range clusters {
			c.ClusterName = TypeClusterName(clusterName)
			inst, err := c.ShowLogs(system, inventory, logger, args, action)
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

	// Filter to specific node
	cluster = cluster.WithNodeNo(c.Node.Int())
	if cluster.Count() == 0 {
		return nil, fmt.Errorf("node %d not found in cluster %s", c.Node.Int(), c.ClusterName.String())
	}

	// Filter to running instances
	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		return nil, fmt.Errorf("node %d is not running in cluster %s", c.Node.Int(), c.ClusterName.String())
	}

	instance := cluster.Describe()[0]
	logger.Info("Showing logs for node %d in cluster %s", c.Node.Int(), c.ClusterName.String())

	var command []string
	if c.Journal {
		command = []string{"journalctl", "-u", "aerospike"}
		if c.Follow {
			command = append(command, "-f")
		} else {
			command = append(command, "--no-pager")
		}
	} else {
		if c.Follow {
			command = []string{"tail", "-f", c.LogLocation}
		} else {
			command = []string{"cat", c.LogLocation}
		}
	}

	// Execute command on the instance
	output := instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        command,
			Stdin:          os.Stdin,  // Required for Ctrl+C handling in follow mode
			Stdout:         os.Stdout, // Direct output to stdout
			Stderr:         os.Stderr, // Direct output to stderr
			SessionTimeout: 0,         // No timeout for follow mode
			Env:            []*sshexec.Env{},
			Terminal:       c.Follow, // Only use terminal mode for follow
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
		MaxRetries:      c.MaxRetries,
		RetrySleep:      c.RetrySleep,
	})

	// Check for errors
	if output.Output.Err != nil {
		return nil, fmt.Errorf("failed to show logs: %s (%s) (%s)", output.Output.Err, output.Output.Stdout, output.Output.Stderr)
	}

	return cluster.Describe(), nil
}

package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

// AgiStopCmd stops an AGI instance gracefully.
// It first stops the AGI services, then stops the underlying instance.
//
// Usage:
//
//	aerolab agi stop -n myagi
type AgiStopCmd struct {
	Name    TypeAgiClusterName `short:"n" long:"name" description:"AGI instance name" default:"agi"`
	Force   bool               `short:"f" long:"force" description:"Force stop without waiting for services"`
	NoWait  bool               `short:"w" long:"no-wait" description:"Do not wait for the instance to stop"`
	DryRun  bool               `short:"d" long:"dry-run" description:"Print what would be done but don't do it"`
	Threads int                `short:"t" long:"threads" description:"Threads to use for service stop" default:"1"`
	Help    HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi stop.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiStopCmd) Execute(args []string) error {
	cmd := []string{"agi", "stop"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	_, err = c.StopAGI(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// StopAGI stops an AGI instance.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - backends.InstanceList: The stopped AGI instance
//   - error: nil on success, or an error describing what failed
func (c *AgiStopCmd) StopAGI(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "stop"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Find AGI instance
	instances := inventory.Instances.WithTags(map[string]string{
		"aerolab.type": "agi",
	}).WithClusterName(c.Name.String()).WithState(backends.LifeCycleStateRunning).Describe()

	if instances.Count() == 0 {
		// Check if it's already stopped
		stopped := inventory.Instances.WithTags(map[string]string{
			"aerolab.type": "agi",
		}).WithClusterName(c.Name.String()).WithState(backends.LifeCycleStateStopped).Describe()
		if stopped.Count() > 0 {
			return stopped, fmt.Errorf("AGI instance %s is already stopped", c.Name)
		}
		return nil, fmt.Errorf("AGI instance %s not found or not in running state", c.Name)
	}

	if c.DryRun {
		logger.Info("DRY-RUN: Would stop AGI instance %s", c.Name)
		return instances, nil
	}

	// Stop AGI services gracefully (unless forced)
	if !c.Force {
		logger.Info("Stopping AGI services gracefully")
		script := `for service in agi-ingest agi-proxy agi-grafanafix agi-plugin grafana-server aerospike; do
    systemctl stop "$service" 2>/dev/null || true
done
sync
`

		instances.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "-c", script},
				SessionTimeout: 5 * time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: c.Threads,
		})
	}

	// Stop the instance
	logger.Info("Stopping AGI instance %s", c.Name)
	waitDur := 10 * time.Minute
	if c.NoWait {
		waitDur = 0
	}

	err := instances.Stop(c.Force, waitDur)
	if err != nil {
		return nil, fmt.Errorf("failed to stop instance: %w", err)
	}

	logger.Info("AGI instance %s stopped", c.Name)
	return instances, nil
}

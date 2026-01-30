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

// AgiDestroyCmd destroys an AGI instance.
// This terminates the instance but preserves any associated EFS/volumes
// so the AGI can be recreated with the same data.
//
// Usage:
//
//	aerolab agi destroy -n myagi
//	aerolab agi destroy -n myagi --force
type AgiDestroyCmd struct {
	Name   TypeAgiClusterName `short:"n" long:"name" description:"AGI instance name(s), comma-separated" default:"agi"`
	Force  bool               `short:"f" long:"force" description:"Do not ask for confirmation"`
	NoWait bool               `short:"w" long:"no-wait" description:"Do not wait for the instance to terminate"`
	DryRun bool               `short:"d" long:"dry-run" description:"Print what would be done but don't do it"`
	Help   HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi destroy.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiDestroyCmd) Execute(args []string) error {
	cmd := []string{"agi", "destroy"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	_, err = c.DestroyAGI(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// DestroyAGI destroys AGI instance(s).
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - backends.InstanceList: The destroyed AGI instance(s)
//   - error: nil on success, or an error describing what failed
func (c *AgiDestroyCmd) DestroyAGI(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "destroy"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Handle comma-separated names
	names := strings.Split(c.Name.String(), ",")
	var allInstances backends.InstanceList

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		// Find AGI instance(s)
		instances := inventory.Instances.WithTags(map[string]string{
			"aerolab.type": "agi",
		}).WithClusterName(name).WithNotState(backends.LifeCycleStateTerminating, backends.LifeCycleStateTerminated).Describe()

		if instances.Count() == 0 {
			logger.Warn("AGI instance %s not found", name)
			continue
		}

		allInstances = append(allInstances, instances...)
	}

	if len(allInstances) == 0 {
		return nil, fmt.Errorf("no AGI instances found matching: %s", c.Name)
	}

	if c.DryRun {
		logger.Info("DRY-RUN: Would destroy %d AGI instance(s)", len(allInstances))
		for _, inst := range allInstances {
			logger.Info("  - %s (state: %s)", inst.ClusterName, inst.InstanceState.String())
		}
		return allInstances, nil
	}

	// Confirmation prompt
	if !c.Force && IsInteractive() {
		choice, quitting, err := choice.Choice("Are you sure you want to destroy "+strconv.Itoa(len(allInstances))+" AGI instance(s)? (EFS/volumes will be preserved)", choice.Items{
			choice.Item("Yes"),
			choice.Item("No"),
		})
		if err != nil {
			return nil, err
		}
		if quitting {
			return nil, errors.New("aborted")
		}
		if choice == "No" {
			return nil, errors.New("aborted")
		}
	}

	// Destroy instances
	logger.Info("Destroying %d AGI instance(s)", len(allInstances))
	for _, inst := range allInstances {
		logger.Debug("  - %s", inst.ClusterName)
	}

	waitDur := 10 * time.Minute
	if c.NoWait {
		waitDur = 0
	}

	err := allInstances.Terminate(waitDur)
	if err != nil {
		return allInstances, fmt.Errorf("failed to destroy instances: %w", err)
	}

	logger.Info("AGI instance(s) destroyed")

	// Only show volume preservation note if volumes exist for this AGI
	backendType := system.Opts.Config.Backend.Type
	if backendType == "aws" || backendType == "gcp" {
		// Check if any volumes exist for the destroyed AGI instances
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			vols := inventory.Volumes.WithName(name).WithDeleteOnTermination(false)
			if vols.Count() > 0 {
				logger.Info("Note: EFS/volumes have been preserved. Use 'aerolab agi delete' to also delete volumes.")
				break
			}
		}
	}

	return allInstances, nil
}

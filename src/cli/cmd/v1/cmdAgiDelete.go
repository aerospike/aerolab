package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/rglonek/logger"
)

// AgiDeleteCmd destroys an AGI instance AND its associated EFS/volume.
// This is a destructive operation that permanently deletes all AGI data.
//
// Usage:
//
//	aerolab agi delete -n myagi
//	aerolab agi delete -n myagi --force
type AgiDeleteCmd struct {
	Name   TypeAgiClusterName `short:"n" long:"name" description:"AGI instance name(s), comma-separated" default:"agi"`
	Force  bool               `short:"f" long:"force" description:"Do not ask for confirmation"`
	NoWait bool               `short:"w" long:"no-wait" description:"Do not wait for the instance to terminate"`
	DryRun bool               `short:"d" long:"dry-run" description:"Print what would be done but don't do it"`
	Help   HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi delete.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiDeleteCmd) Execute(args []string) error {
	cmd := []string{"agi", "delete"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	_, err = c.DeleteAGI(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// DeleteAGI destroys AGI instance(s) and their associated volumes.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - backends.InstanceList: The deleted AGI instance(s)
//   - error: nil on success, or an error describing what failed
func (c *AgiDeleteCmd) DeleteAGI(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "delete"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	backendType := system.Opts.Config.Backend.Type

	// Handle comma-separated names
	names := strings.Split(c.Name.String(), ",")
	var allInstances backends.InstanceList
	var volumeNames []string

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
		} else {
			allInstances = append(allInstances, instances...)
		}

		// Track volume names for cloud backends
		if backendType == "aws" || backendType == "gcp" {
			volumeNames = append(volumeNames, name)
		}
	}

	// Find associated volumes (exclude volumes with deleteOnTermination=true as AWS deletes those automatically)
	var volumesToDelete backends.VolumeList
	for _, volName := range volumeNames {
		vols := inventory.Volumes.WithName(volName).WithDeleteOnTermination(false)
		if vols.Count() > 0 {
			volumesToDelete = append(volumesToDelete, vols.Describe()...)
		}
	}

	if len(allInstances) == 0 && len(volumesToDelete) == 0 {
		return nil, fmt.Errorf("no AGI instances or volumes found matching: %s", c.Name)
	}

	if c.DryRun {
		logger.Info("DRY-RUN: Would delete %d AGI instance(s) and %d volume(s)", len(allInstances), len(volumesToDelete))
		for _, inst := range allInstances {
			logger.Info("  Instance: %s (state: %s)", inst.ClusterName, inst.InstanceState.String())
		}
		for _, vol := range volumesToDelete {
			logger.Info("  Volume: %s", vol.Name)
		}
		return allInstances, nil
	}

	// Confirmation prompt with stronger warning
	if !c.Force && IsInteractive() {
		msg := fmt.Sprintf("Are you sure you want to DELETE %d AGI instance(s) and %d volume(s)?\n"+
			"WARNING: This will permanently delete ALL AGI data including processed logs!",
			len(allInstances), len(volumesToDelete))
		choice, quitting, err := choice.Choice(msg, choice.Items{
			choice.Item("Yes, delete everything"),
			choice.Item("No, keep my data"),
		})
		if err != nil {
			return nil, err
		}
		if quitting {
			return nil, errors.New("aborted")
		}
		if choice == "No, keep my data" {
			return nil, errors.New("aborted")
		}
	}

	// Destroy instances first
	if len(allInstances) > 0 {
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
			logger.Warn("Failed to destroy some instances: %s", err)
		}
	}

	// Delete volumes
	if len(volumesToDelete) > 0 {
		logger.Info("Deleting %d volume(s)", len(volumesToDelete))
		for _, vol := range volumesToDelete {
			logger.Debug("  - %s", vol.Name)
		}
		err := volumesToDelete.DeleteVolumes(nil, 10*time.Minute)
		if err != nil {
			logger.Warn("Some volumes could not be deleted: %s", err)
		}
	}

	// Cleanup any orphaned DNS records (handles cases where instance was already
	// terminated but DNS records remain, e.g., spot instance termination)
	if backendType == "aws" || backendType == "gcp" {
		logger.Debug("Cleaning up orphaned DNS records")
		if err := system.Backend.CleanupDNS(); err != nil {
			logger.Warn("Failed to cleanup DNS records: %s", err)
		}
	}

	logger.Info("AGI instance(s) and associated data deleted")
	return allInstances, nil
}

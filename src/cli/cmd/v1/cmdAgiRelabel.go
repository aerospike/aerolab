package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

// AgiRelabelCmd changes the label of an AGI instance.
// The label is stored in /opt/agi/label and is also updated in instance tags.
type AgiRelabelCmd struct {
	ClusterName TypeAgiClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	NewLabel    string          `short:"l" long:"label" description:"New label for the AGI instance" required:"true"`
	GcpZone     string          `short:"z" long:"zone" description:"GCP only: zone where the instance is"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi change-label.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiRelabelCmd) Execute(args []string) error {
	cmd := []string{"agi", "change-label"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.Relabel(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// Relabel changes the label of the AGI instance.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiRelabelCmd) Relabel(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "change-label"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Validate new label
	if c.NewLabel == "" {
		return fmt.Errorf("new label is required")
	}

	// Get AGI instance
	instance := inventory.Instances.WithClusterName(string(c.ClusterName))
	if instance.Count() == 0 {
		return fmt.Errorf("AGI instance %s not found", c.ClusterName)
	}
	inst := instance.Describe()[0]

	// Update instance tags
	backendType := system.Opts.Config.Backend.Type
	switch backendType {
	case "aws":
		err := c.updateAWSTags(system, inst, logger)
		if err != nil {
			return err
		}
	case "gcp":
		err := c.updateGCPLabels(system, inst, logger)
		if err != nil {
			return err
		}
	case "docker":
		// Docker labels cannot be changed after container creation
		// We'll just update the file
		logger.Warn("Docker labels cannot be updated after container creation, updating file only")
	}

	// Update label file on instance if running
	if inst.InstanceState == backends.LifeCycleStateRunning {
		confs, err := backends.InstanceList{inst}.GetSftpConfig("root")
		if err != nil {
			logger.Warn("Could not get SFTP config to update label file: %s", err)
		} else {
			for _, conf := range confs {
				cli, err := sshexec.NewSftp(conf)
				if err != nil {
					logger.Warn("Could not create SFTP client: %s", err)
					continue
				}
				defer cli.Close()

				err = cli.WriteFile(true, &sshexec.FileWriter{
					DestPath:    "/opt/agi/label",
					Source:      bytes.NewReader([]byte(c.NewLabel)),
					Permissions: 0644,
				})
				if err != nil {
					logger.Warn("Could not update label file: %s", err)
				}
			}
		}

		// Signal proxy to reload (it monitors the label file)
		backends.InstanceList{inst}.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "-c", "kill -HUP $(systemctl show --property MainPID --value agi-proxy) 2>/dev/null || true"},
				SessionTimeout: time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
	}

	logger.Info("Label changed to: %s", c.NewLabel)
	return nil
}

// updateAWSTags updates the AWS instance tags.
func (c *AgiRelabelCmd) updateAWSTags(system *System, inst *backends.Instance, logger *logger.Logger) error {
	// Update the agiLabel tag
	newTags := map[string]string{
		"agiLabel": c.NewLabel,
	}

	err := backends.InstanceList{inst}.AddTags(newTags)
	if err != nil {
		return fmt.Errorf("failed to update AWS tags: %w", err)
	}

	return nil
}

// updateGCPLabels updates the GCP instance labels.
func (c *AgiRelabelCmd) updateGCPLabels(system *System, inst *backends.Instance, logger *logger.Logger) error {
	// GCP labels have restrictions (lowercase, hyphens, etc.)
	// Convert the label to a valid GCP label
	gcpLabel := strings.ToLower(strings.ReplaceAll(c.NewLabel, " ", "-"))

	newTags := map[string]string{
		"agilabel": gcpLabel,
	}

	err := backends.InstanceList{inst}.AddTags(newTags)
	if err != nil {
		return fmt.Errorf("failed to update GCP labels: %w", err)
	}

	return nil
}


package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/rglonek/logger"
)

type CloudDatabasesDeleteCmd struct {
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	DatabaseID string  `short:"d" long:"database-id" description:"Database ID"`
	Wait       bool    `short:"w" long:"wait" description:"Wait until database status is decommissioned"`
	Force      bool    `short:"f" long:"force" description:"Skip confirmation prompt"`
}

func (c *CloudDatabasesDeleteCmd) Execute(args []string) error {
	cmd := []string{"cloud", "db", "delete"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	if c.DatabaseID == "" {
		return fmt.Errorf("database ID is required")
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	// Ask for confirmation if interactive and not forced
	if IsInteractive() && !c.Force {
		choice, quitting, err := choice.Choice(fmt.Sprintf("Are you sure you want to delete database %s?", c.DatabaseID), choice.Items{
			choice.Item("Yes"),
			choice.Item("No"),
		})
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
		if quitting {
			return Error(errors.New("aborted"), system, cmd, c, args)
		}
		switch choice {
		case "No":
			return Error(errors.New("aborted"), system, cmd, c, args)
		}
	}

	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Delete route before deleting database (only for AWS backend)
	if system.Opts.Config.Backend.Type == "aws" {
		err = c.deleteRouteIfExists(system, client, system.Logger)
		if err != nil {
			system.Logger.Warn("Failed to delete route before database deletion: %s", err.Error())
			// Continue with database deletion even if route deletion fails
		}
	}

	path := fmt.Sprintf("%s/%s", cloudDbPath, c.DatabaseID)
	err = client.Delete(path)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	if c.Wait {
		err = c.waitForDatabaseDecommissioned(client, c.DatabaseID)
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// deleteRouteIfExists deletes the route table entry if it exists.
// This should be called before deleting the database to clean up the route.
func (c *CloudDatabasesDeleteCmd) deleteRouteIfExists(system *System, client *cloud.Client, logger *logger.Logger) error {
	// Get database information to extract CIDR block and region
	logger.Info("Getting database information to extract route details...")
	var dbResult interface{}
	dbPath := fmt.Sprintf("%s/%s", cloudDbPath, c.DatabaseID)
	err := client.Get(dbPath, &dbResult)
	if err != nil {
		return fmt.Errorf("failed to get database information: %w", err)
	}

	dbMap, ok := dbResult.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected database response type: %T", dbResult)
	}

	// Extract CIDR block from infrastructure
	infrastructure, ok := dbMap["infrastructure"].(map[string]interface{})
	if !ok {
		logger.Debug("Infrastructure field not found in database response, skipping route deletion")
		return nil
	}

	cidrBlock, ok := infrastructure["cidrBlock"].(string)
	if !ok || cidrBlock == "" {
		logger.Debug("CIDR block not found in infrastructure, skipping route deletion")
		return nil
	}

	region, ok := infrastructure["region"].(string)
	if !ok || region == "" {
		logger.Debug("Region not found in infrastructure, skipping route deletion")
		return nil
	}

	logger.Info("Found CIDR block: %s, region: %s", cidrBlock, region)

	// Get VPC peerings to find peering connection ID and VPC ID
	logger.Info("Getting VPC peerings to find peering connection details...")
	var peeringsResult interface{}
	peeringsPath := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, c.DatabaseID)
	err = client.Get(peeringsPath, &peeringsResult)
	if err != nil {
		logger.Debug("Failed to get VPC peerings list: %s, skipping route deletion", err.Error())
		return nil
	}

	peeringsMap, ok := peeringsResult.(map[string]interface{})
	if !ok {
		logger.Debug("Unexpected VPC peerings response type: %T, skipping route deletion", peeringsResult)
		return nil
	}

	peerings, ok := peeringsMap["vpcPeerings"].([]interface{})
	if !ok {
		logger.Debug("No VPC peerings found, skipping route deletion")
		return nil
	}

	// Find the peering to extract peering connection ID and VPC ID
	var peeringConnectionID string
	var vpcID string
	for _, peering := range peerings {
		peeringMap, ok := peering.(map[string]interface{})
		if !ok {
			continue
		}

		peeringId, ok := peeringMap["peeringId"].(string)
		if !ok || peeringId == "" {
			// Try alternative field name
			peeringId, _ = peeringMap["peering_id"].(string)
		}

		peeringVpcID, ok := peeringMap["vpcId"].(string)
		if !ok {
			// Try alternative field name
			peeringVpcID, _ = peeringMap["vpc_id"].(string)
		}

		if peeringId != "" && peeringVpcID != "" {
			peeringConnectionID = peeringId
			vpcID = peeringVpcID
			logger.Info("Found peering connection ID: %s, VPC ID: %s", peeringConnectionID, vpcID)
			break
		}
	}

	if peeringConnectionID == "" || vpcID == "" {
		logger.Debug("Peering connection ID or VPC ID not found, skipping route deletion")
		return nil
	}

	// Delete the route
	logger.Info("Deleting route in VPC %s for CIDR block %s via peering connection %s", vpcID, cidrBlock, peeringConnectionID)
	err = system.Backend.DeleteRoute(backends.BackendTypeAWS, vpcID, peeringConnectionID, cidrBlock)
	if err != nil {
		// Check if route doesn't exist (this is okay - might have been deleted already)
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "does not exist") || strings.Contains(errStr, "not found") || strings.Contains(errStr, "no matching routes") {
			logger.Debug("Route not found or already deleted: %s", err.Error())
			return nil
		}
		return fmt.Errorf("failed to delete route: %w", err)
	}

	logger.Info("Route deleted successfully")
	return nil
}

// waitForDatabaseDecommissioned polls the database list at 10 second intervals
// until the database status is decommissioned
func (c *CloudDatabasesDeleteCmd) waitForDatabaseDecommissioned(client *cloud.Client, databaseID string) error {
	timeout := time.Hour
	interval := 10 * time.Second
	startTime := time.Now()

	fmt.Printf("Waiting for database %s to be decommissioned...\n", databaseID)

	for {
		if time.Since(startTime) > timeout {
			return fmt.Errorf("timeout waiting for database decommissioning after %v", timeout)
		}

		var result interface{}
		// Call list endpoint without status_ne filter to include decommissioned databases
		path := cloudDbPath
		err := client.Get(path, &result)
		if err != nil {
			return fmt.Errorf("failed to get database list: %w", err)
		}

		resultMap, ok := result.(map[string]interface{})
		if !ok {
			return fmt.Errorf("unexpected response type: %T", result)
		}

		databases, ok := resultMap["databases"].([]interface{})
		if !ok {
			return fmt.Errorf("databases field not found or invalid in response")
		}

		// Look for the database with the matching ID
		found := false
		for _, db := range databases {
			dbMap, ok := db.(map[string]interface{})
			if !ok {
				continue
			}

			id, ok := dbMap["id"].(string)
			if !ok || id != databaseID {
				continue
			}

			// Found the database, check its status
			found = true

			// Check status - try top-level status first, then health.status
			var status string
			var statusOK bool

			// Try top-level status first
			statusVal, exists := dbMap["status"]
			if exists && statusVal != nil {
				status, statusOK = statusVal.(string)
			}

			// If not found at top level, try health.status
			if !statusOK {
				health, healthExists := dbMap["health"].(map[string]interface{})
				if healthExists {
					healthStatusVal, healthStatusExists := health["status"]
					if healthStatusExists && healthStatusVal != nil {
						status, statusOK = healthStatusVal.(string)
					}
				}
			}

			// If still not found, treat as decommissioned
			if !statusOK || status == "" {
				fmt.Printf("Database status field not found or null, assuming decommissioned\n")
				fmt.Printf("Database decommissioned successfully\n")
				return nil
			}

			fmt.Printf("Database status: %s\n", status)

			if status == "decommissioned" {
				fmt.Printf("Database decommissioned successfully\n")
				return nil
			}

			fmt.Printf("Database still %s, waiting %v...\n", status, interval)
			time.Sleep(interval)
			break
		}

		// Database not found in list - assume it's already deleted/decommissioned
		if !found {
			fmt.Printf("Database not found in list, assuming it's already decommissioned\n")
			return nil
		}
	}
}

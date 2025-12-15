package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type CloudDatabasesUpdateCmd struct {
	DatabaseID   string  `short:"d" long:"database-id" description:"Database ID"`
	Name         string  `short:"n" long:"name" description:"Name of the database"`
	InstanceType string  `short:"i" long:"instance-type" description:"Instance type (vertical scaling)"`
	ClusterSize  int     `long:"cluster-size" description:"Number of nodes in cluster (horizontal scaling)"`
	Wait         bool    `long:"wait" description:"Wait for database update to complete"`
	Help         HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudDatabasesUpdateCmd) Execute(args []string) error {
	cmd := []string{"cloud", "db", "update"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	var stdout, stderr *os.File
	var stdin io.ReadCloser
	if system.logLevel >= 5 {
		stdout = os.Stdout
		stderr = os.Stderr
		stdin = io.NopCloser(os.Stdin)
	}
	err = c.UpdateCloudDb(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *CloudDatabasesUpdateCmd) UpdateCloudDb(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) error {
	if c.DatabaseID == "" {
		return fmt.Errorf("database ID is required")
	}
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cloud", "db", "update"}, c, args...)
		if err != nil {
			return err
		}
	}
	if logger == nil {
		logger = system.Logger
	}

	logger.Info("Updating cloud database: %s", c.DatabaseID)

	// create cloud client
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	// Build update request - only include fields that are provided
	request := cloud.UpdateDatabaseRequest{}

	if c.Name != "" {
		request.Name = c.Name
	}

	// Build infrastructure if instanceType is provided
	// Note: Only instanceType can be updated (provider, region, availabilityZoneCount are read-only)
	if c.InstanceType != "" {
		infrastructure := cloud.Infrastructure{
			InstanceType: c.InstanceType,
		}
		request.Infrastructure = &infrastructure
		logger.Info("Updating infrastructure: instanceType=%s", c.InstanceType)
	}

	// Build aerospikeCloud if clusterSize is provided
	// Note: All AerospikeCloud types share AerospikeCloudShared which contains ClusterSize
	if c.ClusterSize > 0 {
		// Use AerospikeCloudMemory as the simplest structure - the API should handle partial updates
		aerospikeCloud := cloud.AerospikeCloudMemory{
			AerospikeCloudShared: cloud.AerospikeCloudShared{
				ClusterSize: c.ClusterSize,
			},
		}
		request.AerospikeCloud = aerospikeCloud
		logger.Info("Updating aerospikeCloud: clusterSize=%d", c.ClusterSize)
	}

	// Validate that at least one field is being updated
	if request.Name == "" && request.Infrastructure == nil && request.AerospikeCloud == nil {
		return fmt.Errorf("at least one update parameter must be provided")
	}

	var result interface{}

	// Pretty print the request for debugging
	requestJson, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		logger.Error("failed to marshal request for logging purposes: %s", err.Error())
	} else {
		logger.Debug("Update request:\n%s", string(requestJson))
	}
	path := fmt.Sprintf("%s/%s", cloudDbPath, c.DatabaseID)
	err = client.Patch(path, request, &result)
	if err != nil {
		return err
	}

	logger.Info("Database update successfully queued")

	// Extract database ID from result
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected response type: %T", result)
	}

	databaseID, ok := resultMap["id"].(string)
	if !ok {
		return fmt.Errorf("id field not found or invalid in response")
	}

	// Wait for update to complete if --wait is specified
	if c.Wait {
		logger.Info("Waiting for database update to complete...")
		dbResult, err := c.waitForDatabaseUpdateComplete(client, databaseID, logger)
		// Print the database result regardless of success or error
		if dbResult != nil {
			resultJson, err := json.MarshalIndent(dbResult, "", "  ")
			if err != nil {
				logger.Error("failed to marshal database result for logging purposes: %s", err.Error())
			} else {
				logger.Info("Database update result:\n%s", string(resultJson))
			}
		}
		if err != nil {
			return fmt.Errorf("failed to wait for database update: %w", err)
		}
		logger.Info("Database update completed")
	} else {
		// json-dump result in logger.Debug for debugging purposes
		resultJson, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			logger.Error("failed to marshal database update result for logging purposes: %s", err.Error())
		}
		logger.Info("Database update result:\n%s", string(resultJson))
	}

	return nil
}

// waitForDatabaseUpdateComplete waits for the database update to complete
// It polls the database list until status != "updating"
// Returns the database result map and an error (if any)
func (c *CloudDatabasesUpdateCmd) waitForDatabaseUpdateComplete(client *cloud.Client, databaseID string, logger *logger.Logger) (map[string]interface{}, error) {
	timeout := time.Hour
	interval := 10 * time.Second

	// Wait 10 seconds before the first check
	logger.Debug("Waiting 10 seconds before first check...")
	time.Sleep(10 * time.Second)

	startTime := time.Now()
	var lastDatabaseResult map[string]interface{}

	for {
		if time.Since(startTime) > timeout {
			if lastDatabaseResult != nil {
				return lastDatabaseResult, fmt.Errorf("timeout waiting for database update after %v", timeout)
			}
			return nil, fmt.Errorf("timeout waiting for database update after %v", timeout)
		}

		var result interface{}
		path := cloudDbPath
		err := client.Get(path, &result)
		if err != nil {
			if lastDatabaseResult != nil {
				return lastDatabaseResult, fmt.Errorf("failed to get database list: %w", err)
			}
			return nil, fmt.Errorf("failed to get database list: %w", err)
		}

		resultMap, ok := result.(map[string]interface{})
		if !ok {
			if lastDatabaseResult != nil {
				return lastDatabaseResult, fmt.Errorf("unexpected response type: %T", result)
			}
			return nil, fmt.Errorf("unexpected response type: %T", result)
		}

		databases, ok := resultMap["databases"].([]interface{})
		if !ok {
			if lastDatabaseResult != nil {
				return lastDatabaseResult, fmt.Errorf("databases field not found or invalid in response")
			}
			return nil, fmt.Errorf("databases field not found or invalid in response")
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
			lastDatabaseResult = dbMap // Store the last result

			status, ok := dbMap["status"].(string)
			if !ok {
				return lastDatabaseResult, fmt.Errorf("status field not found or invalid in response")
			}

			logger.Info("Database status: %s", status)

			if status != "updating" {
				logger.Info("Database update complete, status: %s", status)
				return lastDatabaseResult, nil
			}

			logger.Info("Database still updating, waiting %v...", interval)
			time.Sleep(interval)
			break
		}

		// Database not found in list
		if !found {
			if lastDatabaseResult != nil {
				return lastDatabaseResult, fmt.Errorf("database %s not found in list", databaseID)
			}
			return nil, fmt.Errorf("database %s not found in list", databaseID)
		}
	}
}

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

type CloudClustersUpdateCmd struct {
	ClusterID    string  `short:"c" long:"cluster-id" description:"Cluster ID"`
	Name         string  `short:"n" long:"name" description:"Name of the cluster"`
	InstanceType string  `short:"i" long:"instance-type" description:"Instance type (vertical scaling)"`
	ClusterSize  int     `short:"s" long:"cluster-size" description:"Number of nodes in cluster (horizontal scaling)"`
	Wait         bool    `short:"w" long:"wait" description:"Wait for cluster update to complete"`
	Help         HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudClustersUpdateCmd) Execute(args []string) error {
	cmd := []string{"cloud", "clusters", "update"}
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
	err = c.UpdateCloudCluster(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *CloudClustersUpdateCmd) UpdateCloudCluster(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) error {
	if c.ClusterID == "" {
		return fmt.Errorf("cluster ID is required")
	}
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cloud", "clusters", "update"}, c, args...)
		if err != nil {
			return err
		}
	}
	if logger == nil {
		logger = system.Logger
	}

	logger.Info("Updating cloud cluster: %s", c.ClusterID)

	// create cloud client
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	// Build update request - only include fields that are provided
	request := cloud.UpdateClusterRequest{}

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
	path := fmt.Sprintf("%s/%s", cloudDbPath, c.ClusterID)
	err = client.Patch(path, request, &result)
	if err != nil {
		return err
	}

	logger.Info("Cluster update successfully queued")

	// Extract cluster ID from result
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected cluster response type: %T", result)
	}

	clusterID, ok := resultMap["id"].(string)
	if !ok {
		return fmt.Errorf("id field not found or invalid in cluster response")
	}

	// Wait for update to complete if --wait is specified
	if c.Wait {
		logger.Info("Waiting for cluster update to complete...")
		clusterResult, err := c.waitForClusterUpdateComplete(client, clusterID, logger)
		// Print the cluster result regardless of success or error
		if clusterResult != nil {
			resultJson, err := json.MarshalIndent(clusterResult, "", "  ")
			if err != nil {
				logger.Error("failed to marshal cluster result for logging purposes: %s", err.Error())
			} else {
				logger.Info("Cluster update result:\n%s", string(resultJson))
			}
		}
		if err != nil {
			return fmt.Errorf("failed to wait for cluster update: %w", err)
		}
		logger.Info("Cluster update completed")
	} else {
		// json-dump result in logger.Debug for debugging purposes
		resultJson, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			logger.Error("failed to marshal cluster update result for logging purposes: %s", err.Error())
		}
		logger.Info("Cluster update result:\n%s", string(resultJson))
	}

	return nil
}

// waitForClusterUpdateComplete waits for the cluster update to complete
// It polls the cluster list until status != "updating"
// Returns the cluster result map and an error (if any)
func (c *CloudClustersUpdateCmd) waitForClusterUpdateComplete(client *cloud.Client, clusterID string, logger *logger.Logger) (map[string]interface{}, error) {
	timeout := time.Hour
	interval := 10 * time.Second

	// Wait 10 seconds before the first check
	logger.Debug("Waiting 10 seconds before first check...")
	time.Sleep(10 * time.Second)

	startTime := time.Now()
	var lastClusterResult map[string]interface{}

	for {
		if time.Since(startTime) > timeout {
			if lastClusterResult != nil {
				return lastClusterResult, fmt.Errorf("timeout waiting for cluster update after %v", timeout)
			}
			return nil, fmt.Errorf("timeout waiting for cluster update after %v", timeout)
		}

		var result interface{}
		path := cloudDbPath
		err := client.Get(path, &result)
		if err != nil {
			if lastClusterResult != nil {
				return lastClusterResult, fmt.Errorf("failed to get cluster list: %w", err)
			}
			return nil, fmt.Errorf("failed to get cluster list: %w", err)
		}

		resultMap, ok := result.(map[string]interface{})
		if !ok {
			if lastClusterResult != nil {
				return lastClusterResult, fmt.Errorf("unexpected response type: %T", result)
			}
			return nil, fmt.Errorf("unexpected response type: %T", result)
		}

		clusters, ok := resultMap["clusters"].([]interface{})
		if !ok {
			if lastClusterResult != nil {
				return lastClusterResult, fmt.Errorf("clusters field not found or invalid in response")
			}
			return nil, fmt.Errorf("clusters field not found or invalid in response")
		}

		// Look for the cluster with the matching ID
		found := false
		for _, db := range clusters {
			dbMap, ok := db.(map[string]interface{})
			if !ok {
				continue
			}

			id, ok := dbMap["id"].(string)
			if !ok || id != clusterID {
				continue
			}

			// Found the cluster, check its status
			found = true
			lastClusterResult = dbMap // Store the last result

			status, ok := dbMap["status"].(string)
			if !ok {
				return lastClusterResult, fmt.Errorf("status field not found or invalid in response")
			}

			logger.Info("Cluster status: %s", status)

			if status != "updating" {
				logger.Info("Cluster update complete, status: %s", status)
				return lastClusterResult, nil
			}

			logger.Info("Cluster still updating, waiting %v...", interval)
			time.Sleep(interval)
			break
		}

		// Cluster not found in list
		if !found {
			if lastClusterResult != nil {
				return lastClusterResult, fmt.Errorf("cluster %s not found in list", clusterID)
			}
			return nil, fmt.Errorf("cluster %s not found in list", clusterID)
		}
	}
}

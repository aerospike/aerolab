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
	DryRun       bool    `long:"dry-run" description:"Print the JSON request that would be sent without actually sending it"`
	CustomConf   string  `short:"o" long:"custom-conf" description:"Path to custom JSON configuration file (full request body or aerospikeServer section only). Custom config takes precedence over flags."`
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

	// Apply custom configuration if provided
	if c.CustomConf != "" {
		logger.Info("Loading custom configuration from: %s", c.CustomConf)
		customRequest, err := c.loadAndMergeCustomConfig(c.CustomConf, request, logger)
		if err != nil {
			return fmt.Errorf("failed to load custom configuration: %w", err)
		}
		request = customRequest
		logger.Info("Custom configuration applied successfully")
	}

	// Validate that at least one field is being updated
	hasUpdates := request.Name != "" || request.Infrastructure != nil || request.AerospikeCloud != nil || request.AerospikeServer != nil || request.Logging != nil
	if !hasUpdates {
		return fmt.Errorf("at least one update parameter must be provided")
	}

	// Pretty print the request
	requestJson, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Handle dry-run mode
	if c.DryRun {
		logger.Info("=== DRY RUN MODE ===")
		logger.Info("")
		logger.Info("Would update cluster %s with the following request:", c.ClusterID)
		logger.Info("")
		fmt.Println(string(requestJson))
		logger.Info("")
		logger.Info("API endpoint: PATCH %s/%s", cloudDbPath, c.ClusterID)
		logger.Info("")
		logger.Info("No changes were made.")
		return nil
	}

	logger.Debug("Update request:\n%s", string(requestJson))

	var result interface{}
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

// loadAndMergeCustomConfig loads a custom JSON configuration file and merges it with the base request.
// It auto-detects whether the JSON is a full request body or just the aerospikeServer section.
// Custom configuration takes precedence over the base request values.
func (c *CloudClustersUpdateCmd) loadAndMergeCustomConfig(filePath string, baseRequest cloud.UpdateClusterRequest, logger *logger.Logger) (cloud.UpdateClusterRequest, error) {
	// Read the custom config file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return baseRequest, fmt.Errorf("failed to read custom config file: %w", err)
	}

	// Parse as generic map to detect the type
	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return baseRequest, fmt.Errorf("failed to parse custom config JSON: %w", err)
	}

	// Detect if this is a full request or just aerospikeServer section
	// Full request has keys like: name, infrastructure, aerospikeCloud, aerospikeServer, logging
	// AerospikeServer-only has keys like: service, network, logging, xdr, namespaces
	isFullRequest := false
	if _, hasInfra := rawConfig["infrastructure"]; hasInfra {
		isFullRequest = true
	}
	if _, hasCloud := rawConfig["aerospikeCloud"]; hasCloud {
		isFullRequest = true
	}
	if _, hasName := rawConfig["name"]; hasName {
		isFullRequest = true
	}
	if _, hasServer := rawConfig["aerospikeServer"]; hasServer {
		isFullRequest = true
	}

	if isFullRequest {
		logger.Debug("Custom config detected as full request body")
		return c.mergeFullRequest(data, baseRequest, logger)
	}

	logger.Debug("Custom config detected as aerospikeServer section only")
	return c.mergeAerospikeServerOnly(data, baseRequest, logger)
}

// mergeFullRequest merges a full request JSON with the base request
func (c *CloudClustersUpdateCmd) mergeFullRequest(data []byte, baseRequest cloud.UpdateClusterRequest, logger *logger.Logger) (cloud.UpdateClusterRequest, error) {
	// First, convert base request to a map for merging
	baseData, err := json.Marshal(baseRequest)
	if err != nil {
		return baseRequest, fmt.Errorf("failed to marshal base request: %w", err)
	}

	var baseMap map[string]interface{}
	if err := json.Unmarshal(baseData, &baseMap); err != nil {
		return baseRequest, fmt.Errorf("failed to unmarshal base request: %w", err)
	}

	// Parse custom config
	var customMap map[string]interface{}
	if err := json.Unmarshal(data, &customMap); err != nil {
		return baseRequest, fmt.Errorf("failed to parse custom config: %w", err)
	}

	// Deep merge: custom config takes precedence
	mergedMap := deepMergeUpdate(baseMap, customMap)

	// Convert back to UpdateClusterRequest
	mergedData, err := json.Marshal(mergedMap)
	if err != nil {
		return baseRequest, fmt.Errorf("failed to marshal merged config: %w", err)
	}

	var result cloud.UpdateClusterRequest
	if err := json.Unmarshal(mergedData, &result); err != nil {
		return baseRequest, fmt.Errorf("failed to unmarshal merged config: %w", err)
	}

	return result, nil
}

// mergeAerospikeServerOnly merges an aerospikeServer-only JSON with the base request
func (c *CloudClustersUpdateCmd) mergeAerospikeServerOnly(data []byte, baseRequest cloud.UpdateClusterRequest, logger *logger.Logger) (cloud.UpdateClusterRequest, error) {
	// Parse the aerospikeServer section
	var customServer map[string]interface{}
	if err := json.Unmarshal(data, &customServer); err != nil {
		return baseRequest, fmt.Errorf("failed to parse custom aerospikeServer config: %w", err)
	}

	// Convert base aerospikeServer to map
	var baseServerMap map[string]interface{}
	if baseRequest.AerospikeServer != nil {
		if err := json.Unmarshal(baseRequest.AerospikeServer, &baseServerMap); err != nil {
			return baseRequest, fmt.Errorf("failed to unmarshal base aerospikeServer: %w", err)
		}
	} else {
		baseServerMap = make(map[string]interface{})
	}

	// Deep merge: custom config takes precedence
	mergedServerMap := deepMergeUpdate(baseServerMap, customServer)

	// Convert to json.RawMessage to preserve all fields
	mergedData, err := json.Marshal(mergedServerMap)
	if err != nil {
		return baseRequest, fmt.Errorf("failed to marshal merged aerospikeServer: %w", err)
	}

	baseRequest.AerospikeServer = mergedData
	return baseRequest, nil
}

// deepMergeUpdate recursively merges two maps, with the override map taking precedence
func deepMergeUpdate(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all base values
	for k, v := range base {
		result[k] = v
	}

	// Override/merge with custom values
	for k, v := range override {
		if baseVal, exists := result[k]; exists {
			// If both are maps, merge recursively
			baseMap, baseIsMap := baseVal.(map[string]interface{})
			overrideMap, overrideIsMap := v.(map[string]interface{})
			if baseIsMap && overrideIsMap {
				result[k] = deepMergeUpdate(baseMap, overrideMap)
				continue
			}
		}
		// Otherwise, override takes precedence
		result[k] = v
	}

	return result
}

package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
)

type CloudDatabasesWaitCmd struct {
	DatabaseID  string   `short:"i" long:"database-id" description:"Database ID" required:"true"`
	Status      []string `short:"s" long:"status" description:"Wait for health.status to match any of these values (can be specified multiple times)"`
	StatusNe    []string `long:"status-ne" description:"Wait for health.status to NOT match any of these values (can be specified multiple times)"`
	WaitTimeout int      `long:"wait-timeout" description:"Timeout in seconds (0 = no timeout)" default:"3600"`
	Help        HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudDatabasesWaitCmd) Execute(args []string) error {
	cmd := []string{"cloud", "databases", "wait"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	// Validate that at least one status option is provided
	if len(c.Status) == 0 && len(c.StatusNe) == 0 {
		return Error(fmt.Errorf("at least one --status or --status-ne must be provided"), system, cmd, c, args)
	}

	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	timeout := time.Duration(c.WaitTimeout) * time.Second
	interval := 10 * time.Second
	startTime := time.Now()

	system.Logger.Info("Waiting for database %s health.status...", c.DatabaseID)
	if len(c.Status) > 0 {
		system.Logger.Info("Waiting for status to match any of: %s", strings.Join(c.Status, ", "))
	}
	if len(c.StatusNe) > 0 {
		system.Logger.Info("Waiting for status to NOT match any of: %s", strings.Join(c.StatusNe, ", "))
	}

	for {
		// Check timeout
		if c.WaitTimeout > 0 && time.Since(startTime) > timeout {
			return Error(fmt.Errorf("timeout waiting for database status after %v", timeout), system, cmd, c, args)
		}

		// Get database by ID
		var result interface{}
		path := fmt.Sprintf("%s/%s", cloudDbPath, c.DatabaseID)
		err := client.Get(path, &result)
		if err != nil {
			system.Logger.Debug("Failed to get database: %s, waiting %v...", err, interval)
			time.Sleep(interval)
			continue
		}

		// Parse the result to get health.status
		resultBytes, err := json.Marshal(result)
		if err != nil {
			system.Logger.Debug("Failed to marshal result: %s, waiting %v...", err, interval)
			time.Sleep(interval)
			continue
		}

		var dbMap map[string]interface{}
		err = json.Unmarshal(resultBytes, &dbMap)
		if err != nil {
			system.Logger.Debug("Failed to unmarshal result: %s, waiting %v...", err, interval)
			time.Sleep(interval)
			continue
		}

		// Get health.status
		healthStatus, err := getHealthStatus(dbMap)
		if err != nil {
			system.Logger.Debug("Failed to get health.status: %s, waiting %v...", err, interval)
			time.Sleep(interval)
			continue
		}

		system.Logger.Debug("Current health.status: %s", healthStatus)

		// Check status conditions
		statusMatches := false
		statusNeMatches := false

		// Check if status matches any of the --status values
		if len(c.Status) > 0 {
			for _, expectedStatus := range c.Status {
				if healthStatus == expectedStatus {
					statusMatches = true
					system.Logger.Debug("Status %s matches one of --status values: %s", healthStatus, expectedStatus)
					break
				}
			}
		} else {
			// If --status is not specified, consider it as met
			statusMatches = true
		}

		// Check if status does NOT match any of the --status-ne values
		if len(c.StatusNe) > 0 {
			matchesAnyExcluded := false
			for _, excludedStatus := range c.StatusNe {
				if healthStatus == excludedStatus {
					matchesAnyExcluded = true
					break
				}
			}
			// Status doesn't match any excluded status, condition met
			statusNeMatches = !matchesAnyExcluded
			if statusNeMatches {
				system.Logger.Debug("Status %s does not match any --status-ne values", healthStatus)
			}
		} else {
			// If --status-ne is not specified, consider it as met
			statusNeMatches = true
		}

		// Both conditions must be met (if both are specified)
		if statusMatches && statusNeMatches {
			if len(c.Status) > 0 && len(c.StatusNe) > 0 {
				system.Logger.Info("Database health.status is now %s (matched --status and does not match --status-ne)", healthStatus)
			} else if len(c.Status) > 0 {
				system.Logger.Info("Database health.status is now %s (matched --status)", healthStatus)
			} else {
				system.Logger.Info("Database health.status is now %s (does not match --status-ne)", healthStatus)
			}
			return Error(nil, system, cmd, c, args)
		}

		// Status doesn't match yet, wait and retry
		system.Logger.Debug("Current status %s does not match criteria, waiting %v...", healthStatus, interval)
		time.Sleep(interval)
	}
}

// getHealthStatus extracts health.status from the database response
func getHealthStatus(dbMap map[string]interface{}) (string, error) {
	// Try health.status first
	health, ok := dbMap["health"].(map[string]interface{})
	if ok {
		healthStatusVal, exists := health["status"]
		if exists && healthStatusVal != nil {
			healthStatus, ok := healthStatusVal.(string)
			if ok && healthStatus != "" {
				return healthStatus, nil
			}
		}
	}

	// Fallback to top-level status if health.status is not available
	status, ok := dbMap["status"].(string)
	if ok && status != "" {
		return status, nil
	}

	return "", fmt.Errorf("health.status or status not found in database response")
}

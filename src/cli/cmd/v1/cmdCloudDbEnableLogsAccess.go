package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

// CloudClustersEnableLogsAccessCmd enables log access for an Aerospike Cloud cluster
// by granting specified AWS IAM roles or accounts access to the cluster's S3 log bucket.
type CloudClustersEnableLogsAccessCmd struct {
	ClusterID       string   `short:"c" long:"cluster-id" description:"Cluster ID"`
	ClusterName     string   `short:"n" long:"name" description:"Cluster name (alternative to cluster-id)"`
	AuthorizedRoles []string `short:"r" long:"role" description:"AWS IAM role/user ARN or account ID to authorize (can be specified multiple times). If not specified, uses the current AWS account root ARN."`
	Append          bool     `short:"a" long:"append" description:"Append to existing authorized roles instead of replacing them"`
	Help            HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudClustersEnableLogsAccessCmd) Execute(args []string) error {
	cmd := []string{"cloud", "clusters", "enable-logs-access"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.EnableLogsAccess(system, system.Backend.GetInventory(), system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// EnableLogsAccess enables log access for an Aerospike Cloud cluster by updating the logging configuration.
//
// Parameters:
//   - system: The system context
//   - inventory: The backend inventory
//   - logger: The logger instance
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *CloudClustersEnableLogsAccessCmd) EnableLogsAccess(system *System, inventory *backends.Inventory, logger *logger.Logger) error {
	if c.ClusterID == "" && c.ClusterName == "" {
		return fmt.Errorf("cluster ID or name is required")
	}

	// Validate role ARN format (basic validation) for provided roles
	for _, role := range c.AuthorizedRoles {
		if !isValidAWSPrincipal(role) {
			return fmt.Errorf("invalid AWS principal ARN or account ID: %s", role)
		}
	}

	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cloud", "clusters", "enable-logs-access"}, c)
		if err != nil {
			return err
		}
	}
	if logger == nil {
		logger = system.Logger
	}

	// If no roles specified, use the current AWS account root ARN
	rolesToAuthorize := c.AuthorizedRoles
	if len(rolesToAuthorize) == 0 {
		accountId, err := system.Backend.GetAccountID(backends.BackendTypeAWS)
		if err != nil {
			return fmt.Errorf("failed to get AWS account ID: %w", err)
		}
		if accountId == "" {
			return fmt.Errorf("could not determine AWS account ID. Please specify --role explicitly")
		}
		rootArn := fmt.Sprintf("arn:aws:iam::%s:root", accountId)
		rolesToAuthorize = []string{rootArn}
		logger.Info("No --role specified, using account root ARN: %s", rootArn)
	}

	// Create cloud client
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	// Resolve cluster ID from name if necessary
	clusterID := c.ClusterID
	if clusterID == "" {
		logger.Info("Resolving cluster ID for name: %s", c.ClusterName)
		clusterID, err = c.resolveClusterID(client, c.ClusterName)
		if err != nil {
			return err
		}
		logger.Info("Resolved cluster ID: %s", clusterID)
	}

	// Get current cluster details to get existing authorized roles if appending
	var rolesToSet []string
	if c.Append {
		logger.Info("Fetching current cluster configuration to append roles...")
		currentCluster, err := c.getClusterDetails(client, clusterID)
		if err != nil {
			return fmt.Errorf("failed to get current cluster details: %w", err)
		}

		// Get existing authorized roles
		if logging, ok := currentCluster["logging"].(map[string]interface{}); ok {
			if roles, ok := logging["authorizedRoles"].([]interface{}); ok {
				for _, role := range roles {
					if roleStr, ok := role.(string); ok {
						rolesToSet = append(rolesToSet, roleStr)
					}
				}
			}
		}
		logger.Debug("Existing authorized roles: %v", rolesToSet)

		// Add new roles (avoiding duplicates)
		for _, newRole := range rolesToAuthorize {
			found := false
			for _, existingRole := range rolesToSet {
				if existingRole == newRole {
					found = true
					break
				}
			}
			if !found {
				rolesToSet = append(rolesToSet, newRole)
			}
		}
	} else {
		rolesToSet = rolesToAuthorize
	}

	logger.Info("Enabling log access for cluster: %s", clusterID)
	logger.Info("Authorized roles: %v", rolesToSet)

	// Build update request
	request := cloud.UpdateClusterRequest{
		Logging: &cloud.Logging{
			AuthorizedRoles: rolesToSet,
		},
	}

	var result interface{}

	// Pretty print the request for debugging
	requestJson, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		logger.Error("failed to marshal request for logging purposes: %s", err.Error())
	} else {
		logger.Debug("Update request:\n%s", string(requestJson))
	}

	path := fmt.Sprintf("%s/%s", cloudDbPath, clusterID)
	err = client.Patch(path, request, &result)
	if err != nil {
		return fmt.Errorf("failed to update cluster logging configuration: %w", err)
	}

	// Get updated cluster details to show the log bucket
	updatedCluster, err := c.getClusterDetails(client, clusterID)
	if err != nil {
		logger.Warn("Failed to get updated cluster details: %s", err)
	} else {
		if logging, ok := updatedCluster["logging"].(map[string]interface{}); ok {
			if logBucket, ok := logging["logBucket"].(string); ok && logBucket != "" {
				logger.Info("Log bucket: %s", logBucket)
			}
			if roles, ok := logging["authorizedRoles"].([]interface{}); ok {
				logger.Info("Authorized roles configured: %d", len(roles))
				for _, role := range roles {
					logger.Debug("  - %v", role)
				}
			}
		}
	}

	logger.Info("Log access successfully enabled for cluster %s", clusterID)
	return nil
}

// resolveClusterID resolves a cluster name to its ID
func (c *CloudClustersEnableLogsAccessCmd) resolveClusterID(client *cloud.Client, name string) (string, error) {
	var result map[string]interface{}
	path := cloudDbPath + "?status_ne=decommissioned"
	err := client.Get(path, &result)
	if err != nil {
		return "", fmt.Errorf("failed to list clusters: %w", err)
	}

	clusters, ok := result["clusters"].([]interface{})
	if !ok {
		return "", fmt.Errorf("no clusters found")
	}

	for _, db := range clusters {
		dbMap, ok := db.(map[string]interface{})
		if !ok {
			continue
		}

		dbName, _ := dbMap["name"].(string)
		if dbName == name {
			dbID, _ := dbMap["id"].(string)
			return dbID, nil
		}
	}

	return "", fmt.Errorf("cluster with name '%s' not found", name)
}

// getClusterDetails gets the full cluster details
func (c *CloudClustersEnableLogsAccessCmd) getClusterDetails(client *cloud.Client, clusterID string) (map[string]interface{}, error) {
	var result map[string]interface{}
	path := fmt.Sprintf("%s/%s", cloudDbPath, clusterID)
	err := client.Get(path, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// isValidAWSPrincipal performs basic validation of AWS principal ARNs or account IDs
// Valid formats:
//   - arn:aws:iam::123456789012:role/RoleName
//   - arn:aws:iam::123456789012:user/UserName
//   - arn:aws:iam::123456789012:root
//   - arn:aws:sts::123456789012:assumed-role/RoleName/SessionName
//   - arn:aws:sts::123456789012:federated-user/UserName
//   - 123456789012 (12-digit account ID)
func isValidAWSPrincipal(principal string) bool {
	// Check for 12-digit AWS account ID
	if len(principal) == 12 {
		for _, c := range principal {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}

	// Check for ARN format
	if strings.HasPrefix(principal, "arn:aws:iam::") || strings.HasPrefix(principal, "arn:aws:sts::") {
		return true
	}

	return false
}

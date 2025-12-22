package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
)

type CloudClustersGetCmd struct {
	Host    CloudClustersGetHostCmd    `command:"host" subcommands-optional:"true" description:"Get cluster host" webicon:"fas fa-network-wired"`
	TlsCert CloudClustersGetTlsCertCmd `command:"tls-cert" subcommands-optional:"true" description:"Get cluster TLS certificate" webicon:"fas fa-lock"`
	Help    HelpCmd                    `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudClustersGetCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type CloudClustersGetHostCmd struct {
	ClusterID   string  `short:"c" long:"cluster-id" description:"Cluster ID"`
	ClusterName string  `short:"n" long:"name" description:"Cluster name"`
	Help        HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudClustersGetHostCmd) Execute(args []string) error {
	cmd := []string{"cloud", "clusters", "get", "host"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	var result interface{}
	path := cloudDbPath + "?status_ne=decommissioned"
	err = client.Get(path, &result)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Parse the JSON response to find the cluster and extract host
	host, err := extractConnectionField(result, c.ClusterID, c.ClusterName, "host")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	fmt.Println(host)
	return Error(nil, system, cmd, c, args)
}

type CloudClustersGetTlsCertCmd struct {
	ClusterID   string  `short:"c" long:"cluster-id" description:"Cluster ID"`
	ClusterName string  `short:"n" long:"name" description:"Cluster name"`
	Help        HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudClustersGetTlsCertCmd) Execute(args []string) error {
	cmd := []string{"cloud", "clusters", "get", "tls-cert"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	var result interface{}
	path := cloudDbPath + "?status_ne=decommissioned"
	err = client.Get(path, &result)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Parse the JSON response to find the cluster and extract tlsCertificate
	cert, err := extractConnectionField(result, c.ClusterID, c.ClusterName, "tlsCertificate")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	fmt.Println(cert)
	return Error(nil, system, cmd, c, args)
}

// extractConnectionField extracts a field from connectionDetails in the cluster list response
func extractConnectionField(result interface{}, clusterID, clusterName, field string) (string, error) {
	// Convert result to JSON bytes and unmarshal to map
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	var resultMap map[string]interface{}
	err = json.Unmarshal(resultBytes, &resultMap)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal result: %w", err)
	}

	// Get clusters array
	clusters, ok := resultMap["clusters"].([]interface{})
	if !ok {
		return "", fmt.Errorf("clusters field not found or not an array")
	}

	// Find the cluster by ID or name
	var foundCluster map[string]interface{}
	for _, db := range clusters {
		dbMap, ok := db.(map[string]interface{})
		if !ok {
			continue
		}
		// Check by ID
		if clusterID != "" {
			if id, ok := dbMap["id"].(string); ok && id == clusterID {
				foundCluster = dbMap
				break
			}
		}

		// Check by name
		if clusterName != "" {
			if name, ok := dbMap["name"].(string); ok && name == clusterName {
				foundCluster = dbMap
				break
			}
		}
	}

	if foundCluster == nil {
		if clusterID != "" {
			return "", fmt.Errorf("cluster with ID %s not found", clusterID)
		}
		if clusterName != "" {
			return "", fmt.Errorf("cluster with name %s not found", clusterName)
		}
		return "", fmt.Errorf("cluster ID or name must be provided")
	}

	// Get connectionDetails
	connectionDetails, ok := foundCluster["connectionDetails"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("connectionDetails not found or not an object")
	}

	// Get the requested field
	fieldValue, ok := connectionDetails[field].(string)
	if !ok {
		// Try camelCase version
		camelCaseField := strings.ToLower(field[:1]) + field[1:]
		fieldValue, ok = connectionDetails[camelCaseField].(string)
		if !ok {
			return "", fmt.Errorf("field %s not found in connectionDetails", field)
		}
	}

	if fieldValue == "" {
		return "", fmt.Errorf("field %s is empty", field)
	}

	return fieldValue, nil
}

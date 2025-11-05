package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
)

type CloudDatabasesGetCmd struct {
	Host    CloudDatabasesGetHostCmd    `command:"host" subcommands-optional:"true" description:"Get database host" webicon:"fas fa-network-wired"`
	TlsCert CloudDatabasesGetTlsCertCmd `command:"tls-cert" subcommands-optional:"true" description:"Get database TLS certificate" webicon:"fas fa-lock"`
	Help    HelpCmd                     `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudDatabasesGetCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type CloudDatabasesGetHostCmd struct {
	DatabaseID   string  `short:"i" long:"database-id" description:"Database ID"`
	DatabaseName string  `short:"n" long:"name" description:"Database name"`
	Help         HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudDatabasesGetHostCmd) Execute(args []string) error {
	cmd := []string{"cloud", "databases", "get", "host"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	client, err := cloud.NewClient()
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	var result interface{}
	err = client.Get("/databases", &result)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Parse the JSON response to find the database and extract host
	host, err := extractConnectionField(result, c.DatabaseID, c.DatabaseName, "host")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	fmt.Println(host)
	return Error(nil, system, cmd, c, args)
}

type CloudDatabasesGetTlsCertCmd struct {
	DatabaseID   string  `short:"i" long:"database-id" description:"Database ID"`
	DatabaseName string  `short:"n" long:"name" description:"Database name"`
	Help         HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudDatabasesGetTlsCertCmd) Execute(args []string) error {
	cmd := []string{"cloud", "databases", "get", "tls-cert"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	client, err := cloud.NewClient()
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	var result interface{}
	err = client.Get("/databases", &result)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Parse the JSON response to find the database and extract tlsCertificate
	cert, err := extractConnectionField(result, c.DatabaseID, c.DatabaseName, "tlsCertificate")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	fmt.Println(cert)
	return Error(nil, system, cmd, c, args)
}

// extractConnectionField extracts a field from connectionDetails in the database list response
func extractConnectionField(result interface{}, databaseID, databaseName, field string) (string, error) {
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

	// Get databases array
	databases, ok := resultMap["databases"].([]interface{})
	if !ok {
		return "", fmt.Errorf("databases field not found or not an array")
	}

	// Find the database by ID or name
	var foundDatabase map[string]interface{}
	for _, db := range databases {
		dbMap, ok := db.(map[string]interface{})
		if !ok {
			continue
		}

		// Check by ID
		if databaseID != "" {
			if id, ok := dbMap["id"].(string); ok && id == databaseID {
				foundDatabase = dbMap
				break
			}
		}

		// Check by name
		if databaseName != "" {
			if name, ok := dbMap["name"].(string); ok && name == databaseName {
				foundDatabase = dbMap
				break
			}
		}
	}

	if foundDatabase == nil {
		if databaseID != "" {
			return "", fmt.Errorf("database with ID %s not found", databaseID)
		}
		if databaseName != "" {
			return "", fmt.Errorf("database with name %s not found", databaseName)
		}
		return "", fmt.Errorf("database ID or name must be provided")
	}

	// Get connectionDetails
	connectionDetails, ok := foundDatabase["connectionDetails"].(map[string]interface{})
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

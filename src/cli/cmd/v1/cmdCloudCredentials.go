package cmd

import (
	"fmt"
	"time"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/rglonek/logger"
)

type CloudDatabasesCredentialsListCmd struct {
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	DatabaseID string  `short:"d" long:"database-id" description:"Database ID" required:"true"`
}

func (c *CloudDatabasesCredentialsListCmd) Execute(args []string) error {
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := fmt.Sprintf("/databases/%s/credentials", c.DatabaseID)

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type CloudDatabasesCredentialsCreateCmd struct {
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	DatabaseID string  `short:"d" long:"database-id" description:"Database ID" required:"true"`
	Username   string  `short:"u" long:"username" description:"Username" required:"true"`
	Password   string  `short:"p" long:"password" description:"Password" required:"true"`
	Privileges string  `short:"r" long:"privileges" description:"Privileges (read, write, read-write)" default:"read-write"`
	Wait       bool    `long:"wait" description:"Wait for credentials to become active"`
}

func (c *CloudDatabasesCredentialsCreateCmd) Execute(args []string) error {
	cmd := []string{"cloud", "db", "credentials", "create"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	logger := system.Logger

	client, err := cloud.NewClient()
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Convert privileges string to roles array
	// The API expects roles as an array, but we accept privileges as a string for convenience
	roles := []string{c.Privileges}
	if c.Privileges == "" {
		roles = []string{"read-write"} // default
	}

	request := cloud.CreateDatabaseCredentialsRequest{
		Name:     c.Username, // username maps to name in the API
		Password: c.Password,
		Roles:    roles,
	}
	var result interface{}

	path := fmt.Sprintf("/databases/%s/credentials", c.DatabaseID)
	err = client.Post(path, request, &result)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// If --wait is specified, wait for credentials to become active
	if c.Wait {
		// Extract the ID from the response
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			return Error(fmt.Errorf("unexpected response type: %T", result), system, cmd, c, args)
		}

		credentialID, ok := resultMap["id"].(string)
		if !ok || credentialID == "" {
			return Error(fmt.Errorf("id not found in credentials creation response"), system, cmd, c, args)
		}

		logger.Info("Waiting for credentials to become active (id: %s)...", credentialID)
		waitResult, err := c.waitForCredentialsActive(client, c.DatabaseID, credentialID, logger)
		if err != nil {
			return Error(fmt.Errorf("failed to wait for credentials to become active: %w", err), system, cmd, c, args)
		}
		logger.Info("Credentials are now active")
		// Use the wait result instead of the creation result
		result = waitResult
	}

	err = client.PrettyPrint(result)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	return Error(nil, system, cmd, c, args)
}

// waitForCredentialsActive polls the credentials list until the credential with the given ID has status "active"
// Returns the credential object when it becomes active
func (c *CloudDatabasesCredentialsCreateCmd) waitForCredentialsActive(client *cloud.Client, databaseID string, credentialID string, logger *logger.Logger) (map[string]interface{}, error) {
	timeout := 10 * time.Minute
	interval := 5 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeout {
			return nil, fmt.Errorf("timeout waiting for credentials to become active after %v", timeout)
		}

		var result interface{}
		path := fmt.Sprintf("/databases/%s/credentials", databaseID)
		err := client.Get(path, &result)
		if err != nil {
			return nil, fmt.Errorf("failed to get credentials list: %w", err)
		}

		resultMap, ok := result.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected response type from credentials list: %T", result)
		}

		credentials, ok := resultMap["credentials"].([]interface{})
		if !ok {
			logger.Debug("No credentials found yet, waiting %v...", interval)
			time.Sleep(interval)
			continue
		}

		// Find the credential with the matching ID
		for _, cred := range credentials {
			credMap, ok := cred.(map[string]interface{})
			if !ok {
				continue
			}

			id, ok := credMap["id"].(string)
			if !ok || id != credentialID {
				continue
			}

			// Found the credential, check its status
			status, _ := credMap["status"].(string)
			logger.Debug("Credential status: %s", status)

			if status == "active" {
				// Return the credential object
				return credMap, nil
			}

			logger.Debug("Credentials still %s, waiting %v...", status, interval)
			time.Sleep(interval)
			break
		}

		// Credential not found yet, wait and retry
		logger.Debug("Credential not found in list yet, waiting %v...", interval)
		time.Sleep(interval)
	}
}

type CloudDatabasesCredentialsDeleteCmd struct {
	Help          HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	DatabaseID    string  `short:"d" long:"database-id" description:"Database ID" required:"true"`
	CredentialsID string  `short:"c" long:"credentials-id" description:"Credentials ID" required:"true"`
}

func (c *CloudDatabasesCredentialsDeleteCmd) Execute(args []string) error {
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/databases/%s/credentials/%s", c.DatabaseID, c.CredentialsID)
	err = client.Delete(path)
	if err != nil {
		return err
	}

	fmt.Println("Database credentials deleted successfully")
	return nil
}

package cmd

import (
	"fmt"

	"github.com/aerospike-community/aerolab/cli/cmd/v1/cloud"
)

type CloudSecretsListCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudSecretsListCmd) Execute(args []string) error {
	client, err := newCloudClient()
	if err != nil {
		return err
	}

	var result any
	err = client.Get("/secrets", &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type CloudSecretsCreateCmd struct {
	Name        string  `short:"n" long:"name" description:"Secret name" webicon:"fas fa-plus"`
	Description string  `short:"d" long:"description" description:"Secret description" webicon:"fas fa-info"`
	Value       string  `short:"v" long:"value" description:"Secret value" telemetry:"redact"`
	Help        HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudSecretsCreateCmd) Execute(args []string) error {
	client, err := newCloudClient()
	if err != nil {
		return err
	}

	c.Name, err = RequireString(c.Name, "name")
	if err != nil {
		return err
	}
	c.Description, err = RequireString(c.Description, "description")
	if err != nil {
		return err
	}
	c.Value, err = RequireSecret(c.Value, "value")
	if err != nil {
		return err
	}
	request := cloud.CreateSecretRequest{
		Name:        c.Name,
		Description: c.Description,
		Value:       c.Value,
	}
	var result any

	err = client.Post("/secrets", request, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type CloudSecretsDeleteCmd struct {
	SecretID string  `short:"s" long:"secret-id" description:"Secret ID" webicon:"fas fa-trash"`
	Help     HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudSecretsDeleteCmd) Execute(args []string) error {
	secretID, err := RequireString(c.SecretID, "secret ID")
	if err != nil {
		return err
	}
	c.SecretID = secretID
	client, err := newCloudClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/secrets/%s", c.SecretID)
	err = client.Delete(path)
	if err != nil {
		return err
	}

	fmt.Println("Secret deleted successfully")
	return nil
}

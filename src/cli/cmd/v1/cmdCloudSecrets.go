package cmd

import (
	"fmt"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
)

type CloudSecretsListCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudSecretsListCmd) Execute(args []string) error {
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	err = client.Get("/secrets", &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type CloudSecretsCreateCmd struct {
	Name        string  `short:"n" long:"name" description:"Secret name" required:"true" webicon:"fas fa-plus"`
	Description string  `short:"d" long:"description" description:"Secret description" webicon:"fas fa-info"`
	Value       string  `short:"v" long:"value" description:"Secret value" required:"true"`
	Help        HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudSecretsCreateCmd) Execute(args []string) error {
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	request := cloud.CreateSecretRequest{
		Name:        c.Name,
		Description: c.Description,
		Value:       c.Value,
	}
	var result interface{}

	err = client.Post("/secrets", request, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type CloudSecretsDeleteCmd struct {
	SecretID string  `short:"s" long:"secret-id" description:"Secret ID" required:"true" webicon:"fas fa-trash"`
	Help     HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudSecretsDeleteCmd) Execute(args []string) error {
	client, err := cloud.NewClient()
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

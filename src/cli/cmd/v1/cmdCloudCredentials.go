package cmd

import (
	"fmt"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
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
}

func (c *CloudDatabasesCredentialsCreateCmd) Execute(args []string) error {
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	request := cloud.CreateDatabaseCredentialsRequest{
		Username:   c.Username,
		Password:   c.Password,
		Privileges: c.Privileges,
	}
	var result interface{}

	path := fmt.Sprintf("/databases/%s/credentials", c.DatabaseID)
	err = client.Post(path, request, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
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

package cmd

import (
	"fmt"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
)

type CloudDatabasesDeleteCmd struct {
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	DatabaseID string  `short:"d" long:"database-id" description:"Database ID" required:"true"`
}

func (c *CloudDatabasesDeleteCmd) Execute(args []string) error {
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/databases/%s", c.DatabaseID)
	err = client.Delete(path)
	if err != nil {
		return err
	}

	return nil
}

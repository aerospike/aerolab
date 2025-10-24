package cmd

import "github.com/aerospike/aerolab/cli/cmd/v1/cloud"

type CloudDatabasesListCmd struct {
	Help     HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
	StatusNe string  `long:"status-ne" description:"Filter databases to exclude specified statuses (comma-separated)" default:"decommissioned"`
}

func (c *CloudDatabasesListCmd) Execute(args []string) error {
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := "/databases"
	if c.StatusNe != "" {
		path += "?status_ne=" + c.StatusNe
	}

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

package cmd

import "github.com/aerospike/aerolab/cli/cmd/v1/cloud"

type CloudListInstanceTypesCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudListInstanceTypesCmd) Execute(args []string) error {
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := "/cloud-providers"

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

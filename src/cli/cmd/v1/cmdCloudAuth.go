package cmd

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

type CloudAuthCmd struct {
	GetToken CloudAuthGetTokenCmd `command:"get-token" subcommands-optional:"true" description:"Get and print the auth token" webicon:"fas fa-key"`
	Help     HelpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudAuthCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type CloudAuthGetTokenCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudAuthGetTokenCmd) Execute(args []string) error {
	client, err := newCloudClient()
	if err != nil {
		return err
	}

	token := client.GetAccessToken()
	if term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Println(token)
	} else {
		fmt.Print(token)
	}
	return nil
}

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/bestmethod/inslice"
	"github.com/jessevdk/go-flags"
)

type clientCreateBaseCmd struct {
	ClientName    TypeClientName         `short:"n" long:"group-name" description:"Client group name" default:"client"`
	ClientCount   int                    `short:"c" long:"count" description:"Number of clients" default:"1"`
	NoSetHostname bool                   `short:"H" long:"no-set-hostname" description:"by default, hostname of each machine will be set, use this to prevent hostname change"`
	StartScript   flags.Filename         `short:"X" long:"start-script" description:"optionally specify a script to be installed which will run when the client machine starts"`
	Aws           clusterCreateCmdAws    `no-flag:"true"`
	Docker        clusterCreateCmdDocker `no-flag:"true"`
	osSelectorCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientCreateBaseCmd) isGrow() bool {
	if len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow" {
		return true
	}
	return false
}

func (c *clientCreateBaseCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	_, err := c.createBase(args)
	return err
}

func (c *clientCreateBaseCmd) createBase(args []string) (machines []int, err error) {
	b.WorkOnClients()
	clist, err := b.ClusterList()
	if err != nil {
		return nil, err
	}

	if inslice.HasString(clist, c.ClientName.String()) && !c.isGrow() {
		return nil, errors.New("cluster already exists, did you mean 'grow'?")
	}

	if !inslice.HasString(clist, c.ClientName.String()) && c.isGrow() {
		return nil, errors.New("cluster doesn't exist, did you mean 'create'?")
	}

	// TODO HERE
	return nil, fmt.Errorf("isGgrow:%t", c.isGrow())
}

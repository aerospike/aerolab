package main

import "os"

type clientCreateCmd struct {
	Base  clientCreateBaseCmd  `command:"base" subcommands-optional:"true" description:"simple base image"`
	Tools clientCreateToolsCmd `command:"tools" subcommands-optional:"true" description:"aerospike-tools"`
	// NEW_CLIENTS_CREATE
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientAddCmd struct {
	Tools clientAddToolsCmd `command:"tools" subcommands-optional:"true" description:"aerospike-tools"`
	// NEW_CLIENTS_ADD
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientGrowCmd struct {
	clientCreateCmd
}

func (c *clientCreateCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

func (c *clientAddCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

func init() {
	addBackendSwitch("client.create.base", "aws", &a.opts.Client.Create.Base.Aws)
	addBackendSwitch("client.create.base", "docker", &a.opts.Client.Create.Base.Docker)
	addBackendSwitch("client.grow.base", "aws", &a.opts.Client.Grow.Base.Aws)
	addBackendSwitch("client.grow.base", "docker", &a.opts.Client.Grow.Base.Docker)

	addBackendSwitch("client.create.tools", "aws", &a.opts.Client.Create.Tools.Aws)
	addBackendSwitch("client.create.tools", "docker", &a.opts.Client.Create.Tools.Docker)
	addBackendSwitch("client.grow.tools", "aws", &a.opts.Client.Grow.Tools.Aws)
	addBackendSwitch("client.grow.tools", "docker", &a.opts.Client.Grow.Tools.Docker)

	// NEW_CLIENTS_BACKEND

	addBackendSwitch("client.destroy", "docker", &a.opts.Client.Destroy.Docker)
}

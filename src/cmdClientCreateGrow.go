package main

import "os"

type clientCreateCmd struct {
	Base    clientCreateBaseCmd    `command:"base" subcommands-optional:"true" description:"simple base image"`
	Tools   clientCreateToolsCmd   `command:"tools" subcommands-optional:"true" description:"aerospike-tools"`
	AMS     clientCreateAMSCmd     `command:"ams" subcommands-optional:"true" description:"prometheus and grafana for AMS; for exporter see: cluster add exporter"`
	Jupyter clientCreateJupyterCmd `command:"jupyter" subcommands-optional:"true" description:"launch a jupyter IDE client"`
	Trino   clientCreateTrinoCmd   `command:"trino" subcommands-optional:"true" description:"launch a trino server (use 'client attach trino' to get trino shell)"`
	// NEW_CLIENTS_CREATE
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientAddCmd struct {
	Tools   clientAddToolsCmd   `command:"tools" subcommands-optional:"true" description:"aerospike-tools"`
	AMS     clientAddAMSCmd     `command:"ams" subcommands-optional:"true" description:"prometheus and grafana for AMS; for exporter see: cluster add exporter"`
	Jupyter clientAddJupyterCmd `command:"jupyter" subcommands-optional:"true" description:"launch a jupyter IDE client"`
	Trino   clientAddTrinoCmd   `command:"trino" subcommands-optional:"true" description:"launch a trino server (use 'client attach trino' to get trino shell)"`
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

	addBackendSwitch("client.create.ams", "aws", &a.opts.Client.Create.AMS.Aws)
	addBackendSwitch("client.create.ams", "docker", &a.opts.Client.Create.AMS.Docker)
	addBackendSwitch("client.grow.ams", "aws", &a.opts.Client.Grow.AMS.Aws)
	addBackendSwitch("client.grow.ams", "docker", &a.opts.Client.Grow.AMS.Docker)

	addBackendSwitch("client.create.jupyter", "aws", &a.opts.Client.Create.Jupyter.Aws)
	addBackendSwitch("client.create.jupyter", "docker", &a.opts.Client.Create.Jupyter.Docker)
	addBackendSwitch("client.grow.jupyter", "aws", &a.opts.Client.Grow.Jupyter.Aws)
	addBackendSwitch("client.grow.jupyter", "docker", &a.opts.Client.Grow.Jupyter.Docker)

	addBackendSwitch("client.create.trino", "aws", &a.opts.Client.Create.Trino.Aws)
	addBackendSwitch("client.create.trino", "docker", &a.opts.Client.Create.Trino.Docker)
	addBackendSwitch("client.grow.trino", "aws", &a.opts.Client.Grow.Trino.Aws)
	addBackendSwitch("client.grow.trino", "docker", &a.opts.Client.Grow.Trino.Docker)

	// NEW_CLIENTS_BACKEND

	addBackendSwitch("client.destroy", "docker", &a.opts.Client.Destroy.Docker)
}

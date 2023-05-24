package main

import "os"

type clientCreateCmd struct {
	None          clientCreateNoneCmd          `command:"none" subcommands-optional:"true" description:"vanilla OS image with no package modifications"`
	Base          clientCreateBaseCmd          `command:"base" subcommands-optional:"true" description:"simple base image"`
	Tools         clientCreateToolsCmd         `command:"tools" subcommands-optional:"true" description:"aerospike-tools"`
	AMS           clientCreateAMSCmd           `command:"ams" subcommands-optional:"true" description:"prometheus and grafana for AMS; for exporter see: cluster add exporter"`
	Jupyter       clientCreateJupyterCmd       `command:"jupyter" subcommands-optional:"true" description:"launch a jupyter IDE client"`
	VSCode        clientCreateVSCodeCmd        `command:"vscode" subcommands-optional:"true" description:"launch a VSCode IDE client"`
	Trino         clientCreateTrinoCmd         `command:"trino" subcommands-optional:"true" description:"launch a trino server (use 'client attach trino' to get trino shell)"`
	ElasticSearch clientCreateElasticSearchCmd `command:"elasticsearch" subcommands-optional:"true" description:"deploy elasticsearch with the es connector for aerospike"`
	RestGateway   clientCreateRestGatewayCmd   `command:"rest-gateway" subcommands-optional:"true" description:"deploy a rest-gateway client machine"`
	// NEW_CLIENTS_CREATE
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientAddCmd struct {
	Tools         clientAddToolsCmd         `command:"tools" subcommands-optional:"true" description:"aerospike-tools"`
	AMS           clientAddAMSCmd           `command:"ams" subcommands-optional:"true" description:"prometheus and grafana for AMS; for exporter see: cluster add exporter"`
	VSCode        clientAddVSCodeCmd        `command:"vscode" subcommands-optional:"true" description:"launch a VSCode IDE client"`
	Jupyter       clientAddJupyterCmd       `command:"jupyter" subcommands-optional:"true" description:"launch a jupyter IDE client"`
	Trino         clientAddTrinoCmd         `command:"trino" subcommands-optional:"true" description:"launch a trino server (use 'client attach trino' to get trino shell)"`
	ElasticSearch clientAddElasticSearchCmd `command:"elasticsearch" subcommands-optional:"true" description:"deploy elasticsearch with the es connector for aerospike"`
	RestGateway   clientAddRestGatewayCmd   `command:"rest-gateway" subcommands-optional:"true" description:"deploy a rest-gateway client machine"`
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

	addBackendSwitch("client.create.vscode", "aws", &a.opts.Client.Create.VSCode.Aws)
	addBackendSwitch("client.create.vscode", "docker", &a.opts.Client.Create.VSCode.Docker)
	addBackendSwitch("client.grow.vscode", "aws", &a.opts.Client.Grow.VSCode.Aws)
	addBackendSwitch("client.grow.vscode", "docker", &a.opts.Client.Grow.VSCode.Docker)

	addBackendSwitch("client.create.trino", "aws", &a.opts.Client.Create.Trino.Aws)
	addBackendSwitch("client.create.trino", "docker", &a.opts.Client.Create.Trino.Docker)
	addBackendSwitch("client.grow.trino", "aws", &a.opts.Client.Grow.Trino.Aws)
	addBackendSwitch("client.grow.trino", "docker", &a.opts.Client.Grow.Trino.Docker)

	addBackendSwitch("client.create.elasticsearch", "aws", &a.opts.Client.Create.ElasticSearch.Aws)
	addBackendSwitch("client.create.elasticsearch", "docker", &a.opts.Client.Create.ElasticSearch.Docker)
	addBackendSwitch("client.grow.elasticsearch", "aws", &a.opts.Client.Grow.ElasticSearch.Aws)
	addBackendSwitch("client.grow.elasticsearch", "docker", &a.opts.Client.Grow.ElasticSearch.Docker)

	addBackendSwitch("client.create.rest-gateway", "aws", &a.opts.Client.Create.RestGateway.Aws)
	addBackendSwitch("client.create.rest-gateway", "docker", &a.opts.Client.Create.RestGateway.Docker)
	addBackendSwitch("client.grow.rest-gateway", "aws", &a.opts.Client.Grow.RestGateway.Aws)
	addBackendSwitch("client.grow.rest-gateway", "docker", &a.opts.Client.Grow.RestGateway.Docker)

	addBackendSwitch("client.create.base", "gcp", &a.opts.Client.Create.Base.Gcp)
	addBackendSwitch("client.grow.base", "gcp", &a.opts.Client.Grow.Base.Gcp)
	addBackendSwitch("client.create.tools", "gcp", &a.opts.Client.Create.Tools.Gcp)
	addBackendSwitch("client.grow.tools", "gcp", &a.opts.Client.Grow.Tools.Gcp)
	addBackendSwitch("client.create.ams", "gcp", &a.opts.Client.Create.AMS.Gcp)
	addBackendSwitch("client.grow.ams", "gcp", &a.opts.Client.Grow.AMS.Gcp)
	addBackendSwitch("client.create.jupyter", "gcp", &a.opts.Client.Create.Jupyter.Gcp)
	addBackendSwitch("client.grow.jupyter", "gcp", &a.opts.Client.Grow.Jupyter.Gcp)
	addBackendSwitch("client.create.vscode", "gcp", &a.opts.Client.Create.VSCode.Gcp)
	addBackendSwitch("client.grow.vscode", "gcp", &a.opts.Client.Grow.VSCode.Gcp)
	addBackendSwitch("client.create.trino", "gcp", &a.opts.Client.Create.Trino.Gcp)
	addBackendSwitch("client.grow.trino", "gcp", &a.opts.Client.Grow.Trino.Gcp)
	addBackendSwitch("client.create.elasticsearch", "gcp", &a.opts.Client.Create.ElasticSearch.Gcp)
	addBackendSwitch("client.grow.elasticsearch", "gcp", &a.opts.Client.Grow.ElasticSearch.Gcp)
	addBackendSwitch("client.create.rest-gateway", "gcp", &a.opts.Client.Create.RestGateway.Gcp)
	addBackendSwitch("client.grow.rest-gateway", "gcp", &a.opts.Client.Grow.RestGateway.Gcp)

	// NEW_CLIENTS_BACKEND

	addBackendSwitch("client.destroy", "docker", &a.opts.Client.Destroy.Docker)
}

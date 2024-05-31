package main

import "os"

type clientCreateCmd struct {
	None          clientCreateNoneCmd          `command:"none" subcommands-optional:"true" description:"vanilla OS image with no package modifications" webicon:"fas fa-file" invwebforce:"true"`
	Base          clientCreateBaseCmd          `command:"base" subcommands-optional:"true" description:"simple base image" webicon:"fas fa-grip-lines" invwebforce:"true"`
	Tools         clientCreateToolsCmd         `command:"tools" subcommands-optional:"true" description:"aerospike-tools" webicon:"fas fa-toolbox" invwebforce:"true"`
	AMS           clientCreateAMSCmd           `command:"ams" subcommands-optional:"true" description:"prometheus and grafana for AMS; for exporter see: cluster add exporter" webicon:"fas fa-layer-group" invwebforce:"true"`
	VSCode        clientCreateVSCodeCmd        `command:"vscode" subcommands-optional:"true" description:"launch a VSCode IDE client" webicon:"fas fa-code" invwebforce:"true"`
	Trino         clientCreateTrinoCmd         `command:"trino" subcommands-optional:"true" description:"launch a trino server (use 'attach trino' to get trino shell)" webicon:"fas fa-tachograph-digital" invwebforce:"true"`
	ElasticSearch clientCreateElasticSearchCmd `command:"elasticsearch" subcommands-optional:"true" description:"deploy elasticsearch with the es connector for aerospike" webicon:"fas fa-magnifying-glass" invwebforce:"true"`
	RestGateway   clientCreateRestGatewayCmd   `command:"rest-gateway" subcommands-optional:"true" description:"deploy a rest-gateway client machine" webicon:"fas fa-dungeon" invwebforce:"true"`
	Graph         clientCreateGraphCmd         `command:"graph" subcommands-optional:"true" description:"deploy a graph client machine" webicon:"fas fa-diagram-project" invwebforce:"true"`
	Vector        clientCreateVectorCmd        `command:"vector" subcommands-optional:"true" description:"deploy a vector client machine" webicon:"fa-solid fa-wand-sparkles" invwebforce:"true"`
	EksCtl        clientCreateEksCtlCmd        `command:"eksctl" subcommands-optional:"true" description:"deploy a client machine with preconfigured eksctl for k8s aerospike cluster deployments" webicon:"fas fa-box-open" invwebforce:"true"`
	// NEW_CLIENTS_CREATE
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientAddCmd struct {
	Tools         clientAddToolsCmd         `command:"tools" subcommands-optional:"true" description:"aerospike-tools"`
	AMS           clientAddAMSCmd           `command:"ams" subcommands-optional:"true" description:"prometheus and grafana for AMS; for exporter see: cluster add exporter"`
	VSCode        clientAddVSCodeCmd        `command:"vscode" subcommands-optional:"true" description:"launch a VSCode IDE client"`
	Trino         clientAddTrinoCmd         `command:"trino" subcommands-optional:"true" description:"launch a trino server (use 'attach trino' to get trino shell)"`
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
	addBackendSwitch("client.create.none", "aws", &a.opts.Client.Create.None.Aws)
	addBackendSwitch("client.create.none", "docker", &a.opts.Client.Create.None.Docker)
	addBackendSwitch("client.grow.none", "aws", &a.opts.Client.Grow.None.Aws)
	addBackendSwitch("client.grow.none", "docker", &a.opts.Client.Grow.None.Docker)
	addBackendSwitch("client.create.none", "gcp", &a.opts.Client.Create.None.Gcp)
	addBackendSwitch("client.grow.none", "gcp", &a.opts.Client.Grow.None.Gcp)

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
	addBackendSwitch("client.create.vscode", "gcp", &a.opts.Client.Create.VSCode.Gcp)
	addBackendSwitch("client.grow.vscode", "gcp", &a.opts.Client.Grow.VSCode.Gcp)
	addBackendSwitch("client.create.trino", "gcp", &a.opts.Client.Create.Trino.Gcp)
	addBackendSwitch("client.grow.trino", "gcp", &a.opts.Client.Grow.Trino.Gcp)
	addBackendSwitch("client.create.elasticsearch", "gcp", &a.opts.Client.Create.ElasticSearch.Gcp)
	addBackendSwitch("client.grow.elasticsearch", "gcp", &a.opts.Client.Grow.ElasticSearch.Gcp)
	addBackendSwitch("client.create.rest-gateway", "gcp", &a.opts.Client.Create.RestGateway.Gcp)
	addBackendSwitch("client.grow.rest-gateway", "gcp", &a.opts.Client.Grow.RestGateway.Gcp)

	// NEW_CLIENTS_BACKEND
	addBackendSwitch("client.create.graph", "aws", &a.opts.Client.Create.Graph.Aws)
	addBackendSwitch("client.create.graph", "gcp", &a.opts.Client.Create.Graph.Gcp)
	addBackendSwitch("client.create.graph", "docker", &a.opts.Client.Create.Graph.Docker)
	addBackendSwitch("client.grow.graph", "aws", &a.opts.Client.Grow.Graph.Aws)
	addBackendSwitch("client.grow.graph", "gcp", &a.opts.Client.Grow.Graph.Gcp)
	addBackendSwitch("client.grow.graph", "docker", &a.opts.Client.Grow.Graph.Docker)

	addBackendSwitch("client.create.vector", "aws", &a.opts.Client.Create.Vector.Aws)
	addBackendSwitch("client.create.vector", "gcp", &a.opts.Client.Create.Vector.Gcp)
	addBackendSwitch("client.create.vector", "docker", &a.opts.Client.Create.Vector.Docker)
	addBackendSwitch("client.grow.vector", "aws", &a.opts.Client.Grow.Vector.Aws)
	addBackendSwitch("client.grow.vector", "gcp", &a.opts.Client.Grow.Vector.Gcp)
	addBackendSwitch("client.grow.vector", "docker", &a.opts.Client.Grow.Vector.Docker)

	addBackendSwitch("client.create.eksctl", "aws", &a.opts.Client.Create.EksCtl.Aws)
	addBackendSwitch("client.create.eksctl", "gcp", &a.opts.Client.Create.EksCtl.Gcp)
	addBackendSwitch("client.create.eksctl", "docker", &a.opts.Client.Create.EksCtl.Docker)
	addBackendSwitch("client.grow.eksctl", "aws", &a.opts.Client.Grow.EksCtl.Aws)
	addBackendSwitch("client.grow.eksctl", "gcp", &a.opts.Client.Grow.EksCtl.Gcp)
	addBackendSwitch("client.grow.eksctl", "docker", &a.opts.Client.Grow.EksCtl.Docker)
}

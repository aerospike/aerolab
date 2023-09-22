package main

import "os"

type agiExecCmd struct {
	Plugin     agiExecPluginCmd     `command:"plugin" subcommands-optional:"true" description:"Aerospike-Grafana plugin"`
	GrafanaFix agiExecGrafanaFixCmd `command:"grafanafix" subcommands-optional:"true" description:"Deploy dashboards, configure grafana and load/save annotations"`
	Ingest     agiExecIngestCmd     `command:"ingest" subcommands-optional:"true" description:"Ingest logs into aerospike"`
	Proxy      agiExecProxyCmd      `command:"proxy" subcommands-optional:"true" description:"Proxy from aerolab to AGI services"`
	Help       helpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type agiExecPluginCmd struct {
	YamlFile       string  `short:"y" long:"yaml" description:"Yaml config file"`
	RedactPassword bool    `short:"r" long:"redact-passwords" description:"Redact passwords in yaml file after loading"`
	Help           helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecPluginCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	return nil
}

type agiExecGrafanaFixCmd struct {
	YamlFile       string  `short:"y" long:"yaml" description:"Yaml config file"`
	RedactPassword bool    `short:"r" long:"redact-passwords" description:"Redact passwords in yaml file after loading"`
	Help           helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecGrafanaFixCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	return nil
}

type agiExecIngestCmd struct {
	YamlFile       string  `short:"y" long:"yaml" description:"Yaml config file"`
	RedactPassword bool    `short:"r" long:"redact-passwords" description:"Redact passwords in yaml file after loading"`
	Help           helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecIngestCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	return nil
}

type agiExecProxyCmd struct {
	YamlFile       string  `short:"y" long:"yaml" description:"Yaml config file"`
	RedactPassword bool    `short:"r" long:"redact-passwords" description:"Redact passwords in yaml file after loading"`
	Help           helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *agiExecProxyCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	return nil
}

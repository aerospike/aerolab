package main

import "os"

type clientConfigureCmd struct {
	AMS         clientConfigureAMSCmd         `command:"ams" subcommands-optional:"true" description:"change which clusters prometheus points at"`
	Tools       clientConfigureToolsCmd       `command:"tools" subcommands-optional:"true" description:"add graph monitoring for AMS for asbenchmark"`
	VSCode      clientConfigureVSCodeCmd      `command:"vscode" subcommands-optional:"true" description:"add languages to VSCode"`
	Trino       clientConfigureTrinoCmd       `command:"trino" subcommands-optional:"true" description:"change aerospike seed IPs for trino"`
	RestGateway clientConfigureRestGatewayCmd `command:"rest-gateway" subcommands-optional:"true" description:"change aerospike seed IPs for the rest-gateway"`
	Firewall    clientConfigureFirewallCmd    `command:"firewall" subcommands-optional:"true" description:"Add firewall rules to existing client machines"`
	Help        helpCmd                       `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

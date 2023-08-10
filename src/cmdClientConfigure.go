package main

import (
	"errors"
	"os"
	"time"
)

type clientConfigureCmd struct {
	AMS         clientConfigureAMSCmd         `command:"ams" subcommands-optional:"true" description:"change which clusters prometheus points at"`
	Tools       clientConfigureToolsCmd       `command:"tools" subcommands-optional:"true" description:"add graph monitoring for AMS for asbenchmark"`
	VSCode      clientConfigureVSCodeCmd      `command:"vscode" subcommands-optional:"true" description:"add languages to VSCode"`
	Trino       clientConfigureTrinoCmd       `command:"trino" subcommands-optional:"true" description:"change aerospike seed IPs for trino"`
	RestGateway clientConfigureRestGatewayCmd `command:"rest-gateway" subcommands-optional:"true" description:"change aerospike seed IPs for the rest-gateway"`
	Firewall    clientConfigureFirewallCmd    `command:"firewall" subcommands-optional:"true" description:"Add firewall rules to existing client machines"`
	Expiry      clientAddExpiryCmd            `command:"expiry" subcommands-optional:"true" description:"Add or change hours until expiry for a client group (aws|gcp only)"`
	Help        helpCmd                       `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

type clientAddExpiryCmd struct {
	ClusterName TypeClientName `short:"n" long:"name" description:"Client name" default:"mydc"`
	Expires     time.Duration  `long:"expire" description:"length of life of nodes prior to expiry from now; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry" default:"30h"`
	Help        helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientAddExpiryCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type == "docker" {
		return errors.New("feature not supported on docker")
	}
	b.WorkOnClients()
	return b.ClusterExpiry(c.ClusterName.String(), c.Expires)
}

package main

import "os"

type clientConfigureCmd struct {
	AMS     clientConfigureAMSCmd     `command:"ams" subcommands-optional:"true" description:"change which clusters prometheus points at"`
	Jupyter clientConfigureJupyterCmd `command:"jupyter" subcommands-optional:"true" description:"add language kernels to jupyter"`
	VSCode  clientConfigureVSCodeCmd  `command:"vscode" subcommands-optional:"true" description:"add languages to VSCode"`
	Trino   clientConfigureTrinoCmd   `command:"trino" subcommands-optional:"true" description:"change aerospike seed IPs for trino"`
	Help    helpCmd                   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

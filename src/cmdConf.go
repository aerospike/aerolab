package main

import (
	"os"
)

type confCmd struct {
	Generator       confGeneratorCmd       `command:"generate" subcommands-optional:"true" description:"Generate or modify Aerospike configuration files"`
	FixMesh         confFixMeshCmd         `command:"fix-mesh" subcommands-optional:"true" description:"Fix mesh configuration in the cluster"`
	RackID          confRackIdCmd          `command:"rackid" subcommands-optional:"true" description:"Change/add rack-id to namespaces in the existing cluster nodes"`
	NamespaceMemory confNamespaceMemoryCmd `command:"namespace-memory" subcommands-optional:"true" description:"Adjust memory for a namespace using total percentages"`
	Adjust          confAdjustCmd          `command:"adjust" subcommands-optional:"true" description:"Adjust running Aerospike configuration parameters"`
	Help            helpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *confCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

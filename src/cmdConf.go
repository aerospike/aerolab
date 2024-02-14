package main

import (
	"os"
)

type confCmd struct {
	Generator       confGeneratorCmd       `command:"generate" subcommands-optional:"true" description:"Generate or modify Aerospike configuration files" webicon:"fas fa-gears" webhidden:"true"`
	FixMesh         confFixMeshCmd         `command:"fix-mesh" subcommands-optional:"true" description:"Fix mesh configuration in the cluster" webicon:"fas fa-screwdriver"`
	RackID          confRackIdCmd          `command:"rackid" subcommands-optional:"true" description:"Change/add rack-id to namespaces in the existing cluster nodes" webicon:"fas fa-id-badge"`
	NamespaceMemory confNamespaceMemoryCmd `command:"namespace-memory" subcommands-optional:"true" description:"Adjust memory for a namespace using total percentages" webicon:"fas fa-sd-card"`
	Adjust          confAdjustCmd          `command:"adjust" subcommands-optional:"true" description:"Adjust running Aerospike configuration parameters" webicon:"fas fa-sliders"`
	Help            helpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *confCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

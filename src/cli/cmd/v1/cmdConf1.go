package cmd

type ConfCmd struct {
	Generator       ConfGeneratorCmd       `command:"generate" subcommands-optional:"true" description:"Generate or modify Aerospike configuration files" webicon:"fas fa-gears" webhidden:"true"`
	SC              ConfSCCmd              `command:"sc" subcommands-optional:"true" description:"Configure the cluster to use strong-consistency, with roster and optional RF changes" webicon:"fas fa-gears"`
	FixMesh         ConfFixMeshCmd         `command:"fix-mesh" subcommands-optional:"true" description:"Fix mesh configuration in the cluster" webicon:"fas fa-screwdriver"`
	RackID          ConfRackIdCmd          `command:"rackid" subcommands-optional:"true" description:"Change/add rack-id to namespaces in the existing cluster nodes" webicon:"fas fa-id-badge"`
	NamespaceMemory ConfNamespaceMemoryCmd `command:"namespace-memory" subcommands-optional:"true" description:"Adjust memory for a namespace using total percentages" webicon:"fas fa-sd-card"`
	Adjust          ConfAdjustCmd          `command:"adjust" subcommands-optional:"true" description:"Adjust running Aerospike configuration parameters" webicon:"fas fa-sliders"`
	Help            HelpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

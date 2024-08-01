package main

import (
	"os"
)

type inventoryCmd struct {
	List          inventoryListCmd          `command:"list" subcommands-optional:"true" description:"List clusters, clients and templates" webicon:"fas fa-list"`
	Ansible       inventoryAnsibleCmd       `command:"ansible" subcommands-optional:"true" description:"Export inventory as ansible inventory" webicon:"fas fa-list"`
	Genders       inventoryGendersCmd       `command:"genders" subcommands-optional:"true" description:"Export inventory as genders file" webicon:"fas fa-list"`
	Hostfile      inventoryHostfileCmd      `command:"hostfile" subcommands-optional:"true" description:"Export inventory as hosts file" webicon:"fas fa-list"`
	InstanceTypes inventoryInstanceTypesCmd `command:"instance-types" subcommands-optional:"true" description:"Lookup GCP|AWS available instance types" webicon:"fas fa-table-list"`
	Help          helpCmd                   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *inventoryCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

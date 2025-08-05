package cmd

type InventoryCmd struct {
	List InventoryListCmd `command:"list" subcommands-optional:"true" description:"List clusters, clients and templates" webicon:"fas fa-list"`
	//Ansible       InventoryAnsibleCmd       `command:"ansible" subcommands-optional:"true" description:"Export inventory as ansible inventory" webicon:"fas fa-list"`
	//Genders       InventoryGendersCmd       `command:"genders" subcommands-optional:"true" description:"Export inventory as genders file" webicon:"fas fa-list"`
	//Hostfile      InventoryHostfileCmd      `command:"hostfile" subcommands-optional:"true" description:"Export inventory as hosts file" webicon:"fas fa-list"`
	InstanceTypes InventoryInstanceTypesCmd `command:"instance-types" subcommands-optional:"true" description:"Lookup GCP|AWS available instance types" webicon:"fas fa-table-list"`
	Help          HelpCmd                   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InventoryCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

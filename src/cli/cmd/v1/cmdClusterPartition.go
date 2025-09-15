package cmd

type ClusterPartitionCmd struct {
	//Create ClusterPartitionCreateCmd `command:"create" subcommands-optional:"true" description:"Blkdiscard disks and/or create partitions on disks" webicon:"fas fa-circle-plus"`
	//Mkfs   ClusterPartitionMkfsCmd   `command:"mkfs" subcommands-optional:"true" description:"Make filesystems on partitions and mount - for allflash" webicon:"fas fa-folder-tree"`
	//Conf   ClusterPartitionConfCmd   `command:"conf" subcommands-optional:"true" description:"Adjust Aerospike configuration files on nodes to use created partitions" webicon:"fas fa-gear"`
	//List   ClusterPartitionListCmd   `command:"list" subcommands-optional:"true" description:"List disks and partitions" webicon:"fas fa-list"`
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterPartitionCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

// TODO: all the partition commands

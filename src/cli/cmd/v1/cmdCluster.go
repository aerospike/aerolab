package cmd

type ClusterCmd struct {
	Create    ClusterCreateCmd    `command:"create" subcommands-optional:"true" description:"Create a new cluster" webicon:"fas fa-circle-plus" invwebforce:"true"`
	Grow      ClusterGrowCmd      `command:"grow" subcommands-optional:"true" description:"Add nodes to cluster" webicon:"fas fa-circle-plus" invwebforce:"true"`
	Apply     ClusterApplyCmd     `command:"apply" subcommands-optional:"true" description:"Apply a cluster size (grow/shrink/create)" webicon:"fas fa-gear"`
	List      ClusterListCmd      `command:"list" subcommands-optional:"true" description:"List clusters" webicon:"fas fa-list"`
	Start     ClusterStartCmd     `command:"start" subcommands-optional:"true" description:"Start cluster" webicon:"fas fa-play" invwebforce:"true"`
	Stop      ClusterStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop cluster" webicon:"fas fa-stop" invwebforce:"true"`
	Destroy   ClusterDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy cluster" webicon:"fas fa-trash" invwebforce:"true"`
	Add       ClusterAddCmd       `command:"add" subcommands-optional:"true" description:"Add features to clusters, ex: ams" webicon:"fas fa-gear"`
	Partition ClusterPartitionCmd `command:"partition" subcommands-optional:"true" description:"node disk partitioner" webicon:"fas fa-divide"`
	Attach    AttachShellCmd      `command:"attach" subcommands-optional:"true" description:"symlink to: attach shell" webicon:"fas fa-terminal" simplemode:"false"`
	Share     ClusterShareCmd     `command:"share" subcommands-optional:"true" description:"AWS/GCP: share the cluster by importing a provided ssh public key file" webicon:"fas fa-share"`
	Help      HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

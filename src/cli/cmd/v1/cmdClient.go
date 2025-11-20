package cmd

type ClientCmd struct {
	Create    ClientCreateCmd    `command:"create" subcommands-optional:"true" description:"Create new client machines" webicon:"fas fa-circle-plus"`
	Configure ClientConfigureCmd `command:"configure" subcommands-optional:"true" description:"(re)configure some clients, such as ams" webicon:"fas fa-gear"`
	List      ClientListCmd      `command:"list" subcommands-optional:"true" description:"List client machine groups" webicon:"fas fa-list"`
	Start     ClientStartCmd     `command:"start" subcommands-optional:"true" description:"Start a client machine group" webicon:"fas fa-play"`
	Stop      ClientStopCmd      `command:"stop" subcommands-optional:"true" description:"Stop a client machine group" webicon:"fas fa-stop"`
	Grow      ClientGrowCmd      `command:"grow" subcommands-optional:"true" description:"Grow a client machine group" webicon:"fas fa-circle-plus"`
	Destroy   ClientDestroyCmd   `command:"destroy" subcommands-optional:"true" description:"Destroy client(s)" webicon:"fas fa-trash"`
	Attach    AttachClientCmd    `command:"attach" subcommands-optional:"true" description:"Attach to a client - symlink to: attach client" webicon:"fas fa-terminal" simplemode:"false"`
	Share     ClientShareCmd     `command:"share" subcommands-optional:"true" description:"Share a client with other users - wrapper around ssh-copy-id" webicon:"fas fa-share"`
	Help      HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type ClientCreateCmd struct {
	None    ClientCreateNoneCmd    `command:"none" subcommands-optional:"true" description:"Vanilla OS image with no package modifications" webicon:"fas fa-file"`
	Base    ClientCreateBaseCmd    `command:"base" subcommands-optional:"true" description:"Simple base image" webicon:"fas fa-grip-lines"`
	Tools   ClientCreateToolsCmd   `command:"tools" subcommands-optional:"true" description:"Aerospike-tools" webicon:"fas fa-toolbox"`
	AMS     ClientCreateAMSCmd     `command:"ams" subcommands-optional:"true" description:"Prometheus and grafana for AMS; for exporter see: cluster add exporter" webicon:"fas fa-layer-group"`
	VSCode  ClientCreateVSCodeCmd  `command:"vscode" subcommands-optional:"true" description:"Launch a VSCode IDE client" webicon:"fas fa-code"`
	Graph   ClientCreateGraphCmd   `command:"graph" subcommands-optional:"true" description:"Deploy a graph client machine" webicon:"fas fa-diagram-project"`
	EksCtl  ClientCreateEksCtlCmd  `command:"eksctl" subcommands-optional:"true" description:"Deploy a client machine with preconfigured eksctl for k8s aerospike cluster deployments" webicon:"fas fa-box-open"`
	Help    HelpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientCreateCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type ClientGrowCmd struct {
	None    ClientCreateNoneCmd    `command:"none" subcommands-optional:"true" description:"Vanilla OS image with no package modifications" webicon:"fas fa-file"`
	Base    ClientCreateBaseCmd    `command:"base" subcommands-optional:"true" description:"Simple base image" webicon:"fas fa-grip-lines"`
	Tools   ClientCreateToolsCmd   `command:"tools" subcommands-optional:"true" description:"Aerospike-tools" webicon:"fas fa-toolbox"`
	AMS     ClientCreateAMSCmd     `command:"ams" subcommands-optional:"true" description:"Prometheus and grafana for AMS; for exporter see: cluster add exporter" webicon:"fas fa-layer-group"`
	VSCode  ClientCreateVSCodeCmd  `command:"vscode" subcommands-optional:"true" description:"Launch a VSCode IDE client" webicon:"fas fa-code"`
	Graph   ClientCreateGraphCmd   `command:"graph" subcommands-optional:"true" description:"Deploy a graph client machine" webicon:"fas fa-diagram-project"`
	EksCtl  ClientCreateEksCtlCmd  `command:"eksctl" subcommands-optional:"true" description:"Deploy a client machine with preconfigured eksctl for k8s aerospike cluster deployments" webicon:"fas fa-box-open"`
	Help    HelpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientGrowCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}


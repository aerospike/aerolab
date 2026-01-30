package cmd

var cloudVersion = "v1"
var cloudDbPath = "/database/clusters"

type CloudCmd struct {
	ListInstanceTypes CloudListInstanceTypesCmd `command:"list-instance-types" subcommands-optional:"true" description:"List instance types" webicon:"fas fa-list"`
	Secrets           CloudSecretsCmd           `command:"secrets" subcommands-optional:"true" description:"Secrets operations" webicon:"fas fa-key"`
	Clusters          CloudClustersCmd          `command:"clusters" subcommands-optional:"true" description:"Clusters operations" webicon:"fas fa-database"`
	GenConfTemplates  CloudGenConfTemplatesCmd  `command:"gen-conf-templates" subcommands-optional:"true" description:"Generate configuration templates from OpenAPI spec" webicon:"fas fa-file-code"`
	Help              HelpCmd                   `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type CloudSecretsCmd struct {
	List   CloudSecretsListCmd   `command:"list" subcommands-optional:"true" description:"List secrets" webicon:"fas fa-list"`
	Create CloudSecretsCreateCmd `command:"create" subcommands-optional:"true" description:"Create secret" webicon:"fas fa-plus"`
	Delete CloudSecretsDeleteCmd `command:"delete" subcommands-optional:"true" description:"Delete secret" webicon:"fas fa-trash"`
	Help   HelpCmd               `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudSecretsCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type CloudClustersCmd struct {
	List             CloudClustersListCmd             `command:"list" subcommands-optional:"true" description:"List aerospike clusters" webicon:"fas fa-list"`
	Create           CloudClustersCreateCmd           `command:"create" subcommands-optional:"true" description:"Create an aerospike cluster and VPC peering" webicon:"fas fa-plus"`
	Delete           CloudClustersDeleteCmd           `command:"delete" subcommands-optional:"true" description:"Delete an aerospike cluster and VPC peering" webicon:"fas fa-trash"`
	Update           CloudClustersUpdateCmd           `command:"update" subcommands-optional:"true" description:"Update an aerospike cluster" webicon:"fas fa-pencil"`
	PeerVPC          CloudClustersPeerVPCCmd          `command:"peer-vpc" subcommands-optional:"true" description:"Initiate and accept VPC peering for a cluster" webicon:"fas fa-network-wired"`
	VPCPeeringStatus CloudClustersVPCPeeringStatusCmd `command:"vpc-peering-status" subcommands-optional:"true" description:"Get VPC peering status for a cluster" webicon:"fas fa-info-circle"`
	Get              CloudClustersGetCmd              `command:"get" subcommands-optional:"true" description:"Get cluster connection details" webicon:"fas fa-info"`
	Wait             CloudClustersWaitCmd             `command:"wait" subcommands-optional:"true" description:"Wait for cluster health.status" webicon:"fas fa-hourglass"`
	Credentials      CloudClustersCredentialsCmd      `command:"credentials" subcommands-optional:"true" description:"Cluster credentials operations" webicon:"fas fa-key"`
	EnableLogsAccess CloudClustersEnableLogsAccessCmd `command:"enable-logs-access" subcommands-optional:"true" description:"Enable S3 log bucket access for specified IAM roles" webicon:"fas fa-file-alt"`
	Help             HelpCmd                          `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudClustersCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type CloudClustersCredentialsCmd struct {
	List   CloudClustersCredentialsListCmd   `command:"list" subcommands-optional:"true" description:"List cluster credentials" webicon:"fas fa-list"`
	Create CloudClustersCredentialsCreateCmd `command:"create" subcommands-optional:"true" description:"Create cluster credential" webicon:"fas fa-plus"`
	Delete CloudClustersCredentialsDeleteCmd `command:"delete" subcommands-optional:"true" description:"Delete cluster credential" webicon:"fas fa-trash"`
	Help   HelpCmd                           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudClustersCredentialsCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

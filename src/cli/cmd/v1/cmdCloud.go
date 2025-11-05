package cmd

type CloudCmd struct {
	ListInstanceTypes CloudListInstanceTypesCmd `command:"list-instance-types" subcommands-optional:"true" description:"List instance types" webicon:"fas fa-list"`
	Secrets           CloudSecretsCmd           `command:"secrets" subcommands-optional:"true" description:"Secrets operations" webicon:"fas fa-key"`
	Databases         CloudDatabasesCmd         `command:"databases" subcommands-optional:"true" description:"Databases operations" webicon:"fas fa-database"`
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

type CloudDatabasesCmd struct {
	List        CloudDatabasesListCmd        `command:"list" subcommands-optional:"true" description:"List aerospike databases" webicon:"fas fa-list"`
	Create      CloudDatabasesCreateCmd      `command:"create" subcommands-optional:"true" description:"Create an aerospike database and VPC peering" webicon:"fas fa-plus"`
	Delete      CloudDatabasesDeleteCmd      `command:"delete" subcommands-optional:"true" description:"Delete an aerospike database and VPC peering" webicon:"fas fa-trash"`
	Update      CloudDatabasesUpdateCmd      `command:"update" subcommands-optional:"true" description:"Update an aerospike database" webicon:"fas fa-pencil"`
	PeerVPC     CloudDatabasesPeerVPCCmd     `command:"peer-vpc" subcommands-optional:"true" description:"Initiate and accept VPC peering for a database" webicon:"fas fa-network-wired"`
	Get         CloudDatabasesGetCmd         `command:"get" subcommands-optional:"true" description:"Get database connection details" webicon:"fas fa-info"`
	Wait        CloudDatabasesWaitCmd        `command:"wait" subcommands-optional:"true" description:"Wait for database health.status" webicon:"fas fa-hourglass"`
	Credentials CloudDatabasesCredentialsCmd `command:"credentials" subcommands-optional:"true" description:"Database credentials operations" webicon:"fas fa-key"`
	Help        HelpCmd                      `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudDatabasesCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

type CloudDatabasesCredentialsCmd struct {
	List   CloudDatabasesCredentialsListCmd   `command:"list" subcommands-optional:"true" description:"List database credentials" webicon:"fas fa-list"`
	Create CloudDatabasesCredentialsCreateCmd `command:"create" subcommands-optional:"true" description:"Create database credential" webicon:"fas fa-plus"`
	Delete CloudDatabasesCredentialsDeleteCmd `command:"delete" subcommands-optional:"true" description:"Delete database credential" webicon:"fas fa-trash"`
	Help   HelpCmd                            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudDatabasesCredentialsCmd) Execute(args []string) error {
	return c.Help.Execute(args)
}

package main

type commands struct {
	Config     configCmd     `command:"config" subcommands-optional:"true" description:"Show or change aerolab configuration"`
	Cluster    clusterCmd    `command:"cluster" subcommands-optional:"true" description:"Create and manage Aerospike clusters and nodes"`
	Aerospike  aerospikeCmd  `command:"aerospike" subcommands-optional:"true" description:"Aerospike daemon controls"`
	Client     clientCmd     `command:"client" subcommands-optional:"true" description:"Create and manage Client machine groups"`
	Inventory  inventoryCmd  `command:"inventory" subcommands-optional:"true" description:"List or operate on all clusters, clients and templates"`
	Attach     attachCmd     `command:"attach" subcommands-optional:"true" description:"Attach to a node and run a command"`
	Net        netCmd        `command:"net" subcommands-optional:"true" description:"Firewall and latency simulation"`
	Conf       confCmd       `command:"conf" subcommands-optional:"true" description:"Manage Aerospike configuration on running nodes"`
	Tls        tlsCmd        `command:"tls" subcommands-optional:"true" description:"Create or copy TLS certificates"`
	Data       dataCmd       `command:"data" subcommands-optional:"true" description:"Insert/delete Aerospike data"`
	Template   templateCmd   `command:"template" subcommands-optional:"true" description:"Manage or delete template images"`
	Installer  installerCmd  `command:"installer" subcommands-optional:"true" description:"List or download Aerospike installer versions"`
	Logs       logsCmd       `command:"logs" subcommands-optional:"true" description:"show or download logs"`
	Files      filesCmd      `command:"files" subcommands-optional:"true" description:"Upload/Download files to/from clients or clusters"`
	XDR        xdrCmd        `command:"xdr" subcommands-optional:"true" description:"Mange clusters' xdr configuration"`
	Roster     rosterCmd     `command:"roster" subcommands-optional:"true" description:"Show or apply strong-consistency rosters"`
	Version    versionCmd    `command:"version" subcommands-optional:"true" description:"Print AeroLab version"`
	Completion completionCmd `command:"completion" subcommands-optional:"true" description:"Install shell completion scripts"`
	Rest       restCmd       `command:"rest-api" subcommands-optional:"true" description:"Launch HTTP rest API"`
	commandsDefaults
}

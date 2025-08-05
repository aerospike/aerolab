package cmd

type Commands struct {
	Config  ConfigCmd  `command:"config" subcommands-optional:"true" description:"Show or change aerolab configuration" webicon:"fas fa-toolbox"`
	Cluster clusterCmd `command:"cluster" subcommands-optional:"true" description:"Create and manage Aerospike clusters and nodes" webicon:"fas fa-database"`
	//Aerospike    aerospikeCmd    `command:"aerospike" subcommands-optional:"true" description:"Aerospike daemon controls" webicon:"fas fa-a"`
	//Client       clientCmd       `command:"client" subcommands-optional:"true" description:"Create and manage Client machine groups" webicon:"fas fa-tv"`
	Inventory InventoryCmd `command:"inventory" subcommands-optional:"true" description:"List or operate on all clusters, clients and templates" webicon:"fas fa-warehouse"`
	//Attach       attachCmd       `command:"attach" subcommands-optional:"true" description:"Attach to a node and run a command" webicon:"fas fa-plug" simplemode:"false"`
	//Net          netCmd          `command:"net" subcommands-optional:"true" description:"Firewall and latency simulation" webicon:"fas fa-network-wired"`
	//Conf         confCmd         `command:"conf" subcommands-optional:"true" description:"Manage Aerospike configuration on running nodes" webicon:"fas fa-wrench"`
	//Tls          tlsCmd          `command:"tls" subcommands-optional:"true" description:"Create or copy TLS certificates" webicon:"fas fa-lock"`
	//Data         dataCmd         `command:"data" subcommands-optional:"true" description:"Insert/delete Aerospike data" webicon:"fas fa-folder-open"`
	//Template     templateCmd     `command:"template" subcommands-optional:"true" description:"Manage or delete template images" webicon:"fas fa-file-image"`
	Installer InstallerCmd `command:"installer" subcommands-optional:"true" description:"List or download Aerospike installer versions" webicon:"fas fa-plus"`
	//Logs         logsCmd         `command:"logs" subcommands-optional:"true" description:"show or download logs" webicon:"fas fa-bars-progress"`
	//Files        filesCmd        `command:"files" subcommands-optional:"true" description:"Upload/Download files to/from clients or clusters" webicon:"fas fa-file"`
	//XDR          xdrCmd          `command:"xdr" subcommands-optional:"true" description:"Mange clusters' xdr configuration" webicon:"fas fa-object-group"`
	//Roster       rosterCmd       `command:"roster" subcommands-optional:"true" description:"Show or apply strong-consistency rosters" webicon:"fas fa-sliders"`
	Completion CompletionCmd `command:"completion" subcommands-optional:"true" description:"Install shell completion scripts" webicon:"fas fa-arrows-turn-to-dots" webhidden:"true"`
	//AGI          agiCmd          `command:"agi" subcommands-optional:"true" description:"Launch or manage AGI troubleshooting instances" webicon:"fas fa-chart-line"`
	//Volume       volumeCmd       `command:"volume" subcommands-optional:"true" description:"Volume management (AWS EFS/GCP Volume only)" webicon:"fas fa-hard-drive" simplemode:"false"`
	ShowCommands ShowcommandsCmd `command:"showcommands" subcommands-optional:"true" description:"Install showsysinfo,showconf,showinterrupts on the current system" webicon:"fas fa-terminal"`
	//Rest         restCmd         `command:"rest-api" subcommands-optional:"true" description:"Launch HTTP rest API" webicon:"fas fa-globe" webhidden:"true"`
	//Web          webCmd          `command:"webui" subcommands-optional:"true" description:"Launch AeroLab Web UI" webicon:"fas fa-globe" webhidden:"true"`
	Version VersionCmd `command:"version" subcommands-optional:"true" description:"Print AeroLab version" webicon:"fas fa-code-branch"`
	Upgrade UpgradeCmd `command:"upgrade" subcommands-optional:"true" description:"Upgrade AeroLab binary" webicon:"fas fa-circle-up"`
	//WebRun       webRunCmd       `command:"webrun" subcommands-optional:"true" description:"Upgrade AeroLab binary" hidden:"true"`
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// TODO: delete me: I am here so that the code will work before I code this function
type clusterCmd struct {
	Create clusterCreateCmd `command:"create" subcommands-optional:"true" description:"Create a new Aerospike cluster" webicon:"fas fa-plus"`
}

// TODO: delete me: I am here so that the code will work before I code this function
type clusterCreateCmd struct {
	FeaturesFilePath string `short:"f" long:"features-file-path" description:"Path to the features file" webtype:"text"`
}

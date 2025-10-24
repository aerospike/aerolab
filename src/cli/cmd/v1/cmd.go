package cmd

import "embed"

//go:embed scripts
var scripts embed.FS

type Commands struct {
	Config    ConfigCmd    `command:"config" subcommands-optional:"true" description:"Show or change aerolab configuration" webicon:"fas fa-toolbox"`
	Cloud     CloudCmd     `command:"cloud" subcommands-optional:"true" description:"Aerospike Cloud" webicon:"fa-brands fa-warehouse"`
	Cluster   ClusterCmd   `command:"cluster" subcommands-optional:"true" description:"Create and manage Aerospike clusters and nodes" webicon:"fas fa-database"`
	Aerospike AerospikeCmd `command:"aerospike" subcommands-optional:"true" description:"Aerospike daemon controls" webicon:"fas fa-a"`
	//Client       clientCmd       `command:"client" subcommands-optional:"true" description:"Create and manage Client machine groups" webicon:"fas fa-tv"`
	Inventory InventoryCmd `command:"inventory" subcommands-optional:"true" description:"List or operate on all clusters, clients and images" webicon:"fas fa-warehouse"`
	Instances InstancesCmd `command:"instances" subcommands-optional:"true" description:"Create and manage instance clusters (cluster/client create use this)" webicon:"fas fa-server"`
	Attach    AttachCmd    `command:"attach" subcommands-optional:"true" description:"Attach to a node and run a command" webicon:"fas fa-plug" simplemode:"false"`
	//Net          netCmd          `command:"net" subcommands-optional:"true" description:"Firewall and latency simulation" webicon:"fas fa-network-wired"`
	Conf ConfCmd `command:"conf" subcommands-optional:"true" description:"Manage Aerospike configuration on running nodes" webicon:"fas fa-wrench"`
	//Tls          tlsCmd          `command:"tls" subcommands-optional:"true" description:"Create or copy TLS certificates" webicon:"fas fa-lock"`
	//Data         dataCmd         `command:"data" subcommands-optional:"true" description:"Insert/delete Aerospike data" webicon:"fas fa-folder-open"`
	Images    ImagesCmd    `command:"images" subcommands-optional:"true" description:"Manage or delete images" webicon:"fas fa-file-image"`
	Template  TemplateCmd  `command:"template" subcommands-optional:"true" description:"Manage or delete aerospike server templates" webicon:"fas fa-file-image"`
	Installer InstallerCmd `command:"installer" subcommands-optional:"true" description:"List or download Aerospike installer versions" webicon:"fas fa-plus"`
	Logs      LogsCmd      `command:"logs" subcommands-optional:"true" description:"show or download logs" webicon:"fas fa-bars-progress"`
	Files     FilesCmd     `command:"files" subcommands-optional:"true" description:"Upload/Download files to/from instances" webicon:"fas fa-file"`
	//XDR          xdrCmd          `command:"xdr" subcommands-optional:"true" description:"Mange clusters' xdr configuration" webicon:"fas fa-object-group"`
	Roster     RosterCmd     `command:"roster" subcommands-optional:"true" description:"Show or apply strong-consistency rosters" webicon:"fas fa-sliders"`
	Completion CompletionCmd `command:"completion" subcommands-optional:"true" description:"Install shell completion scripts" webicon:"fas fa-arrows-turn-to-dots" webhidden:"true"`
	//AGI          agiCmd          `command:"agi" subcommands-optional:"true" description:"Launch or manage AGI troubleshooting instances" webicon:"fas fa-chart-line"`
	Volumes      VolumesCmd      `command:"volumes" subcommands-optional:"true" description:"Volume management (AWS EFS/GCP Volume only)" webicon:"fas fa-hard-drive" simplemode:"false"`
	ShowCommands ShowcommandsCmd `command:"showcommands" subcommands-optional:"true" description:"Install showsysinfo,showconf,showinterrupts on the current system" webicon:"fas fa-terminal"`
	//Rest         restCmd         `command:"rest-api" subcommands-optional:"true" description:"Launch HTTP rest API" webicon:"fas fa-globe" webhidden:"true"`
	//Web          webCmd          `command:"webui" subcommands-optional:"true" description:"Launch AeroLab Web UI" webicon:"fas fa-globe" webhidden:"true"`
	Version VersionCmd `command:"version" subcommands-optional:"true" description:"Print AeroLab version" webicon:"fas fa-code-branch"`
	Upgrade UpgradeCmd `command:"upgrade" subcommands-optional:"true" description:"Upgrade AeroLab binary" webicon:"fas fa-circle-up"`
	//WebRun       webRunCmd       `command:"webrun" subcommands-optional:"true" description:"Upgrade AeroLab binary" hidden:"true"`
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type commands struct {
	Config       configCmd       `command:"config" subcommands-optional:"true" description:"Show or change aerolab configuration"`
	Cluster      clusterCmd      `command:"cluster" subcommands-optional:"true" description:"Create and manage Aerospike clusters and nodes"`
	Aerospike    aerospikeCmd    `command:"aerospike" subcommands-optional:"true" description:"Aerospike daemon controls"`
	Client       clientCmd       `command:"client" subcommands-optional:"true" description:"Create and manage Client machine groups"`
	Inventory    inventoryCmd    `command:"inventory" subcommands-optional:"true" description:"List or operate on all clusters, clients and templates"`
	Attach       attachCmd       `command:"attach" subcommands-optional:"true" description:"Attach to a node and run a command"`
	Net          netCmd          `command:"net" subcommands-optional:"true" description:"Firewall and latency simulation"`
	Conf         confCmd         `command:"conf" subcommands-optional:"true" description:"Manage Aerospike configuration on running nodes"`
	Tls          tlsCmd          `command:"tls" subcommands-optional:"true" description:"Create or copy TLS certificates"`
	Data         dataCmd         `command:"data" subcommands-optional:"true" description:"Insert/delete Aerospike data"`
	Template     templateCmd     `command:"template" subcommands-optional:"true" description:"Manage or delete template images"`
	Installer    installerCmd    `command:"installer" subcommands-optional:"true" description:"List or download Aerospike installer versions"`
	Logs         logsCmd         `command:"logs" subcommands-optional:"true" description:"show or download logs"`
	Files        filesCmd        `command:"files" subcommands-optional:"true" description:"Upload/Download files to/from clients or clusters"`
	XDR          xdrCmd          `command:"xdr" subcommands-optional:"true" description:"Mange clusters' xdr configuration"`
	Roster       rosterCmd       `command:"roster" subcommands-optional:"true" description:"Show or apply strong-consistency rosters"`
	Version      versionCmd      `command:"version" subcommands-optional:"true" description:"Print AeroLab version"`
	Completion   completionCmd   `command:"completion" subcommands-optional:"true" description:"Install shell completion scripts"`
	Rest         restCmd         `command:"rest-api" subcommands-optional:"true" description:"Launch HTTP rest API"`
	AGI          agiCmd          `command:"agi" subcommands-optional:"true" description:"Launch or manage AGI troubleshooting instances"`
	Volume       volumeCmd       `command:"volume" subcommands-optional:"true" description:"Volume management (AWS EFS only)"`
	ShowCommands showcommandsCmd `command:"showcommands" subcommands-optional:"true" description:"Install showsysinfo,showconf,showinterrupts on the current system"`
	commandsDefaults
}

type showcommandsCmd struct {
	DestDir string  `short:"d" long:"destination" default:"/usr/local/bin/"`
	Help    helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *showcommandsCmd) Execute(args []string) error {
	if earlyProcessNoBackend(args) {
		return nil
	}
	cur, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get absolute path os self: %s", err)
	}
	log.Printf("Discovered absolute path: %s", cur)
	for _, dest := range []string{"showconf", "showsysinfo", "showinterrupts"} {
		d := filepath.Join(c.DestDir, dest)
		log.Printf("> ln -s %s %s", cur, d)
		if _, err := os.Stat(d); err == nil {
			os.Remove(d)
		}
		err = os.Symlink(cur, d)
		if err != nil {
			log.Printf("ERROR symlinking %s->%s : %s", cur, d, err)
		}
	}
	log.Println("Done")
	return nil
}

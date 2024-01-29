package main

import (
	"os"
)

type filesCmd struct {
	Download filesDownloadCmd `command:"download" alias:"d" subcommands-optional:"true" description:"Download files from a node" webicon:"fas fa-download"`
	Upload   filesUploadCmd   `command:"upload" alias:"u" subcommands-optional:"true" description:"Upload files to a node" webicon:"fas fa-upload"`
	Edit     filesEditCmd     `command:"edit" subcommands-optional:"true" description:"Download, edit and re-upload a file" webicon:"fas fa-pen-to-square"`
	Sync     filesSyncCmd     `command:"sync" subcommands-optional:"true" description:"Sync files or a directory from one node to others" webicon:"fas fa-rotate"`
	Help     helpCmd          `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *filesCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

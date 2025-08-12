package cmd

type FilesCmd struct {
	Download FilesDownloadCmd `command:"download" alias:"d" subcommands-optional:"true" description:"Download files from a node" webicon:"fas fa-download" webcommandtype:"download"`
	Upload   FilesUploadCmd   `command:"upload" alias:"u" subcommands-optional:"true" description:"Upload files to a node" webicon:"fas fa-upload"`
	Edit     FilesEditCmd     `command:"edit" subcommands-optional:"true" description:"Download, edit and re-upload a file" webicon:"fas fa-pen-to-square" webhidden:"true"`
	Sync     FilesSyncCmd     `command:"sync" subcommands-optional:"true" description:"Sync files or a directory from one node to others" webicon:"fas fa-rotate"`
	Help     HelpCmd          `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *FilesCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

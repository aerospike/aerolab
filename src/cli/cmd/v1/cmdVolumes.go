package cmd

type VolumesCmd struct {
	Create     VolumesCreateCmd     `command:"create" subcommands-optional:"true" description:"Create a volume" webicon:"fas fa-circle-plus" invwebforce:"true" simplemode:"false"`
	List       VolumesListCmd       `command:"list" subcommands-optional:"true" description:"List volumes" webicon:"fas fa-list" simplemode:"false"`
	Attach     VolumesAttachCmd     `command:"attach" subcommands-optional:"true" description:"Mount a volume on a node" webicon:"fas fa-hard-drive" simplemode:"false"`
	Resize     VolumesResizeCmd     `command:"grow" subcommands-optional:"true" description:"GCP only: grow a volume; if the volume is not attached, the filesystem will not be resized automatically" webicon:"fas fa-expand" simplemode:"false"`
	Detach     VolumesDetachCmd     `command:"detach" subcommands-optional:"true" description:"GCP only: detach a volume for an instance" webicon:"fas fa-square-minus" simplemode:"false"`
	AddTags    VolumesAddTagsCmd    `command:"add-tags" subcommands-optional:"true" description:"Add tags to volumes" webicon:"fas fa-tags" simplemode:"false"`
	RemoveTags VolumesRemoveTagsCmd `command:"remove-tags" subcommands-optional:"true" description:"Remove tags from volumes" webicon:"fas fa-tags" simplemode:"false"`
	Delete     VolumesDeleteCmd     `command:"delete" subcommands-optional:"true" description:"Delete a volume" webicon:"fas fa-trash" invwebforce:"true" simplemode:"false"`
	Help       HelpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *VolumesCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

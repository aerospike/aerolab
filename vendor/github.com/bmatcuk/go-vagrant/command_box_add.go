package vagrant

import "errors"

type CheckSumType string

const (
	MD5    CheckSumType = "md5"
	SHA1   CheckSumType = "sha1"
	SHA256 CheckSumType = "sha256"
)

// A BoxAddCommand specifies the options and output of vagrant box add.
type BoxAddCommand struct {
	BaseCommand
	ErrorResponse

	// Clean any temporary download files. This will prevent resuming a
	// previously started download.
	Clean bool

	// Overwrite an existing box if it exists
	Force bool

	// Name, url, or path of the box.
	Location string

	// Name of the box. Only has to be set if the box is provided as a path or
	// url without metadata.
	Name string

	// Checksum for the box
	Checksum string

	// Type of the supplied Checksum. Allowed values are vagrant.MD5,
	// vagrant.SHA1, vagrant.SHA256
	CheckSumType CheckSumType
}

// BoxAdd adds a vagrant box to your local boxes.  Its parameter location is
// the name, url, or path of the box. After setting options as appropriate, you
// must call Run() or Start() followed by Wait() to execute. Errors will be
// recorded in Error.
func (client *VagrantClient) BoxAdd(location string) *BoxAddCommand {
	return &BoxAddCommand{
		Location:      location,
		BaseCommand:   newBaseCommand(client),
		ErrorResponse: newErrorResponse(),
	}
}

func (cmd *BoxAddCommand) buildArguments() ([]string, error) {
	args := []string{"add"}
	if cmd.Clean {
		args = append(args, "-c")
	}
	if cmd.Force {
		args = append(args, "-f")
	}
	if cmd.Name != "" {
		args = append(args, "--name", cmd.Name)
	}
	if cmd.Checksum != "" {
		args = append(args, "--checksum", cmd.Checksum)
	}
	if cmd.CheckSumType != "" {
		if cmd.Checksum == "" {
			return nil, errors.New("box add: no checksum set even though checksum type was set")
		}
		args = append(args, "--checksum-type", string(cmd.CheckSumType))
	}
	if cmd.Location == "" {
		return nil, errors.New("box add: location must be provided")
	}
	args = append(args, cmd.Location)
	return args, nil
}

func (cmd *BoxAddCommand) init() error {
	args, err := cmd.buildArguments()
	if err != nil {
		return err
	}
	return cmd.BaseCommand.init(&cmd.ErrorResponse, "box", args...)
}

// Run the command
func (cmd *BoxAddCommand) Run() error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start the command. You must call Wait() to complete execution.
func (cmd *BoxAddCommand) Start() error {
	if err := cmd.init(); err != nil {
		return err
	}
	return cmd.BaseCommand.Start()
}

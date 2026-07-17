package vagrant

import (
	"os"
	"os/exec"
	"path/filepath"
)

// VagrantClient is the main entry point to the library. Users should construct
// a new VagrantClient using NewVagrantClient().
type VagrantClient struct {
	// Directory where the Vagrantfile can be found.
	//
	// Normally this would be set by NewVagrantClient() and shouldn't
	// be changed afterward.
	VagrantfileDir string

	executable   string
	preArguments []string
}

// NewVagrantClient creates a new VagrantClient.
//
// vagrantfileDir should be the path to a directory where the Vagrantfile
// exists.
func NewVagrantClient(vagrantfileDir string) (*VagrantClient, error) {
	// Verify the vagrant command is in the path
	path, err := exec.LookPath("vagrant")
	if err != nil {
		return nil, err
	}

	// Verify a Vagrantfile exists
	vagrantfilePath := filepath.Join(vagrantfileDir, "Vagrantfile")
	if _, err := os.Stat(vagrantfilePath); err != nil {
		return nil, err
	}

	return &VagrantClient{
		VagrantfileDir: vagrantfileDir,
		executable:     path,
	}, nil
}

func (client *VagrantClient) buildArguments(subcommand string) []string {
	if client.preArguments == nil || len(client.preArguments) == 0 {
		return []string{subcommand, "--machine-readable"}
	}
	return append(client.preArguments, subcommand, "--machine-readable")
}

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClientCreateNoneCmd struct {
	ClientName         TypeClientName           `short:"n" long:"group-name" description:"Client group name" default:"client"`
	ClientCount        int                      `short:"c" long:"count" description:"Number of clients" default:"1"`
	Owner              string                   `short:"o" long:"owner" description:"Owner of the instances"`
	AWS                InstancesCreateCmdAws    `group:"AWS" description:"backend-aws" namespace:"aws"`
	GCP                InstancesCreateCmdGcp    `group:"GCP" description:"backend-gcp" namespace:"gcp"`
	Docker             InstancesCreateCmdDocker `group:"Docker" description:"backend-docker" namespace:"docker"`
	OS                 string                   `long:"os" description:"OS to use for the instances" default:"ubuntu"`
	Version            string                   `long:"version" description:"Version of the OS to use for the instances" default:"24.04"`
	Arch               string                   `long:"arch" description:"Architecture override to use for the instances (amd64, arm64)"`
	NoSetHostname      bool                     `short:"H" long:"no-set-hostname" description:"By default, hostname of each machine will be set, use this to prevent hostname change"`
	NoSetDNS           bool                     `long:"no-set-dns" description:"Set to prevent aerolab from updating resolved to use 1.1.1.1/8.8.8.8 DNS"`
	StartScript        flags.Filename           `short:"X" long:"start-script" description:"Optionally specify a script to be installed which will run when the client machine starts"`
	TypeOverride       string                   `long:"type-override" description:"Override the client type label"`
	ParallelSSHThreads int                      `long:"threads" description:"Number of threads to use for the execution" default:"10"`
	Help               HelpCmd                  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientCreateNoneCmd) Execute(args []string) error {
	isGrow := len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow"

	var cmd []string
	if isGrow {
		cmd = []string{"client", "grow", "none"}
	} else {
		cmd = []string{"client", "create", "none"}
	}

	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	_, err = c.createNoneClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateNoneCmd) createNoneClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "create", "none"}, c)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	// Check if client already exists
	existing := inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).WithClusterName(c.ClientName.String())
	if existing != nil && existing.Count() > 0 && !isGrow {
		return nil, fmt.Errorf("client %s already exists, did you mean 'grow'?", c.ClientName.String())
	}
	if (existing == nil || existing.Count() == 0) && isGrow {
		return nil, fmt.Errorf("client %s doesn't exist, did you mean 'create'?", c.ClientName.String())
	}

	// Set client type tag
	clientType := "none"
	if c.TypeOverride != "" {
		clientType = c.TypeOverride
		logger.Info("Overriding client type: %s", clientType)
	}

	// Prepare tags slice
	tags := []string{
		"aerolab.old.type=client",
		fmt.Sprintf("aerolab.client.type=%s", clientType),
	}

	// Configure hostname and DNS settings
	if !c.NoSetHostname {
		tags = append(tags, "aerolab.set-hostname=true")
	}
	if !c.NoSetDNS {
		tags = append(tags, "aerolab.set-dns=true")
	}

	// Handle start script
	if string(c.StartScript) != "" {
		scriptData, err := os.ReadFile(string(c.StartScript))
		if err != nil {
			return nil, fmt.Errorf("failed to read start script: %w", err)
		}
		tags = append(tags, fmt.Sprintf("aerolab.start-script=%s", string(scriptData)))
	}

	// Create instances using base command by properly mapping all fields
	instancesCmd := InstancesCreateCmd{
		ClusterName:        c.ClientName.String(),
		Count:              c.ClientCount,
		Owner:              c.Owner,
		Type:               clientType,
		Tags:               tags,
		OS:                 c.OS,
		Version:            c.Version,
		Arch:               c.Arch,
		AWS:                c.AWS,
		GCP:                c.GCP,
		Docker:             c.Docker,
		ParallelSSHThreads: c.ParallelSSHThreads,
	}

	// Create instances using base command
	action := "create"
	if isGrow {
		action = "grow"
	}

	instances, err := instancesCmd.CreateInstances(system, inventory, args, action)
	if err != nil {
		return nil, fmt.Errorf("failed to create client instances: %w", err)
	}

	logger.Info("Created %d client instances", len(instances))
	return instances, nil
}

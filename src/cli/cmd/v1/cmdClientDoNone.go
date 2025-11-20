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
	InstancesCreateCmd
	ClientName    TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	ClientCount   int            `short:"c" long:"count" description:"Number of clients" default:"1"`
	NoSetHostname bool           `short:"H" long:"no-set-hostname" description:"By default, hostname of each machine will be set, use this to prevent hostname change"`
	NoSetDNS      bool           `long:"no-set-dns" description:"Set to prevent aerolab from updating resolved to use 1.1.1.1/8.8.8.8 DNS"`
	StartScript   flags.Filename `short:"X" long:"start-script" description:"Optionally specify a script to be installed which will run when the client machine starts"`
	TypeOverride  string         `long:"type-override" description:"Override the client type label"`
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

	err = c.createNoneClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateNoneCmd) createNoneClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) error {
	// Set cluster name from client name
	c.InstancesCreateCmd.ClusterName = c.ClientName.String()
	c.InstancesCreateCmd.Count = c.ClientCount

	// Check if client already exists
	existing := inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).WithClusterName(c.ClientName.String())
	if existing != nil && existing.Count() > 0 && !isGrow {
		return fmt.Errorf("client %s already exists, did you mean 'grow'?", c.ClientName.String())
	}
	if (existing == nil || existing.Count() == 0) && isGrow {
		return fmt.Errorf("client %s doesn't exist, did you mean 'create'?", c.ClientName.String())
	}

	// Set client type tag
	clientType := "none"
	if c.TypeOverride != "" {
		clientType = c.TypeOverride
		logger.Info("Overriding client type: %s", clientType)
	}

	// Add client-specific tags (Tags is a slice of strings in "key=value" format)
	c.InstancesCreateCmd.Tags = append(c.InstancesCreateCmd.Tags,
		"aerolab.old.type=client",
		fmt.Sprintf("aerolab.client.type=%s", clientType),
	)

	// Configure hostname and DNS settings
	if !c.NoSetHostname {
		c.InstancesCreateCmd.Tags = append(c.InstancesCreateCmd.Tags, "aerolab.set-hostname=true")
	}
	if !c.NoSetDNS {
		c.InstancesCreateCmd.Tags = append(c.InstancesCreateCmd.Tags, "aerolab.set-dns=true")
	}

	// Handle start script
	if string(c.StartScript) != "" {
		scriptData, err := os.ReadFile(string(c.StartScript))
		if err != nil {
			return fmt.Errorf("failed to read start script: %w", err)
		}
		c.InstancesCreateCmd.Tags = append(c.InstancesCreateCmd.Tags, fmt.Sprintf("aerolab.start-script=%s", string(scriptData)))
	}

	// Create instances using base command
	action := "create"
	if isGrow {
		action = "grow"
	}
	
	instances, err := c.InstancesCreateCmd.CreateInstances(system, inventory, args, action)
	if err != nil {
		return fmt.Errorf("failed to create client instances: %w", err)
	}

	logger.Info("Created %d client instances", len(instances))
	return nil
}


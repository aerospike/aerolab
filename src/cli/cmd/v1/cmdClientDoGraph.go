package cmd

import (
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type ClientCreateGraphCmd struct {
	ClientCreateNoneCmd
}

func (c *ClientCreateGraphCmd) Execute(args []string) error {
	isGrow := len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow"

	var cmd []string
	if isGrow {
		cmd = []string{"client", "grow", "graph"}
	} else {
		cmd = []string{"client", "create", "graph"}
	}

	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.createGraphClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateGraphCmd) createGraphClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "create", "graph"}, c)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	// Override type
	if c.TypeOverride == "" {
		c.TypeOverride = "graph"
	}

	// Create base client first
	baseCmd := &ClientCreateBaseCmd{ClientCreateNoneCmd: c.ClientCreateNoneCmd}
	clients, err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return err
	}

	// Get created instances
	_ = clients
	logger.Info("Graph client created successfully. Install Aerospike Graph separately.")
	return nil
}

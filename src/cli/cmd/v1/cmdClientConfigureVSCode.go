package cmd

import (
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type ClientConfigureVSCodeCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	Help       HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientConfigureVSCodeCmd) Execute(args []string) error {
	cmd := []string{"client", "configure", "vscode"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.configureVSCode(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientConfigureVSCodeCmd) configureVSCode(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	// Get client instances
	clients, err := getClientInstancesHelper(inventory, c.ClientName.String(), c.Machines.String())
	if err != nil {
		return err
	}

	if len(clients) == 0 {
		return fmt.Errorf("no client instances found")
	}

	logger.Info("VSCode configuration is handled during creation")
	logger.Info("Access VSCode at: http://<client-ip>:8080")
	
	return nil
}


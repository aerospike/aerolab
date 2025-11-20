package cmd

import (
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type ClientConfigureFirewallCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	Help       HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientConfigureFirewallCmd) Execute(args []string) error {
	cmd := []string{"client", "configure", "firewall"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.configureFirewall(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientConfigureFirewallCmd) configureFirewall(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	// Get client instances
	clients, err := getClientInstancesHelper(inventory, c.ClientName.String(), c.Machines.String())
	if err != nil {
		return err
	}

	if len(clients) == 0 {
		return fmt.Errorf("no client instances found")
	}

	logger.Info("Firewall configuration should be done using cloud provider security groups")
	logger.Info("For advanced firewall rules, use the 'net' commands")
	
	return nil
}


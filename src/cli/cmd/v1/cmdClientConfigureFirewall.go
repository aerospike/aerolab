package cmd

import (
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type ClientConfigureFirewallCmd struct {
	ClientName   TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines     TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	FirewallName string         `short:"f" long:"firewall" description:"Firewall name to assign to the client machines" required:"true"`
	Help         HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
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
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "configure", "firewall"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	if c.ClientName.String() == "" {
		return fmt.Errorf("client group name is required")
	}

	// Support comma-separated client names
	if strings.Contains(c.ClientName.String(), ",") {
		clientNames := strings.Split(c.ClientName.String(), ",")
		for _, clientName := range clientNames {
			c.ClientName = TypeClientName(clientName)
			err := c.configureFirewall(system, inventory, logger, args)
			if err != nil {
				return err
			}
		}
		return nil
	}

	// Get client instances
	clients := inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).WithClusterName(c.ClientName.String())
	if clients == nil || clients.Count() == 0 {
		return fmt.Errorf("client group '%s' not found", c.ClientName.String())
	}

	// Filter by specific machines if requested
	if c.Machines.String() != "" {
		machines, err := expandNodeNumbers(c.Machines.String())
		if err != nil {
			return err
		}
		clients = clients.WithNodeNo(machines...)
		if clients.Count() != len(machines) {
			return fmt.Errorf("some machines in %s not found", c.Machines.String())
		}
	}

	if clients.Count() == 0 {
		logger.Info("No running client instances to configure firewall for")
		return nil
	}

	// Get the firewall
	firewalls := inventory.Firewalls.WithName(c.FirewallName).Describe()
	if len(firewalls) == 0 {
		return fmt.Errorf("firewall '%s' not found", c.FirewallName)
	}

	logger.Info("Assigning firewall '%s' to %d client machines", c.FirewallName, clients.Count())

	// Assign firewall to all selected client instances
	err := clients.AssignFirewalls(firewalls)
	if err != nil {
		return fmt.Errorf("failed to assign firewall: %w", err)
	}

	logger.Info("Successfully assigned firewall to client machines")
	return nil
}

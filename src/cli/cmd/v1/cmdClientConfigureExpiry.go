package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type ClientChangeExpiryCmd struct {
	ClientName TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines   TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	ExpireIn   time.Duration  `short:"e" long:"expiry" description:"Expiry in duration from now (0 to remove expiry)" default:"30h"`
	Help       HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientChangeExpiryCmd) Execute(args []string) error {
	cmd := []string{"client", "configure", "expiry"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = c.ChangeExpiryClient(system, system.Backend.GetInventory(), args, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientChangeExpiryCmd) ChangeExpiryClient(system *System, inventory *backends.Inventory, args []string, logger *logger.Logger) (err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "configure", "expiry"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	if c.ClientName.String() == "" {
		return fmt.Errorf("client name is required")
	}

	// Handle comma-separated client names
	var clients backends.InstanceList
	if strings.Contains(c.ClientName.String(), ",") {
		clientNames := strings.Split(c.ClientName.String(), ",")
		// Pre-validation loop - check ALL clients exist before processing any
		for _, clientName := range clientNames {
			if inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).WithClusterName(clientName).WithNotState(backends.LifeCycleStateTerminated, backends.LifeCycleStateTerminating).Count() == 0 {
				return fmt.Errorf("client '%s' not found", clientName)
			}
		}
		// Processing loop - actually perform the operation
		for _, clientName := range clientNames {
			c.ClientName = TypeClientName(clientName)
			err := c.ChangeExpiryClient(system, inventory, args, logger)
			if err != nil {
				return err
			}
		}
		return nil
	} else {
		_, err = c.ClientName.GetInstanceList(inventory)
		if err != nil {
			return err
		}
	}

	// Get client instances using the helper
	clients, err = getClientInstancesHelper(inventory, c.ClientName.String(), c.Machines.String())
	if err != nil {
		return err
	}

	// Filter to running instances
	clients = clients.WithState(backends.LifeCycleStateRunning).Describe()
	if len(clients) == 0 {
		logger.Info("No running client machines found for %s", c.ClientName.String())
		return nil
	}

	if c.ExpireIn == 0 {
		logger.Info("Removing expiry from %d client machines", len(clients))
		err = clients.ChangeExpiry(time.Time{})
	} else {
		logger.Info("Adding expiry to %d client machines", len(clients))
		err = clients.ChangeExpiry(time.Now().Add(c.ExpireIn))
	}
	if err != nil {
		return err
	}
	return nil
}

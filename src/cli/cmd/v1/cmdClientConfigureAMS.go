package cmd

import (
	"fmt"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type ClientConfigureAMSCmd struct {
	ClientName      TypeClientName `short:"n" long:"group-name" description:"Client group name" default:"client"`
	Machines        TypeMachines   `short:"l" long:"machines" description:"Machine list, comma separated. Empty=ALL" default:""`
	ConnectClusters TypeClusterName `short:"s" long:"clusters" description:"Comma-separated list of clusters to configure as source for this AMS"`
	Help            HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientConfigureAMSCmd) Execute(args []string) error {
	cmd := []string{"client", "configure", "ams"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.configureAMS(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientConfigureAMSCmd) configureAMS(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	// Get client instances
	clients, err := getClientInstancesHelper(inventory, c.ClientName.String(), c.Machines.String())
	if err != nil {
		return err
	}

	if len(clients) == 0 {
		return fmt.Errorf("no client instances found")
	}

	logger.Info("AMS configuration is complex and requires manual setup")
	logger.Info("Please configure Prometheus and Grafana manually on the client machines")
	logger.Info("See: https://github.com/aerospike/aerolab/tree/master/docs/usage/monitoring")
	
	return nil
}


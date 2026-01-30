package cmd

import (
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

// AgiOpenCmd opens an AGI instance in the default browser.
// It generates an authentication token URL and opens it in the browser
// for convenient access to the AGI Grafana interface.
type AgiOpenCmd struct {
	ClusterName TypeAgiClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	Help        HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi open.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiOpenCmd) Execute(args []string) error {
	cmd := []string{"agi", "open"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.OpenAGI(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// OpenAGI generates an authentication token URL and opens it in the browser.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiOpenCmd) OpenAGI(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "open"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Use AgiAddTokenCmd with --open flag to generate token and open browser
	addToken := &AgiAddTokenCmd{
		ClusterName: c.ClusterName,
		TokenSize:   128,
		Open:        true,
	}

	return addToken.AddToken(system, inventory, logger, args)
}

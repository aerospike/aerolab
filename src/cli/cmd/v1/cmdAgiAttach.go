package cmd

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/go-flags"
)

// AgiAttachCmd attaches to an AGI instance shell.
// This command provides interactive shell access to the AGI instance
// for administration, debugging, and manual operations.
//
// The command supports:
//   - Interactive terminal sessions
//   - Command execution with arguments
//   - Detached process mode
//   - Parallel execution across multiple nodes (if applicable)
type AgiAttachCmd struct {
	ClusterName     TypeAgiClusterName     `short:"n" long:"name" description:"AGI name" default:"agi"`
	Node            TypeNodesPlusAllOption `short:"l" long:"node" description:"Node to attach to (typically 1 for AGI)" default:"1"`
	Detach          bool                   `short:"d" long:"detach" description:"Detach process stdin - will not kill process on CTRL+C"`
	Parallel        bool                   `short:"p" long:"parallel" description:"Enable parallel execution across all machines"`
	ParallelThreads int                    `short:"t" long:"threads" description:"Number of parallel threads for execution" default:"10"`
	Env             []string               `short:"e" long:"env" description:"Environment variables to set, as k=v"`
	ConnectTimeout  time.Duration          `short:"C" long:"connect-timeout" description:"Connect timeout" default:"10s"`
	SessionTimeout  time.Duration          `short:"S" long:"session-timeout" description:"Session timeout (0 for no timeout)"`
	Out             flags.Filename         `short:"O" long:"stdout" description:"Path to redirect stdout to"`
	Err             flags.Filename         `short:"E" long:"stderr" description:"Path to redirect stderr to (only with --no-terminal)"`
	Tail            []string               `description:"Command parameters to execute, ex: [\"ls\",\"/opt\"]" webrequired:"true"`
	Help            AgiAttachCmdHelp       `command:"help" subcommands-optional:"true" description:"Print help"`
}

// AgiAttachCmdHelp provides help for the agi attach command.
type AgiAttachCmdHelp struct{}

// Execute prints help text for the agi attach command.
func (c *AgiAttachCmdHelp) Execute(args []string) error {
	PrintHelp(false, `To specify a command to run, use the following syntax:

  aerolab agi attach <parameters> -- <command>

For example:

  aerolab agi attach --name=myagi -- ls -l /opt
  aerolab agi attach --name=myagi -- cat /var/log/agi-ingest.log
  aerolab agi attach --name=myagi -- systemctl status aerospike
`)
	return nil
}

// Execute implements the command execution for agi attach.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiAttachCmd) Execute(args []string) error {
	cmd := []string{"agi", "attach"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.AttachAGI(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// AttachAGI attaches to the AGI instance shell.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - args: Command arguments to execute (if any)
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiAttachCmd) AttachAGI(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "attach"}, c)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	parallelThreads := c.ParallelThreads
	if !c.Parallel {
		parallelThreads = 1
	}

	// Use command arguments from Tail if args is empty
	if args == nil && c.Tail != nil {
		args = c.Tail
	}

	// Handle "all" node selection
	node := c.Node.String()
	if node == "all" {
		node = ""
	}

	// Create an InstancesAttachCmd to perform the actual attachment
	attach := &InstancesAttachCmd{
		ParallelThreads: parallelThreads,
		ConnectTimeout:  c.ConnectTimeout,
		SessionTimeout:  c.SessionTimeout,
		Env:             c.Env,
		Out:             c.Out,
		Err:             c.Err,
		Detach:          c.Detach,
		NoTerminal:      false,
		Filters: InstancesListFilter{
			ClusterName: c.ClusterName.String(),
			NodeNo:      node,
			Type:        "agi",
		},
	}

	out, err := attach.AttachInstances(system, inventory, args, io.NopCloser(os.Stdin), os.Stdout, os.Stderr)
	if err != nil {
		return err
	}

	// Check for errors in output
	var ret error
	for _, o := range out {
		if o.Output.Err != nil {
			system.Logger.Error("Error (%s:%d): %s", o.Instance.ClusterName, o.Instance.NodeNo, o.Output.Err)
			system.Logger.Error("Output: %s", string(o.Output.Stdout))
			ret = ErrSomeNodesReturnedAnError
		}
	}

	return ret
}

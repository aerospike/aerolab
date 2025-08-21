package cmd

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/go-flags"
)

type AttachAGICmd struct {
	ClusterName     TypeClusterName        `short:"n" long:"name" description:"Cluster name" default:"agi"`
	Node            TypeNodesPlusAllOption `short:"l" long:"node" description:"Node to attach to (or comma-separated list, when using '-- ...'). Example: 'attach shell --node=all -- /some/command' will execute command on all nodes" default:"1"`
	Detach          bool                   `short:"d" long:"detach" description:"detach the process stdin - will not kill process on CTRL+C; it is up to the process to detach stdout/err"`
	Parallel        bool                   `short:"p" long:"parallel" description:"enable parallel execution across all machines"`
	ParallelThreads int                    `short:"t" long:"threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Env             []string               `short:"e" long:"env" description:"Environment variables to set, as k=v"`
	ConnectTimeout  time.Duration          `short:"C" long:"connect-timeout" description:"Connect timeout" default:"10s"`
	SessionTimeout  time.Duration          `short:"S" long:"session-timeout" description:"Session timeout"`
	Out             flags.Filename         `short:"O" long:"stdout" description:"Path output file to redirect stdout to"`
	Err             flags.Filename         `short:"E" long:"stderr" description:"Path output file to redirect stderr to (only works if --no-terminal is specified, otherwise all output goes to stdout)"`
	Tail            []string               `description:"List containing command parameters to execute, ex: [\"ls\",\"/opt\"]" webrequired:"true"`
	Help            AttachAGICmdHelp       `command:"help" subcommands-optional:"true" description:"Print help"`
}

type AttachAGICmdHelp struct{}

func (c *AttachAGICmdHelp) Execute(args []string) error {
	PrintHelp(false, "To specify a command to run, use the following syntax:\n\n  aerolab attach agi <parameters> -- <command>\n\nFor example:\n\n  aerolab attach agi --name=bob -- ls -l /tmp\n")
	return nil
}

func (c *AttachAGICmd) Execute(args []string) error {
	cmd := []string{"attach", "agi"}
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

func (c *AttachAGICmd) AttachAGI(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"attach", "agi"}, c)
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
	if args == nil && c.Tail != nil {
		args = c.Tail
	}
	node := c.Node.String()
	if node == "all" {
		node = ""
	}
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

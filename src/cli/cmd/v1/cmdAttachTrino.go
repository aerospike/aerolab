package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
)

type AttachTrinoCmd struct {
	ClusterName    TypeClusterName        `short:"n" long:"name" description:"Client name" default:"client"`
	Node           TypeNodesPlusAllOption `short:"l" long:"node" description:"Node to attach to (or comma-separated list, when using '-- ...'). Example: 'attach shell --node=all -- /some/command' will execute command on all nodes" default:"1"`
	Namespace      string                 `short:"N" long:"namespace" description:"Namespace to use" default:"test"`
	Env            []string               `short:"e" long:"env" description:"Environment variables to set, as k=v"`
	ConnectTimeout time.Duration          `short:"C" long:"connect-timeout" description:"Connect timeout" default:"10s"`
	SessionTimeout time.Duration          `short:"S" long:"session-timeout" description:"Session timeout"`
	Help           AttachTrinoCmdHelp     `command:"help" subcommands-optional:"true" description:"Print help"`
}

type AttachTrinoCmdHelp struct{}

func (c *AttachTrinoCmdHelp) Execute(args []string) error {
	PrintHelp(false, "To specify a command to run, use the following syntax:\n\n  aerolab attach trino <parameters> -- <trino-parameters>\n\nFor example:\n\n  aerolab attach trino --name=bob -- -v service\n")
	return nil
}

func (c *AttachTrinoCmd) Execute(args []string) error {
	cmd := []string{"attach", "trino"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.AttachTrino(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *AttachTrinoCmd) AttachTrino(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"attach", "trino"}, c)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	parallelThreads := 1
	node := c.Node.String()
	if node == "all" {
		node = ""
	}
	attach := &InstancesAttachCmd{
		ParallelThreads: parallelThreads,
		ConnectTimeout:  c.ConnectTimeout,
		SessionTimeout:  c.SessionTimeout,
		Env:             c.Env,
		NoTerminal:      false,
		Filters: InstancesListFilter{
			ClusterName: c.ClusterName.String(),
			NodeNo:      node,
			Type:        "aerospike",
		},
	}
	args = []string{"su", "-", "trino", "-c", fmt.Sprintf("bash ./trino --server 127.0.0.1:8080 --catalog aerospike --schema %s", c.Namespace)}
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

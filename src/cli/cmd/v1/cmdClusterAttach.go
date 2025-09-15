package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClusterAttachCmd struct {
	ClusterName     TypeClusterName      `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes           TypeNodes            `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	ParallelThreads int                  `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	ConnectTimeout  time.Duration        `short:"C" long:"connect-timeout" description:"Connect timeout" default:"10s"`
	SessionTimeout  time.Duration        `short:"S" long:"session-timeout" description:"Session timeout"`
	Env             []string             `short:"e" long:"env" description:"Environment variables to set, as k=v"`
	NoTerminal      bool                 `long:"no-terminal" description:"Do not use a terminal"`
	Out             flags.Filename       `long:"stdout" description:"Path output file to redirect stdout to"`
	Err             flags.Filename       `long:"stderr" description:"Path output file to redirect stderr to (only works if --no-terminal is specified, otherwise all output goes to stdout)"`
	Detach          bool                 `long:"detach" description:"detach the process stdin - will not kill process on CTRL+C; it is up to the process to detach stdout/err"`
	Help            ClusterAttachHelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ClusterAttachHelpCmd struct{}

func (c *ClusterAttachHelpCmd) Execute(args []string) error {
	return PrintHelp(true, "To specify a command to run, use the following syntax:\n\n  aerolab cluster attach <parameters> -- <command>\n\nFor example:\n\n  aerolab cluster attach --cluster-name=bob -- ls -l /tmp\n")
}

func (c *ClusterAttachCmd) Execute(args []string) error {
	cmd := []string{"cluster", "attach"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	_, err = c.AttachCluster(system, system.Backend.GetInventory(), args, io.NopCloser(os.Stdin), os.Stdout, os.Stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterAttachCmd) AttachCluster(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) (output []*backends.ExecOutput, err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "attach"}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	if c.ClusterName.String() == "" {
		return nil, fmt.Errorf("cluster name is required")
	}
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		var output []*backends.ExecOutput
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			inst, err := c.AttachCluster(system, inventory, args, stdin, stdout, stderr, logger)
			if err != nil {
				return nil, err
			}
			output = append(output, inst...)
		}
		return output, nil
	}
	cluster := inventory.Instances.WithClusterName(c.ClusterName.String())
	if cluster == nil {
		return nil, fmt.Errorf("cluster %s not found", c.ClusterName.String())
	}
	if c.Nodes.String() != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return nil, err
		}
		cluster = cluster.WithNodeNo(nodes...)
		if cluster.Count() != len(nodes) {
			return nil, fmt.Errorf("some nodes in %s not found", c.Nodes.String())
		}
	}
	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		logger.Info("No nodes to attach")
		return nil, nil
	}
	logger.Info("Attaching %d nodes", cluster.Count())
	i := &InstancesAttachCmd{
		ParallelThreads: c.ParallelThreads,
		ConnectTimeout:  c.ConnectTimeout,
		SessionTimeout:  c.SessionTimeout,
		Env:             c.Env,
		NoTerminal:      c.NoTerminal,
		Out:             c.Out,
		Err:             c.Err,
		Detach:          c.Detach,
		Filters: InstancesListFilter{
			ClusterName: c.ClusterName.String(),
			NodeNo:      c.Nodes.String(),
		},
	}
	return i.AttachInstances(system, inventory, args, stdin, stdout, stderr)
}

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/go-flags"
)

type InstancesAttachCmd struct {
	ParallelThreads int                 `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	ConnectTimeout  time.Duration       `short:"C" long:"connect-timeout" description:"Connect timeout" default:"10s"`
	SessionTimeout  time.Duration       `short:"S" long:"session-timeout" description:"Session timeout"`
	Env             []string            `short:"e" long:"env" description:"Environment variables to set, as k=v"`
	NoTerminal      bool                `long:"no-terminal" description:"Do not use a terminal"`
	Out             flags.Filename      `long:"stdout" description:"Path output file to redirect stdout to"`
	Err             flags.Filename      `long:"stderr" description:"Path output file to redirect stderr to (only works if --no-terminal is specified, otherwise all output goes to stdout)"`
	Detach          bool                `long:"detach" description:"detach the process stdin - will not kill process on CTRL+C; it is up to the process to detach stdout/err"`
	Filters         InstancesListFilter `group:"Filters" namespace:"filter"`
	Help            AttachHelpCmd       `command:"help" subcommands-optional:"true" description:"Print help"`
}

type AttachHelpCmd struct{}

func (c *AttachHelpCmd) Execute(args []string) error {
	return PrintHelp(true, "To specify a command to run, use the following syntax:\n\n  aerolab instances attach <parameters> -- <command>\n\nFor example:\n\n  aerolab instances attach --cluster-name=bob -- ls -l /tmp\n")
}

func (c *InstancesAttachCmd) Execute(args []string) error {
	cmd := []string{"instances", "attach"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	_, err = c.AttachInstances(system, system.Backend.GetInventory(), args, io.NopCloser(os.Stdin), os.Stdout, os.Stderr)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// if c.Out or c.Err is set, it will redirect stdout/stderr to the file, ignoring the stdout/stderr parameters
// if c.Detach is set, stdin will be nil, and the prameter will be ignored
func (c *InstancesAttachCmd) AttachInstances(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer) (output []*backends.ExecOutput, err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "attach"}, c)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	instances := inventory.Instances.WithState(backends.LifeCycleStateRunning).Describe()

	instances, err = c.Filters.filter(instances, true)
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances found")
	}
	if len(instances) > 1 && len(args) == 0 {
		return nil, fmt.Errorf("multiple instances found, please use a filter to select a single instance or specify a command to run -- see help")
	}
	env := []*sshexec.Env{}
	for _, e := range c.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid env: %s, must be in the format k=v", e)
		}
		env = append(env, &sshexec.Env{Key: parts[0], Value: parts[1]})
	}

	if c.Detach {
		stdin = nil
	}
	if c.Out != "" {
		out, err := os.OpenFile(string(c.Out), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		stdout = out
		defer out.Close()
	}
	if c.Err != "" {
		eout, err := os.OpenFile(string(c.Err), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		stderr = eout
		defer eout.Close()
	}
	output = instances.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        args,
			Stdin:          stdin,
			Stdout:         stdout,
			Stderr:         stderr,
			SessionTimeout: c.SessionTimeout,
			Env:            env,
			Terminal:       !c.NoTerminal,
		},
		Username:        "root",
		ConnectTimeout:  c.ConnectTimeout,
		ParallelThreads: c.ParallelThreads,
	})
	for _, o := range output {
		if o.Output.Err != nil {
			return output, ErrSomeNodesReturnedAnError
		}
	}
	return output, nil
}

var ErrSomeNodesReturnedAnError = errors.New("some nodes returned an error")

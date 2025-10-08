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
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/logger"
)

type ClusterAddPublicIPCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	ParallelThreads int             `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help            HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterAddPublicIPCmd) Execute(args []string) error {
	cmd := []string{"cluster", "add", "public-ip"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	var stdout, stderr *os.File
	var stdin io.ReadCloser
	if system.logLevel >= 5 {
		stdout = os.Stdout
		stderr = os.Stderr
		stdin = io.NopCloser(os.Stdin)
	}
	_, err = c.AddPublicIPCluster(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done, restart aerospike to apply the changes")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterAddPublicIPCmd) AddPublicIPCluster(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) (output []*backends.ExecOutput, err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "add", "public-ip"}, c, args...)
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
			inst, err := c.AddPublicIPCluster(system, inventory, args, stdin, stdout, stderr, logger)
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
		logger.Info("No nodes to add public-ip")
		return nil, nil
	}
	logger.Info("Adding public-ip to %d nodes", cluster.Count())

	var installScript []byte
	if system.Opts.Config.Backend.Type == "docker" {
		logger.Info("Public IP access address is not supported for docker backend")
		return nil, nil
	}
	if system.Opts.Config.Backend.Type == "aws" {
		scr, err := scripts.ReadFile("scripts/cluster-create/aws-public-ip.sh.tpl")
		if err != nil {
			return nil, err
		}
		installScript = scr
	}
	if system.Opts.Config.Backend.Type == "gcp" {
		scr, err := scripts.ReadFile("scripts/cluster-create/gcp-public-ip.sh.tpl")
		if err != nil {
			return nil, err
		}
		installScript = scr
	}

	var hasErr error
	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		conf, err := inst.GetSftpConfig("root")
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		client, err := sshexec.NewSftp(conf)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		defer client.Close()
		now := time.Now().Format("20060102150405")
		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/tmp/install.sh." + now,
			Source:      strings.NewReader(string(installScript)),
			Permissions: 0755,
		})
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		output := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "/tmp/install.sh." + now},
				Stdin:          stdin,
				Stdout:         stdout,
				Stderr:         stderr,
				SessionTimeout: 5 * time.Minute,
				Env:            []*sshexec.Env{},
				Terminal:       false,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: c.ParallelThreads,
		})
		if output.Output.Err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s (%s) (%s)", inst.ClusterName, inst.NodeNo, output.Output.Err, string(output.Output.Stdout), string(output.Output.Stderr)))
		}
	})
	if hasErr != nil {
		return nil, hasErr
	}
	return nil, nil
}

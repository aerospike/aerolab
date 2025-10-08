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
	"github.com/aerospike/aerolab/pkg/utils/installers/aerolab"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClusterAddAerolabCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	CustomConf      flags.Filename  `short:"o" long:"custom-conf" description:"To deploy a custom ape.toml configuration file, specify it's path here"`
	AerolabVersion  string          `short:"v" long:"aerolab-version" description:"Aerolab version to install" default:"latest"`
	Prerelease      bool            `short:"r" long:"prerelease" description:"Install prerelease version"`
	ParallelThreads int             `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help            HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterAddAerolabCmd) Execute(args []string) error {
	cmd := []string{"cluster", "add", "aerolab"}
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
	_, err = c.AddAerolabCluster(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterAddAerolabCmd) AddAerolabCluster(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) (output []*backends.ExecOutput, err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "add", "aerolab"}, c, args...)
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
			inst, err := c.AddAerolabCluster(system, inventory, args, stdin, stdout, stderr, logger)
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
		logger.Info("No nodes to add aerolab")
		return nil, nil
	}
	logger.Info("Adding aerolab to %d nodes", cluster.Count())

	var alVer *string
	if c.AerolabVersion != "latest" {
		alVer = &c.AerolabVersion
	}
	var pre *bool
	if !c.Prerelease {
		pre = &c.Prerelease
	}
	installScript, err := aerolab.GetLinuxInstallScript(alVer, pre)
	if err != nil {
		return nil, err
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

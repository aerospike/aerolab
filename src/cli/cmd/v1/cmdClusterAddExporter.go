package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike"
	"github.com/aerospike/aerolab/pkg/utils/installers/nodeexporter"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClusterAddExporterCmd struct {
	ClusterName         TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes               TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	CustomConf          flags.Filename  `short:"o" long:"custom-conf" description:"To deploy a custom ape.toml configuration file, specify it's path here"`
	ExporterVersion     string          `short:"v" long:"exporter-version" description:"Exporter version to install" default:"latest"`
	NodeExporterVersion string          `long:"node-exporter-version" description:"Node exporter version to install (e.g., 1.5.0)" default:"latest"`
	ParallelThreads     int             `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	MaxRetries          int             `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep          time.Duration   `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`
	Help                HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterAddExporterCmd) Execute(args []string) error {
	cmd := []string{"cluster", "add", "exporter"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	var stdout, stderr *io.Writer
	var stdin *io.ReadCloser
	if system.logLevel >= 5 {
		var stdoutp io.Writer = os.Stdout
		stdout = &stdoutp
		var stderrp io.Writer = os.Stderr
		stderr = &stderrp
		var stdinp = io.NopCloser(os.Stdin)
		stdin = &stdinp
	}
	_, err = c.AddExporterCluster(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterAddExporterCmd) AddExporterCluster(system *System, inventory *backends.Inventory, args []string, stdin *io.ReadCloser, stdout *io.Writer, stderr *io.Writer, logger *logger.Logger) (output []*backends.ExecOutput, err error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "add", "exporter"}, c, args...)
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
	if c.CustomConf != "" {
		if _, err := os.Stat(string(c.CustomConf)); err != nil {
			return nil, fmt.Errorf("custom conf file %s does not exist", c.CustomConf)
		}
	}
	var cluster backends.Instances
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		var output []*backends.ExecOutput
		for _, cluster := range clusters {
			if inventory.Instances.WithClusterName(cluster).WithState(backends.LifeCycleStateRunning).Count() == 0 {
				return nil, fmt.Errorf("cluster %s not found", cluster)
			}
		}
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			inst, err := c.AddExporterCluster(system, inventory, args, stdin, stdout, stderr, logger)
			if err != nil {
				return nil, err
			}
			output = append(output, inst...)
		}
		return output, nil
	} else {
		var err error
		cluster, err = c.ClusterName.GetInstanceList(inventory, backends.LifeCycleStateRunning)
		if err != nil {
			return nil, err
		}
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
		logger.Info("No nodes to add exporter")
		return nil, nil
	}
	logger.Info("Adding exporter to %d nodes", cluster.Count())
	prod, err := aerospike.GetProducts(time.Second * 15)
	if err != nil {
		return nil, err
	}
	product := prod.WithName("aerospike-prometheus-exporter")
	if len(product) == 0 {
		return nil, fmt.Errorf("product not found")
	}
	versions, err := aerospike.GetVersions(time.Second*15, product[0])
	if err != nil {
		return nil, err
	}
	if c.ExporterVersion != "latest" {
		versions = versions.WithName(c.ExporterVersion)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("version %s not found", c.ExporterVersion)
	}
	version := versions.Latest()
	if version == nil {
		return nil, fmt.Errorf("version not found")
	}
	files, err := aerospike.GetFiles(time.Second*15, *version)
	if err != nil {
		return nil, err
	}
	var hasErr error
	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		installScript, err := files.GetInstallScript(aerospike.ArchitectureTypeX86_64, aerospike.OSName(inst.OperatingSystem.Name), inst.OperatingSystem.Version, system.logLevel >= 5, true, true, true)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		installScript = append(installScript, []byte(`
			systemctl enable aerospike-prometheus-exporter
			systemctl start aerospike-prometheus-exporter
		`)...)
		conf, err := inst.GetSftpConfig("root")
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		conf.MaxRetries = c.MaxRetries
		conf.RetrySleep = c.RetrySleep
		client, err := sshexec.NewSftp(conf)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		defer client.Close()
		now := time.Now().Format("20060102150405")
		scriptPath := "/opt/aerolab/scripts/add-exporter." + now + ".sh"
		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath:    scriptPath,
			Source:      strings.NewReader(string(installScript)),
			Permissions: 0755,
		})
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		detail := sshexec.ExecDetail{
			Command:        []string{"bash", scriptPath},
			SessionTimeout: 5 * time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		}
		if stdin != nil {
			detail.Stdin = *stdin
		}
		if stdout != nil {
			detail.Stdout = *stdout
		}
		if stderr != nil {
			detail.Stderr = *stderr
		}
		output := inst.Exec(&backends.ExecInput{
			ExecDetail:      detail,
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: c.ParallelThreads,
			MaxRetries:      c.MaxRetries,
			RetrySleep:      c.RetrySleep,
		})
		if output.Output.Err != nil {
			// Save script failure to local machine for debugging
			failure := scriptlog.NewScriptFailureWithPath(
				inst.ClusterName,
				inst.NodeNo,
				scriptPath,
				installScript,
				output.Output.Stdout,
				output.Output.Stderr,
				output.Output.Err,
			)
			logPath, saveErr := scriptlog.SaveFailure(failure)
			if saveErr != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s (%s) (%s) (also failed to save logs: %v)", inst.ClusterName, inst.NodeNo, output.Output.Err, string(output.Output.Stdout), string(output.Output.Stderr), saveErr))
			} else {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s", scriptlog.FormatError(logPath, inst.ClusterName, inst.NodeNo, output.Output.Err)))
			}
			return
		}
		// upload the custom conf if provided
		if c.CustomConf != "" {
			conf, err := inst.GetSftpConfig("root")
			if err != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
				return
			}
			conf.MaxRetries = c.MaxRetries
			conf.RetrySleep = c.RetrySleep
			client, err := sshexec.NewSftp(conf)
			if err != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
				return
			}
			defer client.Close()
			err = client.Upload(string(c.CustomConf), "/etc/aerospike-prometheus-exporter/ape.toml")
			if err != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
				return
			}
		}
	})
	if hasErr != nil {
		return nil, hasErr
	}

	// Install node_exporter
	logger.Info("Installing node_exporter on %d nodes", cluster.Count())
	var nodeExporterVersion *string
	if c.NodeExporterVersion != "latest" {
		nodeExporterVersion = &c.NodeExporterVersion
	}

	nodeExporterScript, err := nodeexporter.GetLinuxInstallScript(nodeExporterVersion, nil, true, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get node_exporter installer: %w", err)
	}

	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		logger.Debug("Installing node_exporter on %s:%d", inst.ClusterName, inst.NodeNo)

		conf, err := inst.GetSftpConfig("root")
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		conf.MaxRetries = c.MaxRetries
		conf.RetrySleep = c.RetrySleep
		client, err := sshexec.NewSftp(conf)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}
		defer client.Close()

		now := time.Now().Format("20060102150405")
		nodeExporterScriptPath := "/opt/aerolab/scripts/install-node-exporter." + now + ".sh"
		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath:    nodeExporterScriptPath,
			Source:      bytes.NewReader(nodeExporterScript),
			Permissions: 0755,
		})
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s", inst.ClusterName, inst.NodeNo, err))
			return
		}

		detail := sshexec.ExecDetail{
			Command:        []string{"bash", nodeExporterScriptPath},
			SessionTimeout: 5 * time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		}
		if stdin != nil {
			detail.Stdin = *stdin
		}
		if stdout != nil {
			detail.Stdout = *stdout
		}
		if stderr != nil {
			detail.Stderr = *stderr
		}

		output := inst.Exec(&backends.ExecInput{
			ExecDetail:      detail,
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: c.ParallelThreads,
			MaxRetries:      c.MaxRetries,
			RetrySleep:      c.RetrySleep,
		})
		if output.Output.Err != nil {
			// Save script failure to local machine for debugging
			failure := scriptlog.NewScriptFailureWithPath(
				inst.ClusterName,
				inst.NodeNo,
				nodeExporterScriptPath,
				nodeExporterScript,
				output.Output.Stdout,
				output.Output.Stderr,
				output.Output.Err,
			)
			logPath, saveErr := scriptlog.SaveFailure(failure)
			if saveErr != nil {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %s (%s) (%s) (also failed to save logs: %v)", inst.ClusterName, inst.NodeNo, output.Output.Err, string(output.Output.Stdout), string(output.Output.Stderr), saveErr))
			} else {
				hasErr = errors.Join(hasErr, fmt.Errorf("%s", scriptlog.FormatError(logPath, inst.ClusterName, inst.NodeNo, output.Output.Err)))
			}
			return
		}
		logger.Debug("Successfully installed node_exporter on %s:%d", inst.ClusterName, inst.NodeNo)
	})

	if hasErr != nil {
		return nil, hasErr
	}

	logger.Info("NOTE: Remember to install the AMS stack client to monitor the cluster, using `aerolab client create ams` command")
	return nil, nil
}

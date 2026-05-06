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
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClusterAddAgiClientCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	SendLogsTo      string          `long:"send-logs-to" description:"AGI instance name to dispatch logs to" required:"true"`
	AerolabBinary   flags.Filename  `long:"aerolab-binary" description:"Path to local aerolab binary to install on cluster nodes"`
	InsecureTLS     bool            `long:"insecure-tls" description:"Skip AGI TLS certificate verification in the dispatcher"`
	ParallelThreads int             `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	MaxRetries      int             `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep      time.Duration   `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`
	Help            HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterAddAgiClientCmd) Execute(args []string) error {
	cmd := []string{"cluster", "add", "agi-client"}
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
	_, err = c.AddAgiClientCluster(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClusterAddAgiClientCmd) AddAgiClientCluster(system *System, inventory *backends.Inventory, args []string, stdin *io.ReadCloser, stdout *io.Writer, stderr *io.Writer, logger *logger.Logger) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "add", "agi-client"}, c, args...)
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
	if c.AerolabBinary != "" {
		if _, err := os.Stat(string(c.AerolabBinary)); err != nil {
			return nil, fmt.Errorf("aerolab binary %s does not exist: %w", c.AerolabBinary, err)
		}
	}
	agi := inventory.Instances.WithClusterName(c.SendLogsTo).WithTags(map[string]string{"aerolab.type": "agi"}).WithState(backends.LifeCycleStateRunning)
	if agi.Count() == 0 {
		return nil, fmt.Errorf("running AGI instance %s not found", c.SendLogsTo)
	}
	agiInst := agi.Describe()[0]
	token, err := c.readDispatcherToken(agiInst)
	if err != nil {
		return nil, err
	}
	target := agiTargetURL(agiInst)

	var cluster backends.Instances
	if strings.Contains(c.ClusterName.String(), ",") {
		var out backends.InstanceList
		for _, name := range strings.Split(c.ClusterName.String(), ",") {
			c.ClusterName = TypeClusterName(name)
			inst, err := c.AddAgiClientCluster(system, inventory, args, stdin, stdout, stderr, logger)
			if err != nil {
				return nil, err
			}
			out = append(out, inst...)
		}
		return out, nil
	}
	cluster, err = c.ClusterName.GetInstanceList(inventory, backends.LifeCycleStateRunning)
	if err != nil {
		return nil, err
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
		logger.Info("No running cluster nodes found")
		return nil, nil
	}
	if c.AerolabBinary == "" {
		_, err := (&ClusterAddAerolabCmd{
			ClusterName:     c.ClusterName,
			Nodes:           c.Nodes,
			ParallelThreads: c.ParallelThreads,
			MaxRetries:      c.MaxRetries,
			RetrySleep:      c.RetrySleep,
		}).AddAerolabCluster(system, inventory, args, stdin, stdout, stderr, logger)
		if err != nil {
			return nil, fmt.Errorf("install aerolab on cluster nodes: %w", err)
		}
	}
	logger.Info("Installing AGI live dispatcher on %d nodes", cluster.Count())
	var hasErr error
	binaryData := []byte(nil)
	if c.AerolabBinary != "" {
		binaryData, err = os.ReadFile(string(c.AerolabBinary))
		if err != nil {
			return nil, fmt.Errorf("read aerolab binary: %w", err)
		}
	}
	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		if err := c.configureNode(inst, token, target, binaryData); err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", inst.ClusterName, inst.NodeNo, err))
		}
	})
	if hasErr != nil {
		return nil, hasErr
	}
	logger.Info("AGI live dispatcher installed; target=%s", target)
	return cluster.Describe(), nil
}

func (c *ClusterAddAgiClientCmd) readDispatcherToken(inst *backends.Instance) (string, error) {
	conf, err := inst.GetSftpConfig("root")
	if err != nil {
		return "", fmt.Errorf("get AGI SFTP config: %w", err)
	}
	conf.MaxRetries = c.MaxRetries
	conf.RetrySleep = c.RetrySleep
	cli, err := sshexec.NewSftp(conf)
	if err != nil {
		return "", fmt.Errorf("connect to AGI SFTP: %w", err)
	}
	defer cli.Close()
	var buf bytes.Buffer
	if err := cli.ReadFile(&sshexec.FileReader{SourcePath: "/opt/agi/tokens/dispatcher", Destination: &buf}); err != nil {
		return "", fmt.Errorf("read /opt/agi/tokens/dispatcher from AGI %s: %w", inst.ClusterName, err)
	}
	token := strings.TrimSpace(buf.String())
	if len(token) < 64 {
		return "", fmt.Errorf("dispatcher token on AGI %s is too short", inst.ClusterName)
	}
	return token, nil
}

func (c *ClusterAddAgiClientCmd) configureNode(inst *backends.Instance, token, target string, binaryData []byte) error {
	conf, err := inst.GetSftpConfig("root")
	if err != nil {
		return err
	}
	conf.MaxRetries = c.MaxRetries
	conf.RetrySleep = c.RetrySleep
	cli, err := sshexec.NewSftp(conf)
	if err != nil {
		return err
	}
	defer cli.Close()
	_ = cli.RawClient().MkdirAll("/etc/aerolab")
	_ = cli.RawClient().MkdirAll("/var/lib/aerolab")
	if len(binaryData) > 0 {
		if err := cli.WriteFile(true, &sshexec.FileWriter{DestPath: "/usr/local/bin/aerolab", Source: bytes.NewReader(binaryData), Permissions: 0755}); err != nil {
			return fmt.Errorf("upload aerolab binary: %w", err)
		}
	}
	if err := cli.WriteFile(true, &sshexec.FileWriter{DestPath: "/etc/aerolab/agi-dispatch.token", Source: strings.NewReader(token), Permissions: 0600}); err != nil {
		return fmt.Errorf("write dispatcher token: %w", err)
	}
	unit := renderAgiDispatchUnit(target, c.InsecureTLS)
	if err := cli.WriteFile(true, &sshexec.FileWriter{DestPath: "/etc/systemd/system/aerolab-agi-dispatch.service", Source: strings.NewReader(unit), Permissions: 0644}); err != nil {
		return fmt.Errorf("write systemd unit: %w", err)
	}
	out := inst.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", "systemctl daemon-reload && systemctl enable --now aerolab-agi-dispatch.service && systemctl restart aerolab-agi-dispatch.service"},
			SessionTimeout: 2 * time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
		MaxRetries:      c.MaxRetries,
		RetrySleep:      c.RetrySleep,
	})
	if out.Output.Err != nil {
		failure := scriptlog.NewScriptFailureWithPath(inst.ClusterName, inst.NodeNo, "systemctl aerolab-agi-dispatch", []byte(unit), out.Output.Stdout, out.Output.Stderr, out.Output.Err)
		if logPath, saveErr := scriptlog.SaveFailure(failure); saveErr == nil {
			return fmt.Errorf("%s", scriptlog.FormatError(logPath, inst.ClusterName, inst.NodeNo, out.Output.Err))
		}
		return fmt.Errorf("%s (%s) (%s)", out.Output.Err, out.Output.Stdout, out.Output.Stderr)
	}
	return nil
}

func renderAgiDispatchUnit(target string, insecure bool) string {
	insecureFlag := ""
	if insecure {
		insecureFlag = " --insecure-tls"
	}
	return fmt.Sprintf(`[Unit]
Description=AeroLab AGI live log dispatcher
After=aerospike.service network-online.target
Wants=aerospike.service network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/aerolab agi exec dispatch --target %s --token-file /etc/aerolab/agi-dispatch.token --state-file /var/lib/aerolab/agi-dispatch.state%s
Restart=always
RestartSec=2s

[Install]
WantedBy=multi-user.target
`, target, insecureFlag)
}

func agiTargetURL(inst *backends.Instance) string {
	if strings.EqualFold(inst.Tags["aerolab4ssl"], "false") {
		return "http://" + inst.IP.Routable() + ":80"
	}
	return "https://" + inst.IP.Routable() + ":443"
}

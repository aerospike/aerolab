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
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

// ClusterAddAgiClientCmd installs the AGI live-log dispatcher on Aerospike cluster nodes.
type ClusterAddAgiClientCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Aerospike cluster name" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	SendLogsTo      string          `long:"send-logs-to" description:"AGI cluster name to stream logs to" required:"true"`
	AerolabBinary   flags.Filename  `long:"aerolab-binary" description:"Path to local aerolab binary to install on nodes (use with unofficial builds)"`
	InsecureTLS     bool            `long:"insecure-tls" description:"Pass --insecure-tls to agi exec dispatch (e.g. self-signed AGI certificate)"`
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
		stdoutp := io.Writer(os.Stdout)
		stdout = &stdoutp
		stderrp := io.Writer(os.Stderr)
		stderr = &stderrp
		stdinp := io.NopCloser(os.Stdin)
		stdin = &stdinp
	}
	if err := c.AddAgiClientCluster(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger); err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// AddAgiClientCluster installs aerolab, the dispatcher token, and systemd on the selected nodes.
func (c *ClusterAddAgiClientCmd) AddAgiClientCluster(system *System, inventory *backends.Inventory, args []string, stdin *io.ReadCloser, stdout *io.Writer, stderr *io.Writer, logger *logger.Logger) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "add", "agi-client"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	if c.ClusterName.String() == "" {
		return fmt.Errorf("cluster name is required")
	}
	if strings.Contains(c.ClusterName.String(), ",") {
		return fmt.Errorf("--name must be a single cluster name for agi-client (comma-separated lists are not supported)")
	}
	cluster, err := c.ClusterName.GetInstanceList(inventory, backends.LifeCycleStateRunning)
	if err != nil {
		return err
	}
	if c.Nodes.String() != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return err
		}
		cluster = cluster.WithNodeNo(nodes...)
		if cluster.Count() != len(nodes) {
			return fmt.Errorf("some nodes in %s not found", c.Nodes.String())
		}
	}
	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		logger.Info("No nodes to configure")
		return nil
	}

	agi := inventory.Instances.
		WithNotState(backends.LifeCycleStateTerminated, backends.LifeCycleStateTerminating).
		WithTags(map[string]string{"aerolab.type": "agi"}).
		WithClusterName(c.SendLogsTo).
		WithState(backends.LifeCycleStateRunning)
	if agi.Count() == 0 {
		return fmt.Errorf("no running AGI instance named %q with aerolab.type=agi in inventory; create the AGI with --enable-live-ingest first", c.SendLogsTo)
	}

	token, err := readAgiDispatchToken(agi, logger, c.MaxRetries, c.RetrySleep)
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("dispatcher token is empty on AGI %q; ensure the instance was created with --enable-live-ingest", c.SendLogsTo)
	}

	agiIP := firstAgiReachableIP(agi)
	if agiIP == "" {
		return fmt.Errorf("AGI %q has no public or private IP in inventory", c.SendLogsTo)
	}
	target := fmt.Sprintf("https://%s:443", agiIP)

	alAdd := &ClusterAddAerolabCmd{
		ClusterName:     c.ClusterName,
		Nodes:           c.Nodes,
		AerolabVersion:  "latest",
		ParallelThreads: c.ParallelThreads,
		MaxRetries:      c.MaxRetries,
		RetrySleep:      c.RetrySleep,
	}
	if _, err := alAdd.AddAerolabCluster(system, inventory, args, stdin, stdout, stderr, logger); err != nil {
		return fmt.Errorf("install aerolab on cluster nodes: %w", err)
	}

	if c.AerolabBinary != "" {
		if _, err := os.Stat(string(c.AerolabBinary)); err != nil {
			return fmt.Errorf("aerolab binary %s: %w", c.AerolabBinary, err)
		}
		bin, err := os.ReadFile(string(c.AerolabBinary))
		if err != nil {
			return err
		}
		var uploadErr error
		parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
			if err := uploadAerolabBinaryToInstance(inst, bin, c.MaxRetries, c.RetrySleep); err != nil {
				uploadErr = errors.Join(uploadErr, fmt.Errorf("%s:%d: %w", inst.ClusterName, inst.NodeNo, err))
			}
		})
		if uploadErr != nil {
			return uploadErr
		}
	}

	unit := buildAgiDispatchUnit(target, c.InsecureTLS)

	var hasErr error
	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		if err := deployAgiDispatchOnNode(inst, token, unit, stdin, stdout, stderr, c.MaxRetries, c.RetrySleep, c.ParallelThreads); err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", inst.ClusterName, inst.NodeNo, err))
		}
	})
	if hasErr != nil {
		return hasErr
	}
	logger.Info("Installed aerolab-agi-dispatch.service on %d node(s); logs stream to AGI %q (%s)", cluster.Count(), c.SendLogsTo, target)
	return nil
}

func firstAgiReachableIP(agi backends.Instances) string {
	for _, inst := range agi.Describe() {
		if inst.IP.Public != "" {
			return inst.IP.Public
		}
	}
	for _, inst := range agi.Describe() {
		if inst.IP.Private != "" {
			return inst.IP.Private
		}
	}
	return ""
}

func readAgiDispatchToken(agi backends.Instances, logger *logger.Logger, maxRetries int, retrySleep time.Duration) (string, error) {
	for _, inst := range agi.Describe() {
		out := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"cat", "/opt/agi/tokens/dispatcher"},
				SessionTimeout: 30 * time.Second,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
			MaxRetries:      maxRetries,
			RetrySleep:      retrySleep,
		})
		if out == nil || out.Output == nil {
			continue
		}
		if out.Output.Err != nil {
			logger.Warn("failed reading dispatcher token from %s:%d: %v", inst.ClusterName, inst.NodeNo, out.Output.Err)
			continue
		}
		tok := strings.TrimSpace(string(out.Output.Stdout))
		if tok != "" {
			return tok, nil
		}
	}
	return "", fmt.Errorf("could not read non-empty /opt/agi/tokens/dispatcher from any AGI node")
}

func buildAgiDispatchUnit(targetURL string, insecure bool) []byte {
	flagsLine := "--target=" + targetURL + " --token-file=/etc/aerolab/agi-dispatch.token --state-file=/var/lib/aerolab/agi-dispatch.state"
	if insecure {
		flagsLine += " --insecure-tls"
	}
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Aerolab AGI live log dispatcher\n")
	b.WriteString("After=aerospike.service network-online.target\n")
	b.WriteString("Wants=aerospike.service\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	b.WriteString("ExecStart=/usr/local/bin/aerolab agi exec dispatch " + flagsLine + "\n")
	b.WriteString("Restart=always\n")
	b.WriteString("RestartSec=2\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")
	return []byte(b.String())
}

func deployAgiDispatchOnNode(inst *backends.Instance, token string, unit []byte, stdin *io.ReadCloser, stdout *io.Writer, stderr *io.Writer, maxRetries int, retrySleep time.Duration, par int) error {
	conf, err := inst.GetSftpConfig("root")
	if err != nil {
		return err
	}
	conf.MaxRetries = maxRetries
	conf.RetrySleep = retrySleep
	cli, err := sshexec.NewSftp(conf)
	if err != nil {
		return err
	}
	defer cli.Close()
	_ = cli.RawClient().MkdirAll("/etc/aerolab")
	_ = cli.RawClient().MkdirAll("/var/lib/aerolab")
	if err := cli.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/etc/aerolab/agi-dispatch.token",
		Source:      strings.NewReader(strings.TrimSpace(token) + "\n"),
		Permissions: 0600,
	}); err != nil {
		return fmt.Errorf("write token: %w", err)
	}
	if err := cli.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/etc/systemd/system/aerolab-agi-dispatch.service",
		Source:      bytes.NewReader(unit),
		Permissions: 0644,
	}); err != nil {
		return fmt.Errorf("write unit: %w", err)
	}

	detail := sshexec.ExecDetail{
		Command:        []string{"bash", "-c", "systemctl daemon-reload && systemctl enable --now aerolab-agi-dispatch.service"},
		SessionTimeout: 2 * time.Minute,
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
	out := inst.Exec(&backends.ExecInput{
		ExecDetail:      detail,
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: par,
		MaxRetries:      maxRetries,
		RetrySleep:      retrySleep,
	})
	if out == nil || out.Output == nil || out.Output.Err != nil {
		msg := "no exec output"
		if out != nil && out.Output != nil {
			msg = fmt.Sprintf("%v (stdout=%q stderr=%q)", out.Output.Err, string(out.Output.Stdout), string(out.Output.Stderr))
		}
		return fmt.Errorf("systemctl enable --now aerolab-agi-dispatch: %s", msg)
	}
	return nil
}

func uploadAerolabBinaryToInstance(inst *backends.Instance, binaryData []byte, maxRetries int, retrySleep time.Duration) error {
	conf, err := inst.GetSftpConfig("root")
	if err != nil {
		return err
	}
	conf.MaxRetries = maxRetries
	conf.RetrySleep = retrySleep
	cli, err := sshexec.NewSftp(conf)
	if err != nil {
		return err
	}
	defer cli.Close()
	_ = cli.RawClient().Remove("/usr/local/bin/aerolab")
	if err := cli.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/usr/local/bin/aerolab",
		Source:      bytes.NewReader(binaryData),
		Permissions: 0755,
	}); err != nil {
		return err
	}
	return nil
}

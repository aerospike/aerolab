package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

// ClusterAddAgiClientCmd installs the AGI live log streaming
// dispatcher on every node of an existing Aerospike cluster, pointing
// it at a target AGI instance. It is the user-facing complement to
// `aerolab agi create --enable-live-ingest` (cmdAgiCreate.go) — that
// flag opens the listener inside AGI; this command stands up the
// dispatcher on the cluster side that feeds it.
//
// On each node, in parallel, it:
//
//  1. Pushes the local aerolab binary to /usr/local/bin/aerolab
//     (using --aerolab-binary if provided, else the binary running
//     this command — same behaviour as `cluster add aerolab`).
//  2. Reads the dispatcher token from the AGI instance via SSH
//     (pulled from /opt/agi/dispatcher.token, written by
//     `agi create --enable-live-ingest`) and writes it to
//     /etc/aerolab/agi-dispatch.token (mode 0600).
//  3. Renders /etc/systemd/system/aerolab-agi-dispatch.service with
//     ExecStart=/usr/local/bin/aerolab agi exec dispatch ...
//     pointing at the target AGI's URL.
//  4. systemctl daemon-reload && enable --now aerolab-agi-dispatch.
//
// The command is idempotent: re-running it with a different
// --send-logs-to re-renders the unit and restarts it.
type ClusterAddAgiClientCmd struct {
	ClusterName     TypeClusterName    `short:"n" long:"name" description:"Cluster name (the source of logs)" default:"mydc"`
	Nodes           TypeNodes          `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	SendLogsTo      TypeAgiClusterName `long:"send-logs-to" description:"AGI instance name to dispatch logs to (must be created with --enable-live-ingest)" required:"true"`
	AerolabBinary   flags.Filename     `long:"aerolab-binary" description:"Path to local aerolab binary to push to nodes; default: the binary running this command"`
	InsecureTLS     bool               `long:"insecure-tls" description:"Skip server cert verification when the dispatcher POSTs to AGI"`
	ParallelThreads int                `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	MaxRetries      int                `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep      time.Duration      `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`
	Help            HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute is the CLI entry point. The heavy lifting lives in
// AddAgiClient so other commands can compose it.
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
	err = c.AddAgiClient(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// AddAgiClient performs the actual install across the named cluster.
// Split out from Execute so the test harness and other commands
// (e.g. agi create's optional auto-attach path) can call it without
// going back through the CLI parser.
func (c *ClusterAddAgiClientCmd) AddAgiClient(system *System, inventory *backends.Inventory, args []string, stdin *io.ReadCloser, stdout *io.Writer, stderr *io.Writer, log *logger.Logger) error {
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
	if c.SendLogsTo.String() == "" {
		return fmt.Errorf("--send-logs-to is required")
	}

	// Resolve the AGI target instance (must exist and be running).
	agiList, err := c.SendLogsTo.GetInstanceList(inventory, backends.LifeCycleStateRunning)
	if err != nil {
		return fmt.Errorf("could not look up AGI %s: %w", c.SendLogsTo, err)
	}
	if agiList.Count() == 0 {
		return fmt.Errorf("AGI %s not running (or not found)", c.SendLogsTo)
	}
	agi := agiList.Describe()[0]
	agiIP := agi.IP.Public
	if agiIP == "" {
		agiIP = agi.IP.Private
	}
	if agiIP == "" {
		return fmt.Errorf("AGI %s has no reachable IP", c.SendLogsTo)
	}

	// Determine the AGI's TLS port from its tags. The proxy listens
	// on 443 by default (HTTPS) or 80 when --proxy-ssl-disable is
	// set; the aerolab4ssl tag, if present, says which.
	port := 443
	scheme := "https"
	if agi.Tags != nil {
		if v, ok := agi.Tags["aerolab4ssl"]; ok && v == "false" {
			port = 80
			scheme = "http"
		}
	}
	target := fmt.Sprintf("%s://%s:%d", scheme, agiIP, port)
	log.Info("Live ingest target: %s", target)

	// Pull the dispatcher token from the AGI side. Stored at
	// /opt/agi/dispatcher.token by `agi create --enable-live-ingest`.
	token, err := c.fetchDispatcherToken(agi)
	if err != nil {
		return fmt.Errorf("read dispatcher token from AGI %s: %w (was the AGI created with --enable-live-ingest?)", c.SendLogsTo, err)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("dispatcher token on AGI %s is empty", c.SendLogsTo)
	}

	// Build the local aerolab binary payload that we'll push to
	// every node. Default: the binary currently running.
	aerolabBinary := string(c.AerolabBinary)
	if aerolabBinary == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("could not resolve current aerolab binary: %w", err)
		}
		// Resolve symlinks so we ship the actual binary even when
		// invoked through e.g. /usr/local/bin/aerolab -> ../something.
		resolved, err := filepath.EvalSymlinks(exe)
		if err == nil {
			aerolabBinary = resolved
		} else {
			aerolabBinary = exe
		}
	}
	binaryBytes, err := os.ReadFile(aerolabBinary)
	if err != nil {
		return fmt.Errorf("read aerolab binary %s: %w", aerolabBinary, err)
	}

	// Resolve the source cluster's instance list.
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
		log.Info("No nodes to deploy AGI dispatcher to")
		return nil
	}
	log.Info("Deploying AGI dispatcher to %d nodes (cluster=%s -> AGI=%s)", cluster.Count(), c.ClusterName, c.SendLogsTo)

	unitText := renderDispatcherUnit(target, c.InsecureTLS)

	var hasErr error
	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(inst *backends.Instance) {
		if err := c.installOnNode(inst, binaryBytes, []byte(token), []byte(unitText), stdin, stdout, stderr); err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", inst.ClusterName, inst.NodeNo, err))
		}
	})
	return hasErr
}

// installOnNode performs the per-node deploy: SFTP the aerolab
// binary, the token, and the unit, then run a tiny activation script.
// The activation script does daemon-reload, enable, and restart so
// re-running the command is idempotent (a different --send-logs-to
// rewrites the unit and triggers a clean restart).
func (c *ClusterAddAgiClientCmd) installOnNode(inst *backends.Instance, binaryBytes, tokenBytes, unitBytes []byte, stdin *io.ReadCloser, stdout *io.Writer, stderr *io.Writer) error {
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

	// Ensure target dirs exist. _ = ignored: MkdirAll is idempotent
	// and a not-here error from the remote means subsequent writes
	// will surface the real cause.
	_ = cli.RawClient().MkdirAll("/usr/local/bin")
	_ = cli.RawClient().MkdirAll("/etc/aerolab")
	_ = cli.RawClient().MkdirAll("/var/lib/aerolab")
	_ = cli.RawClient().MkdirAll("/etc/systemd/system")

	if err := cli.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/usr/local/bin/aerolab",
		Source:      bytes.NewReader(binaryBytes),
		Permissions: 0o755,
	}); err != nil {
		return fmt.Errorf("upload aerolab binary: %w", err)
	}
	if err := cli.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/etc/aerolab/agi-dispatch.token",
		Source:      bytes.NewReader(tokenBytes),
		Permissions: 0o600,
	}); err != nil {
		return fmt.Errorf("upload dispatcher token: %w", err)
	}
	if err := cli.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/etc/systemd/system/aerolab-agi-dispatch.service",
		Source:      bytes.NewReader(unitBytes),
		Permissions: 0o644,
	}); err != nil {
		return fmt.Errorf("upload systemd unit: %w", err)
	}

	// Activation script. We restart even if it was already running
	// so a token rotation or --send-logs-to change takes effect.
	script := []byte(`#!/usr/bin/env bash
set -euo pipefail
systemctl daemon-reload
systemctl enable aerolab-agi-dispatch.service
if systemctl is-active --quiet aerolab-agi-dispatch.service; then
    systemctl restart aerolab-agi-dispatch.service
else
    systemctl start aerolab-agi-dispatch.service
fi
`)
	now := time.Now().Format("20060102150405")
	scriptPath := "/opt/aerolab/scripts/add-agi-client." + now + ".sh"
	_ = cli.RawClient().MkdirAll("/opt/aerolab/scripts")
	if err := cli.WriteFile(true, &sshexec.FileWriter{
		DestPath:    scriptPath,
		Source:      bytes.NewReader(script),
		Permissions: 0o755,
	}); err != nil {
		return fmt.Errorf("upload activation script: %w", err)
	}

	detail := sshexec.ExecDetail{
		Command:        []string{"bash", scriptPath},
		SessionTimeout: 3 * time.Minute,
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
		failure := scriptlog.NewScriptFailureWithPath(
			inst.ClusterName,
			inst.NodeNo,
			scriptPath,
			script,
			output.Output.Stdout,
			output.Output.Stderr,
			output.Output.Err,
		)
		logPath, saveErr := scriptlog.SaveFailure(failure)
		if saveErr != nil {
			return fmt.Errorf("activate dispatcher: %s (stdout: %s, stderr: %s) (also failed to save logs: %v)", output.Output.Err, string(output.Output.Stdout), string(output.Output.Stderr), saveErr)
		}
		return fmt.Errorf("%s", scriptlog.FormatError(logPath, inst.ClusterName, inst.NodeNo, output.Output.Err))
	}
	return nil
}

// fetchDispatcherToken reads /opt/agi/dispatcher.token from the AGI
// instance via SSH. The token is written there by
// `agi create --enable-live-ingest`; absence indicates the AGI was
// created without live ingest enabled.
func (c *ClusterAddAgiClientCmd) fetchDispatcherToken(agi *backends.Instance) (string, error) {
	output := backends.InstanceList{agi}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"cat", "/opt/agi/dispatcher.token"},
			SessionTimeout: 30 * time.Second,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
		MaxRetries:      c.MaxRetries,
		RetrySleep:      c.RetrySleep,
	})
	if len(output) == 0 {
		return "", fmt.Errorf("no exec output")
	}
	if output[0].Output.Err != nil {
		return "", fmt.Errorf("%s (stderr: %s)", output[0].Output.Err, string(output[0].Output.Stderr))
	}
	return strings.TrimSpace(string(output[0].Output.Stdout)), nil
}

// renderDispatcherUnit produces the systemd unit text for the
// per-node dispatcher service. The unit is intentionally simple:
// Restart=always means a transient AGI outage doesn't kill the
// service, only delays it (the dispatcher's internal backoff loop
// does the same; the systemd-level Restart is the second line of
// defence for hard process exits).
//
// Wants/After=aerospike.service so the dispatcher waits for the
// server to be up before starting; if aerospike is down we have
// nothing to tail anyway.
func renderDispatcherUnit(target string, insecureTLS bool) string {
	insecure := ""
	if insecureTLS {
		insecure = " --insecure-tls"
	}
	return fmt.Sprintf(`[Unit]
Description=AeroLab AGI live log dispatcher
After=network.target aerospike.service
Wants=aerospike.service

[Service]
Type=simple
ExecStart=/usr/local/bin/aerolab agi exec dispatch --target %s --token-file /etc/aerolab/agi-dispatch.token --state-file /var/lib/aerolab/agi-dispatch.state%s
Restart=always
RestartSec=2s
User=root
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, target, insecure)
}

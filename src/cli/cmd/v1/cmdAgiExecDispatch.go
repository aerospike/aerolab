//go:build !noagi

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aerospike/aerolab/pkg/agi/dispatcher"
	flags "github.com/rglonek/go-flags"
)

// AgiExecDispatchCmd runs the live log streaming dispatcher on an
// Aerospike cluster node. It tails the local aerospike.log (or
// journald) and POSTs the stream to a target AGI instance's
// /agi/ingest/stream endpoint.
//
// This is a hidden subcommand: end-users never invoke it directly.
// `aerolab cluster add agi-client` (cmdClusterAddAgiClient.go) pushes
// the aerolab binary onto each cluster node and installs a systemd
// unit whose ExecStart calls this command.
type AgiExecDispatchCmd struct {
	Target            string         `long:"target" description:"AGI URL, e.g. https://10.0.0.5:443" required:"true"`
	TokenFile         flags.Filename `long:"token-file" description:"Path to bearer token file" default:"/etc/aerolab/agi-dispatch.token"`
	ClusterName       string         `long:"cluster" description:"Cluster name (default: auto-detect via asinfo)"`
	NodeID            string         `long:"node-id" description:"Node ID (default: auto-detect via asinfo)"`
	SourceFile        string         `long:"source-file" description:"Path to log file (default: auto-detect from aerospike.conf)"`
	SourceJournal     string         `long:"source-journal" description:"Systemd unit to follow via journalctl (default: aerospike.service if console logging detected)"`
	AerospikeConf     flags.Filename `long:"aerospike-conf" description:"Path to aerospike.conf for source/identity auto-discovery" default:"/etc/aerospike/aerospike.conf"`
	StateFile         flags.Filename `long:"state-file" description:"Where to persist the byte-offset/cursor checkpoint" default:"/var/lib/aerolab/agi-dispatch.state"`
	InsecureTLS       bool           `long:"insecure-tls" description:"Skip server cert verification (default: trust the system CA bundle)"`
	BackfillFromStart bool           `long:"backfill-from-start" description:"Open file sources at offset 0 instead of EOF on first run; ignored when state-file already has progress"`
	Help              HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute is the entry point for `aerolab agi exec dispatch`.
//
// It:
//  1. Reads the bearer token from --token-file (token-on-argv would
//     surface in `ps` output).
//  2. Builds a dispatcher.Config and calls dispatcher.New(cfg).Run(ctx).
//  3. Cancels the context on SIGINT/SIGTERM so the dispatcher can
//     flush its state file before exit.
func (c *AgiExecDispatchCmd) Execute(args []string) error {
	tokenBytes, err := os.ReadFile(string(c.TokenFile))
	if err != nil {
		return fmt.Errorf("read token file %s: %w", c.TokenFile, err)
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return fmt.Errorf("token file %s is empty", c.TokenFile)
	}

	if err := dispatcher.EnsureStateDir(string(c.StateFile)); err != nil {
		return fmt.Errorf("state dir: %w", err)
	}

	cfg := dispatcher.Config{
		Target:            c.Target,
		Token:             token,
		ClusterName:       c.ClusterName,
		NodeID:            c.NodeID,
		AerospikeConf:     string(c.AerospikeConf),
		SourceFile:        c.SourceFile,
		SourceJournal:     c.SourceJournal,
		StateFile:         string(c.StateFile),
		InsecureTLS:       c.InsecureTLS,
		BackfillFromStart: c.BackfillFromStart,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	d := dispatcher.New(cfg)
	if err := d.Run(ctx); err != nil {
		return err
	}
	return nil
}

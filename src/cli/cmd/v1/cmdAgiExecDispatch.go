//go:build !noagi

package cmd

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aerospike/aerolab/pkg/agi/dispatcher"
)

// AgiExecDispatchCmd runs the AGI live log dispatcher (tails Aerospike logs and POSTs to AGI).
type AgiExecDispatchCmd struct {
	Target            string  `long:"target" description:"AGI URL, e.g. https://10.0.0.5:443" required:"true"`
	TokenFile         string  `long:"token-file" description:"Path to bearer token" default:"/etc/aerolab/agi-dispatch.token"`
	ClusterName       string  `long:"cluster" description:"Cluster name (default: auto-detect via asinfo)"`
	NodeID            string  `long:"node-id" description:"Node ID (default: auto-detect via asinfo)"`
	SourceFile        string  `long:"source-file" description:"Path to log file (default: auto-detect from aerospike.conf)"`
	SourceJournal     string  `long:"source-journal" description:"Systemd unit for journalctl (default from aerospike.conf console logging)"`
	AerospikeConf     string  `long:"aerospike-conf" default:"/etc/aerospike/aerospike.conf"`
	StateFile         string  `long:"state-file" default:"/var/lib/aerolab/agi-dispatch.state"`
	InsecureTLS       bool    `long:"insecure-tls" description:"Skip TLS certificate verification"`
	BackfillFromStart bool    `long:"backfill-from-start"`
	Help              HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AgiExecDispatchCmd) Execute(args []string) error {
	cmd := []string{"agi", "exec", "dispatch"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	tokBytes, err := os.ReadFile(c.TokenFile)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	token := strings.TrimSpace(string(tokBytes))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	d := dispatcher.New(dispatcher.Config{
		Target:            c.Target,
		Token:             token,
		ClusterName:       c.ClusterName,
		NodeID:            c.NodeID,
		AerospikeConf:     c.AerospikeConf,
		SourceFile:        c.SourceFile,
		SourceJournal:     c.SourceJournal,
		StateFile:         c.StateFile,
		InsecureTLS:       c.InsecureTLS,
		BackfillFromStart: c.BackfillFromStart,
	})
	if err := d.Run(ctx); err != nil && err != context.Canceled {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

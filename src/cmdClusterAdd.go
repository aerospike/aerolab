package main

import (
	"errors"
	"os"
	"time"
)

type clusterAddCmd struct {
	Exporter clusterAddExporterCmd `command:"exporter" subcommands-optional:"true" description:"Install ams exporter in a cluster or clusters"`
	Firewall clusterAddFirewallCmd `command:"firewall" subcommands-optional:"true" description:"Add firewall rules to an existing cluster"`
	Expiry   clusterAddExpiryCmd   `command:"expiry" subcommands-optional:"true" description:"Add or change hours until expiry for a cluster (aws|gcp only)"`
	Help     helpCmd               `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterAddCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type clusterAddExpiryCmd struct {
	ClusterName TypeClusterName        `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Expires     time.Duration          `long:"expire" description:"length of life of nodes prior to expiry from now; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry" default:"30h"`
	Gcp         clusterAddExpiryCmdGcp `no-flag:"true"`
	Help        helpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clusterAddExpiryCmdGcp struct {
	Zone string `long:"zone" description:"zone name where the nodes are"`
}

func init() {
	addBackendSwitch("cluster.add.expiry", "gcp", &a.opts.Cluster.Add.Expiry.Gcp)
}

func (c *clusterAddExpiryCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type == "docker" {
		return errors.New("feature not supported on docker")
	}
	b.WorkOnServers()
	return b.ClusterExpiry(c.Gcp.Zone, c.ClusterName.String(), c.Expires)
}

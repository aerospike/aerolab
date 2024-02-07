package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bestmethod/inslice"
)

type clusterAddFirewallCmd struct {
	ClusterName TypeClusterName          `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"mydc"`
	Remove      bool                     `short:"r" long:"remove" description:"Set to remove the given firewalls instead of adding them"`
	Gcp         clusterAddFirewallCmdGcp `no-flag:"true"`
	Aws         clusterAddFirewallCmdAws `no-flag:"true"`
	Help        helpCmd                  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clusterAddFirewallCmdGcp struct {
	NamePrefix []string `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external"`
	Zone       string   `long:"zone" description:"zone name" webrequired:"true"`
}

type clusterAddFirewallCmdAws struct {
	NamePrefix []string `long:"secgroup-name" description:"Name prefix to use to add, can be specified multiple times" default:"AeroLab"`
}

func init() {
	addBackendSwitch("cluster.add.firewall", "gcp", &a.opts.Cluster.Add.Firewall.Gcp)
	addBackendSwitch("cluster.add.firewall", "aws", &a.opts.Cluster.Add.Firewall.Aws)
}

func (c *clusterAddFirewallCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	b.WorkOnServers()
	if a.opts.Config.Backend.Type == "docker" {
		return errors.New("feature not supported on docker backend")
	}
	if a.opts.Config.Backend.Type == "gcp" {
		if len(c.Gcp.NamePrefix) == 0 {
			return errors.New("specify at least one firewall name to add or remove")
		}
		if c.Gcp.Zone == "" {
			return errors.New("zone must be specified; list clusters with their zones using `aerolab inventory list`")
		}
	} else {
		if len(c.Aws.NamePrefix) == 0 {
			return errors.New("specify at least one firewall name to add or remove")
		}

	}
	log.Println("Running cluster.add.firewall")
	clusterList, err := b.ClusterList()
	if err != nil {
		return err
	}
	clusters := []string{}
	for _, cc := range strings.Split(c.ClusterName.String(), ",") {
		if !inslice.HasString(clusterList, cc) {
			return fmt.Errorf("cluster %s does not exist", cc)
		}
		clusters = append(clusters, cc)
	}
	for _, cluster := range clusters {
		np := c.Gcp.NamePrefix
		nz := c.Gcp.Zone
		if a.opts.Config.Backend.Type == "aws" {
			np = c.Aws.NamePrefix
			nz = ""
		}
		err = b.AssignSecurityGroups(cluster, np, nz, c.Remove, false)
		if err != nil {
			return err
		}
	}
	log.Println("Done")
	return nil
}

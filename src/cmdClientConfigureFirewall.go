package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bestmethod/inslice"
)

type clientConfigureFirewallCmd struct {
	ClusterName TypeClientName                `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"client"`
	Remove      bool                          `short:"r" long:"remove" description:"Set to remove the given firewalls instead of adding them"`
	Gcp         clientConfigureFirewallCmdGcp `no-flag:"true"`
	Aws         clientConfigureFirewallCmdAws `no-flag:"true"`
	Help        helpCmd                       `command:"help" subcommands-optional:"true" description:"Print help"`
}

type clientConfigureFirewallCmdGcp struct {
	NamePrefix []string `long:"firewall" description:"Name to use for the firewall, can be specified multiple times" default:"aerolab-managed-external"`
	Zone       string   `long:"zone" description:"zone name" webrequired:"true"`
}

type clientConfigureFirewallCmdAws struct {
	NamePrefix []string `long:"secgroup-name" description:"Name prefix to use to add, can be specified multiple times" default:"AeroLab"`
}

func init() {
	addBackendSwitch("client.configure.firewall", "gcp", &a.opts.Client.Configure.Firewall.Gcp)
	addBackendSwitch("client.configure.firewall", "aws", &a.opts.Client.Configure.Firewall.Aws)
}

func (c *clientConfigureFirewallCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	b.WorkOnClients()
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
	log.Println("Running client.configure.firewall")
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
		err = b.AssignSecurityGroups(cluster, np, nz, c.Remove)
		if err != nil {
			return err
		}
	}
	log.Println("Done")
	return nil
}

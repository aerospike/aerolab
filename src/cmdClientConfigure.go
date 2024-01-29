package main

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type clientConfigureCmd struct {
	AMS         clientConfigureAMSCmd         `command:"ams" subcommands-optional:"true" description:"change which clusters prometheus points at" webicon:"fas fa-layer-group"`
	Tools       clientConfigureToolsCmd       `command:"tools" subcommands-optional:"true" description:"add graph monitoring for AMS for asbenchmark" webicon:"fas fa-toolbox"`
	VSCode      clientConfigureVSCodeCmd      `command:"vscode" subcommands-optional:"true" description:"add languages to VSCode" webicon:"fas fa-code"`
	Trino       clientConfigureTrinoCmd       `command:"trino" subcommands-optional:"true" description:"change aerospike seed IPs for trino" webicon:"fas fa-tachograph-digital"`
	RestGateway clientConfigureRestGatewayCmd `command:"rest-gateway" subcommands-optional:"true" description:"change aerospike seed IPs for the rest-gateway" webicon:"fas fa-dungeon"`
	Firewall    clientConfigureFirewallCmd    `command:"firewall" subcommands-optional:"true" description:"Add firewall rules to existing client machines" webicon:"fas fa-fire"`
	Expiry      clientAddExpiryCmd            `command:"expiry" subcommands-optional:"true" description:"Add or change hours until expiry for a client group (aws|gcp only)" webicon:"fas fa-user-xmark"`
	AeroLab     clientConfigureAerolabCmd     `command:"aerolab" subcommands-optional:"true" description:"Deploy aerolab binary on a client group" webicon:"fas fa-flask"`
	Help        helpCmd                       `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureCmd) Execute(args []string) error {
	c.Help.Execute(args)
	os.Exit(1)
	return nil
}

type clientConfigureAerolabCmd struct {
	ClusterName TypeClientName `short:"n" long:"name" description:"Client name" default:"client"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clientConfigureAerolabCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Cluster.Add.AeroLab.ClusterName = TypeClusterName(c.ClusterName)
	a.opts.Cluster.Add.AeroLab.ParallelThreads = c.ParallelThreads
	return a.opts.Cluster.Add.AeroLab.run(true)
}

type clientAddExpiryCmd struct {
	ClusterName TypeClientName         `short:"n" long:"name" description:"Client name" default:"mydc"`
	Nodes       TypeMachines           `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Expires     time.Duration          `long:"expire" description:"length of life of nodes prior to expiry from now; smh - seconds, minutes, hours, ex 20h 30m; 0: no expiry" default:"30h"`
	Gcp         clusterAddExpiryCmdGcp `no-flag:"true"`
	Help        helpCmd                `command:"help" subcommands-optional:"true" description:"Print help"`
}

func init() {
	addBackendSwitch("client.configure.expiry", "gcp", &a.opts.Client.Configure.Expiry.Gcp)
}

func (c *clientAddExpiryCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type == "docker" {
		return errors.New("feature not supported on docker")
	}
	b.WorkOnClients()
	err := c.Nodes.ExpandNodes(c.ClusterName.String())
	if err != nil {
		return err
	}
	nodes, err := c.Nodes.Translate(c.ClusterName.String())
	if err != nil {
		return err
	}
	return b.ClusterExpiry(c.Gcp.Zone, c.ClusterName.String(), c.Expires, nodes)
}

func (n *TypeMachines) Translate(clusterName string) ([]int, error) {
	if n.String() == "" {
		return b.NodeListInCluster(clusterName)
	}
	nodes := []int{}
	for _, ns := range strings.Split(n.String(), ",") {
		nn, err := strconv.Atoi(ns)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, nn)
	}
	return nodes, nil
}

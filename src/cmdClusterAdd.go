package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aerospike/aerolab/parallelize"
)

type clusterAddCmd struct {
	Exporter clusterAddExporterCmd `command:"exporter" subcommands-optional:"true" description:"Install ams exporter in a cluster or clusters"`
	Firewall clusterAddFirewallCmd `command:"firewall" subcommands-optional:"true" description:"Add firewall rules to an existing cluster"`
	Expiry   clusterAddExpiryCmd   `command:"expiry" subcommands-optional:"true" description:"Add or change hours until expiry for a cluster (aws|gcp only)"`
	AeroLab  clusterAddAerolabCmd  `command:"aerolab" subcommands-optional:"true" description:"Deploy aerolab binary on a cluster"`
	Help     helpCmd               `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterAddCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type clusterAddExpiryCmd struct {
	ClusterName TypeClusterName        `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	Nodes       TypeNodes              `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
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

type clusterAddAerolabCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster name" default:"mydc"`
	parallelThreadsCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *clusterAddAerolabCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	return c.run(false)
}

func (c *clusterAddAerolabCmd) run(onClient bool) error {
	if onClient {
		b.WorkOnClients()
	} else {
		b.WorkOnServers()
	}
	nodes, err := b.NodeListInCluster(string(c.ClusterName))
	if err != nil {
		return err
	}

	erro := parallelize.MapLimit(nodes, c.ParallelThreads, func(node int) error {
		// we need to know if node is arm
		isArm, err := b.IsNodeArm(c.ClusterName.String(), node)
		if err != nil {
			return fmt.Errorf("could not identify node architecture: %s", err)
		}

		// upload aerolab to remote
		nLinuxBinary := nLinuxBinaryX64
		if isArm {
			nLinuxBinary = nLinuxBinaryArm64
		}
		if len(nLinuxBinary) == 0 {
			execName, err := findExec()
			if err != nil {
				return err
			}
			nLinuxBinary, err = os.ReadFile(execName)
			if err != nil {
				return err
			}
		}
		flist := []fileListReader{
			{
				filePath:     "/usr/local/bin/aerolab",
				fileContents: bytes.NewReader(nLinuxBinary),
				fileSize:     len(nLinuxBinary),
			},
		}
		err = b.CopyFilesToClusterReader(c.ClusterName.String(), flist, []int{node})
		if err != nil {
			return fmt.Errorf("could not upload aerolab to instance: %s", err)
		}
		out, err := b.RunCommands(c.ClusterName.String(), [][]string{{"chmod", "755", "/usr/local/bin/aerolab"}}, []int{node})
		if err != nil {
			return fmt.Errorf("failed to chmod 755 aerolab: %s\n%s", err, string(out[0]))
		}
		return nil
	})
	isErr := false
	for nid, err := range erro {
		if err != nil {
			log.Printf("Node %d error: %s", nodes[nid], err)
			isErr = true
		}
	}
	if isErr {
		return errors.New("some nodes returned errors")
	}
	return nil
}

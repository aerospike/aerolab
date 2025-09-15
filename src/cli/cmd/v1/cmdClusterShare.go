package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClusterShareCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes           TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	ParallelThreads int             `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	ConnectTimeout  time.Duration   `short:"C" long:"connect-timeout" description:"Connect timeout" default:"10s"`
	KeyFile         flags.Filename  `short:"f" long:"pubkey" description:"Path to a pubkey to import to cluster nodes"`
	Help            HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClusterShareCmd) Execute(args []string) error {
	cmd := []string{"cluster", "share"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ShareCluster(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *ClusterShareCmd) ShareCluster(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cluster", "share"}, c, args...)
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
	// open the key file, read it, and make sure it looks like an ssh public key
	pubkey, err := os.ReadFile(string(c.KeyFile))
	if err != nil {
		return fmt.Errorf("failed to read key file %s: %s", string(c.KeyFile), err)
	}
	if !strings.HasPrefix(string(pubkey), "ssh-") {
		return fmt.Errorf("key file %s does not look like an ssh public key", string(c.KeyFile))
	}

	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			err := c.ShareCluster(system, inventory, args)
			if err != nil {
				return err
			}
		}
		return nil
	}
	cluster := inventory.Instances.WithClusterName(c.ClusterName.String())
	if cluster == nil {
		return fmt.Errorf("cluster %s not found", c.ClusterName.String())
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
		logger.Info("No nodes to share")
		return nil
	}
	logger.Info("Sharing %d nodes", cluster.Count())
	parallelize.ForEachLimit(cluster.Describe(), c.ParallelThreads, func(node *backends.Instance) {
		s, err := node.GetSftpConfig("root")
		if err != nil {
			logger.Error("Failed to get sftp config for node %s: %s", node.Name, err)
			return
		}
		client, err := sshexec.NewSftp(s)
		if err != nil {
			logger.Error("Failed to create sftp client for node %s: %s", node.Name, err)
			return
		}
		defer client.Close()
		if !client.IsExists("/root/.ssh") {
			err = client.Mkdir("/root/.ssh", 0700)
			if err != nil {
				logger.Error("Failed to create .ssh directory for node %s: %s", node.Name, err)
				return
			}
		}
		var authorizedKeys []byte
		if client.IsExists("/root/.ssh/authorized_keys") {
			var buf bytes.Buffer
			fr := sshexec.FileReader{
				SourcePath:  "/root/.ssh/authorized_keys",
				Destination: &buf,
			}
			err = client.ReadFile(&fr)
			if err != nil {
				logger.Error("Failed to read authorized keys for node %s: %s", node.Name, err)
				return
			}
			authorizedKeys = buf.Bytes()
			if len(authorizedKeys) > 0 && authorizedKeys[len(authorizedKeys)-1] != '\n' {
				authorizedKeys = append(authorizedKeys, '\n')
			}
		}
		authorizedKeys = append(authorizedKeys, string(pubkey)...)
		if len(authorizedKeys) > 0 && authorizedKeys[len(authorizedKeys)-1] != '\n' {
			authorizedKeys = append(authorizedKeys, '\n')
		}
		fw := sshexec.FileWriter{
			DestPath:    "/root/.ssh/authorized_keys",
			Source:      bytes.NewReader(authorizedKeys),
			Permissions: 600,
		}
		err = client.WriteFile(false, &fw)
		if err != nil {
			logger.Error("Failed to write authorized keys for node %s: %s", node.Name, err)
			return
		}
	})
	return nil
}

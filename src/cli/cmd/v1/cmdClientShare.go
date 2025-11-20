package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClientShareCmd struct {
	ClusterName     TypeClusterName `short:"n" long:"name" description:"Client name" default:"client"`
	KeyFile         flags.Filename  `short:"f" long:"pubkey" description:"Path to a pubkey to import to client nodes"`
	ParallelThreads int             `short:"p" long:"parallel-threads" description:"Number of parallel threads to use for the execution" default:"10"`
	Help            HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ClientShareCmd) Execute(args []string) error {
	cmd := []string{"client", "share"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.shareClient(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientShareCmd) shareClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	// Check if ssh-copy-id exists
	isComm, err := exec.LookPath("ssh-copy-id")
	if (err != nil && !errors.Is(err, exec.ErrDot)) || isComm == "" {
		return errors.New("command `ssh-copy-id` not found; this command relies on existence of `ssh-copy-id`, part of the ssh-client")
	}

	// Check if key file exists
	if _, err := os.Stat(string(c.KeyFile)); err != nil {
		return fmt.Errorf("could not access the provided key file %s: %w", string(c.KeyFile), err)
	}

	// Get client instances
	clients := inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).
		WithClusterName(c.ClusterName.String()).
		WithState(backends.LifeCycleStateRunning)

	if clients.Count() == 0 {
		return fmt.Errorf("client %s not found or has no running instances", c.ClusterName.String())
	}

	clientList := clients.Describe()

	// Get SSH key path for the cluster
	myKey := clientList[0].GetSSHKeyPath()

	// Copy public key to all client nodes
	var hasErr error
	parallelize.ForEachLimit(clientList, c.ParallelThreads, func(client *backends.Instance) {
		params := []string{
			"-f",
			"-i",
			string(c.KeyFile),
			"-o",
			"IdentityFile=" + myKey,
			"-o",
			"PreferredAuthentications=publickey",
			"-o",
			"StrictHostKeyChecking=no",
			"root@" + client.IP.Public,
		}
		out, err := exec.Command("ssh-copy-id", params...).CombinedOutput()
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: could not copy id: %w: %s", client.ClusterName, client.NodeNo, err, string(out)))
		}
	})

	return hasErr
}


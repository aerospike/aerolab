package cmd

import (
	"fmt"
	"os"
	"path"
	"strings"
	"text/tabwriter"

	"github.com/citrusleaf/aerolab/pkg/sshexec"
)

// ConfigHostKeysCmd groups the commands that inspect and edit the local SSH
// host key store.
//
// AeroLab learns each instance's SSH host key on first connect and verifies it
// on every later connection. A node rebuilt outside AeroLab presents a new key,
// which shows up as a warning (or, with 'config backend --ssh-strict-host-key',
// a refused connection) until the old entry is forgotten.
type ConfigHostKeysCmd struct {
	List   ConfigHostKeysListCmd   `command:"list" subcommands-optional:"true" description:"List remembered SSH host keys" webicon:"fas fa-list"`
	Forget ConfigHostKeysForgetCmd `command:"forget" subcommands-optional:"true" description:"Forget remembered SSH host keys so they are learned again" webicon:"fas fa-trash"`
	Help   HelpCmd                 `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigHostKeysCmd) Execute(args []string) error {
	//nolint:errcheck
	c.Help.Execute(args)
	return nil
}

// hostKeyStore opens the host key store for the current project. It is derived
// from the same path the backend uses, so no backend initialization (and no
// cloud API call) is needed just to list or prune entries.
func hostKeyStore() (*sshexec.HostKeyStore, error) {
	rootDir, err := AerolabRootDir()
	if err != nil {
		return nil, err
	}
	project := os.Getenv("AEROLAB_PROJECT")
	if project == "" {
		project = "default"
	}
	return sshexec.NewHostKeyStore(path.Join(rootDir, "projects", project, "known-hosts.json")), nil
}

type ConfigHostKeysListCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigHostKeysListCmd) Execute(args []string) error {
	cmd := []string{"config", "host-keys", "list"}
	system, err := Initialize(&Init{InitBackend: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	store, err := hostKeyStore()
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	entries, err := store.List()
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	if len(entries) == 0 {
		fmt.Printf("No SSH host keys are remembered yet (%s)\n", store.Path())
		return Error(nil, system, cmd, c, args)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tFINGERPRINT\tHOST\tLEARNED")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.ID, e.KeyType, e.Fingerprint, e.Host, e.LearnedAt.Format("2006-01-02 15:04:05"))
	}
	//nolint:errcheck
	w.Flush()

	for _, e := range entries {
		if e.PreviousFingerprint != "" {
			fmt.Printf("NOTE: %s replaced fingerprint %s at %s\n", e.ID, e.PreviousFingerprint, e.ReplacedAt.Format("2006-01-02 15:04:05"))
		}
	}
	return Error(nil, system, cmd, c, args)
}

type ConfigHostKeysForgetCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated; forgets every node unless --nodes is given"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL"`
	All         bool            `long:"all" description:"Forget every remembered host key in this project"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigHostKeysForgetCmd) Execute(args []string) error {
	cmd := []string{"config", "host-keys", "forget"}

	// Forgetting by cluster name needs the inventory to map names to cluster
	// UUIDs; --all works purely on the local file.
	needBackend := !c.All
	system, err := Initialize(&Init{InitBackend: needBackend}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	store, err := hostKeyStore()
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	if c.All {
		if err := store.ForgetAll(); err != nil {
			return Error(err, system, cmd, c, args)
		}
		system.Logger.Info("Forgot all remembered SSH host keys")
		return Error(nil, system, cmd, c, args)
	}

	if c.ClusterName.String() == "" {
		return Error(fmt.Errorf("specify --name with the cluster to forget, or --all"), system, cmd, c, args)
	}

	var nodeFilter []int
	if c.Nodes.String() != "" {
		nodeFilter, err = expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
	}

	inventory := system.Backend.GetInventory()
	ids := []string{}
	for _, name := range strings.Split(c.ClusterName.String(), ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		instances := inventory.Instances.WithClusterName(name)
		if len(nodeFilter) > 0 {
			instances = instances.WithNodeNo(nodeFilter...)
		}
		for _, i := range instances.Describe() {
			if id := i.HostKeyID(); id != "" {
				ids = append(ids, id)
			}
		}
	}

	if len(ids) == 0 {
		system.Logger.Info("No matching instances with remembered host keys found")
		return Error(nil, system, cmd, c, args)
	}
	if err := store.Forget(ids...); err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Forgot %d SSH host key(s); they will be relearned on the next connection", len(ids))
	for _, id := range ids {
		system.Logger.Debug("forgot host key %s", id)
	}
	return Error(nil, system, cmd, c, args)
}

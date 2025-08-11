package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/go-flags"
)

type InstancesApplyCmd struct {
	ClusterName        string                   `short:"n" long:"cluster-name" description:"Name of the cluster to apply the state to" default:"mydc"`
	Count              int                      `short:"c" long:"count" description:"Desired number of instances to have in the cluster" default:"1"`
	Owner              string                   `short:"o" long:"owner" description:"Owner of the instances"`
	Tags               []string                 `short:"t" long:"tag" description:"Tags to add to the instances, format: k=v"`
	Description        string                   `short:"d" long:"description" description:"Description of the instances"`
	TerminateOnStop    bool                     `short:"T" long:"terminate-on-stop" description:"Terminate the instances when they are stopped"`
	ParallelSSHThreads int                      `short:"p" long:"parallel-ssh-threads" description:"Number of parallel SSH threads to use for the instances" default:"10"`
	SSHKeyName         string                   `short:"k" long:"ssh-key-name" description:"Name of a custom SSH key to use for the instances"`
	Hooks              InstancesApplyCmdHooks   `group:"Hooks" description:"hooks" namespace:"hooks"`
	AWS                InstancesCreateCmdAws    `group:"AWS" description:"backend-aws" namespace:"aws"`
	GCP                InstancesCreateCmdGcp    `group:"GCP" description:"backend-gcp" namespace:"gcp"`
	Docker             InstancesCreateCmdDocker `group:"Docker" description:"backend-docker" namespace:"docker"`
	NoInstallExpiry    bool                     `long:"no-install-expiry" description:"Do not install the expiry system, even if instance expiry is set"`
	Force              bool                     `long:"force" description:"Do not ask for confirmation when destroying instances"`
	DryRun             bool                     `long:"dry-run" description:"Dry run, print what would be done but don't do it"`
	Help               HelpCmd                  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type InstancesApplyCmdHooks struct {
	BeforeEachCreate flags.Filename `short:"B" long:"before-each-create" description:"Path to a command or script to run before each instance is created if cluster does not exist"`
	BeforeEachGrow   flags.Filename `short:"G" long:"before-each-grow" description:"Path to a command or script to run before each instance is added when growing the cluster"`
	BeforeEachShrink flags.Filename `short:"S" long:"before-each-shrink" description:"Path to a command or script to run before each instance is removed when shrinking the cluster"`
	Noop             flags.Filename `short:"N" long:"noop" description:"Path to a command or script to run if cluster is already at the desired size"`
	OutputToStdout   bool           `long:"output-to-stdout" description:"Output the output of the hooks to stdout"`
}

func (c *InstancesApplyCmd) Execute(args []string) error {
	cmd := []string{"instances", "grow"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.Apply(system, system.Backend.GetInventory(), args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	err = UpdateDiskCache(system)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *InstancesApplyCmd) Apply(system *System, inventory *backends.Inventory, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"instances", "apply"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	if c.Count < 0 {
		return errors.New("count must be at least 0")
	}

	cluster := inventory.Instances.WithClusterName(c.ClusterName)
	action := ""
	if cluster.Count() == 0 && c.Count == 0 {
		action = "noop"
	} else if cluster.Count() == 0 && c.Count > 0 {
		action = "create"
	} else if cluster.Count() < c.Count {
		action = "grow"
	} else if cluster.Count() == c.Count {
		action = "noop"
	} else {
		action = "shrink"
	}

	switch action {
	case "create":
		system.Logger.Info("Creating cluster %s with %d instances", c.ClusterName, c.Count)
		return c.create(system, inventory, args)
	case "grow":
		system.Logger.Info("Growing cluster %s by %d instances", c.ClusterName, c.Count-cluster.Count())
		return c.grow(system, inventory, args, c.Count-cluster.Count())
	case "noop":
		system.Logger.Info("Cluster %s is already at the desired size of %d instances", c.ClusterName, c.Count)
		err := c.runHook(system, c.Hooks.Noop)
		if err != nil {
			return err
		}
		return nil
	case "shrink":
		system.Logger.Info("Shrinking cluster %s by %d instances", c.ClusterName, cluster.Count()-c.Count)
		return c.shrink(system, inventory, args, cluster, cluster.Count()-c.Count)
	}
	return nil
}

func (c *InstancesApplyCmd) create(system *System, inventory *backends.Inventory, args []string) error {
	create := &InstancesCreateCmd{
		ClusterName:        c.ClusterName,
		Count:              c.Count,
		Name:               "",
		Owner:              c.Owner,
		Tags:               c.Tags,
		Description:        c.Description,
		TerminateOnStop:    c.TerminateOnStop,
		ParallelSSHThreads: c.ParallelSSHThreads,
		SSHKeyName:         c.SSHKeyName,
		AWS:                c.AWS,
		GCP:                c.GCP,
		Docker:             c.Docker,
		NoInstallExpiry:    c.NoInstallExpiry,
		DryRun:             c.DryRun,
		Help:               c.Help,
	}
	if c.Hooks.BeforeEachCreate == "" {
		_, err := create.CreateInstances(system, inventory, args, "create")
		if err != nil {
			return err
		}
	}
	create.Count = 1
	for i := 0; i < c.Count; i++ {
		a := "grow"
		if i == 0 {
			a = "create"
		}
		err := c.runHook(system, c.Hooks.BeforeEachCreate)
		if err != nil {
			return err
		}
		_, err = create.CreateInstances(system, inventory, args, a)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *InstancesApplyCmd) grow(system *System, inventory *backends.Inventory, args []string, count int) error {
	create := &InstancesCreateCmd{
		ClusterName:        c.ClusterName,
		Count:              c.Count,
		Name:               "",
		Owner:              c.Owner,
		Tags:               c.Tags,
		Description:        c.Description,
		TerminateOnStop:    c.TerminateOnStop,
		ParallelSSHThreads: c.ParallelSSHThreads,
		SSHKeyName:         c.SSHKeyName,
		AWS:                c.AWS,
		GCP:                c.GCP,
		Docker:             c.Docker,
		NoInstallExpiry:    c.NoInstallExpiry,
		DryRun:             c.DryRun,
		Help:               c.Help,
	}
	if c.Hooks.BeforeEachGrow == "" {
		_, err := create.CreateInstances(system, inventory, args, "grow")
		if err != nil {
			return err
		}
	}
	create.Count = 1
	for i := 0; i < count; i++ {
		err := c.runHook(system, c.Hooks.BeforeEachGrow)
		if err != nil {
			return err
		}
		_, err = create.CreateInstances(system, inventory, args, "grow")
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *InstancesApplyCmd) shrink(system *System, inventory *backends.Inventory, args []string, cluster backends.Instances, count int) error {
	var nodes []int
	for _, node := range cluster.Describe() {
		nodes = append(nodes, node.NodeNo)
	}
	sort.Ints(nodes)
	nodes = nodes[len(nodes)-count:]
	nodesStr := []string{}
	for _, node := range nodes {
		nodesStr = append(nodesStr, strconv.Itoa(node))
	}

	destroy := &InstancesDestroyCmd{
		DryRun: c.DryRun,
		Force:  c.Force,
		NoWait: false,
		Filters: InstancesListFilter{
			ClusterName: c.ClusterName,
			NodeNo:      strings.Join(nodesStr, ","),
		},
		Help: c.Help,
	}
	if c.Hooks.BeforeEachShrink == "" {
		_, err := destroy.DestroyInstances(system, inventory, args)
		if err != nil {
			return err
		}
	}
	for i := 0; i < count; i++ {
		destroy.Filters.NodeNo = nodesStr[i]
		err := c.runHook(system, c.Hooks.BeforeEachShrink)
		if err != nil {
			return err
		}
		_, err = destroy.DestroyInstances(system, inventory, args)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *InstancesApplyCmd) runHook(system *System, hook flags.Filename) error {
	if c.DryRun {
		system.Logger.Info("DRY-RUN: Would run hook %s", string(hook))
		return nil
	}
	system.Logger.Info("Running hook %s", string(hook))
	cmd := exec.Command("bash", "-c", string(hook))
	var buf bytes.Buffer
	if c.Hooks.OutputToStdout {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout
	} else {
		cmd.Stdout = &buf
		cmd.Stderr = &buf
	}
	if err := cmd.Run(); err != nil {
		return errors.Join(err, errors.New(buf.String()))
	}
	return nil
}

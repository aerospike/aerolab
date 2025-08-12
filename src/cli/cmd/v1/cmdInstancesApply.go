package cmd

import (
	"bytes"
	"errors"
	"fmt"
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
	Help               InstancesApplyCmdHelp    `command:"help" subcommands-optional:"true" description:"Print help"`
}

type InstancesApplyCmdHelp struct {
	AllBackends bool `long:"all" description:"Show help for all backends, not just the selected one"`
}

func (c *InstancesApplyCmdHelp) Execute(args []string) error {
	PrintHelp(c.AllBackends, "Each hook can be specified multiple times, and will be run in the order specified.\n\nCreate/Grow after- hooks and all shrink hooks the following environment variables available:\n\n  CLUSTER_NAME - the name of the cluster the action is running on\n\n  NODE_NO - the node number(s) of the instance(s) the action is running on, comma separated\n\n")
	return nil
}

type InstancesApplyCmdHooks struct {
	BeforeAllCreate  []flags.Filename `long:"before-all-create" description:"Path to a command or script to run before all instances are created if cluster does not exist"`
	AfterAllCreate   []flags.Filename `long:"after-all-create" description:"Path to a command or script to run after all instances are created if cluster does not exist"`
	BeforeEachCreate []flags.Filename `long:"before-each-create" description:"Path to a command or script to run before each instance is created if cluster does not exist"`
	AfterEachCreate  []flags.Filename `long:"after-each-create" description:"Path to a command or script to run after each instance is created if cluster does not exist"`
	BeforeAllGrow    []flags.Filename `long:"before-all-grow" description:"Path to a command or script to run before all instances are added when growing the cluster"`
	AfterAllGrow     []flags.Filename `long:"after-all-grow" description:"Path to a command or script to run after all instances are added when growing the cluster"`
	BeforeEachGrow   []flags.Filename `long:"before-each-grow" description:"Path to a command or script to run before each instance is added when growing the cluster"`
	AfterEachGrow    []flags.Filename `long:"after-each-grow" description:"Path to a command or script to run after each instance is added when growing the cluster"`
	BeforeAllShrink  []flags.Filename `long:"before-all-shrink" description:"Path to a command or script to run before all instances are removed when shrinking the cluster"`
	AfterAllShrink   []flags.Filename `long:"after-all-shrink" description:"Path to a command or script to run after all instances are removed when shrinking the cluster"`
	BeforeEachShrink []flags.Filename `long:"before-each-shrink" description:"Path to a command or script to run before each instance is removed when shrinking the cluster"`
	AfterEachShrink  []flags.Filename `long:"after-each-shrink" description:"Path to a command or script to run after each instance is removed when shrinking the cluster"`
	Noop             []flags.Filename `long:"noop" description:"Path to a command or script to run if cluster is already at the desired size"`
	OutputToStdout   bool             `long:"output-to-stdout" description:"Output the output of the hooks to stdout"`
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
		for _, hook := range c.Hooks.Noop {
			err := c.runHook(system, hook, map[string]string{})
			if err != nil {
				return err
			}
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
	}
	for _, hook := range c.Hooks.BeforeAllCreate {
		err := c.runHook(system, hook, map[string]string{})
		if err != nil {
			return err
		}
	}
	if len(c.Hooks.BeforeEachCreate) == 0 && len(c.Hooks.AfterEachCreate) == 0 {
		re, err := create.CreateInstances(system, inventory, args, "create")
		if err != nil {
			return err
		}
		nodes := []int{}
		for _, instance := range re.Describe() {
			nodes = append(nodes, instance.NodeNo)
		}
		sort.Ints(nodes)
		nodesStr := []string{}
		for _, node := range nodes {
			nodesStr = append(nodesStr, strconv.Itoa(node))
		}
		for _, hook := range c.Hooks.AfterAllCreate {
			err := c.runHook(system, hook, map[string]string{
				"CLUSTER_NAME": c.ClusterName,
				"NODE_NO":      strings.Join(nodesStr, ","),
			})
			if err != nil {
				return err
			}
		}
		return nil
	}
	create.Count = 1
	nodes := []string{}
	for i := 0; i < c.Count; i++ {
		a := "grow"
		if i == 0 {
			a = "create"
		}
		for _, hook := range c.Hooks.BeforeEachCreate {
			err := c.runHook(system, hook, map[string]string{})
			if err != nil {
				return err
			}
		}
		re, err := create.CreateInstances(system, inventory, args, a)
		if err != nil {
			return err
		}
		nodes = append(nodes, strconv.Itoa(re.Describe()[0].NodeNo))
		for _, hook := range c.Hooks.AfterEachCreate {
			err := c.runHook(system, hook, map[string]string{
				"CLUSTER_NAME": c.ClusterName,
				"NODE_NO":      nodes[i],
			})
			if err != nil {
				return err
			}
		}
	}
	for _, hook := range c.Hooks.AfterAllCreate {
		err := c.runHook(system, hook, map[string]string{
			"CLUSTER_NAME": c.ClusterName,
			"NODE_NO":      strings.Join(nodes, ","),
		})
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
	}
	for _, hook := range c.Hooks.BeforeAllGrow {
		err := c.runHook(system, hook, map[string]string{})
		if err != nil {
			return err
		}
	}
	if len(c.Hooks.BeforeEachGrow) == 0 && len(c.Hooks.AfterEachGrow) == 0 {
		re, err := create.CreateInstances(system, inventory, args, "grow")
		if err != nil {
			return err
		}
		nodes := []int{}
		for _, instance := range re.Describe() {
			nodes = append(nodes, instance.NodeNo)
		}
		sort.Ints(nodes)
		nodesStr := []string{}
		for _, node := range nodes {
			nodesStr = append(nodesStr, strconv.Itoa(node))
		}
		for _, hook := range c.Hooks.AfterAllGrow {
			err := c.runHook(system, hook, map[string]string{
				"CLUSTER_NAME": c.ClusterName,
				"NODE_NO":      strings.Join(nodesStr, ","),
			})
			if err != nil {
				return err
			}
		}
		return nil
	}
	create.Count = 1
	nodes := []string{}
	for i := 0; i < count; i++ {
		for _, hook := range c.Hooks.BeforeEachGrow {
			err := c.runHook(system, hook, map[string]string{})
			if err != nil {
				return err
			}
		}
		re, err := create.CreateInstances(system, inventory, args, "grow")
		if err != nil {
			return err
		}
		nodes = append(nodes, strconv.Itoa(re.Describe()[0].NodeNo))
		for _, hook := range c.Hooks.AfterEachGrow {
			err := c.runHook(system, hook, map[string]string{
				"CLUSTER_NAME": c.ClusterName,
				"NODE_NO":      nodes[i],
			})
			if err != nil {
				return err
			}
		}
	}
	for _, hook := range c.Hooks.AfterAllGrow {
		err := c.runHook(system, hook, map[string]string{
			"CLUSTER_NAME": c.ClusterName,
			"NODE_NO":      strings.Join(nodes, ","),
		})
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
	}
	for _, hook := range c.Hooks.BeforeAllShrink {
		err := c.runHook(system, hook, map[string]string{
			"CLUSTER_NAME": c.ClusterName,
			"NODE_NO":      strings.Join(nodesStr, ","),
		})
		if err != nil {
			return err
		}
	}
	if len(c.Hooks.BeforeEachShrink) == 0 && len(c.Hooks.AfterEachShrink) == 0 {
		_, err := destroy.DestroyInstances(system, inventory, args)
		if err != nil {
			return err
		}
		for _, hook := range c.Hooks.AfterAllShrink {
			err := c.runHook(system, hook, map[string]string{
				"CLUSTER_NAME": c.ClusterName,
				"NODE_NO":      strings.Join(nodesStr, ","),
			})
			if err != nil {
				return err
			}
		}
		return nil
	}
	for i := 0; i < count; i++ {
		destroy.Filters.NodeNo = nodesStr[i]
		for _, hook := range c.Hooks.BeforeEachShrink {
			err := c.runHook(system, hook, map[string]string{
				"CLUSTER_NAME": c.ClusterName,
				"NODE_NO":      nodesStr[i],
			})
			if err != nil {
				return err
			}
		}
		_, err := destroy.DestroyInstances(system, inventory, args)
		if err != nil {
			return err
		}
		for _, hook := range c.Hooks.AfterEachShrink {
			err := c.runHook(system, hook, map[string]string{
				"CLUSTER_NAME": c.ClusterName,
				"NODE_NO":      nodesStr[i],
			})
			if err != nil {
				return err
			}
		}
	}
	for _, hook := range c.Hooks.AfterAllShrink {
		err := c.runHook(system, hook, map[string]string{
			"CLUSTER_NAME": c.ClusterName,
			"NODE_NO":      strings.Join(nodesStr, ","),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *InstancesApplyCmd) runHook(system *System, hook flags.Filename, env map[string]string) error {
	if c.DryRun {
		system.Logger.Info("DRY-RUN: Would run hook %s", string(hook))
		return nil
	}
	system.Logger.Info("Running hook %s", string(hook))
	cmd := exec.Command("bash", "-c", string(hook))
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
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

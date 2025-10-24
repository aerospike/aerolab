package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/bestmethod/inslice"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
	"github.com/rglonek/logger"
)

type RosterApplyCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Namespace   string          `short:"m" long:"namespace" description:"Namespace name" default:"test"`
	Roster      string          `short:"r" long:"roster" description:"set this to specify customer roster; leave empty to apply observed nodes automatically" default:""`
	NoRecluster bool            `short:"c" long:"no-recluster" description:"if set, will not apply recluster command after roster-set"`
	Quiet       bool            `long:"quiet" description:"Do not print the roster after applying"`
	Threads     int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *RosterApplyCmd) Execute(args []string) error {
	cmd := []string{"roster", "apply"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ApplyRoster(system, system.Backend.GetInventory(), system.Logger, args, "apply")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ApplyRoster applies a roster to the cluster namespace.
// This function performs the following operations:
// 1. Gets the list of nodes in the cluster
// 2. If no roster is specified, discovers observed nodes from all nodes
// 3. Applies the roster to all nodes using asinfo roster-set command
// 4. Optionally triggers recluster command
// 5. Optionally shows the roster after applying
//
// Parameters:
//   - system: The system instance for logging and backend operations
//   - inventory: The backend inventory containing cluster information
//   - logger: Logger instance for output
//   - args: Command line arguments
//   - action: The action being performed
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *RosterApplyCmd) ApplyRoster(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"roster", action}, c, args...)
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
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			err := c.ApplyRoster(system, inventory, logger, args, action)
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

	// Get node numbers
	nodes := []int{}
	for _, instance := range cluster.Describe() {
		nodes = append(nodes, instance.NodeNo)
	}

	// Filter nodes if specified
	if c.Nodes.String() != "" {
		filteredNodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return err
		}
		for _, node := range filteredNodes {
			if !inslice.HasInt(nodes, node) {
				return fmt.Errorf("node %d does not exist in cluster", node)
			}
		}
		nodes = filteredNodes
	}

	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		logger.Info("No running instances found for cluster %s", c.ClusterName.String())
		return nil
	}

	logger.Info("Applying roster to %d nodes", len(nodes))

	newRoster := c.Roster
	rf := 0

	// If no roster specified, discover observed nodes
	if newRoster == "" {
		foundNodes := []string{}
		if c.Threads == 1 || len(nodes) == 1 {
			for _, node := range nodes {
				instance := cluster.WithNodeNo(node).Describe()[0]
				observedNodes := c.findNodesOnInstance(instance, logger)
				if observedNodes == nil {
					continue
				}
				if observedNodes.replicationFactor > rf {
					rf = observedNodes.replicationFactor
				}
				for _, on := range observedNodes.nodes {
					if !inslice.HasString(foundNodes, on) {
						foundNodes = append(foundNodes, on)
					}
				}
			}
		} else {
			parallel := make(chan int, c.Threads)
			wait := new(sync.WaitGroup)
			observedNodes := make(chan *rosterNodes, len(nodes))
			for _, node := range nodes {
				instance := cluster.WithNodeNo(node).Describe()[0]
				parallel <- 1
				wait.Add(1)
				go c.findNodesOnInstanceParallel(instance, parallel, wait, observedNodes, logger)
			}
			wait.Wait()
			close(observedNodes)
			for ona := range observedNodes {
				if ona == nil {
					continue
				}
				if ona.replicationFactor > rf {
					rf = ona.replicationFactor
				}
				for _, on := range ona.nodes {
					if !inslice.HasString(foundNodes, on) {
						foundNodes = append(foundNodes, on)
					}
				}
			}
		}
		if len(foundNodes) == 0 || inslice.HasString(foundNodes, "null") {
			return errors.New("found at least one node which thinks the observed list is 'null' or failed to find any nodes in roster")
		}
		if rf > len(foundNodes) {
			logger.Warn("Found %d nodes while replication-factor is %d. This will fail to satisfy strong-consistency requirements!", len(foundNodes), rf)
		}
		newRoster = strings.Join(foundNodes, ",")
	}

	// Apply roster to all nodes
	rosterCmd := []string{"asinfo", "-v", "roster-set:namespace=" + c.Namespace + ";nodes=" + newRoster}
	// Escape semicolon for non-docker backends
	if system.Opts.Config.Backend.Type != "docker" {
		rosterCmd = []string{"asinfo", "-v", "roster-set:namespace=" + c.Namespace + "\\;nodes=" + newRoster}
	}

	if c.Threads == 1 || len(nodes) == 1 {
		c.applyRosterToNodes(cluster.Describe(), nodes, rosterCmd, logger)
	} else {
		parallel := make(chan int, c.Threads)
		wait := new(sync.WaitGroup)
		for _, node := range nodes {
			instance := cluster.WithNodeNo(node).Describe()[0]
			parallel <- 1
			wait.Add(1)
			go c.applyRosterToNodeParallel(instance, rosterCmd, parallel, wait, logger)
		}
		wait.Wait()
	}

	if c.NoRecluster {
		logger.Info("Done. Roster applied, did not recluster!")
		return nil
	}

	// Trigger recluster
	logger.Info("Triggering recluster")
	reclusterCmd := []string{"asinfo", "-v", "recluster:namespace=" + c.Namespace}
	if c.Threads == 1 || len(nodes) == 1 {
		c.applyReclusterToNodes(cluster.Describe(), nodes, reclusterCmd, logger)
	} else {
		parallel := make(chan int, c.Threads)
		wait := new(sync.WaitGroup)
		for _, node := range nodes {
			instance := cluster.WithNodeNo(node).Describe()[0]
			parallel <- 1
			wait.Add(1)
			go c.applyReclusterToNodeParallel(instance, reclusterCmd, parallel, wait, logger)
		}
		wait.Wait()
	}

	// Show roster if not quiet
	if !c.Quiet {
		showCmd := &RosterShowCmd{
			ClusterName: c.ClusterName,
			Nodes:       c.Nodes,
			Namespace:   c.Namespace,
			Threads:     c.Threads,
		}
		err := showCmd.ShowRoster(system, inventory, logger, args, "show")
		if err != nil {
			return err
		}
	}

	return nil
}

type rosterNodes struct {
	nodes             []string
	replicationFactor int
}

// findNodesOnInstance finds observed nodes on a single instance.
func (c *RosterApplyCmd) findNodesOnInstance(instance *backends.Instance, logger *logger.Logger) *rosterNodes {
	output := instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"asinfo", "-v", "roster:namespace=" + c.Namespace},
			Stdin:          nil,
			Stdout:         nil,
			Stderr:         nil,
			SessionTimeout: time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if output.Output.Err != nil {
		logger.Warn("ERROR skipping node, running asinfo on node %d: %s: %s", instance.NodeNo, output.Output.Err, string(output.Output.Stdout))
		return nil
	}

	observedNodesSplit := strings.Split(strings.Trim(string(output.Output.Stdout), "\t\r\n "), ":observed_nodes=")
	if len(observedNodesSplit) < 2 {
		logger.Warn("ERROR skipping node, running asinfo on node %d: %s", instance.NodeNo, string(output.Output.Stdout))
		return nil
	}

	// Get replication factor from config file
	rf := 0
	conf, err := instance.GetSftpConfig("root")
	if err == nil {
		client, err := sshexec.NewSftp(conf)
		if err == nil {
			defer client.Close()
			var buf bytes.Buffer
			err = client.ReadFile(&sshexec.FileReader{
				SourcePath:  "/etc/aerospike/aerospike.conf",
				Destination: &buf,
			})
			if err == nil {
				ac, err := aeroconf.Parse(&buf)
				if err == nil {
					ac = ac.Stanza("namespace " + c.Namespace)
					if ac != nil {
						vals, err := ac.GetValues("replication-factor")
						if err == nil && len(vals) > 0 {
							rf, _ = strconv.Atoi(*vals[0])
						}
					}
				}
			}
		}
	}

	return &rosterNodes{
		nodes:             strings.Split(observedNodesSplit[1], ","),
		replicationFactor: rf,
	}
}

// findNodesOnInstanceParallel finds observed nodes on a single instance in parallel.
func (c *RosterApplyCmd) findNodesOnInstanceParallel(instance *backends.Instance, parallel chan int, wait *sync.WaitGroup, ob chan *rosterNodes, logger *logger.Logger) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	on := c.findNodesOnInstance(instance, logger)
	ob <- on
}

// applyRosterToNodes applies roster to multiple nodes.
func (c *RosterApplyCmd) applyRosterToNodes(cluster backends.InstanceList, nodes []int, rosterCmd []string, logger *logger.Logger) {
	for _, node := range nodes {
		instance := cluster.WithNodeNo(node).Describe()[0]
		c.applyRosterToNode(instance, rosterCmd, logger)
	}
}

// applyRosterToNodeParallel applies roster to a single node in parallel.
func (c *RosterApplyCmd) applyRosterToNodeParallel(instance *backends.Instance, rosterCmd []string, parallel chan int, wait *sync.WaitGroup, logger *logger.Logger) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	c.applyRosterToNode(instance, rosterCmd, logger)
}

// applyRosterToNode applies roster to a single node.
func (c *RosterApplyCmd) applyRosterToNode(instance *backends.Instance, rosterCmd []string, logger *logger.Logger) {
	output := instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        rosterCmd,
			Stdin:          nil,
			Stdout:         nil,
			Stderr:         nil,
			SessionTimeout: time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if strings.Contains(string(output.Output.Stdout), "ERROR") {
		logger.Warn("ERROR: %s", string(output.Output.Stdout))
	}
	if output.Output.Err != nil {
		logger.Warn("WARNING: could not apply roster to %d: %s: %s", instance.NodeNo, output.Output.Err, string(output.Output.Stdout))
	}
}

// applyReclusterToNodes applies recluster to multiple nodes.
func (c *RosterApplyCmd) applyReclusterToNodes(cluster backends.InstanceList, nodes []int, reclusterCmd []string, logger *logger.Logger) {
	for _, node := range nodes {
		instance := cluster.WithNodeNo(node).Describe()[0]
		c.applyReclusterToNode(instance, reclusterCmd, logger)
	}
}

// applyReclusterToNodeParallel applies recluster to a single node in parallel.
func (c *RosterApplyCmd) applyReclusterToNodeParallel(instance *backends.Instance, reclusterCmd []string, parallel chan int, wait *sync.WaitGroup, logger *logger.Logger) {
	defer func() {
		<-parallel
		wait.Done()
	}()
	c.applyReclusterToNode(instance, reclusterCmd, logger)
}

// applyReclusterToNode applies recluster to a single node.
func (c *RosterApplyCmd) applyReclusterToNode(instance *backends.Instance, reclusterCmd []string, logger *logger.Logger) {
	output := instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        reclusterCmd,
			Stdin:          nil,
			Stdout:         nil,
			Stderr:         nil,
			SessionTimeout: time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if output.Output.Err != nil {
		logger.Warn("WARNING: could not send recluster to node %d: %s: %s", instance.NodeNo, output.Output.Err, string(output.Output.Stdout))
	}
}

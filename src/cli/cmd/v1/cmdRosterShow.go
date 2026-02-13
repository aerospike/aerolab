package cmd

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/bestmethod/inslice"
	"github.com/rglonek/logger"
)

type RosterShowCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Namespace   string          `short:"m" long:"namespace" description:"Namespace name" default:"test"`
	Threads     int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	MaxRetries  int             `long:"max-retries" description:"Maximum number of retries for transient SSH/SFTP failures" default:"1" simplemode:"false"`
	RetrySleep  time.Duration   `long:"retry-sleep" description:"Sleep duration between retries" default:"5s" simplemode:"false"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *RosterShowCmd) Execute(args []string) error {
	cmd := []string{"roster", "show"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ShowRoster(system, system.Backend.GetInventory(), system.Logger, args, "show")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ShowRoster displays the roster information for the specified cluster and namespace.
// This function performs the following operations:
// 1. Gets the list of nodes in the cluster
// 2. Runs roster command on all nodes to show current roster state
// 3. Displays the roster information for each node
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
func (c *RosterShowCmd) ShowRoster(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) error {
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
	var cluster backends.Instances
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		// Pre-validation loop - check ALL clusters exist before processing any
		for _, clusterName := range clusters {
			if inventory.Instances.WithClusterName(clusterName).WithState(backends.LifeCycleStateRunning).Count() == 0 {
				return fmt.Errorf("cluster %s not found", clusterName)
			}
		}
		// Processing loop - actually perform the operation
		for _, clusterName := range clusters {
			c.ClusterName = TypeClusterName(clusterName)
			err := c.ShowRoster(system, inventory, logger, args, action)
			if err != nil {
				return err
			}
		}
		return nil
	} else {
		var err error
		cluster, err = c.ClusterName.GetInstanceList(inventory, backends.LifeCycleStateRunning)
		if err != nil {
			return err
		}
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

	// Filter nodes to only include those that are actually running
	runningNodeNos := []int{}
	for _, instance := range cluster.Describe() {
		runningNodeNos = append(runningNodeNos, instance.NodeNo)
	}
	// Update nodes list to only include running nodes
	filteredRunningNodes := []int{}
	for _, node := range nodes {
		if inslice.HasInt(runningNodeNos, node) {
			filteredRunningNodes = append(filteredRunningNodes, node)
		}
	}
	nodes = filteredRunningNodes
	if len(nodes) == 0 {
		logger.Info("No running nodes found to show roster for")
		return nil
	}

	logger.Info("Showing roster for %d nodes", len(nodes))

	if c.Threads == 1 || len(nodes) == 1 {
		for _, node := range nodes {
			instance := cluster.WithNodeNo(node).Describe()[0]
			c.showRosterOnNode(instance, logger)
		}
	} else {
		var wg sync.WaitGroup
		parallel := make(chan int, c.Threads)
		for _, node := range nodes {
			instance := cluster.WithNodeNo(node).Describe()[0]
			parallel <- 1
			wg.Add(1)
			go c.showRosterOnNodeParallel(instance, parallel, &wg, logger)
		}
		wg.Wait()
	}

	return nil
}

// showRosterOnNodeParallel shows roster on a single node in parallel.
func (c *RosterShowCmd) showRosterOnNodeParallel(instance *backends.Instance, parallel chan int, wg *sync.WaitGroup, logger *logger.Logger) {
	defer func() {
		<-parallel
		wg.Done()
	}()
	c.showRosterOnNode(instance, logger)
}

// showRosterOnNode shows roster information on a single node.
func (c *RosterShowCmd) showRosterOnNode(instance *backends.Instance, logger *logger.Logger) {
	output := instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"asinfo", "-v", "roster:namespace=" + c.Namespace},
			SessionTimeout: time.Minute,
			Env:            []*sshexec.Env{},
			Terminal:       false,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
		MaxRetries:      c.MaxRetries,
		RetrySleep:      c.RetrySleep,
	})

	if output.Output.Err != nil {
		fmt.Printf("%s:%d ERROR %s: %s\n", instance.ClusterName, instance.NodeNo, output.Output.Err, strings.Trim(strings.ReplaceAll(string(output.Output.Stdout), "\n", "; "), "\t\r\n "))
	} else {
		fmt.Printf("%s:%d ROSTER %s\n", instance.ClusterName, instance.NodeNo, strings.Trim(string(output.Output.Stdout), "\t\r\n "))
	}
}

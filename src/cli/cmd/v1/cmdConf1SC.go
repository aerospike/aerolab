package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
	"github.com/rglonek/logger"
)

type ConfSCCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Namespace   string          `short:"m" long:"namespace" description:"Namespace to change" default:"test"`
	Path        string          `short:"p" long:"path" description:"Path to aerospike.conf" default:"/etc/aerospike/aerospike.conf"`
	Force       bool            `short:"f" long:"force" description:"If set, will zero out the devices even if strong-consistency was already configured"`
	Racks       int             `short:"r" long:"racks" description:"If rack-aware feature is required, set this to the number of racks you want to divide the cluster into"`
	WithDisks   bool            `short:"d" long:"with-disks" description:"If set, will attempt to configure device storage engine for the namespace, using all available devices"`
	Verbose     bool            `short:"v" long:"verbose" description:"Enable verbose logging"`
	Threads     int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfSCCmd) Execute(args []string) error {
	cmd := []string{"conf", "sc"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	instances, err := c.ConfigureStrongConsistency(system, system.Backend.GetInventory(), system.Logger, args, "sc")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Configured strong consistency on %d instances", instances.Count())
	for _, i := range instances.Describe() {
		system.Logger.Debug("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ConfigureStrongConsistency configures the cluster to use strong-consistency with roster and optional RF changes.
// This function performs the following operations:
// 1. Stops aerospike on all nodes
// 2. Optionally partitions and configures disks if WithDisks is set
// 3. Patches aerospike.conf to enable strong consistency
// 4. Cold starts aerospike
// 5. Waits for cluster to be stable
// 6. Applies roster (if available)
//
// Parameters:
//   - system: The system instance for logging and backend operations
//   - inventory: The backend inventory containing cluster information
//   - logger: Logger instance for output
//   - args: Command line arguments
//   - action: The action being performed
//
// Returns:
//   - backends.InstanceList: List of instances that were processed
//   - error: nil on success, or an error describing what failed
func (c *ConfSCCmd) ConfigureStrongConsistency(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"conf", action}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	if c.ClusterName.String() == "" {
		return nil, fmt.Errorf("cluster name is required")
	}
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		var instances backends.InstanceList
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			inst, err := c.ConfigureStrongConsistency(system, inventory, logger, args, action)
			if err != nil {
				return nil, err
			}
			instances = append(instances, inst...)
		}
		return instances, nil
	}

	cluster := inventory.Instances.WithClusterName(c.ClusterName.String())
	if cluster == nil {
		return nil, fmt.Errorf("cluster %s not found", c.ClusterName.String())
	}

	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		logger.Info("No running instances found for cluster %s", c.ClusterName.String())
		return nil, nil
	}

	logger.Info("Configuring strong consistency on %d nodes", cluster.Count())

	// Step 1: Stop aerospike
	logger.Info("Stopping aerospike")
	stopCmd := &AerospikeStopCmd{
		ClusterName: c.ClusterName,
		Threads:     c.Threads,
	}
	_, err := stopCmd.StopAerospike(system, inventory, logger, args, "stop")
	if err != nil {
		return nil, fmt.Errorf("failed to stop aerospike: %w", err)
	}

	// Step 2: Partition disks if requested
	if c.WithDisks {
		logger.Info("Partitioning all available devices")
		partitionCreateCmd := &ClusterPartitionCreateCmd{
			ClusterName:     c.ClusterName,
			Partitions:      "24,24,24,24",
			ParallelThreads: c.Threads,
		}
		_, err = partitionCreateCmd.PartitionCreateCluster(system, inventory, args, nil, nil, nil, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create partitions: %w", err)
		}

		partitionConfCmd := &ClusterPartitionConfCmd{
			ClusterName:      c.ClusterName,
			FilterPartitions: TypeFilterRange("1-4"),
			ConfDest:         "device",
			Namespace:        c.Namespace,
			ParallelThreads:  c.Threads,
		}
		_, err = partitionConfCmd.PartitionConfCluster(system, inventory, args, nil, nil, nil, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to configure partitions: %w", err)
		}
	}

	// Step 3: Patch aerospike.conf
	logger.Info("Patching aerospike.conf")
	err = c.patchAerospikeConfig(cluster.Describe(), logger)
	if err != nil {
		return nil, fmt.Errorf("failed to patch aerospike.conf: %w", err)
	}

	// Step 4: Cold start aerospike
	logger.Info("Cold-starting aerospike")
	coldStartCmd := &AerospikeColdStartCmd{
		ClusterName: c.ClusterName,
		Threads:     c.Threads,
	}
	_, err = coldStartCmd.ColdStartAerospike(system, inventory, logger, args, "cold-start")
	if err != nil {
		return nil, fmt.Errorf("failed to cold start aerospike: %w", err)
	}

	// Step 5: Wait for cluster to be stable
	logger.Info("Waiting for cluster to be stable")
	isStableCmd := &AerospikeIsStableCmd{
		ClusterName:      c.ClusterName,
		Namespace:        c.Namespace,
		Wait:             true,
		IgnoreMigrations: true,
		Verbose:          c.Verbose,
		Threads:          c.Threads,
	}
	stable, err := isStableCmd.IsStable(system, inventory, logger, args)
	if err != nil {
		return nil, fmt.Errorf("failed to check cluster stability: %w", err)
	}
	if !stable {
		return nil, fmt.Errorf("cluster did not become stable")
	}

	// Step 6: Apply roster
	logger.Info("Applying roster")
	rosterApplyCmd := &RosterApplyCmd{
		ClusterName: c.ClusterName,
		Namespace:   c.Namespace,
		Threads:     c.Threads,
		Quiet:       true,
	}
	err = rosterApplyCmd.ApplyRoster(system, inventory, logger, args, "apply")
	if err != nil {
		return nil, fmt.Errorf("failed to apply roster: %w", err)
	}

	return cluster.Describe(), nil
}

// patchAerospikeConfig patches the aerospike.conf file on all instances to enable strong consistency.
func (c *ConfSCCmd) patchAerospikeConfig(instances backends.InstanceList, logger *logger.Logger) error {
	var hasErr error
	clusterSize := len(instances)
	parallelize.ForEachLimit(instances, c.Threads, func(instance *backends.Instance) {
		err := c.patchInstanceConfig(instance, clusterSize, logger)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", instance.ClusterName, instance.NodeNo, err))
		}
	})
	return hasErr
}

// patchInstanceConfig patches the aerospike.conf file on a single instance.
func (c *ConfSCCmd) patchInstanceConfig(instance *backends.Instance, clusterSize int, logger *logger.Logger) error {
	// Get SFTP configuration
	conf, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	// Create SFTP client
	client, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer client.Close()

	// Read config file
	var buf bytes.Buffer
	err = client.ReadFile(&sshexec.FileReader{
		SourcePath:  c.Path,
		Destination: &buf,
	})
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse configuration
	s, err := aeroconf.Parse(&buf)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Check if namespace exists
	namespaceKey := "namespace " + c.Namespace
	if s.Type(namespaceKey) != aeroconf.ValueStanza {
		return fmt.Errorf("namespace %s not found", c.Namespace)
	}

	changes := false
	x := s.Stanza(namespaceKey)

	// Check and adjust replication factor
	if x.Type("replication-factor") == aeroconf.ValueString {
		vals, err := x.GetValues("replication-factor")
		if err != nil {
			return fmt.Errorf("failed to get replication-factor values: %w", err)
		}
		if len(vals) != 1 {
			return fmt.Errorf("replication-factor parameter error")
		}
		rf, err := strconv.Atoi(*vals[0])
		if err != nil {
			return fmt.Errorf("replication-factor parameter invalid value: %w", err)
		}
		// Get cluster size for RF adjustment
		if rf > clusterSize {
			x.SetValue("replication-factor", strconv.Itoa(clusterSize))
			changes = true
		}
	} else if clusterSize == 1 {
		x.SetValue("replication-factor", "1")
		changes = true
	}

	// Configure strong consistency
	rmFiles := false
	if x.Type("strong-consistency") != aeroconf.ValueString {
		x.SetValue("strong-consistency", "true")
		changes = true
		rmFiles = true
	} else {
		vals, err := x.GetValues("strong-consistency")
		if err != nil {
			return fmt.Errorf("failed to get strong-consistency values: %w", err)
		}
		if len(vals) != 1 {
			return fmt.Errorf("strong-consistency parameter error")
		}
		if *vals[0] != "true" {
			x.SetValue("strong-consistency", "true")
			changes = true
			rmFiles = true
		}
	}

	// Remove storage files if needed
	if rmFiles || c.Force {
		if x.Type("storage-engine device") == aeroconf.ValueStanza {
			if x.Stanza("storage-engine device").Type("file") == aeroconf.ValueString {
				files, err := x.Stanza("storage-engine device").GetValues("file")
				if err != nil {
					return fmt.Errorf("failed to get file values: %w", err)
				}
				cmd := []string{"rm", "-f"}
				for _, file := range files {
					cmd = append(cmd, *file)
				}
				output := instance.Exec(&backends.ExecInput{
					ExecDetail: sshexec.ExecDetail{
						Command:        cmd,
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
					logger.Warn("Failed to remove files on %s:%d: %s", instance.ClusterName, instance.NodeNo, output.Output.Err)
				}
			}
		}
	}

	// Configure rack-id if requested
	if c.Racks > 0 {
		nodesPerRack := int(math.Ceil(float64(clusterSize) / float64(c.Racks)))
		nodeRack := ((instance.NodeNo - 1) / nodesPerRack) + 1
		if x.Type("rack-id") != aeroconf.ValueString {
			x.SetValue("rack-id", strconv.Itoa(nodeRack))
			changes = true
		} else {
			vals, err := x.GetValues("rack-id")
			if err != nil {
				return fmt.Errorf("failed to get rack-id values: %w", err)
			}
			if *vals[0] != strconv.Itoa(nodeRack) {
				x.SetValue("rack-id", strconv.Itoa(nodeRack))
				changes = true
			}
		}
	}

	// Write changes back if any were made
	if changes {
		var newBuf bytes.Buffer
		err = s.Write(&newBuf, "", "    ", true)
		if err != nil {
			return fmt.Errorf("failed to write configuration: %w", err)
		}

		err = client.WriteFile(true, &sshexec.FileWriter{
			DestPath:    c.Path,
			Source:      &newBuf,
			Permissions: 0644,
		})
		if err != nil {
			return fmt.Errorf("failed to write config file: %w", err)
		}
	}

	return nil
}

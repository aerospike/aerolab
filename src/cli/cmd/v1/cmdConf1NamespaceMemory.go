package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
	"github.com/rglonek/logger"
)

type ConfNamespaceMemoryCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	Path        string          `short:"p" long:"path" description:"Path to aerospike.conf on the remote nodes" default:"/etc/aerospike/aerospike.conf"`
	Namespace   string          `short:"m" long:"namespace" description:"Name of the namespace to adjust" default:"test"`
	MemPct      int             `short:"r" long:"mem-pct" description:"The percentage of RAM to use for the namespace memory" default:"50"`
	Threads     int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfNamespaceMemoryCmd) Execute(args []string) error {
	cmd := []string{"conf", "namespace-memory"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	instances, err := c.AdjustNamespaceMemory(system, system.Backend.GetInventory(), system.Logger, args, "namespace-memory")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Adjusted namespace memory on %d instances", instances.Count())
	for _, i := range instances.Describe() {
		system.Logger.Debug("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// AdjustNamespaceMemory adjusts the memory allocation for a namespace based on system RAM percentage.
// This function performs the following operations:
// 1. Gets system memory information from each node
// 2. Calculates the appropriate memory size based on the percentage
// 3. Detects Aerospike version to determine the correct configuration parameter
// 4. Modifies the aerospike.conf file with the new memory settings
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
func (c *ConfNamespaceMemoryCmd) AdjustNamespaceMemory(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
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
			inst, err := c.AdjustNamespaceMemory(system, inventory, logger, args, action)
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

	if c.Nodes.String() != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return nil, err
		}
		cluster = cluster.WithNodeNo(nodes...)
		if cluster.Count() != len(nodes) {
			return nil, fmt.Errorf("some nodes in %s not found", c.Nodes.String())
		}
	}

	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		logger.Info("No running instances found for cluster %s", c.ClusterName.String())
		return nil, nil
	}

	logger.Info("Adjusting namespace memory for %s to %d%% of RAM on %d nodes", c.Namespace, c.MemPct, cluster.Count())

	// Process each instance
	var hasErr error
	parallelize.ForEachLimit(cluster.Describe(), c.Threads, func(instance *backends.Instance) {
		err := c.processInstance(instance, logger)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", instance.ClusterName, instance.NodeNo, err))
		}
	})

	return cluster.Describe(), hasErr
}

// processInstance processes a single instance to adjust namespace memory.
func (c *ConfNamespaceMemoryCmd) processInstance(instance *backends.Instance, logger *logger.Logger) error {
	// Get system memory information
	memSizeGb, err := c.getSystemMemory(instance)
	if err != nil {
		return fmt.Errorf("failed to get system memory: %w", err)
	}

	// Calculate memory size based on percentage
	sysSizeGb := memSizeGb / 1024
	memSizeGb = memSizeGb * c.MemPct / 100 / 1024
	if memSizeGb == 0 {
		return fmt.Errorf("percentage would result in memory size 0")
	}

	// Get Aerospike version
	version, is7, err := c.getAerospikeVersion(instance)
	if err != nil {
		return fmt.Errorf("failed to get Aerospike version: %w", err)
	}

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
	if s.Type(namespaceKey) == aeroconf.ValueNil {
		return fmt.Errorf("namespace %s not found", c.Namespace)
	}

	// Apply memory configuration based on version
	changes := false
	if !is7 {
		logger.Info("Processing NodeVersion %s NodeNumber %d TotalRamGb %d memory-size=%dG", version, instance.NodeNo, sysSizeGb, memSizeGb)
		s.Stanza(namespaceKey).SetValue("memory-size", strconv.Itoa(memSizeGb)+"G")
		changes = true
	} else {
		// For version 7, check storage-engine configuration
		if s.Stanza(namespaceKey).Type("storage-engine memory") != aeroconf.ValueStanza {
			logger.Warn("Skipping NodeVersion %s NodeNumber %d storage-engine is not memory", version, instance.NodeNo)
			return nil
		}
		if s.Stanza(namespaceKey).Stanza("storage-engine memory").Type("device") != aeroconf.ValueNil {
			logger.Warn("Skipping NodeVersion %s NodeNumber %d device backing configured for storage-engine", version, instance.NodeNo)
			return nil
		}
		if s.Stanza(namespaceKey).Stanza("storage-engine memory").Type("file") != aeroconf.ValueNil {
			logger.Warn("Skipping NodeVersion %s NodeNumber %d file backing configured for storage-engine", version, instance.NodeNo)
			return nil
		}
		logger.Info("Processing NodeVersion %s NodeNumber %d TotalRamGb %d data-size=%dG", version, instance.NodeNo, sysSizeGb, memSizeGb)
		s.Stanza(namespaceKey).Stanza("storage-engine memory").SetValue("data-size", strconv.Itoa(memSizeGb)+"G")
		changes = true
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

// getSystemMemory gets the system memory size in bytes from the instance.
func (c *ConfNamespaceMemoryCmd) getSystemMemory(instance *backends.Instance) (int, error) {
	output := instance.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"free", "-b"},
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
		return 0, fmt.Errorf("failed to execute free command: %w", output.Output.Err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(output.Output.Stdout))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "Mem:") {
			continue
		}
		memBytes := strings.Fields(line)
		if len(memBytes) < 2 {
			return 0, errors.New("memory line corrupt from free -b")
		}
		memSizeBytes, err := strconv.Atoi(memBytes[1])
		if err != nil {
			return 0, fmt.Errorf("could not get memory from %s: %w", memBytes[1], err)
		}
		return memSizeBytes, nil
	}

	return 0, errors.New("could not find memory size from free -b")
}

// getAerospikeVersion gets the Aerospike version from instance tags and determines if it's version 7 or later.
func (c *ConfNamespaceMemoryCmd) getAerospikeVersion(instance *backends.Instance) (string, bool, error) {
	version := instance.Tags["aerolab.soft.version"]
	if version == "" {
		return "", false, fmt.Errorf("no aerolab.soft.version tag found on instance")
	}

	// Parse version to determine if it's version 7 or later
	versionParts := strings.Split(version, ".")
	if len(versionParts) == 0 {
		return "", false, fmt.Errorf("invalid version format: %s", version)
	}

	majorVersion, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return "", false, fmt.Errorf("invalid major version: %s", versionParts[0])
	}

	is7 := majorVersion >= 7
	return version, is7, nil
}

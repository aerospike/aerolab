package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/bestmethod/inslice"
	aeroconf "github.com/rglonek/aerospike-config-file-parser"
	"github.com/rglonek/logger"
)

type ConfRackIdCmd struct {
	ClusterName TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes       TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	RackId      string          `short:"i" long:"id" description:"Rack ID to use" default:"0"`
	Namespaces  string          `short:"m" long:"namespaces" description:"comma-separated list of namespaces to modify; empty=all" default:""`
	NoRoster    bool            `short:"r" long:"no-roster" description:"if SC namespaces are found: aerolab will automatically restart aerospike and reset the roster for SC namespaces to reflect the rack-id; set this to not set the roster"`
	NoRestart   bool            `short:"e" long:"no-restart" description:"if no SC namespaces are found: aerolab will automatically restart aerospike when rackid is set; set this to prevent said action"`
	Path        string          `short:"p" long:"path" description:"Path to aerospike.conf" default:"/etc/aerospike/aerospike.conf"`
	Threads     int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	Help        HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfRackIdCmd) Execute(args []string) error {
	cmd := []string{"conf", "rackid"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	instances, err := c.SetRackId(system, system.Backend.GetInventory(), system.Logger, args, "rackid")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Set rack-id on %d instances", instances.Count())
	for _, i := range instances.Describe() {
		system.Logger.Debug("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// SetRackId sets the rack-id for specified namespaces in the cluster.
// This function performs the following operations:
// 1. Validates cluster exists and gets running instances
// 2. Processes each instance to set rack-id in specified namespaces
// 3. Identifies strong-consistency namespaces that need roster updates
// 4. Optionally restarts aerospike and applies roster for SC namespaces
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
func (c *ConfRackIdCmd) SetRackId(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
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
			inst, err := c.SetRackId(system, inventory, logger, args, action)
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

	// Parse namespaces to modify
	namespaces := []string{}
	if c.Namespaces != "" {
		namespaces = strings.Split(c.Namespaces, ",")
		for i, ns := range namespaces {
			namespaces[i] = strings.TrimSpace(ns)
		}
	}

	logger.Info("Setting rack-id to %s on %d nodes", c.RackId, cluster.Count())

	// Track strong-consistency namespaces found
	scFound := []string{}
	scFoundLock := new(sync.Mutex)

	// Process each instance
	var hasErr error
	parallelize.ForEachLimit(cluster.Describe(), c.Threads, func(instance *backends.Instance) {
		err := c.processInstance(instance, namespaces, &scFound, scFoundLock, logger)
		if err != nil {
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", instance.ClusterName, instance.NodeNo, err))
		}
	})

	if hasErr != nil {
		return nil, hasErr
	}

	// Handle strong-consistency namespaces
	if len(scFound) > 0 {
		if c.NoRoster {
			logger.Info("NOTE: strong-consistency namespace found, please set the roster to reflect the rack-id by re-running `aerolab roster apply` on the cluster")
			logger.Info("Do not forget to restart the aerospike service first to reload the configuration files")
		} else {
			logger.Info("Strong-consistency namespaces found, restarting aerospike and setting up the roster")

			// Restart aerospike
			restartCmd := &AerospikeRestartCmd{
				ClusterName: c.ClusterName,
				Threads:     c.Threads,
			}
			_, err := restartCmd.RestartAerospike(system, inventory, logger, args, "restart")
			if err != nil {
				logger.Error("ERROR running 'aerospike restart': %s", err)
			}

			// Apply roster for each SC namespace
			for _, namespace := range scFound {
				rosterApplyCmd := &RosterApplyCmd{
					ClusterName: c.ClusterName,
					Namespace:   namespace,
					Threads:     c.Threads,
					Quiet:       true,
				}
				err = rosterApplyCmd.ApplyRoster(system, inventory, logger, args, "apply")
				if err != nil {
					logger.Error("ERROR running 'roster apply': %s", err)
				}
			}
			logger.Info("Done")
		}
	} else {
		if !c.NoRestart {
			logger.Info("Restarting aerospike to apply configuration changes")
			restartCmd := &AerospikeRestartCmd{
				ClusterName: c.ClusterName,
				Threads:     c.Threads,
			}
			_, err := restartCmd.RestartAerospike(system, inventory, logger, args, "restart")
			if err != nil {
				logger.Error("ERROR running 'aerospike restart': %s", err)
			}
			logger.Info("Done")
		} else {
			logger.Info("Done, remember to restart aerospike for the changes to take effect")
		}
	}

	return cluster.Describe(), nil
}

// processInstance processes a single instance to set rack-id in specified namespaces.
func (c *ConfRackIdCmd) processInstance(instance *backends.Instance, namespaces []string, scFound *[]string, scFoundLock *sync.Mutex, logger *logger.Logger) error {
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

	// Process namespaces
	foundns := 0
	changes := false

	for _, key := range s.ListKeys() {
		if strings.HasPrefix(key, "namespace ") && s.Type(key) == aeroconf.ValueStanza {
			ns := strings.Split(key, " ")
			if len(ns) < 2 || ns[1] == "" {
				logger.Warn("stanza namespace does not have a name, skipping: %s", key)
				continue
			}
			namespaceName := strings.Trim(ns[1], "\r\t\n ")

			// Check if this namespace should be modified
			if len(namespaces) == 0 || inslice.HasString(namespaces, namespaceName) {
				stanza := s.Stanza(key)

				// Set rack-id
				stanza.SetValue("rack-id", c.RackId)
				changes = true
				foundns++

				// Check if this is a strong-consistency namespace
				if stanza.Type("strong-consistency") == aeroconf.ValueString {
					if sc, err := stanza.GetValues("strong-consistency"); err == nil && len(sc) > 0 && strings.ToLower(*sc[0]) == "true" {
						scFoundLock.Lock()
						if !inslice.HasString(*scFound, namespaceName) {
							*scFound = append(*scFound, namespaceName)
						}
						scFoundLock.Unlock()
					}
				}
			}
		}
	}

	if len(namespaces) > 0 && foundns < len(namespaces) {
		return fmt.Errorf("not all listed namespaces were found, or no namespaces found at all")
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

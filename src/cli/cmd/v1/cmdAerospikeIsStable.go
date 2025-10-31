package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

type AerospikeIsStableCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated OR 'all' to affect all clusters" default:"mydc"`
	Nodes            TypeNodes       `short:"l" long:"nodes" description:"Only consider the given nodes, e.g. --nodes=1-4,7,8"`
	Namespace        string          `short:"m" long:"namespace" description:"Namespace to change" default:"test"`
	Wait             bool            `short:"w" long:"wait" description:"If set, will wait in a loop until the cluster is stable, and then return"`
	WaitTimeout      int             `short:"o" long:"wait-timeout" description:"If set, will timeout if the cluster doesn't become stable by this many seconds"`
	IgnoreMigrations bool            `short:"i" long:"ignore-migrations" description:"If set, will ignore migrations when checking if cluster is stable"`
	IgnoreClusterKey bool            `short:"k" long:"ignore-cluster-key" description:"If set, will not check if the cluster key matches on all nodes in the cluster"`
	NotClusterKey    string          `short:"c" long:"not-cluster-key" description:"If specified, then if this cluster key is matched, it will be ignored and treated as no-match"`
	Verbose          bool            `short:"v" long:"verbose" description:"Enable verbose logging"`
	Threads          int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	Help             HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AerospikeIsStableCmd) Execute(args []string) error {
	cmd := []string{"aerospike", "is-stable"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	if c.WaitTimeout != 0 {
		c.Wait = true
	}

	stable, err := c.IsStable(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	if stable {
		system.Logger.Info("Done")
		return Error(nil, system, cmd, c, args)
	}
	return Error(errors.New("cluster not stable"), system, cmd, c, args)
}

// IsStable checks if the aerospike cluster is stable.
// This function performs the following operations:
// 1. Gets the list of nodes in the cluster
// 2. Runs cluster-stable check on all nodes
// 3. Compares cluster keys across all nodes
// 4. Optionally waits in a loop until cluster becomes stable
//
// Parameters:
//   - system: The system instance for logging and backend operations
//   - inventory: The backend inventory containing cluster information
//   - logger: Logger instance for output
//   - args: Command line arguments
//
// Returns:
//   - bool: true if cluster is stable, false otherwise
//   - error: nil on success, or an error describing what failed
func (c *AerospikeIsStableCmd) IsStable(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) (bool, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"aerospike", "is-stable"}, c, args...)
		if err != nil {
			return false, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	instances := inventory.Instances.WithState(backends.LifeCycleStateRunning).WithClusterName(c.ClusterName.String())
	if instances.Count() == 0 {
		return false, fmt.Errorf("no running instances found for cluster %s", c.ClusterName.String())
	}

	// Get node numbers
	nodes := []int{}
	for _, instance := range instances.Describe() {
		nodes = append(nodes, instance.NodeNo)
	}

	// Filter nodes if specified
	if c.Nodes.String() != "" {
		filteredNodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return false, err
		}
		for _, node := range filteredNodes {
			if !slices.Contains(nodes, node) {
				return false, fmt.Errorf("selected node %d not found", node)
			}
		}
		nodes = filteredNodes
	}

	startTime := time.Now()
	firstLoop := true

	for c.WaitTimeout == 0 || time.Since(startTime) < time.Duration(c.WaitTimeout)*time.Second {
		if !firstLoop {
			logger.Info("Cluster Key Mismatch, Getting cluster keys")
		} else {
			logger.Info("Getting cluster keys")
		}

		clusterKeys := []string{}
		keysLock := new(sync.Mutex)
		var hasErr bool

		// Create wait script if needed
		waitScript := ""
		if c.Wait {
			waitScript = fmt.Sprintf(`debug=%t
timeout=%d
start_time=$(date +%%s)
while (( timeout == 0 || $(date +%%s) - start_time < timeout )); do
	RET=$(asinfo -v 'cluster-stable:size=%d;ignore-migrations=%t;namespace=%s' 2>&1)
	if [ $? -eq 0 ]; then
		echo "AEROLAB-SUCCESS-CLUSTER-KEY:${RET}"
		exit 0
	fi
	[ "${debug}" == "true" ] && echo "${RET}"
	sleep 1
done
echo ${RET}
exit 1
`, c.Verbose, c.WaitTimeout, len(nodes), c.IgnoreMigrations, c.Namespace)
		}

		// Check each node
		for _, node := range nodes {
			nodeInstances := instances.WithNodeNo(node).Describe()
			if len(nodeInstances) == 0 {
				logger.Error("Node %d not found in running instances", node)
				hasErr = true
				continue
			}
			instance := nodeInstances[0]

			var cmd []string
			if c.Wait {
				// Upload wait script via SFTP
				conf, err := instance.GetSftpConfig("root")
				if err != nil {
					logger.Error("Failed to get SFTP config for node %d: %s", node, err)
					hasErr = true
					continue
				}
				client, err := sshexec.NewSftp(conf)
				if err != nil {
					logger.Error("Failed to create SFTP client for node %d: %s", node, err)
					hasErr = true
					continue
				}
				defer client.Close()

				err = client.WriteFile(true, &sshexec.FileWriter{
					DestPath:    "/opt/is-stable.sh",
					Source:      strings.NewReader(waitScript),
					Permissions: 0755,
				})
				if err != nil {
					logger.Error("Failed to upload wait script to node %d: %s", node, err)
					hasErr = true
					continue
				}
				cmd = []string{"/bin/bash", "/opt/is-stable.sh"}
			} else {
				cmd = []string{"asinfo", "-v", fmt.Sprintf("cluster-stable:size=%d;ignore-migrations=%t;namespace=%s", len(nodes), c.IgnoreMigrations, c.Namespace)}
			}

			// Run command
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
				logger.Error("Node %d returned error: %s", node, output.Output.Err)
				hasErr = true
				continue
			}

			// Parse output
			outputStr := string(output.Output.Stdout)
			if c.Verbose {
				logger.Info("Node %d output: %s", node, outputStr)
			}

			if c.Wait {
				// Parse wait script output
				lines := strings.Split(outputStr, "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "AEROLAB-SUCCESS-CLUSTER-KEY:") {
						key := strings.TrimRight(strings.Split(line, "-SUCCESS-CLUSTER-KEY:")[1], "\r\n ")
						keysLock.Lock()
						clusterKeys = append(clusterKeys, key)
						keysLock.Unlock()
					}
				}
			} else {
				// Parse direct asinfo output
				key := strings.TrimRight(outputStr, "\r\n ")
				keysLock.Lock()
				clusterKeys = append(clusterKeys, key)
				keysLock.Unlock()
			}
		}

		if hasErr {
			if !c.Wait {
				return false, errors.New("some nodes returned errors")
			}
			time.Sleep(time.Second)
			continue
		}

		firstLoop = false

		// Check if cluster is stable
		same := true

		if len(nodes) != len(clusterKeys) {
			same = false
		} else if !c.IgnoreClusterKey && c.NotClusterKey != "" && len(clusterKeys) > 0 && clusterKeys[0] == c.NotClusterKey {
			same = false
		} else if !c.IgnoreClusterKey && len(clusterKeys) > 0 {
			for _, k := range clusterKeys {
				if clusterKeys[0] != k {
					same = false
					break
				}
			}
		}

		if same {
			logger.Info("Cluster Stable")
			if len(clusterKeys) > 0 {
				fmt.Println("cluster-key:" + clusterKeys[0])
			}
			return true, nil
		}

		if !c.Wait {
			return false, errors.New("cluster not stable")
		}

		time.Sleep(time.Second)
	}

	return false, errors.New("timeout reached, cluster unstable")
}

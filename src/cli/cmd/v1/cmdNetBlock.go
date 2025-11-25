package cmd

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/rglonek/logger"
)

type NetBlockCmd struct {
	SourceClusterName      TypeClusterName      `short:"s" long:"source" description:"Source cluster name" default:"mydc"`
	SourceNodeList         TypeNodes            `short:"l" long:"source-node-list" description:"List of source nodes. Empty=ALL." default:""`
	DestinationClusterName TypeClusterName      `short:"d" long:"destination" description:"Destination cluster name" default:"mydc-xdr"`
	DestinationNodeList    TypeNodes            `short:"i" long:"destination-node-list" description:"List of destination nodes. Empty=ALL." default:""`
	Type                   TypeNetType          `short:"t" long:"type" description:"Block type (reject|drop)." default:"reject" webchoice:"reject,drop"`
	Ports                  string               `short:"p" long:"ports" description:"Comma separated list of ports to block." default:"3000"`
	BlockOn                TypeNetBlockOn       `short:"b" long:"block-on" description:"Block where (input|output). Input=on destination, output=on source." default:"input" webchoice:"input,output"`
	StatisticMode          TypeNetStatisticMode `short:"M" long:"statistic-mode" description:"for partial packet loss, supported are: random | nth. Not set: drop all packets." default:""`
	StatisticProbability   string               `short:"P" long:"probability" description:"for partial packet loss mode random. Supported values are between 0.0 and 1.0 (0% to 100%)" default:"0.5"`
	StatisticEvery         string               `short:"E" long:"every" description:"for partial packet loss mode nth. Match one every nth packet. Default: 2 (50% loss)" default:"2"`
	Help                   HelpCmd              `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *NetBlockCmd) Execute(args []string) error {
	cmd := []string{"net", "block"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.blockUnblock(system, system.Backend.GetInventory(), system.Logger, args, "block", "-I")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *NetBlockCmd) blockUnblock(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string, blockString string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"net", action}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Get source and destination clusters
	sourceCluster := inventory.Instances.WithClusterName(c.SourceClusterName.String()).WithState(backends.LifeCycleStateRunning)
	if sourceCluster.Count() == 0 {
		return fmt.Errorf("source cluster %s not found or has no running instances", c.SourceClusterName.String())
	}

	destCluster := inventory.Instances.WithClusterName(c.DestinationClusterName.String()).WithState(backends.LifeCycleStateRunning)
	if destCluster.Count() == 0 {
		return fmt.Errorf("destination cluster %s not found or has no running instances", c.DestinationClusterName.String())
	}

	// Filter source nodes if specified
	var sourceInstances backends.InstanceList
	if c.SourceNodeList.String() != "" {
		nodes, err := expandNodeNumbers(c.SourceNodeList.String())
		if err != nil {
			return fmt.Errorf("failed to parse source node list: %w", err)
		}
		sourceInstances = sourceCluster.WithNodeNo(nodes...).Describe()
		if sourceInstances.Count() != len(nodes) {
			return fmt.Errorf("some source nodes not found or not running")
		}
	} else {
		sourceInstances = sourceCluster.Describe()
	}

	// Filter destination nodes if specified
	var destInstances backends.InstanceList
	if c.DestinationNodeList.String() != "" {
		nodes, err := expandNodeNumbers(c.DestinationNodeList.String())
		if err != nil {
			return fmt.Errorf("failed to parse destination node list: %w", err)
		}
		destInstances = destCluster.WithNodeNo(nodes...).Describe()
		if destInstances.Count() != len(nodes) {
			return fmt.Errorf("some destination nodes not found or not running")
		}
	} else {
		destInstances = destCluster.Describe()
	}

	// Determine where to apply the rules and build IP maps
	var execInstances backends.InstanceList
	var targetIPs []string
	blockon := "--destination"

	if c.BlockOn.String() == "input" {
		// Apply rules on destination nodes, block source IPs
		execInstances = destInstances
		for _, inst := range sourceInstances.Describe() {
			if inst.IP.Private != "" {
				targetIPs = append(targetIPs, inst.IP.Private)
			}
			if inst.IP.Public != "" && inst.IP.Public != inst.IP.Private {
				targetIPs = append(targetIPs, inst.IP.Public)
			}
		}
		blockon = "--source"
	} else {
		// Apply rules on source nodes, block destination IPs
		execInstances = sourceInstances
		for _, inst := range destInstances.Describe() {
			if inst.IP.Private != "" {
				targetIPs = append(targetIPs, inst.IP.Private)
			}
			if inst.IP.Public != "" && inst.IP.Public != inst.IP.Private {
				targetIPs = append(targetIPs, inst.IP.Public)
			}
		}
	}

	ports := strings.Split(c.Ports, ",")
	logger.Info("Compiling iptables rules")

	// Build command list for each node
	commandList := make(map[int][]string)
	for _, inst := range execInstances.Describe() {
		for _, port := range ports {
			for _, ip := range targetIPs {
				nComm := []string{"/sbin/iptables", blockString, strings.ToUpper(c.BlockOn.String()), "-p", "tcp", "--dport", port, blockon, ip}

				if c.StatisticMode.String() != "" {
					nComm = append(nComm, "-m", "statistic", "--mode", c.StatisticMode.String())
					if c.StatisticMode.String() == "random" {
						nComm = append(nComm, "--probability", c.StatisticProbability)
					} else {
						nComm = append(nComm, "--every", c.StatisticEvery)
					}
				}

				nComm = append(nComm, "-j", strings.ToUpper(c.Type.String()))

				if _, ok := commandList[inst.NodeNo]; !ok {
					commandList[inst.NodeNo] = []string{strings.Join(nComm, " ")}
				} else {
					commandList[inst.NodeNo] = append(commandList[inst.NodeNo], strings.Join(nComm, " "))
				}
			}
		}
	}

	logger.Info("Executing iptables commands on %d nodes", len(commandList))

	// Execute commands in parallel
	isErr := false
	lock := new(sync.Mutex)
	wg := new(sync.WaitGroup)

	for nodeNo, commands := range commandList {
		nodeInstances := execInstances.WithNodeNo(nodeNo).Describe()
		if len(nodeInstances) == 0 {
			continue
		}
		instance := nodeInstances[0]

		wg.Add(1)
		go func(instance *backends.Instance, commands []string) {
			defer wg.Done()

			bashCmd := fmt.Sprintf("/bin/bash -c '%s'", strings.Join(commands, ";"))
			output := instance.Exec(&backends.ExecInput{
				ExecDetail: sshexec.ExecDetail{
					Command:        []string{"/bin/bash", "-c", strings.Join(commands, ";")},
					SessionTimeout: 5 * time.Minute,
				},
				Username:        "root",
				ConnectTimeout:  30 * time.Second,
				ParallelThreads: 1,
			})

			if output.Output.Err != nil {
				logger.Error("ERROR running iptables on cluster %s node %d: %s: %s: %s",
					instance.ClusterName, instance.NodeNo, output.Output.Err, string(output.Output.Stdout), string(output.Output.Stderr))
				lock.Lock()
				isErr = true
				lock.Unlock()
			} else {
				logger.Info("Executed iptables on cluster %s node %d", instance.ClusterName, instance.NodeNo)
				logger.Debug("Command: %s", bashCmd)
			}
		}(instance, commands)
	}

	wg.Wait()

	if isErr {
		return errors.New("errors were encountered while executing iptables commands")
	}

	return nil
}

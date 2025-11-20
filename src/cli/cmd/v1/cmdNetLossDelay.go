package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/logger"
)

type NetLossDelayCmd struct {
	SourceClusterName      TypeClusterName   `short:"s" long:"source" description:"Source cluster name" default:"mydc"`
	SourceNodeList         TypeNodes         `short:"l" long:"source-node-list" description:"List of source nodes. Empty=ALL." default:""`
	DestinationClusterName TypeClusterName   `short:"d" long:"destination" description:"Destination cluster name" default:"mydc-xdr"`
	DestinationNodeList    TypeNodes         `short:"i" long:"destination-node-list" description:"List of destination nodes. Empty=ALL." default:""`
	Action                 TypeNetLossAction `short:"a" long:"action" description:"One of: set|del|reset|show. reset does not require dest cluster, as it removes all rules" default:"show" webchoice:"show,set,del,reset"`
	LatencyMs              string            `short:"D" long:"latency-ms" description:"optional: specify latency (number) of milliseconds"`
	PacketLossPct          string            `short:"L" long:"loss-pct" description:"optional: specify packet loss percentage"`
	LinkSpeedRateBytes     string            `short:"E" long:"rate-bytes" description:"optional: specify link speed rate, in bytes"`
	CorruptPct             string            `short:"O" long:"corrupt-pct" description:"optional: corrupt packets (percentage)"`
	RunOnDestination       bool              `short:"o" long:"on-destination" description:"if set, the rules will be created on destination nodes (avoid EPERM on source, true simulation)"`
	DstPort                int               `short:"p" long:"dst-port" description:"only apply the rule to a specific destination port"`
	SrcPort                int               `short:"P" long:"src-port" description:"only apply the rule to a specific source port"`
	Verbose                bool              `short:"v" long:"verbose" description:"run easytc in verbose mode"`
	Threads                int               `short:"t" long:"threads" description:"Number of parallel threads" default:"10"`
	Help                   HelpCmd           `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *NetLossDelayCmd) Execute(args []string) error {
	cmd := []string{"net", "loss-delay"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.lossDelay(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *NetLossDelayCmd) lossDelay(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"net", "loss-delay"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Get source and destination clusters
	sourceCluster := inventory.Instances.WithClusterName(c.SourceClusterName.String()).WithState(backends.LifeCycleStateRunning).Describe()
	var destCluster backends.InstanceList

	// For reset/show, destination is optional
	requiresDest := c.Action.String() == "set" || c.Action.String() == "del"
	if requiresDest || c.DestinationClusterName.String() != "" {
		destCluster = inventory.Instances.WithClusterName(c.DestinationClusterName.String()).WithState(backends.LifeCycleStateRunning).Describe()
		if destCluster.Count() == 0 {
			return fmt.Errorf("destination cluster %s not found or has no running instances", c.DestinationClusterName.String())
		}
	}

	if sourceCluster.Count() == 0 {
		return fmt.Errorf("source cluster %s not found or has no running instances", c.SourceClusterName.String())
	}

	// Filter source nodes
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

	// Filter destination nodes
	var destInstances backends.InstanceList
	if destCluster.Count() > 0 {
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
	}

	// Build IP map for show command
	ipMap := make(map[string]string)
	for _, inst := range inventory.Instances.Describe() {
		if inst.IP.Private != "" {
			ipMap[inst.IP.Private] = fmt.Sprintf("%s:%d", inst.ClusterName, inst.NodeNo)
		}
		if inst.IP.Public != "" && inst.IP.Public != inst.IP.Private {
			ipMap[inst.IP.Public] = fmt.Sprintf("%s:%d", inst.ClusterName, inst.NodeNo)
		}
	}

	// Build easytc command
	script := c.buildEasytcScript(sourceInstances, destInstances, ipMap, logger)

	// Determine which nodes to execute on
	var execInstances backends.InstanceList
	if c.RunOnDestination && destInstances.Count() > 0 {
		execInstances = destInstances
	} else {
		execInstances = sourceInstances
	}

	// Execute on nodes in parallel
	logger.Info("Executing easytc on %d nodes", execInstances.Count())
	var hasErr error
	parallelize.ForEachLimit(execInstances.Describe(), c.Threads, func(inst *backends.Instance) {
		err := c.execEasytc(inst, script, ipMap, logger)
		if err != nil {
			logger.Error("Node %s:%d returned error: %s", inst.ClusterName, inst.NodeNo, err)
			hasErr = errors.New("some nodes returned errors")
		}
	})

	return hasErr
}

func (c *NetLossDelayCmd) buildEasytcScript(sourceInstances, destInstances backends.InstanceList, ipMap map[string]string, logger *logger.Logger) string {
	var script strings.Builder

	// Start with easytc command
	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n\n")

	switch c.Action.String() {
	case "reset":
		script.WriteString("easytc reset")
		script.WriteString("\n")

	case "show":
		script.WriteString("easytc show rules --quiet")
		script.WriteString("\n")

	case "set", "del":
		// Build command for each destination IP
		baseCmd := []string{"easytc", c.Action.String()}

		if c.SrcPort != 0 {
			baseCmd = append(baseCmd, "-S", strconv.Itoa(c.SrcPort))
		}
		if c.DstPort != 0 {
			baseCmd = append(baseCmd, "-D", strconv.Itoa(c.DstPort))
		}

		if c.Action.String() == "set" {
			if c.CorruptPct != "" {
				baseCmd = append(baseCmd, "-c", c.CorruptPct)
			}
			if c.LinkSpeedRateBytes != "" {
				baseCmd = append(baseCmd, "-e", c.LinkSpeedRateBytes)
			}
			if c.PacketLossPct != "" {
				baseCmd = append(baseCmd, "-p", c.PacketLossPct)
			}
			if c.LatencyMs != "" {
				baseCmd = append(baseCmd, "-l", c.LatencyMs)
			}
		}

		if c.Verbose {
			baseCmd = append(baseCmd, "--verbose")
		}

		// Add IP addresses
		var targetInstances backends.InstanceList
		if c.RunOnDestination {
			targetInstances = sourceInstances
		} else {
			targetInstances = destInstances
		}

		for _, inst := range targetInstances.Describe() {
			cmd := make([]string, len(baseCmd))
			copy(cmd, baseCmd)

			if c.RunOnDestination {
				// Add source IPs when running on destination
				if inst.IP.Private != "" {
					cmdWithIP := append(cmd, "-s", inst.IP.Private)
					script.WriteString(strings.Join(cmdWithIP, " "))
					script.WriteString("\n")
				}
				if inst.IP.Public != "" && inst.IP.Public != inst.IP.Private {
					cmdWithIP := append(cmd, "-s", inst.IP.Public)
					script.WriteString(strings.Join(cmdWithIP, " "))
					script.WriteString("\n")
				}
			} else {
				// Add destination IPs when running on source
				if inst.IP.Private != "" {
					cmdWithIP := append(cmd, "-d", inst.IP.Private)
					script.WriteString(strings.Join(cmdWithIP, " "))
					script.WriteString("\n")
				}
				if inst.IP.Public != "" && inst.IP.Public != inst.IP.Private {
					cmdWithIP := append(cmd, "-d", inst.IP.Public)
					script.WriteString(strings.Join(cmdWithIP, " "))
					script.WriteString("\n")
				}
			}
		}
	}

	return script.String()
}

func (c *NetLossDelayCmd) execEasytc(inst *backends.Instance, script string, ipMap map[string]string, logger *logger.Logger) error {
	// Upload script via SFTP
	conf, err := inst.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	client, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer client.Close()

	err = client.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/tmp/runtc.sh",
		Source:      strings.NewReader(script),
		Permissions: 0755,
	})
	if err != nil {
		return fmt.Errorf("failed to upload script: %w", err)
	}

	// Execute script
	output := inst.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"/bin/bash", "/tmp/runtc.sh"},
			SessionTimeout: 5 * time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if output.Output.Err != nil {
		return fmt.Errorf("(%s:%d) %s: %s", inst.ClusterName, inst.NodeNo, output.Output.Err, string(output.Output.Stdout))
	}

	// Process and display output
	if c.Action.String() == "show" {
		c.displayEasytcOutput(inst, string(output.Output.Stdout), ipMap, logger)
	}

	return nil
}

func (c *NetLossDelayCmd) displayEasytcOutput(inst *backends.Instance, output string, ipMap map[string]string, logger *logger.Logger) {
	lines := strings.Split(output, "\n")
	result := fmt.Sprintf("================== %s:%d ==================", inst.ClusterName, inst.NodeNo)

	for _, line := range lines {
		if strings.Contains(line, "TcFilterHandle") {
			line = line + " IPNode "
		} else if strings.Contains(line, "-----------") {
			line = line + "--------"
		} else {
			ips := c.findIP(line)
			if len(ips) > 0 {
				if val, ok := ipMap[ips[0]]; ok {
					line = line + " " + val
				}
			}
		}
		result = result + "\n" + line
	}

	fmt.Println(result)
}

func (c *NetLossDelayCmd) findIP(input string) []string {
	numBlock := "(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])"
	regexPattern := numBlock + "\\." + numBlock + "\\." + numBlock + "\\." + numBlock

	regEx := regexp.MustCompile(regexPattern)
	return regEx.FindAllString(input, -1)
}

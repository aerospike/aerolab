package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rglonek/logger"
)

type NetListCmd struct {
	Json       bool     `short:"j" long:"json" description:"set to display output in json format"`
	PrettyJson bool     `short:"p" long:"pretty" description:"set to indent json and pretty-print"`
	SortBy     []string `short:"s" long:"sort" description:"sort-by fields, must match column names exactly" default:"Source:asc,Destination:asc,Port:asc"`
	Output     string   `short:"o" long:"output" description:"Output format: table|json|json-indent|csv|tsv|html|markdown" default:"table"`
	TableTheme string   `short:"T" long:"table-theme" description:"Table theme: default|frame|box" default:"default"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type netRule struct {
	SourceCluster      string `json:"sourceCluster"`
	SourceNode         int    `json:"sourceNode"`
	DestinationCluster string `json:"destinationCluster"`
	DestinationNode    int    `json:"destinationNode"`
	Port               string `json:"port"`
	Type               string `json:"type"`
	Chain              string `json:"chain"`
	RuleAppliedOn      string `json:"ruleAppliedOn"`
	Behaviour          string `json:"behaviour"`
}

func (c *NetListCmd) Execute(args []string) error {
	cmd := []string{"net", "list"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.listRules(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *NetListCmd) listRules(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"net", "list"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Build IP to node mapping
	ipMap := make(map[string]*backends.Instance)
	for _, inst := range inventory.Instances.Describe() {
		if inst.IP.Private != "" {
			ipMap[inst.IP.Private] = inst
		}
		if inst.IP.Public != "" && inst.IP.Public != inst.IP.Private {
			ipMap[inst.IP.Public] = inst
		}
	}

	// Collect rules from all nodes
	var rules []*netRule
	instances := inventory.Instances.WithState(backends.LifeCycleStateRunning).Describe()

	for _, inst := range instances {
		// Get INPUT chain rules
		output := inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"/sbin/iptables", "-L", "INPUT", "-vn"},
				SessionTimeout: 30 * time.Second,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})

		if output.Output.Err == nil {
			inputRules := c.parseIptablesOutput(string(output.Output.Stdout), inst, ipMap, "INPUT", logger)
			rules = append(rules, inputRules...)
		} else if !strings.Contains(output.Output.Err.Error(), "no such file or directory") {
			logger.Warn("Could not check INPUT on %s:%d: %s", inst.ClusterName, inst.NodeNo, output.Output.Err)
		}

		// Get OUTPUT chain rules
		output = inst.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"/sbin/iptables", "-L", "OUTPUT", "-vn"},
				SessionTimeout: 30 * time.Second,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})

		if output.Output.Err == nil {
			outputRules := c.parseIptablesOutput(string(output.Output.Stdout), inst, ipMap, "OUTPUT", logger)
			rules = append(rules, outputRules...)
		} else if !strings.Contains(output.Output.Err.Error(), "no such file or directory") {
			logger.Warn("Could not check OUTPUT on %s:%d: %s", inst.ClusterName, inst.NodeNo, output.Output.Err)
		}
	}

	// Handle JSON output
	if c.Json || c.PrettyJson || c.Output == "json" || c.Output == "json-indent" {
		return c.outputJSON(rules, os.Stdout)
	}

	// Handle table output
	return c.outputTable(rules, os.Stdout)
}

func (c *NetListCmd) parseIptablesOutput(output string, ruleNode *backends.Instance, ipMap map[string]*backends.Instance, chain string, logger *logger.Logger) []*netRule {
	var rules []*netRule

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if !strings.Contains(line, "REJECT") && !strings.Contains(line, "DROP") {
			continue
		}

		ruleType := "DROP"
		if strings.Contains(line, "REJECT") {
			ruleType = "REJECT"
		}

		fields := strings.Fields(line)
		if len(fields) < 12 {
			continue
		}

		var srcIP, dstIP, port string
		var srcNode, dstNode *backends.Instance
		ruleOn := ""

		if chain == "INPUT" {
			// For INPUT chain, source IP is the blocker
			srcIP = c.getField(fields, 8)
			// dstIP = ruleNode.IP.Private -- this value is not used
			srcNode = ipMap[srcIP]
			dstNode = ruleNode
			ruleOn = "Destination"
			port = c.extractPort(c.getField(fields, 11))
		} else {
			// For OUTPUT chain, destination IP is the blocked
			// srcIP = ruleNode.IP.Private -- this value is not used
			dstIP = c.getField(fields, 9)
			srcNode = ruleNode
			dstNode = ipMap[dstIP]
			ruleOn = "Source"
			port = c.extractPort(c.getField(fields, 11))
		}

		// Extract behaviour (remaining fields)
		behaviour := strings.Join(fields[12:], " ")

		rule := &netRule{
			Port:          port,
			Type:          ruleType,
			Chain:         chain,
			RuleAppliedOn: ruleOn,
			Behaviour:     behaviour,
		}

		if srcNode != nil {
			rule.SourceCluster = srcNode.ClusterName
			rule.SourceNode = srcNode.NodeNo
		} else {
			rule.SourceCluster = "UNKNOWN"
			rule.SourceNode = 0
		}

		if dstNode != nil {
			rule.DestinationCluster = dstNode.ClusterName
			rule.DestinationNode = dstNode.NodeNo
		} else {
			rule.DestinationCluster = "UNKNOWN"
			rule.DestinationNode = 0
		}

		rules = append(rules, rule)
	}

	return rules
}

func (c *NetListCmd) getField(fields []string, index int) string {
	if index < len(fields) {
		return fields[index]
	}
	return ""
}

func (c *NetListCmd) extractPort(portField string) string {
	parts := strings.Split(portField, ":")
	if len(parts) > 1 {
		return parts[1]
	}
	return portField
}

func (c *NetListCmd) outputJSON(rules []*netRule, out *os.File) error {
	// Sort rules
	c.sortRules(rules)

	j := json.NewEncoder(out)
	if c.PrettyJson || c.Output == "json-indent" {
		j.SetIndent("", "  ")
	}
	return j.Encode(rules)
}

func (c *NetListCmd) outputTable(rules []*netRule, out *os.File) error {
	// Sort rules
	c.sortRules(rules)

	t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, false, false)
	if err != nil {
		if err == printer.ErrTerminalWidthUnknown {
			fmt.Fprintf(os.Stderr, "Warning: Couldn't get terminal width, using default width\n")
		} else {
			return err
		}
	}

	header := table.Row{"Source", "Destination", "Port", "Type", "Chain", "RuleOn", "Behaviour"}
	rows := []table.Row{}

	for _, rule := range rules {
		rows = append(rows, table.Row{
			fmt.Sprintf("%s-%d", rule.SourceCluster, rule.SourceNode),
			fmt.Sprintf("%s-%d", rule.DestinationCluster, rule.DestinationNode),
			rule.Port,
			rule.Type,
			rule.Chain,
			rule.RuleAppliedOn,
			rule.Behaviour,
		})
	}

	title := printer.String("RULES")
	fmt.Fprintln(out, t.RenderTable(title, header, rows))
	fmt.Fprintln(out, "")

	return nil
}

func (c *NetListCmd) sortRules(rules []*netRule) {
	sort.Slice(rules, func(i, j int) bool {
		for _, sortItem := range c.SortBy {
			parts := strings.Split(sortItem, ":")
			field := parts[0]
			order := "asc"
			if len(parts) > 1 {
				order = parts[1]
			}

			var cmp int
			switch strings.ToLower(field) {
			case "source":
				if rules[i].SourceCluster != rules[j].SourceCluster {
					cmp = strings.Compare(rules[i].SourceCluster, rules[j].SourceCluster)
				} else {
					cmp = rules[i].SourceNode - rules[j].SourceNode
				}
			case "destination":
				if rules[i].DestinationCluster != rules[j].DestinationCluster {
					cmp = strings.Compare(rules[i].DestinationCluster, rules[j].DestinationCluster)
				} else {
					cmp = rules[i].DestinationNode - rules[j].DestinationNode
				}
			case "port":
				cmp = strings.Compare(rules[i].Port, rules[j].Port)
			case "type":
				cmp = strings.Compare(rules[i].Type, rules[j].Type)
			case "chain":
				cmp = strings.Compare(rules[i].Chain, rules[j].Chain)
			case "ruleon":
				cmp = strings.Compare(rules[i].RuleAppliedOn, rules[j].RuleAppliedOn)
			case "behaviour":
				cmp = strings.Compare(rules[i].Behaviour, rules[j].Behaviour)
			default:
				continue
			}

			if cmp != 0 {
				if order == "dsc" {
					return cmp > 0
				}
				return cmp < 0
			}
		}
		return false
	})
}

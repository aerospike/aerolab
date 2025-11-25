package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rglonek/logger"
)

type NetListCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum" default:"Source:asc,Destination:asc,Port:asc"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
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

	// Setup pager if requested
	out := io.Writer(os.Stdout)
	var page *pager.Pager
	if c.Pager {
		var err error
		page, err = pager.New(out)
		if err != nil {
			return err
		}
		err = page.Start()
		if err != nil {
			return err
		}
		defer page.Close()
		out = page
	}

	// Handle output based on format
	if c.Output == "json" || c.Output == "json-indent" || c.Output == "jq" {
		return c.outputJSON(rules, out, page)
	}

	// Handle table output
	return c.outputTable(rules, out)
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
		if len(fields) < 8 {
			continue
		}

		var srcIP, dstIP, port, behaviour string
		var srcNode, dstNode *backends.Instance
		ruleOn := ""

		// Parse iptables output format:
		// pkts bytes target prot opt in out source destination [extra options]
		// Find source and destination by looking for IP addresses
		srcIP = ""
		dstIP = ""
		behaviourStartIdx := -1

		for i, field := range fields {
			// Look for source IP (format: IP/mask or IP)
			if i >= 6 && c.isIPAddress(field) && srcIP == "" {
				srcIP = strings.Split(field, "/")[0]
			} else if i >= 7 && c.isIPAddress(field) && srcIP != "" && dstIP == "" {
				dstIP = strings.Split(field, "/")[0]
			}

			// Look for port specification (dpt:PORT or spt:PORT)
			if strings.HasPrefix(field, "dpt:") || strings.HasPrefix(field, "spt:") {
				port = c.extractPort(field)
			}

			// Look for reject-with or other behaviour indicators
			if field == "reject-with" || field == "limit:" || field == "burst" {
				behaviourStartIdx = i
				break
			}
		}

		// Extract behaviour
		if behaviourStartIdx > 0 && behaviourStartIdx < len(fields) {
			behaviour = strings.Join(fields[behaviourStartIdx:], " ")
		}

		if chain == "INPUT" {
			// For INPUT chain, source IP is the blocker
			srcNode = ipMap[srcIP]
			dstNode = ruleNode
			ruleOn = "Destination"
		} else {
			// For OUTPUT chain, destination IP is the blocked
			srcNode = ruleNode
			dstNode = ipMap[dstIP]
			ruleOn = "Source"
		}

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

func (c *NetListCmd) isIPAddress(s string) bool {
	// Remove CIDR notation if present
	ip := strings.Split(s, "/")[0]

	// Check if it looks like an IP address (simple check)
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		if part == "" {
			return false
		}
		// Check if it's a valid number 0-255
		num := 0
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
			num = num*10 + int(c-'0')
			if num > 255 {
				return false
			}
		}
	}
	return true
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

func (c *NetListCmd) outputJSON(rules []*netRule, out io.Writer, page *pager.Pager) error {
	// Sort rules
	c.sortRules(rules)

	switch c.Output {
	case "jq":
		params := []string{}
		if page != nil && page.HasColors() {
			params = append(params, "-C")
		}
		cmd := exec.Command("jq", params...)
		cmd.Stdout = out
		cmd.Stderr = out
		w, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		defer w.Close()
		enc := json.NewEncoder(w)
		go func() {
			enc.Encode(rules)
			w.Close()
		}()
		return cmd.Run()
	case "json-indent":
		j := json.NewEncoder(out)
		j.SetIndent("", "  ")
		return j.Encode(rules)
	default: // json
		return json.NewEncoder(out).Encode(rules)
	}
}

func (c *NetListCmd) outputTable(rules []*netRule, out io.Writer) error {
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
	if len(c.SortBy) == 1 && strings.Contains(c.SortBy[0], ",") {
		c.SortBy = strings.Split(c.SortBy[0], ",")
	}
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

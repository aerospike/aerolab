package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rglonek/logger"
)

// AgiStatusCmd shows the status of an AGI instance.
// This includes service status, system resources, and error summary.
//
// Usage:
//
//	aerolab agi status -n myagi
type AgiStatusCmd struct {
	Name       TypeAgiClusterName `short:"n" long:"name" description:"AGI instance name" default:"agi"`
	Output     string             `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq)" default:"table"`
	TableTheme string             `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	Pager      bool               `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

// AgiStatusOutput represents the output structure for AGI status command.
type AgiStatusOutput struct {
	Name      string             `json:"name"`
	Label     string             `json:"label"`
	State     string             `json:"state"`
	AccessURL string             `json:"accessURL"`
	Services  []AgiServiceStatus `json:"services"`
	System    AgiSystemStatus    `json:"system"`
	Ingest    AgiIngestStatus    `json:"ingest,omitempty"`
	Errors    []string           `json:"errors,omitempty"`
}

// AgiServiceStatus represents the status of a single service.
type AgiServiceStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Active bool   `json:"active"`
}

// AgiSystemStatus represents system resource status.
type AgiSystemStatus struct {
	DiskTotal   string `json:"diskTotal"`
	DiskUsed    string `json:"diskUsed"`
	DiskFree    string `json:"diskFree"`
	DiskPercent int    `json:"diskPercent"`
	MemTotal    string `json:"memTotal"`
	MemUsed     string `json:"memUsed"`
	MemFree     string `json:"memFree"`
	MemPercent  int    `json:"memPercent"`
}

// AgiIngestStatus represents the ingest progress status.
type AgiIngestStatus struct {
	Running            bool   `json:"running"`
	Step               string `json:"step,omitempty"`
	DownloadProgress   int    `json:"downloadProgress,omitempty"`
	ProcessingProgress int    `json:"processingProgress,omitempty"`
	ErrorCount         int    `json:"errorCount,omitempty"`
}

// Execute implements the command execution for agi status.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiStatusCmd) Execute(args []string) error {
	cmd := []string{"agi", "status"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.ShowStatus(system, system.Backend.GetInventory(), system.Logger, args, os.Stdout)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ShowStatus shows the status of an AGI instance.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//   - out: Output writer
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiStatusCmd) ShowStatus(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, out *os.File) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "status"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Find AGI instance - prefer running instances over stopped/terminated
	allInstances := inventory.Instances.WithTags(map[string]string{
		"aerolab.type": "agi",
	}).WithClusterName(c.Name.String())

	if allInstances.Count() == 0 {
		return fmt.Errorf("AGI instance %s not found", c.Name)
	}

	// Try to find instance by state preference: running > stopped > any
	var instances backends.InstanceList
	running := allInstances.WithState(backends.LifeCycleStateRunning)
	if running.Count() > 0 {
		instances = running.Describe()
	} else {
		stopped := allInstances.WithState(backends.LifeCycleStateStopped)
		if stopped.Count() > 0 {
			instances = stopped.Describe()
		} else {
			instances = allInstances.Describe()
		}
	}

	inst := instances[0]

	// Build output
	output := AgiStatusOutput{
		Name:     inst.ClusterName,
		Label:    decodeBase64Tag(inst.Tags["agiLabel"]),
		State:    inst.InstanceState.String(),
		Services: []AgiServiceStatus{},
		Errors:   []string{},
	}

	// Build access URL
	protocol := "https"
	if inst.Tags["aerolab4ssl"] != "true" {
		protocol = "http"
	}
	ip := inst.IP.Public
	if ip == "" {
		ip = inst.IP.Private
	}
	if ip == "" {
		ip = inst.Name
	}
	output.AccessURL = fmt.Sprintf("%s://%s", protocol, ip)

	// If instance is not running, just show basic info
	if inst.InstanceState != backends.LifeCycleStateRunning {
		return c.renderOutput(system, output, out)
	}

	// Collect all status information in a single SSH call
	statusData := c.collectAllStatus(instances)

	output.Services = statusData.Services
	output.System = statusData.System
	output.Ingest = statusData.Ingest

	return c.renderOutput(system, output, out)
}

// agiCollectedStatus holds all status data collected in a single SSH call.
type agiCollectedStatus struct {
	Services []AgiServiceStatus
	System   AgiSystemStatus
	Ingest   AgiIngestStatus
}

// collectAllStatus collects all status information in a single SSH call.
// This is much more efficient than making separate calls for each piece of data.
func (c *AgiStatusCmd) collectAllStatus(instances backends.InstanceList) agiCollectedStatus {
	result := agiCollectedStatus{
		Services: []AgiServiceStatus{},
		System:   AgiSystemStatus{},
		Ingest:   AgiIngestStatus{},
	}

	// Script that collects all status information and outputs JSON
	// This replaces 10+ separate SSH calls with a single call
	statusScript := `#!/bin/bash
set -e

# Collect service status
services="aerospike grafana-server agi-plugin agi-grafanafix agi-proxy agi-ingest"
echo "SERVICES_START"
for svc in $services; do
    # Try systemctl is-active first (works on AWS/GCP)
    status=$(systemctl is-active "$svc" 2>/dev/null || true)
    if [ -n "$status" ] && [ "$status" != "Unknown command" ]; then
        echo "$svc:$status"
    else
        # Fallback for Docker: parse systemctl status output
        svc_status=$(systemctl status 2>/dev/null | grep -E ": ${svc}(\.service)?$" || true)
        if echo "$svc_status" | grep -q "Running"; then
            echo "$svc:active"
        elif echo "$svc_status" | grep -q "Stopped"; then
            echo "$svc:inactive"
        else
            echo "$svc:unknown"
        fi
    fi
done
echo "SERVICES_END"

# Collect disk usage
echo "DISK_START"
df -h /opt/agi 2>/dev/null | tail -1 || echo "unknown"
echo "DISK_END"

# Collect memory usage
echo "MEM_START"
free -h 2>/dev/null | grep -E "^Mem:" || echo "unknown"
echo "MEM_END"

# Collect ingest status
echo "INGEST_START"
if [ -f /opt/agi/ingest.pid ] && ps -p $(cat /opt/agi/ingest.pid 2>/dev/null) > /dev/null 2>&1; then
    echo "running:true"
else
    echo "running:false"
fi
if [ -f /opt/agi/ingest/steps.json ]; then
    cat /opt/agi/ingest/steps.json 2>/dev/null || echo "{}"
else
    echo "{}"
fi
echo "INGEST_END"
`

	outputs := instances.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"bash", "-c", statusScript},
			SessionTimeout: 30 * time.Second,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	if len(outputs) == 0 || outputs[0].Output.Err != nil {
		// Return defaults if script failed
		services := []string{"aerospike", "grafana-server", "agi-plugin", "agi-grafanafix", "agi-proxy", "agi-ingest"}
		for _, svc := range services {
			result.Services = append(result.Services, AgiServiceStatus{Name: svc, Status: "unknown", Active: false})
		}
		return result
	}

	// Parse the output
	output := string(outputs[0].Output.Stdout)
	lines := strings.Split(output, "\n")

	var section string
	var ingestLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch line {
		case "SERVICES_START":
			section = "services"
			continue
		case "SERVICES_END":
			section = ""
			continue
		case "DISK_START":
			section = "disk"
			continue
		case "DISK_END":
			section = ""
			continue
		case "MEM_START":
			section = "mem"
			continue
		case "MEM_END":
			section = ""
			continue
		case "INGEST_START":
			section = "ingest"
			ingestLines = []string{}
			continue
		case "INGEST_END":
			section = ""
			continue
		}

		if line == "" {
			continue
		}

		switch section {
		case "services":
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				status := AgiServiceStatus{
					Name:   parts[0],
					Status: parts[1],
					Active: parts[1] == "active",
				}
				result.Services = append(result.Services, status)
			}

		case "disk":
			if line != "unknown" {
				fields := strings.Fields(line)
				if len(fields) >= 5 {
					result.System.DiskTotal = fields[1]
					result.System.DiskUsed = fields[2]
					result.System.DiskFree = fields[3]
					pctStr := strings.TrimSuffix(fields[4], "%")
					fmt.Sscanf(pctStr, "%d", &result.System.DiskPercent)
				}
			}

		case "mem":
			if line != "unknown" {
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					result.System.MemTotal = fields[1]
					result.System.MemUsed = fields[2]
					result.System.MemFree = fields[3]
					if len(fields) >= 7 {
						result.System.MemFree = fields[6]
					}
				}
			}

		case "ingest":
			ingestLines = append(ingestLines, line)
		}
	}

	// Parse ingest status
	if len(ingestLines) > 0 {
		if strings.HasPrefix(ingestLines[0], "running:") {
			result.Ingest.Running = ingestLines[0] == "running:true"
		}
		if len(ingestLines) > 1 {
			stepsJSON := strings.Join(ingestLines[1:], "\n")
			var steps struct {
				Init           bool `json:"Init"`
				Download       bool `json:"Download"`
				Unpack         bool `json:"Unpack"`
				PreProcess     bool `json:"PreProcess"`
				ProcessLogs    bool `json:"ProcessLogs"`
				ProcessCollect bool `json:"ProcessCollectInfo"`
			}
			if err := json.Unmarshal([]byte(stepsJSON), &steps); err == nil {
				if steps.ProcessLogs || steps.ProcessCollect {
					result.Ingest.Step = "processing"
				} else if steps.PreProcess {
					result.Ingest.Step = "preprocessing"
				} else if steps.Unpack {
					result.Ingest.Step = "unpacking"
				} else if steps.Download {
					result.Ingest.Step = "downloading"
				} else if steps.Init {
					result.Ingest.Step = "initializing"
				} else {
					result.Ingest.Step = "pending"
				}
			}
		}
	}

	return result
}

// renderOutput renders the status output in the requested format.
func (c *AgiStatusCmd) renderOutput(system *System, output AgiStatusOutput, out *os.File) error {
	var page *pager.Pager
	var err error

	if c.Pager {
		page, err = pager.New(out)
		if err != nil {
			return err
		}
		err = page.Start()
		if err != nil {
			return err
		}
		defer page.Close()
	}

	writer := out
	if page != nil {
		writer = nil // Use page instead
	}

	switch c.Output {
	case "jq":
		params := []string{}
		if page != nil && page.HasColors() {
			params = append(params, "-C")
		}
		cmd := exec.Command("jq", params...)
		if writer != nil {
			cmd.Stdout = writer
			cmd.Stderr = writer
		} else {
			cmd.Stdout = page
			cmd.Stderr = page
		}
		w, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		defer w.Close()
		enc := json.NewEncoder(w)
		go func() {
			enc.Encode(output)
			w.Close()
		}()
		return cmd.Run()

	case "json":
		return json.NewEncoder(out).Encode(output)

	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(output)

	case "text":
		fmt.Fprintf(out, "AGI Instance: %s\n", output.Name)
		fmt.Fprintf(out, "Label: %s\n", output.Label)
		fmt.Fprintf(out, "State: %s\n", output.State)
		fmt.Fprintf(out, "Access URL: %s\n", output.AccessURL)
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "Services:")
		for _, svc := range output.Services {
			fmt.Fprintf(out, "  %s: %s\n", svc.Name, svc.Status)
		}
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "System Resources:")
		fmt.Fprintf(out, "  Disk: %s used / %s total (%d%%)\n", output.System.DiskUsed, output.System.DiskTotal, output.System.DiskPercent)
		fmt.Fprintf(out, "  Memory: %s used / %s total\n", output.System.MemUsed, output.System.MemTotal)
		if output.Ingest.Running {
			fmt.Fprintln(out, "")
			fmt.Fprintf(out, "Ingest: %s\n", output.Ingest.Step)
		}
		return nil

	default: // table
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, nil, page == nil || !page.HasColors(), page != nil)
		if err != nil {
			if err != printer.ErrTerminalWidthUnknown {
				return err
			}
		}

		// Instance info
		fmt.Fprintf(out, "AGI Instance: %s (%s)\n", output.Name, output.State)
		fmt.Fprintf(out, "Label: %s\n", output.Label)
		fmt.Fprintf(out, "Access URL: %s\n\n", output.AccessURL)

		// Services table
		svcHeader := table.Row{"Service", "Status"}
		svcRows := []table.Row{}
		for _, svc := range output.Services {
			status := svc.Status
			if t != nil {
				if svc.Active {
					status = t.ColorHiWhite.Sprint(svc.Status)
				} else if svc.Status == "inactive" || svc.Status == "failed" {
					status = t.ColorErr.Sprint(svc.Status)
				}
			}
			svcRows = append(svcRows, table.Row{svc.Name, status})
		}
		fmt.Fprintln(out, t.RenderTable(printer.String("SERVICES"), svcHeader, svcRows))

		// System resources
		fmt.Fprintln(out, "")
		fmt.Fprintf(out, "Disk: %s used / %s total (%d%% used), %s free\n",
			output.System.DiskUsed, output.System.DiskTotal, output.System.DiskPercent, output.System.DiskFree)
		fmt.Fprintf(out, "Memory: %s used / %s total, %s available\n",
			output.System.MemUsed, output.System.MemTotal, output.System.MemFree)

		// Ingest status
		if output.Ingest.Step != "" {
			fmt.Fprintln(out, "")
			ingestStatus := output.Ingest.Step
			if output.Ingest.Running {
				ingestStatus += " (running)"
			}
			fmt.Fprintf(out, "Ingest: %s\n", ingestStatus)
		}

		fmt.Fprintln(out, "")
		return nil
	}
}

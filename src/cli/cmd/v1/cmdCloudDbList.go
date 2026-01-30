package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rglonek/logger"
)

type CloudClustersListCmd struct {
	Output        string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme    string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy        []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager         bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	StatusNe      string   `short:"n" long:"status-ne" description:"Filter clusters to exclude specified statuses (comma-separated)" default:"decommissioned"`
	WithVPCStatus bool     `short:"v" long:"with-vpc-status" description:"Include VPC peering status for each cluster (requires AWS backend)"`
	Help          HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

// ClusterResponse represents the API response structure
type ClusterResponse struct {
	Count    int       `json:"count"`
	Clusters []Cluster `json:"clusters"`
}

// Cluster represents a single cluster entry
type Cluster struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	AerospikeCloud    AerospikeCloud    `json:"aerospikeCloud"`
	AerospikeServer   AerospikeServer   `json:"aerospikeServer"`
	ConnectionDetails ConnectionDetails `json:"connectionDetails"`
	Health            Health            `json:"health"`
	Infrastructure    Infrastructure    `json:"infrastructure"`
	Logging           ClusterLogging    `json:"logging"`
	CreatedAt         string            `json:"createdAt"`
	UpdatedAt         string            `json:"updatedAt"`
}

// ClusterWithVPCStatus extends Cluster with VPC peering status
type ClusterWithVPCStatus struct {
	Cluster
	VPCPeering *VPCPeeringStatusResponse `json:"vpcPeering,omitempty"`
}

type AerospikeCloud struct {
	ClusterSize int    `json:"clusterSize"`
	DataStorage string `json:"dataStorage"`
}

type AerospikeServer struct {
	Namespaces []Namespace `json:"namespaces"`
}

type Namespace struct {
	Name string `json:"name"`
}

type ConnectionDetails struct {
	Host string `json:"host"`
}

type Health struct {
	State  string `json:"state"`
	Status string `json:"status"`
}

// ClusterLogging represents the logging configuration for a cluster
type ClusterLogging struct {
	LogBucket       string   `json:"logBucket"`
	AuthorizedRoles []string `json:"authorizedRoles"`
}

type Infrastructure struct {
	AvailabilityZoneCount int    `json:"availabilityZoneCount"`
	InstanceType          string `json:"instanceType"`
	Region                string `json:"region"`
}

func (c *CloudClustersListCmd) Execute(args []string) error {
	var system *System
	var err error

	// Only initialize system if we need VPC status
	if c.WithVPCStatus {
		cmd := []string{"cloud", "clusters", "list"}
		system, err = Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
		if system.Opts.Config.Backend.Type != "aws" {
			return Error(fmt.Errorf("--with-vpc-status requires AWS backend"), system, cmd, c, args)
		}
	}

	clusters, rawResult, err := c.GetClusters()
	if err != nil {
		return err
	}

	// Get VPC peering status if requested
	var vpcStatuses map[string]*VPCPeeringStatusResponse
	if c.WithVPCStatus && system != nil {
		vpcStatuses = c.getVPCPeeringStatuses(system, system.Backend.GetInventory(), system.Logger, clusters)
	}

	return c.formatOutput(clusters, rawResult, vpcStatuses, os.Stdout)
}

// GetClusters retrieves the list of Aerospike Cloud clusters without formatting.
// This method can be called by other commands that need access to the cluster data.
//
// Returns:
//   - []Cluster: The list of clusters
//   - map[string]interface{}: The raw JSON response for JSON output formats
//   - error: nil on success, or an error describing what failed
func (c *CloudClustersListCmd) GetClusters() ([]Cluster, map[string]interface{}, error) {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return nil, nil, err
	}

	path := cloudDbPath
	if c.StatusNe != "" {
		path += "?status_ne=" + c.StatusNe
	}

	// Get raw JSON response
	var rawResult map[string]interface{}
	err = client.Get(path, &rawResult)
	if err != nil {
		return nil, nil, err
	}

	// Convert raw result to ClusterResponse
	var result ClusterResponse
	jsonBytes, err := json.Marshal(rawResult)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal raw result: %w", err)
	}
	err = json.Unmarshal(jsonBytes, &result)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal to ClusterResponse: %w", err)
	}

	return result.Clusters, rawResult, nil
}

// ListClusters retrieves and formats Aerospike Cloud clusters for table/text output.
// This method can be called by other commands (like inventory list) to include cloud clusters.
//
// Parameters:
//   - out: The io.Writer to write the output to
//   - page: Optional pager for color support detection
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *CloudClustersListCmd) ListClusters(out io.Writer, page *pager.Pager) error {
	clusters, rawResult, err := c.GetClusters()
	if err != nil {
		return err
	}

	return c.formatOutput(clusters, rawResult, nil, out)
}

// getVPCPeeringStatuses retrieves VPC peering status for all clusters
func (c *CloudClustersListCmd) getVPCPeeringStatuses(system *System, inventory *backends.Inventory, log *logger.Logger, clusters []Cluster) map[string]*VPCPeeringStatusResponse {
	statuses := make(map[string]*VPCPeeringStatusResponse)

	for _, cluster := range clusters {
		statusCmd := &CloudClustersVPCPeeringStatusCmd{
			ClusterID: cluster.ID,
		}
		status, err := statusCmd.GetVPCPeeringStatus(system, inventory, log)
		if err != nil {
			log.Warn("Failed to get VPC peering status for cluster %s: %s", cluster.ID, err)
			continue
		}
		statuses[cluster.ID] = status
	}

	return statuses
}

func (c *CloudClustersListCmd) formatOutput(clusters []Cluster, rawResult map[string]interface{}, vpcStatuses map[string]*VPCPeeringStatusResponse, out io.Writer) error {
	var err error
	var page *pager.Pager

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
		out = page
	}

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
			outputData := c.buildJSONOutput(clusters, rawResult, vpcStatuses)
			enc.Encode(outputData)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		outputData := c.buildJSONOutput(clusters, rawResult, vpcStatuses)
		json.NewEncoder(out).Encode(outputData)
	case "json-indent":
		outputData := c.buildJSONOutput(clusters, rawResult, vpcStatuses)
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(outputData)
	case "text":
		fmt.Fprintln(out, "Aerospike Cloud Clusters:")
		for _, db := range clusters {
			namespaceNames := c.getNamespaceNames(db.AerospikeServer.Namespaces)
			createTime := c.formatTime(db.CreatedAt)
			updateTime := c.formatTime(db.UpdatedAt)
			vpcStatus := ""
			if vpcStatuses != nil {
				if status, ok := vpcStatuses[db.ID]; ok {
					vpcStatus = ", VPCPeering: " + GetVPCPeeringStatusSummary(status)
				}
			}
			logBucket := db.Logging.LogBucket
			if logBucket == "" {
				logBucket = "-"
			}
			fmt.Fprintf(out, "ID: %s, Name: %s, AZCount: %d, ClusterSize: %d, State: %s, Status: %s, DataStorage: %s, NamespaceNames: %s, InstanceType: %s, Region: %s, Host: %s, LogBucket: %s, CreateTime: %s, UpdateTime: %s%s\n",
				db.ID, db.Name, db.Infrastructure.AvailabilityZoneCount, db.AerospikeCloud.ClusterSize,
				db.Health.State, db.Health.Status, db.AerospikeCloud.DataStorage, namespaceNames,
				db.Infrastructure.InstanceType, db.Infrastructure.Region, db.ConnectionDetails.Host,
				logBucket, createTime, updateTime, vpcStatus)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Name:asc", "Region:asc", "State:asc"}
		}
		header := table.Row{"ID", "Name", "AZCount", "ClusterSize", "State", "Status", "DataStorage", "NamespaceNames", "InstanceType", "Region", "Host", "LogBucket", "CreateTime", "UpdateTime"}
		if vpcStatuses != nil {
			header = append(header, "VPCPeering")
		}
		rows := []table.Row{}
		for _, db := range clusters {
			namespaceNames := c.getNamespaceNames(db.AerospikeServer.Namespaces)
			createTime := c.formatTime(db.CreatedAt)
			updateTime := c.formatTime(db.UpdatedAt)
			logBucket := db.Logging.LogBucket
			if logBucket == "" {
				logBucket = "-"
			}
			row := table.Row{
				db.ID,
				db.Name,
				db.Infrastructure.AvailabilityZoneCount,
				db.AerospikeCloud.ClusterSize,
				db.Health.State,
				db.Health.Status,
				db.AerospikeCloud.DataStorage,
				namespaceNames,
				db.Infrastructure.InstanceType,
				db.Infrastructure.Region,
				db.ConnectionDetails.Host,
				logBucket,
				createTime,
				updateTime,
			}
			if vpcStatuses != nil {
				if status, ok := vpcStatuses[db.ID]; ok {
					row = append(row, GetVPCPeeringStatusSummary(status))
				} else {
					row = append(row, "-")
				}
			}
			rows = append(rows, row)
		}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				fmt.Fprintf(os.Stderr, "Warning: Couldn't get terminal width, using default width\n")
			} else {
				return err
			}
		}
		title := printer.String("AEROSPIKE CLOUD CLUSTERS")
		fmt.Fprintln(out, t.RenderTable(title, header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

// buildJSONOutput builds the JSON output with optional VPC peering status
func (c *CloudClustersListCmd) buildJSONOutput(clusters []Cluster, rawResult map[string]interface{}, vpcStatuses map[string]*VPCPeeringStatusResponse) interface{} {
	if vpcStatuses == nil {
		return rawResult
	}

	// Build extended cluster list with VPC status
	extendedClusters := make([]ClusterWithVPCStatus, len(clusters))
	for i, cluster := range clusters {
		extendedClusters[i] = ClusterWithVPCStatus{
			Cluster: cluster,
		}
		if status, ok := vpcStatuses[cluster.ID]; ok {
			extendedClusters[i].VPCPeering = status
		}
	}

	return map[string]interface{}{
		"count":    len(extendedClusters),
		"clusters": extendedClusters,
	}
}

func (c *CloudClustersListCmd) getNamespaceNames(namespaces []Namespace) string {
	if len(namespaces) == 0 {
		return ""
	}
	names := make([]string, len(namespaces))
	for i, ns := range namespaces {
		names[i] = ns.Name
	}
	return strings.Join(names, ",")
}

func (c *CloudClustersListCmd) formatTime(timeStr string) string {
	if timeStr == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		// Try RFC3339 format if RFC3339Nano fails
		t, err = time.Parse(time.RFC3339, timeStr)
		if err != nil {
			return timeStr
		}
	}
	return t.Format("2006-01-02 15:04:05")
}

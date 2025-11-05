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
	"github.com/aerospike/aerolab/pkg/utils/pager"
	"github.com/aerospike/aerolab/pkg/utils/printer"
	"github.com/jedib0t/go-pretty/v6/table"
)

type CloudDatabasesListCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	StatusNe   string   `long:"status-ne" description:"Filter databases to exclude specified statuses (comma-separated)" default:"decommissioned"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

// DatabaseResponse represents the API response structure
type DatabaseResponse struct {
	Count     int        `json:"count"`
	Databases []Database `json:"databases"`
}

// Database represents a single database entry
type Database struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	AerospikeCloud    AerospikeCloud    `json:"aerospikeCloud"`
	AerospikeServer   AerospikeServer   `json:"aerospikeServer"`
	ConnectionDetails ConnectionDetails `json:"connectionDetails"`
	Health            Health            `json:"health"`
	Infrastructure    Infrastructure    `json:"infrastructure"`
	CreatedAt         string            `json:"createdAt"`
	UpdatedAt         string            `json:"updatedAt"`
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

type Infrastructure struct {
	AvailabilityZoneCount int    `json:"availabilityZoneCount"`
	InstanceType          string `json:"instanceType"`
	Region                string `json:"region"`
}

func (c *CloudDatabasesListCmd) Execute(args []string) error {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	var result DatabaseResponse
	path := cloudDbPath
	if c.StatusNe != "" {
		path += "?status_ne=" + c.StatusNe
	}

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return c.formatOutput(result.Databases, os.Stdout)
}

func (c *CloudDatabasesListCmd) formatOutput(databases []Database, out io.Writer) error {
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
			enc.Encode(databases)
			w.Close()
		}()
		err = cmd.Run()
		if err != nil {
			return err
		}
	case "json":
		json.NewEncoder(out).Encode(databases)
	case "json-indent":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.Encode(databases)
	case "text":
		fmt.Fprintln(out, "Databases:")
		for _, db := range databases {
			namespaceNames := c.getNamespaceNames(db.AerospikeServer.Namespaces)
			createTime := c.formatTime(db.CreatedAt)
			updateTime := c.formatTime(db.UpdatedAt)
			fmt.Fprintf(out, "ID: %s, Name: %s, AZCount: %d, ClusterSize: %d, State: %s, Status: %s, DataStorage: %s, NamespaceNames: %s, InstanceType: %s, Region: %s, Host: %s, CreateTime: %s, UpdateTime: %s\n",
				db.ID, db.Name, db.Infrastructure.AvailabilityZoneCount, db.AerospikeCloud.ClusterSize,
				db.Health.State, db.Health.Status, db.AerospikeCloud.DataStorage, namespaceNames,
				db.Infrastructure.InstanceType, db.Infrastructure.Region, db.ConnectionDetails.Host,
				createTime, updateTime)
		}
		fmt.Fprintln(out, "")
	default:
		if len(c.SortBy) == 0 {
			c.SortBy = []string{"Name:asc", "Region:asc", "State:asc"}
		}
		header := table.Row{"ID", "Name", "AZCount", "ClusterSize", "State", "Status", "DataStorage", "NamespaceNames", "InstanceType", "Region", "Host", "CreateTime", "UpdateTime"}
		rows := []table.Row{}
		for _, db := range databases {
			namespaceNames := c.getNamespaceNames(db.AerospikeServer.Namespaces)
			createTime := c.formatTime(db.CreatedAt)
			updateTime := c.formatTime(db.UpdatedAt)
			rows = append(rows, table.Row{
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
				createTime,
				updateTime,
			})
		}
		t, err := printer.GetTableWriter(c.Output, c.TableTheme, c.SortBy, !page.HasColors(), page != nil)
		if err != nil {
			if err == printer.ErrTerminalWidthUnknown {
				fmt.Fprintf(os.Stderr, "Warning: Couldn't get terminal width, using default width\n")
			} else {
				return err
			}
		}
		title := printer.String("DATABASES")
		fmt.Fprintln(out, t.RenderTable(title, header, rows))
		fmt.Fprintln(out, "")
	}
	return nil
}

func (c *CloudDatabasesListCmd) getNamespaceNames(namespaces []Namespace) string {
	if len(namespaces) == 0 {
		return ""
	}
	names := make([]string, len(namespaces))
	for i, ns := range namespaces {
		names[i] = ns.Name
	}
	return strings.Join(names, ",")
}

func (c *CloudDatabasesListCmd) formatTime(timeStr string) string {
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

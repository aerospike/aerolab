package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type CloudClustersVPCPeeringStatusCmd struct {
	ClusterID string  `short:"c" long:"cluster-id" description:"Cluster ID"`
	VPCID     string  `short:"v" long:"vpc-id" description:"VPC ID (optional, checks all peerings if not specified)"`
	Output    string  `short:"o" long:"output" description:"Output format (json, json-indent)" default:"json-indent"`
	Help      HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// VPCPeeringStep represents a single step in the VPC peering process
type VPCPeeringStep struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "OK", "PENDING", "NOT_STARTED", "FAILED", "UNKNOWN"
	Error  string `json:"error,omitempty"`
}

// VPCPeeringStatusEntry represents the status of a single VPC peering
type VPCPeeringStatusEntry struct {
	VpcId          string           `json:"vpcId"`
	PeeringId      string           `json:"peeringId,omitempty"`
	CloudStatus    string           `json:"cloudStatus,omitempty"`
	Status         string           `json:"status"` // "OK", "INCOMPLETE", "FAILED"
	CompletedSteps int              `json:"completedSteps"`
	TotalSteps     int              `json:"totalSteps"`
	Steps          []VPCPeeringStep `json:"steps"`
}

// VPCPeeringStatusResponse represents the full status response
type VPCPeeringStatusResponse struct {
	ClusterID string                  `json:"clusterId"`
	Peerings  []VPCPeeringStatusEntry `json:"peerings"`
}

func (c *CloudClustersVPCPeeringStatusCmd) Execute(args []string) error {
	cmd := []string{"cloud", "clusters", "vpc-peering-status"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	status, err := c.GetVPCPeeringStatus(system, system.Backend.GetInventory(), system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Output the status
	switch c.Output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.Encode(status)
	default: // json-indent
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(status)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// GetVPCPeeringStatus gets the VPC peering status for a cluster
func (c *CloudClustersVPCPeeringStatusCmd) GetVPCPeeringStatus(system *System, inventory *backends.Inventory, log *logger.Logger) (*VPCPeeringStatusResponse, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cloud", "clusters", "vpc-peering-status"}, c)
		if err != nil {
			return nil, err
		}
	}

	if system.Opts.Config.Backend.Type != "aws" {
		return nil, fmt.Errorf("cloud clusters VPC peering status can only be checked with AWS backend")
	}

	if log == nil {
		log = system.Logger
	}

	if c.ClusterID == "" {
		return nil, fmt.Errorf("cluster ID is required")
	}

	// Get peerings from cloud API
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud client: %w", err)
	}

	var peeringsResult interface{}
	peeringsPath := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, c.ClusterID)
	err = client.Get(peeringsPath, &peeringsResult)
	if err != nil {
		return nil, fmt.Errorf("failed to get VPC peerings: %w", err)
	}

	peeringsMap, ok := peeringsResult.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected VPC peerings response type: %T", peeringsResult)
	}

	peerings, ok := peeringsMap["vpcPeerings"].([]interface{})
	if !ok {
		// No peerings exist
		return &VPCPeeringStatusResponse{
			ClusterID: c.ClusterID,
			Peerings:  []VPCPeeringStatusEntry{},
		}, nil
	}

	// Get cluster details for CIDR block
	var clusterResult interface{}
	clusterPath := fmt.Sprintf("%s/%s", cloudDbPath, c.ClusterID)
	err = client.Get(clusterPath, &clusterResult)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster details: %w", err)
	}

	clusterMap, ok := clusterResult.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected cluster response type: %T", clusterResult)
	}

	var clusterCIDR string
	if infrastructure, ok := clusterMap["infrastructure"].(map[string]interface{}); ok {
		if cidr, ok := infrastructure["cidrBlock"].(string); ok {
			clusterCIDR = cidr
		}
	}

	response := &VPCPeeringStatusResponse{
		ClusterID: c.ClusterID,
		Peerings:  []VPCPeeringStatusEntry{},
	}

	for _, peering := range peerings {
		peeringMap, ok := peering.(map[string]interface{})
		if !ok {
			continue
		}

		vpcId, _ := peeringMap["vpcId"].(string)

		// Filter by VPC ID if specified
		if c.VPCID != "" && vpcId != c.VPCID {
			continue
		}

		peeringId, _ := peeringMap["peeringId"].(string)
		cloudStatus, _ := peeringMap["status"].(string)
		hostedZoneId, _ := peeringMap["privateHostedZoneId"].(string)

		entry := c.checkPeeringStatus(system, inventory, log, vpcId, peeringId, cloudStatus, hostedZoneId, clusterCIDR)
		response.Peerings = append(response.Peerings, entry)
	}

	return response, nil
}

// checkPeeringStatus checks the status of a single VPC peering
func (c *CloudClustersVPCPeeringStatusCmd) checkPeeringStatus(system *System, inventory *backends.Inventory, log *logger.Logger, vpcId, peeringId, cloudStatus, hostedZoneId, clusterCIDR string) VPCPeeringStatusEntry {
	entry := VPCPeeringStatusEntry{
		VpcId:       vpcId,
		PeeringId:   peeringId,
		CloudStatus: cloudStatus,
		TotalSteps:  4,
		Steps:       make([]VPCPeeringStep, 4),
	}

	// Step 1: Initiate
	entry.Steps[0] = VPCPeeringStep{Name: "Initiate"}
	if peeringId != "" && cloudStatus != "initiating-request" {
		entry.Steps[0].Status = "OK"
		entry.CompletedSteps++
	} else if cloudStatus == "initiating-request" {
		entry.Steps[0].Status = "PENDING"
	} else {
		entry.Steps[0].Status = "NOT_STARTED"
	}

	// Step 2: Accept
	entry.Steps[1] = VPCPeeringStep{Name: "Accept"}
	if cloudStatus == "active" {
		entry.Steps[1].Status = "OK"
		entry.CompletedSteps++
	} else if cloudStatus == "pending-acceptance" {
		entry.Steps[1].Status = "PENDING"
	} else if cloudStatus == "failed" || cloudStatus == "rejected" {
		entry.Steps[1].Status = "FAILED"
		entry.Steps[1].Error = fmt.Sprintf("peering status is %s", cloudStatus)
	} else if entry.Steps[0].Status == "OK" {
		entry.Steps[1].Status = "NOT_STARTED"
	} else {
		entry.Steps[1].Status = "NOT_STARTED"
	}

	// Step 3: Route
	entry.Steps[2] = VPCPeeringStep{Name: "Route"}
	if peeringId != "" && clusterCIDR != "" {
		routeExists, err := system.Backend.CheckRouteExists(backends.BackendTypeAWS, vpcId, peeringId, clusterCIDR)
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "AccessDenied") || strings.Contains(errStr, "not authorized") || strings.Contains(errStr, "not found in any enabled zone") {
				entry.Steps[2].Status = "UNKNOWN"
				entry.Steps[2].Error = "cannot verify: VPC is in a different AWS account or region"
			} else {
				entry.Steps[2].Status = "UNKNOWN"
				entry.Steps[2].Error = errStr
			}
		} else if routeExists {
			entry.Steps[2].Status = "OK"
			entry.CompletedSteps++
		} else {
			entry.Steps[2].Status = "NOT_STARTED"
		}
	} else {
		entry.Steps[2].Status = "NOT_STARTED"
		if clusterCIDR == "" {
			entry.Steps[2].Error = "cluster CIDR not available"
		}
	}

	// Step 4: Associate DNS
	entry.Steps[3] = VPCPeeringStep{Name: "AssociateDNS"}
	if hostedZoneId != "" {
		associated, err := system.Backend.CheckVPCHostedZoneAssociation(backends.BackendTypeAWS, hostedZoneId, vpcId)
		if err != nil {
			// Check if this is a cross-account access denied error
			errStr := err.Error()
			if strings.Contains(errStr, "AccessDenied") || strings.Contains(errStr, "not authorized") {
				entry.Steps[3].Status = "UNKNOWN"
				entry.Steps[3].Error = "cannot verify: hosted zone is in a different AWS account"
			} else {
				entry.Steps[3].Status = "UNKNOWN"
				entry.Steps[3].Error = errStr
			}
		} else if associated {
			entry.Steps[3].Status = "OK"
			entry.CompletedSteps++
		} else {
			entry.Steps[3].Status = "NOT_STARTED"
		}
	} else {
		entry.Steps[3].Status = "NOT_STARTED"
		entry.Steps[3].Error = "hosted zone ID not available"
	}

	// Determine overall status
	if entry.CompletedSteps == entry.TotalSteps {
		entry.Status = "OK"
	} else {
		// Check for any failures
		hasFailed := false
		for _, step := range entry.Steps {
			if step.Status == "FAILED" {
				hasFailed = true
				break
			}
		}
		if hasFailed {
			entry.Status = "FAILED"
		} else {
			entry.Status = "INCOMPLETE"
		}
	}

	return entry
}

// GetVPCPeeringStatusSummary returns a summary string for table display (e.g., "OK" or "3/4")
func GetVPCPeeringStatusSummary(status *VPCPeeringStatusResponse) string {
	if status == nil || len(status.Peerings) == 0 {
		return "-"
	}

	allOK := true
	totalCompleted := 0
	totalSteps := 0

	for _, peering := range status.Peerings {
		totalCompleted += peering.CompletedSteps
		totalSteps += peering.TotalSteps
		if peering.Status != "OK" {
			allOK = false
		}
	}

	if allOK {
		return "OK"
	}

	return fmt.Sprintf("%d/%d", totalCompleted, totalSteps)
}

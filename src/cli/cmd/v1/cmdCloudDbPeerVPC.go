package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/rglonek/logger"
)

type CloudClustersPeerVPCCmd struct {
	ClusterID          string  `short:"c" long:"cluster-id" description:"Cluster ID"`
	Region             string  `short:"r" long:"region" description:"VPC region (auto-detected from VPC if not specified)"`
	VPCID              string  `short:"v" long:"vpc-id" description:"VPC ID to peer the cluster to" default:"default"`
	StageInitiate      bool    `short:"1" long:"stage-initiate" description:"Execute the initiate stage (request VPC peering from cloud). If no stages are specified, all stages are executed."`
	StageAccept        bool    `short:"2" long:"stage-accept" description:"Execute the accept stage (accept the VPC peering request). If no stages are specified, all stages are executed."`
	StageRoute         bool    `short:"3" long:"stage-route" description:"Execute the route stage (create route in VPC route table). If no stages are specified, all stages are executed."`
	StageAssociateDNS  bool    `short:"4" long:"stage-associate-dns" description:"Execute the DNS association stage (associate VPC with hosted zone). If no stages are specified, all stages are executed."`
	ForceRouteCreation bool    `short:"f" long:"force-route-creation" description:"Force route creation even if it already exists"`
	Help               HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`

	// Internal fields for programmatic use (not CLI flags)
	// These allow callers like db create to pass pre-resolved values
	PreResolvedVPCCIDR   string // VPC CIDR if already known
	PreResolvedAccountID string // AWS account ID if already known
	PreResolvedVPCRegion string // VPC region if already known
	CleanupOnError       func() // Cleanup function to call on error (e.g., blackhole route cleanup)
}

func (c *CloudClustersPeerVPCCmd) Execute(args []string) error {
	cmd := []string{"cloud", "clusters", "peer-vpc"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	var stdout, stderr *os.File
	var stdin io.ReadCloser
	if system.logLevel >= 5 {
		stdout = os.Stdout
		stderr = os.Stderr
		stdin = io.NopCloser(os.Stdin)
	}
	err = c.PeerVPC(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *CloudClustersPeerVPCCmd) PeerVPC(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) error {
	if c.ClusterID == "" {
		return fmt.Errorf("cluster ID is required")
	}
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cloud", "clusters", "peer-vpc"}, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != "aws" {
		return fmt.Errorf("cloud clusters VPC peering can only be setup with AWS backend")
	}
	if logger == nil {
		logger = system.Logger
	}

	// Determine if this is a programmatic call from create (pre-resolved values provided)
	calledFromCreate := c.PreResolvedVPCCIDR != "" && c.PreResolvedAccountID != "" && c.PreResolvedVPCRegion != ""

	if !calledFromCreate {
		logger.Info("Setting up VPC peering for cluster: %s", c.ClusterID)
	}

	// Determine if we should run all stages or only specific ones
	runAllStages := !c.StageInitiate && !c.StageAccept && !c.StageRoute && !c.StageAssociateDNS
	if runAllStages && !calledFromCreate {
		logger.Info("No specific stages specified, will attempt all stages (skipping already completed stages)")
	} else if !runAllStages {
		var stages []string
		if c.StageInitiate {
			stages = append(stages, "initiate")
		}
		if c.StageAccept {
			stages = append(stages, "accept")
		}
		if c.StageRoute {
			stages = append(stages, "route")
		}
		if c.StageAssociateDNS {
			stages = append(stages, "associate-dns")
		}
		logger.Info("Running specific stages: %v", stages)
	}

	// VPC resolution
	if c.VPCID == "default" {
		logger.Info("Resolving default VPC")
		if inventory == nil {
			inventory = system.Backend.GetInventory()
		}
		for _, network := range inventory.Networks.Describe() {
			if network.IsDefault {
				c.VPCID = network.NetworkId
				break
			}
		}
		if c.VPCID == "default" {
			return fmt.Errorf("default VPC not found")
		}
	}

	// Get VPC details (needed for most stages)
	// Use pre-resolved values if provided (from db create), otherwise fetch from inventory
	var cidr string
	var accountId string
	var vpcRegion string
	var err error

	if calledFromCreate {
		// Use pre-resolved values from caller (e.g., db create)
		cidr = c.PreResolvedVPCCIDR
		accountId = c.PreResolvedAccountID
		vpcRegion = c.PreResolvedVPCRegion
		logger.Debug("Using pre-resolved VPC details: cidr=%s, accountId=%s, region=%s", cidr, accountId, vpcRegion)
	} else {
		// Fetch VPC details from inventory
		logger.Info("Getting VPC details for VPC-ID: %s", c.VPCID)
		if inventory == nil {
			inventory = system.Backend.GetInventory()
		}
		for _, network := range inventory.Networks.Describe() {
			if network.NetworkId == c.VPCID {
				cidr = network.Cidr
				vpcRegion = network.ZoneName
				break
			}
		}
		if cidr == "" {
			c.callCleanup()
			return fmt.Errorf("VPC %s not found", c.VPCID)
		}
		if vpcRegion == "" {
			if c.Region != "" {
				logger.Warn("Could not determine VPC region from zone name, using provided region: %s", c.Region)
				vpcRegion = c.Region
			} else {
				c.callCleanup()
				return fmt.Errorf("could not determine VPC region and no --region flag provided")
			}
		} else {
			logger.Info("VPC region: %s", vpcRegion)
		}
		accountId, err = system.Backend.GetAccountID(backends.BackendTypeAWS)
		if err != nil {
			c.callCleanup()
			return err
		}
		if accountId == "" {
			c.callCleanup()
			return fmt.Errorf("account ID not found")
		}
	}

	// Wait for cluster to be ready before proceeding with VPC peering
	// The cluster needs to be provisioned before we can initiate VPC peering
	err = c.waitForClusterReady(c.ClusterID, logger)
	if err != nil {
		c.callCleanup()
		return fmt.Errorf("failed waiting for cluster to be ready: %w", err)
	}

	// Check existing VPC peerings to determine current state
	existingPeerings, err := c.getExistingPeerings(c.ClusterID)
	if err != nil {
		c.callCleanup()
		return fmt.Errorf("failed to get existing VPC peerings: %w", err)
	}

	// Check if peering already exists and get its status
	var existingPeering map[string]interface{}
	var peeringConnectionID string
	for _, peering := range existingPeerings {
		if peering["vpcId"] == c.VPCID {
			existingPeering = peering
			if peeringId, ok := peering["peeringId"].(string); ok {
				peeringConnectionID = peeringId
			}
			break
		}
	}

	// Determine peering status
	var peeringStatus string
	if existingPeering != nil {
		if status, ok := existingPeering["status"].(string); ok {
			peeringStatus = status
		}
		logger.Info("Found existing VPC peering: %s (status: %s)", peeringConnectionID, peeringStatus)
	}

	// Stage 1: Initiate
	if runAllStages || c.StageInitiate {
		if existingPeering != nil {
			logger.Info("Stage INITIATE: Skipping - VPC peering already exists (peeringId: %s)", peeringConnectionID)
		} else {
			logger.Info("Stage INITIATE: Initiating VPC peering for cluster=%s, vpcId=%s, cidr=%s, accountId=%s, vpcRegion=%s",
				c.ClusterID, c.VPCID, cidr, accountId, vpcRegion)

			reqId, err := c.initiateVPCPeering(c.ClusterID, cidr, accountId, vpcRegion, logger)
			if err != nil {
				c.callCleanup()
				return fmt.Errorf("stage INITIATE failed: %w", err)
			}
			logger.Info("Stage INITIATE: Completed - VPC peering initiated, peeringId: %s", reqId)
			peeringConnectionID = reqId
			peeringStatus = "pending-acceptance"
		}
	}

	// Stage 2: Accept
	if runAllStages || c.StageAccept {
		if peeringConnectionID == "" {
			if c.StageAccept {
				c.callCleanup()
				return fmt.Errorf("stage ACCEPT failed: no existing VPC peering found for VPC %s", c.VPCID)
			}
			logger.Info("Stage ACCEPT: Skipping - no peering connection ID available")
		} else if peeringStatus == "active" {
			logger.Info("Stage ACCEPT: Skipping - VPC peering is already active")
		} else {
			logger.Info("Stage ACCEPT: Accepting VPC peering: %s", peeringConnectionID)
			err = c.retry(func() error {
				return system.Backend.AcceptVPCPeering(backends.BackendTypeAWS, peeringConnectionID)
			}, c.ClusterID)
			if err != nil {
				// Check if already accepted
				if strings.Contains(err.Error(), "already active") {
					logger.Info("Stage ACCEPT: VPC peering is already active")
				} else {
					c.callCleanup()
					return fmt.Errorf("stage ACCEPT failed: %w", err)
				}
			} else {
				logger.Info("Stage ACCEPT: Completed - VPC peering accepted")
			}
			peeringStatus = "active"
		}
	}

	// Stage 3: Route
	if runAllStages || c.StageRoute {
		if peeringConnectionID == "" {
			if c.StageRoute {
				// Try to find the peering connection from existing peerings
				for _, peering := range existingPeerings {
					if peering["vpcId"] == c.VPCID {
						if peeringId, ok := peering["peeringId"].(string); ok && peeringId != "" {
							peeringConnectionID = peeringId
							break
						}
					}
				}
				if peeringConnectionID == "" {
					c.callCleanup()
					return fmt.Errorf("stage ROUTE failed: no existing VPC peering found for VPC %s", c.VPCID)
				}
			} else {
				logger.Info("Stage ROUTE: Skipping - no peering connection ID available")
			}
		}

		if peeringConnectionID != "" {
			logger.Info("Stage ROUTE: Creating route in VPC route table")
			err = c.createRoute(system, logger, peeringConnectionID)
			if err != nil {
				c.callCleanup()
				return fmt.Errorf("stage ROUTE failed: %w", err)
			}
			// Route created successfully - if cleanup was provided, the blackhole has been replaced
			// Clear the cleanup function so it won't be called on subsequent errors
			c.CleanupOnError = nil
			logger.Info("Stage ROUTE: Completed")
		}
	}

	// Stage 4: Associate DNS
	if runAllStages || c.StageAssociateDNS {
		logger.Info("Stage ASSOCIATE-DNS: Associating VPC with hosted zone")
		err = c.associateHostedZone(system, logger, vpcRegion)
		if err != nil {
			return fmt.Errorf("stage ASSOCIATE-DNS failed: %w", err)
		}
		logger.Info("Stage ASSOCIATE-DNS: Completed")
	}

	return nil
}

// waitForClusterReady waits for the cluster to finish provisioning
// VPC peering can only be initiated after the cluster is ready
func (c *CloudClustersPeerVPCCmd) waitForClusterReady(clusterID string, log *logger.Logger) error {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	timeout := time.Hour * 2
	interval := 1 * time.Minute
	startTime := time.Now()
	loggedWaiting := false
	firstLoop := true

	for {
		// Sleep at the beginning to avoid immediate API calls right after cluster creation
		if !firstLoop {
			time.Sleep(interval)
		}
		firstLoop = false

		if time.Since(startTime) > timeout {
			return fmt.Errorf("timeout waiting for cluster to be ready after %v", timeout)
		}

		var result map[string]interface{}
		path := fmt.Sprintf("%s/%s", cloudDbPath, clusterID)
		err := client.Get(path, &result)
		if err != nil {
			log.Debug("Failed to get cluster status: %s, retrying...", err)
			continue
		}

		// Check health.status
		health, ok := result["health"].(map[string]interface{})
		if !ok {
			log.Debug("Health field not found in cluster response, retrying...")
			continue
		}

		status, ok := health["status"].(string)
		if !ok {
			log.Debug("Health status field not found, retrying...")
			continue
		}

		log.Debug("Cluster status: %s", status)

		if status != "provisioning" {
			if loggedWaiting {
				log.Info("Cluster is now ready (status: %s)", status)
			}
			return nil
		}

		if !loggedWaiting {
			log.Info("Cluster is still provisioning, waiting...")
			loggedWaiting = true
		}
	}
}

func (c *CloudClustersPeerVPCCmd) getExistingPeerings(clusterID string) ([]map[string]interface{}, error) {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return nil, err
	}

	var result interface{}
	path := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, clusterID)
	err = client.Get(path, &result)
	if err != nil {
		return nil, err
	}

	// Parse the result to extract peerings
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format")
	}

	peerings, ok := resultMap["vpcPeerings"].([]interface{})
	if !ok {
		// If no peerings exist, return empty slice
		return []map[string]interface{}{}, nil
	}

	var existingPeerings []map[string]interface{}
	for _, peering := range peerings {
		if peeringMap, ok := peering.(map[string]interface{}); ok {
			existingPeerings = append(existingPeerings, peeringMap)
		}
	}

	return existingPeerings, nil
}

func (c *CloudClustersPeerVPCCmd) initiateVPCPeering(clusterID string, cidr string, accountId string, vpcRegion string, log *logger.Logger) (string, error) {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return "", err
	}

	request := cloud.CreateVPCPeeringRequest{
		VpcID:              c.VPCID,
		CIDRBlock:          cidr,
		AccountID:          accountId,
		Region:             vpcRegion,
		IsSecureConnection: true,
	}
	var result interface{}

	path := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, clusterID)
	err = client.Post(path, request, &result)
	if err != nil {
		return "", err
	}

	// Log the result for debugging
	resultJson, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Error("failed to marshal VPC peering initiation result for logging purposes: %s", err.Error())
	}
	log.Debug("VPC peering initiation result:\n%s", string(resultJson))

	// Wait for the peering to be initiated so that we can get the peeringId from it
	// The post response doesn't include the peeringId, it's all async
	peeringId, err := c.waitForVPCPeeringInitiated(client, clusterID, c.VPCID, log)
	if err != nil {
		return "", fmt.Errorf("failed to wait for VPC peering initiation: %w", err)
	}

	return peeringId, nil
}

// waitForVPCPeeringInitiated waits for the VPC peering to be initiated
// It polls the VPC peerings list until status != "initiating-request" and peeringId != ""
func (c *CloudClustersPeerVPCCmd) waitForVPCPeeringInitiated(client *cloud.Client, clusterID string, vpcId string, log *logger.Logger) (string, error) {
	timeout := time.Hour
	interval := 10 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeout {
			return "", fmt.Errorf("timeout waiting for VPC peering initiation after %v", timeout)
		}

		var result interface{}
		path := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, clusterID)
		err := client.Get(path, &result)
		if err != nil {
			return "", fmt.Errorf("failed to get VPC peerings list: %w", err)
		}

		resultMap, ok := result.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("unexpected response type from peerings list: %T", result)
		}

		peerings, ok := resultMap["vpcPeerings"].([]interface{})
		if !ok {
			// Try alternative field name
			peerings, ok = resultMap["vpc_peerings"].([]interface{})
		}
		if !ok {
			log.Debug("No peerings found yet, waiting %v...", interval)
			time.Sleep(interval)
			continue
		}

		// Find the peering that matches our VPC ID
		for _, peering := range peerings {
			peeringMap, ok := peering.(map[string]interface{})
			if !ok {
				continue
			}

			// Check if this peering matches our VPC ID
			peeringVpcID, ok := peeringMap["vpcId"].(string)
			if !ok {
				// Try alternative field name
				peeringVpcID, _ = peeringMap["vpc_id"].(string)
			}

			if peeringVpcID == vpcId {
				// Found our peering, check status and peeringId
				status, _ := peeringMap["status"].(string)
				peeringId, _ := peeringMap["peeringId"].(string)

				log.Debug("VPC peering status: %s, peeringId: %s", status, peeringId)

				if status != "initiating-request" && peeringId != "" {
					log.Info("VPC peering initiated, peeringId: %s, status: %s", peeringId, status)
					return peeringId, nil
				}

				log.Debug("VPC peering still initiating, waiting %v...", interval)
				time.Sleep(interval)
				break
			}
		}

		// Peering not found yet, wait and retry
		log.Debug("VPC peering not found in list yet, waiting %v...", interval)
		time.Sleep(interval)
	}
}

func (c *CloudClustersPeerVPCCmd) retry(fn func() error, clusterID string) error {
	for {
		err := fn()
		if err == nil {
			return nil
		}
		if !IsInteractive() {
			// Don't call cleanup here - let the caller handle it
			// since they know whether this is before or after route creation
			return err
		}
		logger.Error("%s", err.Error())
		opts, quitting, quittingErr := choice.Choice("Retry VPC Peering, Quit, or Rollback Cluster?", choice.Items{
			choice.Item("Retry"),
			choice.Item("Quit"),
			choice.Item("Rollback"),
		})
		if quittingErr != nil {
			return fmt.Errorf("failed to get user choice: %s", quittingErr.Error())
		}
		if quitting || opts == "Quit" {
			return errors.New("user chose to quit")
		}
		if opts == "Rollback" {
			c.callCleanup()
			delCluster := &CloudClustersDeleteCmd{
				ClusterID: clusterID,
			}
			rollbackErr := delCluster.Execute(nil)
			if rollbackErr != nil {
				return fmt.Errorf("failed to rollback cluster: %s", rollbackErr.Error())
			}
			return err
		}
		if opts == "Retry" {
			continue
		}
	}
}

// createRoute creates a route in the VPC route table
func (c *CloudClustersPeerVPCCmd) createRoute(system *System, logger *logger.Logger, peeringConnectionID string) error {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	// Get the cluster to extract CIDR block
	logger.Info("Getting cluster information to extract CIDR block...")
	var clusterResult interface{}
	clusterPath := fmt.Sprintf("%s/%s", cloudDbPath, c.ClusterID)
	err = client.Get(clusterPath, &clusterResult)
	if err != nil {
		return fmt.Errorf("failed to get cluster information: %w", err)
	}

	// Extract CIDR block from infrastructure
	clusterMap, ok := clusterResult.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected cluster response type: %T", clusterResult)
	}

	infrastructure, ok := clusterMap["infrastructure"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("infrastructure field not found or invalid in cluster response")
	}

	cidrBlock, ok := infrastructure["cidrBlock"].(string)
	if !ok || cidrBlock == "" {
		return fmt.Errorf("cidrBlock field not found or invalid in infrastructure")
	}

	logger.Info("Found CIDR block: %s", cidrBlock)

	// Create route in the VPC
	logger.Info("Creating route in VPC %s for CIDR block %s via peering connection %s", c.VPCID, cidrBlock, peeringConnectionID)
	err = c.retry(func() error {
		return system.Backend.CreateRoute(backends.BackendTypeAWS, c.VPCID, peeringConnectionID, cidrBlock, c.ForceRouteCreation)
	}, c.ClusterID)
	if err != nil {
		return fmt.Errorf("failed to create route: %w", err)
	}
	logger.Info("Route created successfully")
	return nil
}

// associateHostedZone associates the VPC with the hosted zone
func (c *CloudClustersPeerVPCCmd) associateHostedZone(system *System, logger *logger.Logger, vpcRegion string) error {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	// Get VPC peerings list to extract hosted zone ID
	logger.Info("Getting VPC peerings list to extract hosted zone ID...")
	var peeringsResult interface{}
	peeringsPath := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, c.ClusterID)
	err = client.Get(peeringsPath, &peeringsResult)
	if err != nil {
		return fmt.Errorf("failed to get VPC peerings list: %w", err)
	}

	peeringsMap, ok := peeringsResult.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected VPC peerings response type: %T", peeringsResult)
	}

	peerings, ok := peeringsMap["vpcPeerings"].([]interface{})
	if !ok {
		return fmt.Errorf("vpcPeerings field not found or invalid in response")
	}

	// Find the peering that matches our VPC ID
	var hostedZoneID string
	for _, peering := range peerings {
		peeringMap, ok := peering.(map[string]interface{})
		if !ok {
			continue
		}

		peeringVpcID, ok := peeringMap["vpcId"].(string)
		if !ok || peeringVpcID != c.VPCID {
			continue
		}

		// Found our peering, extract hosted zone ID
		if privateHostedZoneId, ok := peeringMap["privateHostedZoneId"].(string); ok && privateHostedZoneId != "" {
			hostedZoneID = privateHostedZoneId
			logger.Info("Found hosted zone ID: %s", hostedZoneID)
			break
		}
	}

	if hostedZoneID == "" {
		logger.Warn("Hosted zone ID not found in VPC peerings, skipping VPC-hosted zone association")
		return nil
	}

	// Associate VPC with hosted zone
	logger.Info("Associating VPC %s with hosted zone %s in region %s", c.VPCID, hostedZoneID, vpcRegion)
	err = c.retry(func() error {
		return system.Backend.AssociateVPCWithHostedZone(backends.BackendTypeAWS, hostedZoneID, c.VPCID, vpcRegion)
	}, c.ClusterID)
	if err != nil {
		return fmt.Errorf("failed to associate VPC with hosted zone: %w", err)
	}
	logger.Info("VPC-hosted zone association completed successfully")
	return nil
}

// callCleanup calls the cleanup function if one was provided
func (c *CloudClustersPeerVPCCmd) callCleanup() {
	if c.CleanupOnError != nil {
		c.CleanupOnError()
	}
}

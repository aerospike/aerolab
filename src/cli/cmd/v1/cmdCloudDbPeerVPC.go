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

type CloudDatabasesPeerVPCCmd struct {
	DatabaseID         string  `short:"d" long:"database-id" description:"Database ID"`
	VPCID              string  `long:"vpc-id" description:"VPC ID to peer the database to" default:"default"`
	Region             string  `short:"r" long:"region" description:"VPC region (auto-detected from VPC if not specified)"`
	StageInitiate      bool    `long:"stage-initiate" description:"Execute the initiate stage (request VPC peering from cloud). If no stages are specified, all stages are executed."`
	StageAccept        bool    `long:"stage-accept" description:"Execute the accept stage (accept the VPC peering request). If no stages are specified, all stages are executed."`
	StageRoute         bool    `long:"stage-route" description:"Execute the route stage (create route in VPC route table). If no stages are specified, all stages are executed."`
	StageAssociateDNS  bool    `long:"stage-associate-dns" description:"Execute the DNS association stage (associate VPC with hosted zone). If no stages are specified, all stages are executed."`
	ForceRouteCreation bool    `long:"force-route-creation" description:"Force route creation even if it already exists"`
	Help               HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudDatabasesPeerVPCCmd) Execute(args []string) error {
	cmd := []string{"cloud", "db", "peer-vpc"}
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

func (c *CloudDatabasesPeerVPCCmd) PeerVPC(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) error {
	if c.DatabaseID == "" {
		return fmt.Errorf("database ID is required")
	}
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cloud", "db", "peer-vpc"}, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != "aws" {
		return fmt.Errorf("cloud databases VPC peering can only be setup with AWS backend")
	}
	if logger == nil {
		logger = system.Logger
	}

	logger.Info("Setting up VPC peering for database: %s", c.DatabaseID)

	// Determine if we should run all stages or only specific ones
	runAllStages := !c.StageInitiate && !c.StageAccept && !c.StageRoute && !c.StageAssociateDNS
	if runAllStages {
		logger.Info("No specific stages specified, will attempt all stages (skipping already completed stages)")
	} else {
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
	var cidr string
	var accountId string
	var vpcRegion string
	var err error
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
		return fmt.Errorf("VPC %s not found", c.VPCID)
	}
	if vpcRegion == "" {
		if c.Region != "" {
			logger.Warn("Could not determine VPC region from zone name, using provided region: %s", c.Region)
			vpcRegion = c.Region
		} else {
			return fmt.Errorf("could not determine VPC region and no --region flag provided")
		}
	} else {
		logger.Info("VPC region: %s", vpcRegion)
	}
	accountId, err = system.Backend.GetAccountID(backends.BackendTypeAWS)
	if err != nil {
		return err
	}
	if accountId == "" {
		return fmt.Errorf("account ID not found")
	}

	// Check existing VPC peerings to determine current state
	existingPeerings, err := c.getExistingPeerings(c.DatabaseID)
	if err != nil {
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
			logger.Info("Stage INITIATE: Initiating VPC peering for database=%s, vpcId=%s, cidr=%s, accountId=%s, vpcRegion=%s",
				c.DatabaseID, c.VPCID, cidr, accountId, vpcRegion)

			reqId, err := c.initiateVPCPeering(c.DatabaseID, cidr, accountId, vpcRegion, logger)
			if err != nil {
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
				return fmt.Errorf("stage ACCEPT failed: no existing VPC peering found for VPC %s", c.VPCID)
			}
			logger.Info("Stage ACCEPT: Skipping - no peering connection ID available")
		} else if peeringStatus == "active" {
			logger.Info("Stage ACCEPT: Skipping - VPC peering is already active")
		} else {
			logger.Info("Stage ACCEPT: Accepting VPC peering: %s", peeringConnectionID)
			err = c.retry(func() error {
				return system.Backend.AcceptVPCPeering(backends.BackendTypeAWS, peeringConnectionID)
			}, c.DatabaseID)
			if err != nil {
				// Check if already accepted
				if strings.Contains(err.Error(), "already active") {
					logger.Info("Stage ACCEPT: VPC peering is already active")
				} else {
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
				return fmt.Errorf("stage ROUTE failed: %w", err)
			}
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

func (c *CloudDatabasesPeerVPCCmd) getExistingPeerings(databaseID string) ([]map[string]interface{}, error) {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return nil, err
	}

	var result interface{}
	path := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, databaseID)
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

func (c *CloudDatabasesPeerVPCCmd) initiateVPCPeering(databaseID string, cidr string, accountId string, vpcRegion string, log *logger.Logger) (string, error) {
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

	path := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, databaseID)
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
	peeringId, err := c.waitForVPCPeeringInitiated(client, databaseID, c.VPCID, log)
	if err != nil {
		return "", fmt.Errorf("failed to wait for VPC peering initiation: %w", err)
	}

	return peeringId, nil
}

// waitForVPCPeeringInitiated waits for the VPC peering to be initiated
// It polls the VPC peerings list until status != "initiating-request" and peeringId != ""
func (c *CloudDatabasesPeerVPCCmd) waitForVPCPeeringInitiated(client *cloud.Client, dbId string, vpcId string, log *logger.Logger) (string, error) {
	timeout := time.Hour
	interval := 10 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeout {
			return "", fmt.Errorf("timeout waiting for VPC peering initiation after %v", timeout)
		}

		var result interface{}
		path := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, dbId)
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

func (c *CloudDatabasesPeerVPCCmd) retry(fn func() error, dbId string) error {
	for {
		err := fn()
		if err == nil {
			return nil
		}
		if !IsInteractive() {
			return err
		}
		logger.Error("%s", err.Error())
		opts, quitting, quittingErr := choice.Choice("Retry VPC Peering, Quit, or Rollback Database?", choice.Items{
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
			delDb := &CloudDatabasesDeleteCmd{
				DatabaseID: dbId,
			}
			rollbackErr := delDb.Execute(nil)
			if rollbackErr != nil {
				return fmt.Errorf("failed to rollback database: %s", rollbackErr.Error())
			}
			return err
		}
		if opts == "Retry" {
			continue
		}
	}
}

// createRoute creates a route in the VPC route table
func (c *CloudDatabasesPeerVPCCmd) createRoute(system *System, logger *logger.Logger, peeringConnectionID string) error {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	// Get the database to extract CIDR block
	logger.Info("Getting database information to extract CIDR block...")
	var dbResult interface{}
	dbPath := fmt.Sprintf("%s/%s", cloudDbPath, c.DatabaseID)
	err = client.Get(dbPath, &dbResult)
	if err != nil {
		return fmt.Errorf("failed to get database information: %w", err)
	}

	// Extract CIDR block from infrastructure
	dbMap, ok := dbResult.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected database response type: %T", dbResult)
	}

	infrastructure, ok := dbMap["infrastructure"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("infrastructure field not found or invalid in database response")
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
	}, c.DatabaseID)
	if err != nil {
		return fmt.Errorf("failed to create route: %w", err)
	}
	logger.Info("Route created successfully")
	return nil
}

// associateHostedZone associates the VPC with the hosted zone
func (c *CloudDatabasesPeerVPCCmd) associateHostedZone(system *System, logger *logger.Logger, vpcRegion string) error {
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	// Get VPC peerings list to extract hosted zone ID
	logger.Info("Getting VPC peerings list to extract hosted zone ID...")
	var peeringsResult interface{}
	peeringsPath := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, c.DatabaseID)
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
	}, c.DatabaseID)
	if err != nil {
		return fmt.Errorf("failed to associate VPC with hosted zone: %w", err)
	}
	logger.Info("VPC-hosted zone association completed successfully")
	return nil
}

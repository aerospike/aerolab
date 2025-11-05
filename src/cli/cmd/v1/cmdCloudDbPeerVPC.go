package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/choice"
	"github.com/rglonek/logger"
)

type CloudDatabasesPeerVPCCmd struct {
	DatabaseID    string  `short:"d" long:"database-id" description:"Database ID" required:"true"`
	VPCID         string  `long:"vpc-id" description:"VPC ID to peer the database to" default:"default"`
	Region        string  `short:"r" long:"region" description:"Region" required:"true"`
	InitiateOnly  bool    `long:"initiate-only" description:"Only initiate peering, do not accept"`
	AcceptOnly    bool    `long:"accept-only" description:"Only accept existing peering, do not initiate"`
	RouteOnly     bool    `long:"route-only" description:"Only create route in VPC route table"`
	AssociateOnly bool    `long:"associate-only" description:"Only associate VPC with hosted zone"`
	Help          HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
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

	// Handle route-only mode
	if c.RouteOnly {
		return c.createRouteOnly(system, logger)
	}

	// Handle associate-only mode
	if c.AssociateOnly {
		return c.associateHostedZoneOnly(system, logger)
	}

	// Get VPC details
	var cidr string
	var accountId string
	var err error
	logger.Info("Getting VPC details for VPC-ID: %s", c.VPCID)
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	for _, network := range inventory.Networks.Describe() {
		if network.NetworkId == c.VPCID {
			cidr = network.Cidr
			break
		}
	}
	if cidr == "" {
		return fmt.Errorf("VPC %s not found", c.VPCID)
	}
	accountId, err = system.Backend.GetAccountID(backends.BackendTypeAWS)
	if err != nil {
		return err
	}
	if accountId == "" {
		return fmt.Errorf("account ID not found")
	}

	// Check existing VPC peerings
	existingPeerings, err := c.getExistingPeerings(c.DatabaseID)
	if err != nil {
		return fmt.Errorf("failed to get existing VPC peerings: %w", err)
	}

	// Check if peering already exists
	peeringExists := false
	var existingPeeringId string
	for _, peering := range existingPeerings {
		if peering["vpcId"] == c.VPCID {
			peeringExists = true
			existingPeeringId = peering["peeringId"].(string)
			logger.Info("Found existing VPC peering: %s", existingPeeringId)
			break
		}
	}

	var peeringConnectionID string

	// Initiate peering if it doesn't exist and not accept-only
	if !peeringExists && !c.AcceptOnly {
		logger.Info("Initiating VPC peering for database=%s, vpcId=%s, cidr=%s, accountId=%s, region=%s, isSecureConnection=%t",
			c.DatabaseID, c.VPCID, cidr, accountId, c.Region, true)

		reqId, err := c.initiateVPCPeering(c.DatabaseID, cidr, accountId)
		if err != nil {
			return fmt.Errorf("failed to initiate VPC peering: %w", err)
		}
		logger.Info("VPC peering initiated, reqId: %s", reqId)
		peeringConnectionID = reqId

		// Accept peering if not initiate-only
		if !c.InitiateOnly {
			logger.Info("Accepting VPC peering for reqId: %s", reqId)
			err = c.retry(func() error {
				return system.Backend.AcceptVPCPeering(backends.BackendTypeAWS, reqId)
			}, c.DatabaseID)
			if err != nil {
				return fmt.Errorf("failed to accept VPC peering: %w", err)
			}
			logger.Info("VPC peering accepted, reqId: %s", reqId)
		}
	} else if peeringExists && !c.InitiateOnly {
		// Accept existing peering if not initiate-only
		logger.Info("Accepting existing VPC peering: %s", existingPeeringId)
		err = c.retry(func() error {
			return system.Backend.AcceptVPCPeering(backends.BackendTypeAWS, existingPeeringId)
		}, c.DatabaseID)
		if err != nil {
			return fmt.Errorf("failed to accept existing VPC peering: %w", err)
		}
		logger.Info("VPC peering accepted, reqId: %s", existingPeeringId)
		peeringConnectionID = existingPeeringId
	} else if peeringExists && c.AcceptOnly {
		logger.Info("VPC peering already exists: %s", existingPeeringId)
		peeringConnectionID = existingPeeringId
	} else if !peeringExists && c.InitiateOnly {
		logger.Info("VPC peering initiated but not accepted (initiate-only mode)")
		// Don't set peeringConnectionID - we can't create routes or associate until peering is accepted
	}

	// Create route and associate hosted zone if peering was accepted (not initiate-only)
	if !c.InitiateOnly && !c.AcceptOnly && peeringConnectionID != "" {
		// Create route
		err = c.createRoute(system, logger, peeringConnectionID)
		if err != nil {
			return err
		}

		// Associate hosted zone
		err = c.associateHostedZone(system, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *CloudDatabasesPeerVPCCmd) getExistingPeerings(databaseID string) ([]map[string]interface{}, error) {
	client, err := cloud.NewClient()
	if err != nil {
		return nil, err
	}

	var result interface{}
	path := fmt.Sprintf("/databases/%s/vpc-peerings", databaseID)
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

func (c *CloudDatabasesPeerVPCCmd) initiateVPCPeering(databaseID string, cidr string, accountId string) (string, error) {
	client, err := cloud.NewClient()
	if err != nil {
		return "", err
	}

	request := cloud.CreateVPCPeeringRequest{
		VpcID:              c.VPCID,
		CIDRBlock:          cidr,
		AccountID:          accountId,
		Region:             c.Region,
		IsSecureConnection: true,
	}
	var result interface{}

	path := fmt.Sprintf("/databases/%s/vpc-peerings", databaseID)
	err = client.Post(path, request, &result)
	if err != nil {
		return "", err
	}

	// Log the result for debugging
	resultJson, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logger.Error("failed to marshal VPC peering initiation result for logging purposes: %s", err.Error())
	}
	logger.Debug("VPC peering initiation result:\n%s", string(resultJson))

	requestID := result.(map[string]interface{})["peeringId"].(string)
	return requestID, nil
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

// createRouteOnly only creates the route in the VPC route table
func (c *CloudDatabasesPeerVPCCmd) createRouteOnly(system *System, logger *logger.Logger) error {
	// Get existing peerings to find the peering connection ID
	existingPeerings, err := c.getExistingPeerings(c.DatabaseID)
	if err != nil {
		return fmt.Errorf("failed to get existing VPC peerings: %w", err)
	}

	// Find peering for this VPC
	var peeringConnectionID string
	for _, peering := range existingPeerings {
		if peering["vpcId"] == c.VPCID {
			peeringConnectionID = peering["peeringId"].(string)
			break
		}
	}

	if peeringConnectionID == "" {
		return fmt.Errorf("no existing VPC peering found for VPC %s", c.VPCID)
	}

	return c.createRoute(system, logger, peeringConnectionID)
}

// associateHostedZoneOnly only associates the VPC with the hosted zone
func (c *CloudDatabasesPeerVPCCmd) associateHostedZoneOnly(system *System, logger *logger.Logger) error {
	return c.associateHostedZone(system, logger)
}

// createRoute creates a route in the VPC route table
func (c *CloudDatabasesPeerVPCCmd) createRoute(system *System, logger *logger.Logger, peeringConnectionID string) error {
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	// Get the database to extract CIDR block
	logger.Info("Getting database information to extract CIDR block...")
	var dbResult interface{}
	dbPath := fmt.Sprintf("/databases/%s", c.DatabaseID)
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
		return system.Backend.CreateRoute(backends.BackendTypeAWS, c.VPCID, peeringConnectionID, cidrBlock)
	}, c.DatabaseID)
	if err != nil {
		return fmt.Errorf("failed to create route: %w", err)
	}
	logger.Info("Route created successfully")
	return nil
}

// associateHostedZone associates the VPC with the hosted zone
func (c *CloudDatabasesPeerVPCCmd) associateHostedZone(system *System, logger *logger.Logger) error {
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	// Get VPC peerings list to extract hosted zone ID
	logger.Info("Getting VPC peerings list to extract hosted zone ID...")
	var peeringsResult interface{}
	peeringsPath := fmt.Sprintf("/databases/%s/vpc-peerings", c.DatabaseID)
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
	logger.Info("Associating VPC %s with hosted zone %s in region %s", c.VPCID, hostedZoneID, c.Region)
	err = c.retry(func() error {
		return system.Backend.AssociateVPCWithHostedZone(backends.BackendTypeAWS, hostedZoneID, c.VPCID, c.Region)
	}, c.DatabaseID)
	if err != nil {
		return fmt.Errorf("failed to associate VPC with hosted zone: %w", err)
	}
	logger.Info("VPC-hosted zone association completed successfully")
	return nil
}

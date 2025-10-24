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
	DatabaseID   string  `short:"d" long:"database-id" description:"Database ID" required:"true"`
	VPCID        string  `long:"vpc-id" description:"VPC ID to peer the database to" default:"default"`
	Region       string  `short:"r" long:"region" description:"Region" required:"true"`
	InitiateOnly bool    `long:"initiate-only" description:"Only initiate peering, do not accept"`
	AcceptOnly   bool    `long:"accept-only" description:"Only accept existing peering, do not initiate"`
	Help         HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
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

	// Initiate peering if it doesn't exist and not accept-only
	if !peeringExists && !c.AcceptOnly {
		logger.Info("Initiating VPC peering for database=%s, vpcId=%s, cidr=%s, accountId=%s, region=%s, isSecureConnection=%t",
			c.DatabaseID, c.VPCID, cidr, accountId, c.Region, true)

		reqId, err := c.initiateVPCPeering(c.DatabaseID, cidr, accountId)
		if err != nil {
			return fmt.Errorf("failed to initiate VPC peering: %w", err)
		}
		logger.Info("VPC peering initiated, reqId: %s", reqId)

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
	} else if peeringExists && c.AcceptOnly {
		logger.Info("VPC peering already exists: %s", existingPeeringId)
	} else if !peeringExists && c.InitiateOnly {
		logger.Info("VPC peering initiated but not accepted (initiate-only mode)")
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

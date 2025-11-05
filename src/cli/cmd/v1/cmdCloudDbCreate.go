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

type CloudDatabasesCreateCmd struct {
	Name                  string  `short:"n" long:"name" description:"Name of the database"`
	InstanceType          string  `short:"i" long:"instance-type" description:"Instance type" required:"true"`
	Region                string  `short:"r" long:"region" description:"Region" required:"true"`
	AvailabilityZoneCount int     `long:"availability-zone-count" description:"Number of availability zones (1-3)" default:"2"`
	ClusterSize           int     `long:"cluster-size" description:"Number of nodes in cluster" required:"true"`
	DataStorage           string  `long:"data-storage" description:"Data storage type (memory, local-disk, network-storage)" required:"true"`
	DataResiliency        string  `long:"data-resiliency" description:"Data resiliency (local-disk, network-storage)"`
	DataPlaneVersion      string  `long:"data-plane-version" description:"Data plane version" default:"latest"`
	VPCID                 string  `long:"vpc-id" description:"VPC ID to peer the database to" default:"default"`
	Help                  HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudDatabasesCreateCmd) Execute(args []string) error {
	cmd := []string{"cloud", "db", "create"}
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
	err = c.CreateCloudDb(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)

}

func (c *CloudDatabasesCreateCmd) CreateCloudDb(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cloud", "db", "create"}, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != "aws" {
		return fmt.Errorf("cloud databases can only be created with AWS backend")
	}
	// vpc resolution
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
	var cidr string
	var accountId string
	var err error
	if c.VPCID != "" {
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
	}

	logger.Info("Creating cloud database: %s", c.Name)

	// create cloud DB
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		return err
	}

	// Build infrastructure
	provider := "aws"
	region := c.Region
	availabilityZoneCount := c.AvailabilityZoneCount
	infrastructure := cloud.Infrastructure{
		Provider:              &provider,
		InstanceType:          c.InstanceType,
		Region:                &region,
		AvailabilityZoneCount: &availabilityZoneCount,
	}

	// Build aerospike cloud configuration
	var aerospikeCloud interface{}
	switch c.DataStorage {
	case "memory":
		aerospikeCloud = cloud.AerospikeCloudMemory{
			AerospikeCloudShared: cloud.AerospikeCloudShared{
				ClusterSize: c.ClusterSize,
				DataStorage: c.DataStorage,
			},
			DataResiliency: c.DataResiliency,
		}
	case "local-disk":
		aerospikeCloud = cloud.AerospikeCloudLocalDisk{
			AerospikeCloudShared: cloud.AerospikeCloudShared{
				ClusterSize: c.ClusterSize,
				DataStorage: c.DataStorage,
			},
			DataResiliency: c.DataResiliency,
		}
	case "network-storage":
		aerospikeCloud = cloud.AerospikeCloudNetworkStorage{
			AerospikeCloudShared: cloud.AerospikeCloudShared{
				ClusterSize: c.ClusterSize,
				DataStorage: c.DataStorage,
			},
			DataResiliency: c.DataResiliency,
		}
	default:
		return fmt.Errorf("invalid data storage type: %s", c.DataStorage)
	}

	// Build aerospike server configuration
	// Always create aerospikeServer object with at least one namespace as it's required by the API
	aerospikeServer := &cloud.AerospikeServer{
		Namespaces: []cloud.AerospikeNamespace{
			{Name: "test"},
		},
	}

	request := cloud.CreateDatabaseRequest{
		Name:             c.Name,
		DataPlaneVersion: c.DataPlaneVersion,
		Infrastructure:   infrastructure,
		AerospikeCloud:   aerospikeCloud,
		AerospikeServer:  aerospikeServer,
	}
	var result interface{}

	err = client.Post(cloudDbPath, request, &result)
	if err != nil {
		return err
	}
	dbId := result.(map[string]interface{})["id"].(string)

	logger.Info("Database create queued: %s", dbId)
	// json-dump result in logger.Debug for debugging purposes
	resultJson, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logger.Error("failed to marshal database creation result for logging purposes: %s", err.Error())
	}
	logger.Debug("Database creation result:\n%s", string(resultJson))

	// initiate VPC peering
	if c.VPCID != "" {
		logger.Info("Initiating VPC peering for database=%s, vpcId=%s, cidr=%s, accountId=%s, region=%s, isSecureConnection=%t", dbId, c.VPCID, cidr, accountId, c.Region, true)
		logger.Info("This process may take up to an hour, as the database is being created and then the VPC peering will be initialized...")
		var reqId string
		err = c.retry(logger, func() error {
			reqId, err = c.initiateVPCPeering(system, inventory, dbId, cidr, accountId, c.Region, logger)
			return err
		}, dbId)
		if err != nil {
			return fmt.Errorf("failed to initiate VPC peering: %w", err)
		}
		logger.Info("Accepting VPC peering for reqId: %s", reqId)
		err = c.retry(logger, func() error {
			return system.Backend.AcceptVPCPeering(backends.BackendTypeAWS, reqId)
		}, dbId)
		if err != nil {
			return fmt.Errorf("failed to accept VPC peering: %w", err)
		}
		logger.Info("VPC peering accepted, reqId: %s", reqId)

		// Get the database to extract CIDR block
		logger.Info("Getting database information to extract CIDR block...")
		var dbResult interface{}
		dbPath := fmt.Sprintf("%s/%s", cloudDbPath, dbId)
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
		logger.Info("Creating route in VPC %s for CIDR block %s via peering connection %s", c.VPCID, cidrBlock, reqId)
		err = c.retry(logger, func() error {
			return system.Backend.CreateRoute(backends.BackendTypeAWS, c.VPCID, reqId, cidrBlock)
		}, dbId)
		if err != nil {
			return fmt.Errorf("failed to create route: %w", err)
		}
		logger.Info("Route created successfully")

		// Get VPC peerings list to extract hosted zone ID
		logger.Info("Getting VPC peerings list to extract hosted zone ID...")
		var peeringsResult interface{}
		peeringsPath := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, dbId)
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
		} else {
			// Associate VPC with hosted zone
			logger.Info("Associating VPC %s with hosted zone %s in region %s", c.VPCID, hostedZoneID, c.Region)
			err = c.retry(logger, func() error {
				return system.Backend.AssociateVPCWithHostedZone(backends.BackendTypeAWS, hostedZoneID, c.VPCID, c.Region)
			}, dbId)
			if err != nil {
				return fmt.Errorf("failed to associate VPC with hosted zone: %w", err)
			}
			logger.Info("VPC-hosted zone association completed successfully")
		}
	} else {
		// Wait for the database to be provisioned
		logger.Info("Waiting for database to be provisioned.")
		logger.Info("This process may take up to an hour...")
		err = c.waitForDatabaseProvisioning(client, dbId, logger)
		if err != nil {
			return fmt.Errorf("failed to wait for database provisioning: %w", err)
		}
		logger.Info("Database provisioned successfully")
		logger.Warn("VPC-ID was not specified. To be able to connect to the database, you will need to peer the VPC to the database using the 'cloud db peer-vpc' command.")
	}
	return nil
}

func (c *CloudDatabasesCreateCmd) initiateVPCPeering(system *System, inventory *backends.Inventory, dbId string, cidr string, accountId string, region string, logger *logger.Logger) (string, error) {
	client, err := cloud.NewClient(cloudVersion)
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

	path := fmt.Sprintf("%s/%s/vpc-peerings", cloudDbPath, dbId)
	err = client.Post(path, request, &result)
	if err != nil {
		return "", err
	}

	// Wait for the peering to be initiated so that we can get the peeringId from it
	// The post response doesn't include the peeringId, it's all async
	peeringId, err := c.waitForVPCPeeringInitiated(client, dbId, c.VPCID, logger)
	if err != nil {
		return "", fmt.Errorf("failed to wait for VPC peering initiation: %w", err)
	}

	return peeringId, nil
}

// waitForDatabaseProvisioning waits for the database to finish provisioning
// It polls the database status until health.status != "provisioning"
func (c *CloudDatabasesCreateCmd) waitForDatabaseProvisioning(client *cloud.Client, dbId string, logger *logger.Logger) error {
	timeout := time.Hour
	interval := 10 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeout {
			return fmt.Errorf("timeout waiting for database provisioning after %v", timeout)
		}

		var result interface{}
		path := fmt.Sprintf("%s/%s", cloudDbPath, dbId)
		err := client.Get(path, &result)
		if err != nil {
			return fmt.Errorf("failed to get database status: %w", err)
		}

		resultMap, ok := result.(map[string]interface{})
		if !ok {
			return fmt.Errorf("unexpected response type: %T", result)
		}

		health, ok := resultMap["health"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("health field not found or invalid in response")
		}

		status, ok := health["status"].(string)
		if !ok {
			return fmt.Errorf("health.status field not found or invalid in response")
		}

		logger.Debug("Database status: %s", status)

		if status != "provisioning" {
			logger.Info("Database provisioning complete, status: %s", status)
			return nil
		}

		logger.Debug("Database still provisioning, waiting %v...", interval)
		time.Sleep(interval)
	}
}

// waitForVPCPeeringInitiated waits for the VPC peering to be initiated
// It polls the VPC peerings list until status != "initiating-request" and peeringId != ""
func (c *CloudDatabasesCreateCmd) waitForVPCPeeringInitiated(client *cloud.Client, dbId string, vpcId string, logger *logger.Logger) (string, error) {
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
			logger.Debug("No peerings found yet, waiting %v...", interval)
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

				logger.Debug("VPC peering status: %s, peeringId: %s", status, peeringId)

				if status != "initiating-request" && peeringId != "" {
					logger.Info("VPC peering initiated, peeringId: %s, status: %s", peeringId, status)
					return peeringId, nil
				}

				logger.Debug("VPC peering still initiating, waiting %v...", interval)
				time.Sleep(interval)
				break
			}
		}

		// Peering not found yet, wait and retry
		logger.Debug("VPC peering not found in list yet, waiting %v...", interval)
		time.Sleep(interval)
	}
}

func (c *CloudDatabasesCreateCmd) retry(logger *logger.Logger, fn func() error, dbId string) error {
	for {
		err := fn()
		if err == nil {
			return nil
		}
		if !IsInteractive() {
			return err
		}
		logger.Error("%s", err.Error())
		opts, quitting, quittingErr := choice.Choice("Retry Peering, Quit, or Rollback Database Creation?", choice.Items{
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
				return fmt.Errorf("failed to rollback database creation: %s", rollbackErr.Error())
			}
			return err
		}
		if opts == "Retry" {
			continue
		}
	}
}

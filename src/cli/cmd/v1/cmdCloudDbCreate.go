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
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	// Build infrastructure
	infrastructure := cloud.Infrastructure{
		Provider:              "aws",
		InstanceType:          c.InstanceType,
		Region:                c.Region,
		AvailabilityZoneCount: c.AvailabilityZoneCount,
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
	var aerospikeServer *cloud.AerospikeServer

	request := cloud.CreateDatabaseRequest{
		Name:             c.Name,
		DataPlaneVersion: c.DataPlaneVersion,
		Infrastructure:   infrastructure,
		AerospikeCloud:   aerospikeCloud,
		AerospikeServer:  aerospikeServer,
	}
	var result interface{}

	err = client.Post("/databases", request, &result)
	if err != nil {
		return err
	}
	dbId := result.(map[string]interface{})["id"].(string)

	logger.Info("Database created: %s", dbId)
	// json-dump result in logger.Debug for debugging purposes
	resultJson, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logger.Error("failed to marshal database creation result for logging purposes: %s", err.Error())
	}
	logger.Debug("Database creation result:\n%s", string(resultJson))

	// initiate VPC peering
	if c.VPCID != "" {
		logger.Info("Initiating VPC peering for database=%s, vpcId=%s, cidr=%s, accountId=%s, region=%s, isSecureConnection=%t", dbId, c.VPCID, cidr, accountId, c.Region, true)
		var reqId string
		err = c.retry(func() error {
			reqId, err = c.initiateVPCPeering(system, inventory, dbId, cidr, accountId, c.Region)
			return err
		}, dbId)
		if err != nil {
			return fmt.Errorf("failed to initiate VPC peering: %w", err)
		}
		logger.Info("Accepting VPC peering for reqId: %s", reqId)
		err = c.retry(func() error {
			return system.Backend.AcceptVPCPeering(backends.BackendTypeAWS, reqId)
		}, dbId)
		if err != nil {
			return fmt.Errorf("failed to accept VPC peering: %w", err)
		}
		logger.Info("VPC peering accepted, reqId: %s", reqId)
	}
	return nil
}

func (c *CloudDatabasesCreateCmd) initiateVPCPeering(system *System, inventory *backends.Inventory, dbId string, cidr string, accountId string, region string) (string, error) {
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

	path := fmt.Sprintf("/databases/%s/vpc-peerings", dbId)
	err = client.Post(path, request, &result)
	if err != nil {
		return "", err
	}
	requestID := result.(map[string]interface{})["peeringId"].(string)
	return requestID, nil
}

func (c *CloudDatabasesCreateCmd) retry(fn func() error, dbId string) error {
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

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type CloudDatabasesUpdateCmd struct {
	DatabaseID            string  `short:"d" long:"database-id" description:"Database ID" required:"true"`
	Name                  string  `short:"n" long:"name" description:"Name of the database"`
	InstanceType          string  `short:"i" long:"instance-type" description:"Instance type"`
	Region                string  `short:"r" long:"region" description:"Region"`
	AvailabilityZoneCount int     `long:"availability-zone-count" description:"Number of availability zones (1-3)"`
	ClusterSize           int     `long:"cluster-size" description:"Number of nodes in cluster"`
	DataStorage           string  `long:"data-storage" description:"Data storage type (memory, local-disk, network-storage)"`
	DataResiliency        string  `long:"data-resiliency" description:"Data resiliency (local-disk, network-storage)"`
	DataPlaneVersion      string  `long:"data-plane-version" description:"Data plane version"`
	Help                  HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudDatabasesUpdateCmd) Execute(args []string) error {
	cmd := []string{"cloud", "db", "update"}
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
	err = c.UpdateCloudDb(system, system.Backend.GetInventory(), args, stdin, stdout, stderr, system.Logger)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *CloudDatabasesUpdateCmd) UpdateCloudDb(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cloud", "db", "update"}, c, args...)
		if err != nil {
			return err
		}
	}
	if logger == nil {
		logger = system.Logger
	}

	logger.Info("Updating cloud database: %s", c.DatabaseID)

	// create cloud client
	client, err := cloud.NewClient()
	if err != nil {
		return err
	}

	// Build update request
	request := cloud.UpdateDatabaseRequest{
		Name:             c.Name,
		DataPlaneVersion: c.DataPlaneVersion,
	}

	// Build infrastructure if any infrastructure parameters are provided
	if c.InstanceType != "" || c.Region != "" || c.AvailabilityZoneCount > 0 {
		infrastructure := cloud.Infrastructure{
			Provider:              "aws", // Default to AWS for now
			InstanceType:          c.InstanceType,
			Region:                c.Region,
			AvailabilityZoneCount: c.AvailabilityZoneCount,
		}
		request.Infrastructure = &infrastructure
		logger.Info("Updating infrastructure: provider=aws, instanceType=%s, region=%s, availabilityZoneCount=%d",
			c.InstanceType, c.Region, c.AvailabilityZoneCount)
	}

	// Build aerospike cloud if any aerospike cloud parameters are provided
	if c.ClusterSize > 0 || c.DataStorage != "" {
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
			if c.DataStorage != "" {
				return fmt.Errorf("invalid data storage type: %s", c.DataStorage)
			}
		}
		request.AerospikeCloud = aerospikeCloud
		logger.Info("Updating aerospike cloud: clusterSize=%d, dataStorage=%s, dataResiliency=%s",
			c.ClusterSize, c.DataStorage, c.DataResiliency)
	}

	var result interface{}

	path := fmt.Sprintf("/databases/%s", c.DatabaseID)
	err = client.Patch(path, request, &result)
	if err != nil {
		return err
	}

	logger.Info("Database updated successfully")
	// json-dump result in logger.Debug for debugging purposes
	resultJson, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logger.Error("failed to marshal database update result for logging purposes: %s", err.Error())
	}
	logger.Debug("Database update result:\n%s", string(resultJson))
	return nil
}

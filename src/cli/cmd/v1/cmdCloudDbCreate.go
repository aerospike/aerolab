package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/cli/cmd/v1/cloud"
	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/rglonek/logger"
)

type CloudClustersCreateCmd struct {
	Name                  string  `short:"n" long:"name" description:"Name of the cluster"`
	InstanceType          string  `short:"i" long:"instance-type" description:"Instance type"`
	Region                string  `short:"r" long:"region" description:"Region"`
	AvailabilityZoneCount int     `short:"a" long:"availability-zone-count" description:"Number of availability zones (1-3)" default:"2"`
	ClusterSize           int     `short:"s" long:"cluster-size" description:"Number of nodes in cluster"`
	DataStorage           string  `short:"d" long:"data-storage" description:"Data storage type (memory, local-disk, network-storage)"`
	Credentials           string  `short:"C" long:"credentials" description:"Create cluster credentials in format USER:PASSWORD. If not specified, credentials must be created manually."`
	VPCID                 string  `short:"v" long:"vpc-id" description:"VPC ID to peer the cluster to" default:"default"`
	ForceRouteCreation    bool    `short:"f" long:"force-route-creation" description:"Force route creation even if it already exists"`
	DataResiliency        string  `long:"data-resiliency" description:"Data resiliency (local-disk, network-storage)"`
	DataPlaneVersion      string  `long:"data-plane-version" description:"Data plane version" default:"latest"`
	CloudCIDR             string  `long:"cloud-cidr" description:"CIDR block for the cloud cluster infrastructure. If 'default', the cloud will auto-assign (starting from 10.130.0.0/19). If VPC-ID is specified, aerolab will check for collisions and find the next available CIDR if default is used." default:"default"`
	DryRun                bool    `long:"dry-run" description:"Perform checks and print what would be done without actually creating anything"`
	Help                  HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *CloudClustersCreateCmd) Execute(args []string) error {
	cmd := []string{"cloud", "clusters", "create"}
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

func (c *CloudClustersCreateCmd) CreateCloudDb(system *System, inventory *backends.Inventory, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, logger *logger.Logger) error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.InstanceType == "" {
		return fmt.Errorf("instance type is required")
	}
	if c.Region == "" {
		return fmt.Errorf("region is required")
	}
	if c.ClusterSize == 0 {
		return fmt.Errorf("cluster size is required")
	}
	if c.DataStorage == "" {
		return fmt.Errorf("data storage is required")
	}
	if c.Credentials != "" {
		// Parse USER:PASSWORD format
		parts := strings.SplitN(c.Credentials, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid credentials format, expected USER:PASSWORD")
		}
		if len(parts[1]) < 8 {
			return fmt.Errorf("password must be at least 8 characters long")
		}
	}
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"cloud", "clusters", "create"}, c, args...)
		if err != nil {
			return err
		}
	}
	if system.Opts.Config.Backend.Type != "aws" {
		return fmt.Errorf("cloud clusters can only be created with AWS backend")
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
	var cloudCIDR string
	var vpcRegion string
	var err error
	if c.VPCID != "" {
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
			logger.Warn("Could not determine VPC region from zone name, using cluster region: %s", c.Region)
			vpcRegion = c.Region
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

		// Check/resolve cloud CIDR before creating the cluster
		if c.CloudCIDR != "" && c.CloudCIDR != "default" {
			// User specified a custom CIDR - validate it's not in use
			logger.Info("Validating requested cloud CIDR: %s", c.CloudCIDR)
			foundCIDR, isRequested, err := system.Backend.FindAvailableCloudCIDR(backends.BackendTypeAWS, c.VPCID, c.CloudCIDR)
			if err != nil {
				return fmt.Errorf("requested cloud CIDR %s validation failed: %w", c.CloudCIDR, err)
			}
			if !isRequested {
				return fmt.Errorf("requested cloud CIDR %s is not available (found alternative: %s)", c.CloudCIDR, foundCIDR)
			}
			cloudCIDR = foundCIDR
			logger.Info("Cloud CIDR %s is available", cloudCIDR)
		} else {
			// Default CIDR - find an available one
			logger.Info("Finding available cloud CIDR for VPC %s", c.VPCID)
			foundCIDR, isDefault, err := system.Backend.FindAvailableCloudCIDR(backends.BackendTypeAWS, c.VPCID, "")
			if err != nil {
				return fmt.Errorf("failed to find available cloud CIDR: %w", err)
			}
			cloudCIDR = foundCIDR
			if isDefault {
				logger.Info("Using default cloud CIDR: %s", cloudCIDR)
			} else {
				logger.Info("Default CIDR was in use, using next available: %s", cloudCIDR)
			}
		}
	}

	if !c.DryRun {
		logger.Info("Creating cloud cluster: %s", c.Name)
	} else {
		logger.Info("Dry-Run: collecting information, name=%s", c.Name)
	}

	// Lock CIDR by creating blackhole route (before cluster creation to prevent race conditions)
	// This ensures that if two users run create at the same time, only one will get this CIDR
	var blackholeCreated bool
	if !c.DryRun && c.VPCID != "" && cloudCIDR != "" {
		logger.Info("Locking CIDR %s with blackhole route to prevent race conditions", cloudCIDR)
		err = system.Backend.CreateBlackholeRoute(backends.BackendTypeAWS, c.VPCID, cloudCIDR)
		if err != nil {
			return fmt.Errorf("failed to lock CIDR with blackhole route: %w", err)
		}
		blackholeCreated = true
		logger.Info("CIDR %s locked successfully", cloudCIDR)
	}

	// Helper function to cleanup blackhole route on failure
	cleanupBlackhole := func() {
		if blackholeCreated {
			logger.Info("Cleaning up blackhole route for CIDR %s", cloudCIDR)
			cleanupErr := system.Backend.DeleteBlackholeRoute(backends.BackendTypeAWS, c.VPCID, cloudCIDR)
			if cleanupErr != nil {
				logger.Warn("Failed to cleanup blackhole route: %s", cleanupErr)
			} else {
				logger.Info("Blackhole route cleaned up successfully")
			}
			blackholeCreated = false
		}
	}

	// create cloud DB
	client, err := cloud.NewClient(cloudVersion)
	if err != nil {
		cleanupBlackhole()
		return err
	}

	// Check if a cluster with the same name already exists
	logger.Info("Checking for existing cluster with name: %s", c.Name)
	existingDb, err := c.getClusterByName(client, c.Name)
	if err != nil {
		cleanupBlackhole()
		return fmt.Errorf("failed to check for existing cluster: %w", err)
	}
	if existingDb != nil {
		cleanupBlackhole()
		return fmt.Errorf("cluster with name '%s' already exists (id: %s, status: %s)", c.Name, existingDb.ID, existingDb.Status)
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
	// Set CIDR block if we have a VPC-ID specified and resolved a CIDR
	if c.VPCID != "" && cloudCIDR != "" {
		infrastructure.CIDRBlock = cloudCIDR
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

	request := cloud.CreateClusterRequest{
		Name:             c.Name,
		DataPlaneVersion: c.DataPlaneVersion,
		Infrastructure:   infrastructure,
		AerospikeCloud:   aerospikeCloud,
		AerospikeServer:  aerospikeServer,
	}

	// Handle dry-run mode
	if c.DryRun {
		logger.Info("=== DRY RUN MODE ===")
		logger.Info("")

		// Print discovered CIDR for VPC peering
		if c.VPCID != "" {
			logger.Info("Discovered the following CIDR for VPC Peering:")
			logger.Info("  VPC ID: %s", c.VPCID)
			logger.Info("  VPC Region: %s", vpcRegion)
			logger.Info("  VPC CIDR: %s", cidr)
			logger.Info("  Cloud Cluster CIDR: %s", cloudCIDR)
			logger.Info("  AWS Account ID: %s", accountId)
			logger.Info("")
		}

		// Print cluster creation request
		requestJson, err := json.MarshalIndent(request, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal request for dry-run: %w", err)
		}
		logger.Info("Would create cluster with following data:")
		logger.Info("%s", string(requestJson))
		logger.Info("")

		// Print VPC peering details
		if c.VPCID != "" {
			logger.Info("Would perform the following steps:")
			logger.Info("  1. Create blackhole route in VPC %s for cloud CIDR %s (to lock CIDR and prevent race conditions)", c.VPCID, cloudCIDR)
			logger.Info("  2. Create cluster in Aerospike Cloud")
			logger.Info("  3. Delegate VPC peering to 'cloud clusters peer-vpc' with:")
			logger.Info("       cluster-id=<new-cluster-id>")
			logger.Info("       vpc-id=%s", c.VPCID)
			logger.Info("       force-route-creation=%t", c.ForceRouteCreation)
			logger.Info("       (pre-resolved: vpc-cidr=%s, account-id=%s, vpc-region=%s)", cidr, accountId, vpcRegion)
			logger.Info("     The peer-vpc command will:")
			logger.Info("       a. Initiate VPC peering request to Aerospike Cloud")
			logger.Info("       b. Accept VPC peering connection in AWS")
			logger.Info("       c. Replace blackhole route with peering route")
			logger.Info("       d. Associate VPC with private hosted zone for DNS resolution")
			logger.Info("")
			logger.Info("Note: If any step fails, the blackhole route will be automatically cleaned up.")
		} else {
			logger.Info("Would wait for cluster provisioning (no VPC peering configured)")
		}

		// Print credentials creation info
		logger.Info("")
		if c.Credentials != "" {
			parts := strings.SplitN(c.Credentials, ":", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				logger.Info("Would create cluster credentials:")
				logger.Info("  Username: %s", parts[0])
				logger.Info("  Password: [provided]")
				logger.Info("  Roles: read-write")
			}
		} else {
			logger.Info("No credentials specified (--credentials flag not provided)")
			logger.Info("Credentials will need to be created manually after cluster creation")
		}

		logger.Info("")
		logger.Info("=== END DRY RUN ===")
		return nil
	}

	var result interface{}

	err = client.Post(cloudDbPath, request, &result)
	if err != nil {
		cleanupBlackhole()
		return err
	}
	dbId := result.(map[string]interface{})["id"].(string)

	logger.Info("Cluster create queued: %s", dbId)
	fmt.Printf("db-id=%s\n", dbId)
	// json-dump result in logger.Debug for debugging purposes
	resultJson, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		logger.Error("failed to marshal cluster creation result for logging purposes: %s", err.Error())
	}
	logger.Debug("Cluster creation result:\n%s", string(resultJson))

	// Setup VPC peering if VPC-ID was specified
	if c.VPCID != "" {
		logger.Info("Setting up VPC peering for cluster=%s, vpcId=%s", dbId, c.VPCID)
		logger.Info("This process may take up to an hour, as the cluster is being created and then the VPC peering will be initialized...")

		// Delegate to PeerVPC command with pre-resolved values
		peerCmd := &CloudClustersPeerVPCCmd{
			ClusterID:            dbId,
			VPCID:                c.VPCID,
			ForceRouteCreation:   c.ForceRouteCreation,
			PreResolvedVPCCIDR:   cidr,
			PreResolvedAccountID: accountId,
			PreResolvedVPCRegion: vpcRegion,
			CleanupOnError:       cleanupBlackhole,
		}
		err = peerCmd.PeerVPC(system, inventory, args, stdin, stdout, stderr, logger)
		if err != nil {
			return fmt.Errorf("failed to setup VPC peering: %w", err)
		}
		// VPC peering completed successfully - blackhole has been replaced with real route
		blackholeCreated = false
		logger.Info("VPC peering setup completed successfully")
	} else {
		// Wait for the cluster to be provisioned
		logger.Info("Waiting for cluster to be provisioned.")
		logger.Info("This process may take up to an hour...")
		err = c.waitForClusterProvisioning(client, dbId, logger)
		if err != nil {
			return fmt.Errorf("failed to wait for cluster provisioning: %w", err)
		}
		logger.Info("Cluster provisioned successfully")
		logger.Warn("VPC-ID was not specified. To be able to connect to the cluster, you will need to peer the VPC to the cluster using the 'cloud clusters peer-vpc' command.")
	}

	// Handle credentials creation at the very end
	if c.Credentials != "" {
		// Parse USER:PASSWORD format
		parts := strings.SplitN(c.Credentials, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid credentials format, expected USER:PASSWORD")
		}
		username := parts[0]
		password := parts[1]

		logger.Info("Creating cluster credentials for user: %s", username)
		credCmd := &CloudClustersCredentialsCreateCmd{
			ClusterID: dbId,
			Username:  username,
			Password:  password,
			Roles:     []string{"read-write"},
			Wait:      true,
		}
		err = credCmd.CreateCloudCredentials(system)
		if err != nil {
			return fmt.Errorf("failed to create cluster credentials: %w", err)
		}
		logger.Info("Cluster credentials created successfully")
	} else {
		logger.Warn("No credentials specified. To create credentials, run: aerolab cloud clusters credentials create -c %s -u USERNAME -p PASSWORD --wait", dbId)
	}

	return nil
}

// waitForClusterProvisioning waits for the cluster to finish provisioning
// It polls the cluster status until health.status != "provisioning"
func (c *CloudClustersCreateCmd) waitForClusterProvisioning(client *cloud.Client, dbId string, logger *logger.Logger) error {
	timeout := time.Hour
	interval := 10 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > timeout {
			return fmt.Errorf("timeout waiting for cluster provisioning after %v", timeout)
		}

		var result interface{}
		path := fmt.Sprintf("%s/%s", cloudDbPath, dbId)
		err := client.Get(path, &result)
		if err != nil {
			return fmt.Errorf("failed to get cluster status: %w", err)
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

		logger.Debug("Cluster status: %s", status)

		if status != "provisioning" {
			logger.Info("Cluster provisioning complete, status: %s", status)
			return nil
		}

		logger.Debug("Cluster still provisioning, waiting %v...", interval)
		time.Sleep(interval)
	}
}

// existingCluster holds basic info about an existing cluster
type existingCluster struct {
	ID     string
	Name   string
	Status string
}

// getClusterByName checks if a cluster with the given name already exists
// Returns nil if no cluster with that name exists
func (c *CloudClustersCreateCmd) getClusterByName(client *cloud.Client, name string) (*existingCluster, error) {
	var result map[string]interface{}
	// Exclude decommissioned clusters from the check
	path := cloudDbPath + "?status_ne=decommissioned"
	err := client.Get(path, &result)
	if err != nil {
		return nil, err
	}

	clusters, ok := result["clusters"].([]interface{})
	if !ok {
		// No clusters found
		return nil, nil
	}

	for _, db := range clusters {
		dbMap, ok := db.(map[string]interface{})
		if !ok {
			continue
		}

		dbName, _ := dbMap["name"].(string)
		if dbName == name {
			dbID, _ := dbMap["id"].(string)
			// Get status from health.status
			var status string
			if health, ok := dbMap["health"].(map[string]interface{}); ok {
				status, _ = health["status"].(string)
			}
			return &existingCluster{
				ID:     dbID,
				Name:   dbName,
				Status: status,
			}, nil
		}
	}

	return nil, nil
}

package cloud

import (
	"fmt"

	"github.com/jessevdk/go-flags"
)

// Main Options structure
type Options struct {
	CloudProvider CloudProviderCmd `command:"cloud-provider" alias:"cp" description:"Cloud provider operations"`
	Organization  OrganizationCmd  `command:"organization" alias:"org" description:"Organization operations" hidden:"true"`
	ApiKeys       ApiKeysCmd       `command:"api-keys" alias:"keys" description:"API key operations" hidden:"true"`
	Secrets       SecretsCmd       `command:"secrets" alias:"secret" description:"Secret operations"`
	Databases     DatabasesCmd     `command:"databases" alias:"db" description:"Database operations"`
	Topologies    TopologiesCmd    `command:"topologies" alias:"top" description:"Topology operations" hidden:"true"`
	WebUI         WebUIServer      `command:"webui" description:"Start interactive web interface"`
	Version       VersionCmd       `command:"version" description:"Show version information"`
}

func (o *Options) Execute() error {
	return fmt.Errorf("please specify one command of: api-keys, cloud-provider, databases, organization, secrets, topologies, webui, or version")
}

// Version Command
type VersionCmd struct{}

func (c *VersionCmd) Execute(args []string) error {
	fmt.Println("Aerospike Cloud CLI v1.0.0")
	return nil
}

// Cloud Provider Commands
type CloudProviderCmd struct {
	List     CloudProviderListCmd     `command:"list" alias:"ls" description:"List cloud providers"`
	GetSpecs CloudProviderGetSpecsCmd `command:"get-specs" description:"Get instance type specifications"`
}

type CloudProviderListCmd struct {
	Filter string `short:"f" long:"filter" description:"Filter by cloud provider, region, or instance type (JSON format)"`
}

func (c *CloudProviderListCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := "/cloud-providers"
	if c.Filter != "" {
		path += "?filter=" + c.Filter
	}

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type CloudProviderGetSpecsCmd struct {
	CloudProviderID string `short:"c" long:"cloud-provider" description:"Cloud provider ID" required:"true"`
	InstanceTypeID  string `short:"i" long:"instance-type" description:"Instance type ID" required:"true"`
}

func (c *CloudProviderGetSpecsCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := fmt.Sprintf("/cloud-providers/%s/instance-types/%s", c.CloudProviderID, c.InstanceTypeID)

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

// Organization Commands
type OrganizationCmd struct {
	Get OrganizationGetCmd `command:"get" description:"Get organization information"`
}

type OrganizationGetCmd struct{}

func (c *OrganizationGetCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	err = client.Get("/organization", &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

// API Keys Commands
type ApiKeysCmd struct {
	List   ApiKeysListCmd   `command:"list" alias:"ls" description:"List API keys"`
	Create ApiKeysCreateCmd `command:"create" description:"Create new API key"`
	Delete ApiKeysDeleteCmd `command:"delete" description:"Delete API key"`
}

type ApiKeysListCmd struct{}

func (c *ApiKeysListCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	err = client.Get("/api-keys", &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type ApiKeysCreateCmd struct {
	Name string `short:"n" long:"name" description:"API key name" required:"true"`
}

func (c *ApiKeysCreateCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	request := CreateApiKeyRequest{Name: c.Name}
	var result interface{}

	err = client.Post("/api-keys", request, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type ApiKeysDeleteCmd struct {
	ClientID string `short:"c" long:"client-id" description:"API key client ID" required:"true"`
}

func (c *ApiKeysDeleteCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api-keys/%s", c.ClientID)
	err = client.Delete(path)
	if err != nil {
		return err
	}

	fmt.Println("API key deleted successfully")
	return nil
}

// Secrets Commands
type SecretsCmd struct {
	List   SecretsListCmd   `command:"list" alias:"ls" description:"List secrets"`
	Create SecretsCreateCmd `command:"create" description:"Create new secret"`
	Delete SecretsDeleteCmd `command:"delete" description:"Delete secret"`
}

type SecretsListCmd struct{}

func (c *SecretsListCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	err = client.Get("/secrets", &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type SecretsCreateCmd struct {
	Name        string `short:"n" long:"name" description:"Secret name" required:"true"`
	Description string `short:"d" long:"description" description:"Secret description"`
	Value       string `short:"v" long:"value" description:"Secret value" required:"true"`
}

func (c *SecretsCreateCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	request := CreateSecretRequest{
		Name:        c.Name,
		Description: c.Description,
		Value:       c.Value,
	}
	var result interface{}

	err = client.Post("/secrets", request, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type SecretsDeleteCmd struct {
	SecretID string `short:"s" long:"secret-id" description:"Secret ID" required:"true"`
}

func (c *SecretsDeleteCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/secrets/%s", c.SecretID)
	err = client.Delete(path)
	if err != nil {
		return err
	}

	fmt.Println("Secret deleted successfully")
	return nil
}

// Databases Commands
type DatabasesCmd struct {
	List        DatabasesListCmd        `command:"list" alias:"ls" description:"List databases"`
	Create      DatabasesCreateCmd      `command:"create" description:"Create new database"`
	Get         DatabasesGetCmd         `command:"get" description:"Get database by ID"`
	Update      DatabasesUpdateCmd      `command:"update" description:"Update database"`
	Delete      DatabasesDeleteCmd      `command:"delete" description:"Delete database"`
	Metrics     DatabasesMetricsCmd     `command:"metrics" description:"Get database metrics"`
	Credentials DatabasesCredentialsCmd `command:"credentials" alias:"creds" description:"Database credentials operations"`
	VpcPeering  DatabasesVpcPeeringCmd  `command:"vpc-peering" alias:"vpc" description:"VPC peering operations"`
}

type DatabasesListCmd struct {
	StatusNe string `long:"status-ne" description:"Filter databases to exclude specified statuses (comma-separated)"`
}

func (c *DatabasesListCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := "/databases"
	if c.StatusNe != "" {
		path += "?status_ne=" + c.StatusNe
	}

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type DatabasesCreateCmd struct {
	// Basic database info
	Name             string `short:"n" long:"name" description:"Database name" required:"true"`
	DataPlaneVersion string `long:"data-plane-version" description:"Data plane version (default: latest)"`

	// Infrastructure parameters
	Provider              string   `short:"c" long:"cloud-provider" description:"Cloud provider (aws, gcp)" required:"true"`
	InstanceType          string   `short:"i" long:"instance-type" description:"Instance type" required:"true"`
	Region                string   `short:"r" long:"region" description:"Region" required:"true"`
	AvailabilityZoneCount int      `long:"availability-zone-count" description:"Number of availability zones (1-3)" default:"2"`
	ZoneIds               []string `long:"zone-ids" description:"Specific availability zone IDs (comma-separated)"`
	CIDRBlock             string   `long:"cidr-block" description:"IPv4 CIDR block for database VPC (e.g., 10.128.0.0/19)"`

	// Aerospike Cloud parameters
	ClusterSize    int    `long:"cluster-size" description:"Number of nodes in cluster" required:"true"`
	DataStorage    string `long:"data-storage" description:"Data storage type (memory, local-disk, network-storage)" required:"true"`
	DataResiliency string `long:"data-resiliency" description:"Data resiliency (local-disk, network-storage)"`

	// Aerospike Server parameters (service)
	AdvertiseIpv6           bool     `long:"advertise-ipv6" description:"Advertise IPv6"`
	AutoPin                 string   `long:"auto-pin" description:"Auto pin mode (none, cpu, numa, adq)"`
	BatchIndexThreads       int      `long:"batch-index-threads" description:"Batch index threads (1-256)"`
	BatchMaxBuffersPerQueue int      `long:"batch-max-buffers-per-queue" description:"Batch max buffers per queue"`
	BatchMaxRequests        int      `long:"batch-max-requests" description:"Batch max requests"`
	BatchMaxUnusedBuffers   int      `long:"batch-max-unused-buffers" description:"Batch max unused buffers"`
	ClusterName             string   `long:"cluster-name" description:"Cluster name"`
	DebugAllocations        bool     `long:"debug-allocations" description:"Debug allocations"`
	DisableUdfExecution     bool     `long:"disable-udf-execution" description:"Disable UDF execution"`
	EnableBenchmarksFabric  bool     `long:"enable-benchmarks-fabric" description:"Enable benchmarks fabric"`
	EnableHealthCheck       bool     `long:"enable-health-check" description:"Enable health check"`
	EnableHistInfo          bool     `long:"enable-hist-info" description:"Enable hist info"`
	EnforceBestPractices    bool     `long:"enforce-best-practices" description:"Enforce best practices"`
	FeatureKeyFile          string   `long:"feature-key-file" description:"Feature key file"`
	FeatureKeyFiles         []string `long:"feature-key-files" description:"Feature key files (comma-separated)"`
	Group                   string   `long:"group" description:"Group"`
	IndentAllocations       bool     `long:"indent-allocations" description:"Indent allocations"`
	InfoMaxMs               int      `long:"info-max-ms" description:"Info max ms (500-10000)"`
	InfoThreads             int      `long:"info-threads" description:"Info threads"`
}

func (c *DatabasesCreateCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	// Build infrastructure
	infrastructure := Infrastructure{
		Provider:              c.Provider,
		InstanceType:          c.InstanceType,
		Region:                c.Region,
		AvailabilityZoneCount: c.AvailabilityZoneCount,
		ZoneIds:               c.ZoneIds,
		CIDRBlock:             c.CIDRBlock,
	}

	// Build aerospike cloud configuration
	var aerospikeCloud interface{}
	switch c.DataStorage {
	case "memory":
		aerospikeCloud = AerospikeCloudMemory{
			AerospikeCloudShared: AerospikeCloudShared{
				ClusterSize: c.ClusterSize,
				DataStorage: c.DataStorage,
			},
			DataResiliency: c.DataResiliency,
		}
	case "local-disk":
		aerospikeCloud = AerospikeCloudLocalDisk{
			AerospikeCloudShared: AerospikeCloudShared{
				ClusterSize: c.ClusterSize,
				DataStorage: c.DataStorage,
			},
			DataResiliency: c.DataResiliency,
		}
	case "network-storage":
		aerospikeCloud = AerospikeCloudNetworkStorage{
			AerospikeCloudShared: AerospikeCloudShared{
				ClusterSize: c.ClusterSize,
				DataStorage: c.DataStorage,
			},
			DataResiliency: c.DataResiliency,
		}
	default:
		return fmt.Errorf("invalid data storage type: %s", c.DataStorage)
	}

	// Build aerospike server configuration
	var aerospikeServer *AerospikeServer
	if c.ClusterName != "" || c.AutoPin != "" || c.BatchIndexThreads > 0 {
		aerospikeServer = &AerospikeServer{
			Service: &AerospikeService{
				AdvertiseIpv6:           c.AdvertiseIpv6,
				AutoPin:                 c.AutoPin,
				BatchIndexThreads:       c.BatchIndexThreads,
				BatchMaxBuffersPerQueue: c.BatchMaxBuffersPerQueue,
				BatchMaxRequests:        c.BatchMaxRequests,
				BatchMaxUnusedBuffers:   c.BatchMaxUnusedBuffers,
				ClusterName:             c.ClusterName,
				DebugAllocations:        c.DebugAllocations,
				DisableUdfExecution:     c.DisableUdfExecution,
				EnableBenchmarksFabric:  c.EnableBenchmarksFabric,
				EnableHealthCheck:       c.EnableHealthCheck,
				EnableHistInfo:          c.EnableHistInfo,
				EnforceBestPractices:    c.EnforceBestPractices,
				FeatureKeyFile:          c.FeatureKeyFile,
				FeatureKeyFiles:         c.FeatureKeyFiles,
				Group:                   c.Group,
				IndentAllocations:       c.IndentAllocations,
				InfoMaxMs:               c.InfoMaxMs,
				InfoThreads:             c.InfoThreads,
			},
		}
	}

	request := CreateDatabaseRequest{
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

	return client.PrettyPrint(result)
}

type DatabasesGetCmd struct {
	DatabaseID string `short:"d" long:"database-id" description:"Database ID" required:"true"`
}

func (c *DatabasesGetCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := fmt.Sprintf("/databases/%s", c.DatabaseID)

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type DatabasesUpdateCmd struct {
	DatabaseID string `short:"d" long:"database-id" description:"Database ID" required:"true"`
	Name       string `short:"n" long:"name" description:"New database name"`

	// Infrastructure update parameters
	Provider              string   `long:"cloud-provider" description:"Cloud provider (aws, gcp)"`
	InstanceType          string   `long:"instance-type" description:"Instance type"`
	Region                string   `long:"region" description:"Region"`
	AvailabilityZoneCount int      `long:"availability-zone-count" description:"Number of availability zones (1-3)"`
	ZoneIds               []string `long:"zone-ids" description:"Specific availability zone IDs (comma-separated)"`
	CIDRBlock             string   `long:"cidr-block" description:"IPv4 CIDR block for database VPC (e.g., 10.128.0.0/19)"`

	// Aerospike Cloud update parameters
	ClusterSize    int    `long:"cluster-size" description:"Number of nodes in cluster"`
	DataStorage    string `long:"data-storage" description:"Data storage type (memory, local-disk, network-storage)"`
	DataResiliency string `long:"data-resiliency" description:"Data resiliency (local-disk, network-storage)"`

	// Data plane version
	DataPlaneVersion string `long:"data-plane-version" description:"Data plane version"`
}

func (c *DatabasesUpdateCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	request := UpdateDatabaseRequest{
		Name:             c.Name,
		DataPlaneVersion: c.DataPlaneVersion,
	}

	// Build infrastructure if any infrastructure parameters are provided
	if c.Provider != "" || c.InstanceType != "" || c.Region != "" || c.AvailabilityZoneCount > 0 {
		infrastructure := Infrastructure{
			Provider:              c.Provider,
			InstanceType:          c.InstanceType,
			Region:                c.Region,
			AvailabilityZoneCount: c.AvailabilityZoneCount,
			ZoneIds:               c.ZoneIds,
			CIDRBlock:             c.CIDRBlock,
		}
		request.Infrastructure = &infrastructure
	}

	// Build aerospike cloud if any aerospike cloud parameters are provided
	if c.ClusterSize > 0 || c.DataStorage != "" {
		var aerospikeCloud interface{}
		switch c.DataStorage {
		case "memory":
			aerospikeCloud = AerospikeCloudMemory{
				AerospikeCloudShared: AerospikeCloudShared{
					ClusterSize: c.ClusterSize,
					DataStorage: c.DataStorage,
				},
				DataResiliency: c.DataResiliency,
			}
		case "local-disk":
			aerospikeCloud = AerospikeCloudLocalDisk{
				AerospikeCloudShared: AerospikeCloudShared{
					ClusterSize: c.ClusterSize,
					DataStorage: c.DataStorage,
				},
				DataResiliency: c.DataResiliency,
			}
		case "network-storage":
			aerospikeCloud = AerospikeCloudNetworkStorage{
				AerospikeCloudShared: AerospikeCloudShared{
					ClusterSize: c.ClusterSize,
					DataStorage: c.DataStorage,
				},
				DataResiliency: c.DataResiliency,
			}
		}
		request.AerospikeCloud = aerospikeCloud
	}

	var result interface{}

	path := fmt.Sprintf("/databases/%s", c.DatabaseID)
	err = client.Patch(path, request, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type DatabasesDeleteCmd struct {
	DatabaseID string `short:"d" long:"database-id" description:"Database ID" required:"true"`
}

func (c *DatabasesDeleteCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/databases/%s", c.DatabaseID)
	err = client.Delete(path)
	if err != nil {
		return err
	}

	fmt.Println("Database deleted successfully")
	return nil
}

type DatabasesMetricsCmd struct {
	DatabaseID string `short:"d" long:"database-id" description:"Database ID" required:"true"`
}

func (c *DatabasesMetricsCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := fmt.Sprintf("/databases/%s/metrics", c.DatabaseID)

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

// Database Credentials Commands
type DatabasesCredentialsCmd struct {
	List   DatabasesCredentialsListCmd   `command:"list" alias:"ls" description:"List database credentials"`
	Create DatabasesCredentialsCreateCmd `command:"create" description:"Create new database credentials"`
	Delete DatabasesCredentialsDeleteCmd `command:"delete" description:"Delete database credentials"`
}

type DatabasesCredentialsListCmd struct {
	DatabaseID string `short:"d" long:"database-id" description:"Database ID" required:"true"`
}

func (c *DatabasesCredentialsListCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := fmt.Sprintf("/databases/%s/credentials", c.DatabaseID)

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type DatabasesCredentialsCreateCmd struct {
	DatabaseID string `short:"d" long:"database-id" description:"Database ID" required:"true"`
	Username   string `short:"u" long:"username" description:"Username" required:"true"`
	Password   string `short:"p" long:"password" description:"Password" required:"true"`
	Privileges string `short:"r" long:"privileges" description:"Privileges (read, write, read-write)" default:"read-write"`
}

func (c *DatabasesCredentialsCreateCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	// Convert privileges string to roles array
	// The API expects roles as an array, but we accept privileges as a string for convenience
	roles := []string{c.Privileges}
	if c.Privileges == "" {
		roles = []string{"read-write"} // default
	}

	request := CreateDatabaseCredentialsRequest{
		Name:     c.Username, // username maps to name in the API
		Password: c.Password,
		Roles:    roles,
	}
	var result interface{}

	path := fmt.Sprintf("/databases/%s/credentials", c.DatabaseID)
	err = client.Post(path, request, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type DatabasesCredentialsDeleteCmd struct {
	DatabaseID    string `short:"d" long:"database-id" description:"Database ID" required:"true"`
	CredentialsID string `short:"c" long:"credentials-id" description:"Credentials ID" required:"true"`
}

func (c *DatabasesCredentialsDeleteCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/databases/%s/credentials/%s", c.DatabaseID, c.CredentialsID)
	err = client.Delete(path)
	if err != nil {
		return err
	}

	fmt.Println("Database credentials deleted successfully")
	return nil
}

// VPC Peering Commands
type DatabasesVpcPeeringCmd struct {
	List   DatabasesVpcPeeringListCmd   `command:"list" alias:"ls" description:"List VPC peerings"`
	Create DatabasesVpcPeeringCreateCmd `command:"create" description:"Create VPC peering"`
	Delete DatabasesVpcPeeringDeleteCmd `command:"delete" description:"Delete VPC peering"`
}

type DatabasesVpcPeeringListCmd struct {
	DatabaseID string `short:"d" long:"database-id" description:"Database ID" required:"true"`
}

func (c *DatabasesVpcPeeringListCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := fmt.Sprintf("/databases/%s/vpc-peerings", c.DatabaseID)

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type DatabasesVpcPeeringCreateCmd struct {
	DatabaseID         string `short:"d" long:"database-id" description:"Database ID" required:"true"`
	VpcID              string `short:"v" long:"vpc-id" description:"VPC ID" required:"true"`
	CIDRBlock          string `long:"cidr-block" description:"CIDR block" required:"true"`
	AccountID          string `long:"account-id" description:"Account ID" required:"true"`
	Region             string `long:"region" description:"Region" required:"true"`
	IsSecureConnection bool   `long:"is-secure-connection" description:"Is secure connection" required:"true"`
}

func (c *DatabasesVpcPeeringCreateCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	request := CreateVPCPeeringRequest{
		VpcID:              c.VpcID,
		CIDRBlock:          c.CIDRBlock,
		AccountID:          c.AccountID,
		Region:             c.Region,
		IsSecureConnection: c.IsSecureConnection,
	}
	var result interface{}

	path := fmt.Sprintf("/databases/%s/vpc-peerings", c.DatabaseID)
	err = client.Post(path, request, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type DatabasesVpcPeeringDeleteCmd struct {
	DatabaseID string `short:"d" long:"database-id" description:"Database ID" required:"true"`
	VpcID      string `short:"v" long:"vpc-id" description:"VPC ID" required:"true"`
}

func (c *DatabasesVpcPeeringDeleteCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/databases/%s/vpc-peerings/%s", c.DatabaseID, c.VpcID)
	err = client.Delete(path)
	if err != nil {
		return err
	}

	fmt.Println("VPC peering deleted successfully")
	return nil
}

// Topologies Commands
type TopologiesCmd struct {
	List   TopologiesListCmd   `command:"list" alias:"ls" description:"List topologies"`
	Create TopologiesCreateCmd `command:"create" description:"Create new topology"`
	Get    TopologiesGetCmd    `command:"get" description:"Get topology by ID"`
	Delete TopologiesDeleteCmd `command:"delete" description:"Delete topology"`
}

type TopologiesListCmd struct{}

func (c *TopologiesListCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	err = client.Get("/topologies", &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type TopologiesCreateCmd struct {
	Name string `short:"n" long:"name" description:"Topology name" required:"true"`
}

func (c *TopologiesCreateCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	request := CreateTopologyRequest{
		Name: c.Name,
	}
	var result interface{}

	err = client.Post("/topologies", request, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type TopologiesGetCmd struct {
	TopologyID string `short:"t" long:"topology-id" description:"Topology ID" required:"true"`
}

func (c *TopologiesGetCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	var result interface{}
	path := fmt.Sprintf("/topologies/%s", c.TopologyID)

	err = client.Get(path, &result)
	if err != nil {
		return err
	}

	return client.PrettyPrint(result)
}

type TopologiesDeleteCmd struct {
	TopologyID string `short:"t" long:"topology-id" description:"Topology ID" required:"true"`
}

func (c *TopologiesDeleteCmd) Execute(args []string) error {
	client, err := NewClient()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/topologies/%s", c.TopologyID)
	err = client.Delete(path)
	if err != nil {
		return err
	}

	fmt.Println("Topology deleted successfully")
	return nil
}

// Helper function to create parser
func NewParser(opts *Options) *flags.Parser {
	parser := flags.NewParser(opts, flags.Default)
	parser.LongDescription = "Aerospike Cloud CLI - Manage your Aerospike Cloud resources"
	parser.ShortDescription = "Aerospike Cloud CLI"
	return parser
}

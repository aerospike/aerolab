package cloud

// Cloud Provider Types
type CloudProviderInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CloudProviderInfoResultSet struct {
	Results []CloudProviderInfo `json:"results"`
	Total   int                 `json:"total"`
}

type CloudProviderInstanceTypeFilter struct {
	CloudProvider string `json:"cloudProvider,omitempty"`
	Region        string `json:"region,omitempty"`
	InstanceType  string `json:"instanceType,omitempty"`
}

type InstanceTypeSpecs struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	VCPUs        int    `json:"vcpus"`
	Memory       int    `json:"memory"`
	Storage      int    `json:"storage"`
	Network      int    `json:"network"`
	MaxBandwidth int    `json:"maxBandwidth"`
}

// Organization Types
type Organization struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// API Key Types
type ApiKey struct {
	Name     string `json:"name"`
	ClientID string `json:"clientId"`
}

type ApiKeyCollection struct {
	ApiKeys []ApiKey `json:"apiKeys"`
	Total   int      `json:"total"`
}

type CreateApiKeyRequest struct {
	Name string `json:"name"`
}

// Secret Types
type Secret struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type SecretCollection struct {
	Secrets []Secret `json:"secrets"`
	Total   int      `json:"total"`
}

type CreateSecretRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Value       string `json:"value"`
}

// Database Types
type DatabaseStatus string

const (
	DatabaseStatusCreating        DatabaseStatus = "creating"
	DatabaseStatusRunning         DatabaseStatus = "running"
	DatabaseStatusStopping        DatabaseStatus = "stopping"
	DatabaseStatusStopped         DatabaseStatus = "stopped"
	DatabaseStatusStarting        DatabaseStatus = "starting"
	DatabaseStatusUpdating        DatabaseStatus = "updating"
	DatabaseStatusDeleting        DatabaseStatus = "deleting"
	DatabaseStatusDeleted         DatabaseStatus = "deleted"
	DatabaseStatusFailed          DatabaseStatus = "failed"
	DatabaseStatusDecommissioned  DatabaseStatus = "decommissioned"
	DatabaseStatusDecommissioning DatabaseStatus = "decommissioning"
)

type Database struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Status        DatabaseStatus `json:"status"`
	CloudProvider string         `json:"cloudProvider"`
	Region        string         `json:"region"`
	CreatedAt     string         `json:"createdAt"`
	UpdatedAt     string         `json:"updatedAt"`
}

type DatabaseCollection struct {
	Databases []Database `json:"databases"`
	Total     int        `json:"total"`
}

type DatabaseFull struct {
	Database
	// Add additional fields as needed based on the full database schema
}

// Infrastructure Types
type Infrastructure struct {
	Provider               string                  `json:"provider"`
	InstanceType           string                  `json:"instanceType"`
	Region                 string                  `json:"region"`
	AvailabilityZoneCount  int                     `json:"availabilityZoneCount"`
	ZoneIds                []string                `json:"zoneIds,omitempty"`
	CIDRBlock              string                  `json:"cidrBlock,omitempty"`
	NetworkID              string                  `json:"networkId,omitempty"`
	AccountID              string                  `json:"accountId,omitempty"`
	NetworkAttachedStorage *NetworkAttachedStorage `json:"networkAttachedStorage,omitempty"`
}

type NetworkAttachedStorage struct {
	// Define based on the actual schema
}

// Aerospike Cloud Types
type AerospikeCloudShared struct {
	ClusterSize int    `json:"clusterSize"`
	DataStorage string `json:"dataStorage"`
}

type AerospikeCloudMemory struct {
	AerospikeCloudShared
	DataResiliency string `json:"dataResiliency,omitempty"`
}

type AerospikeCloudLocalDisk struct {
	AerospikeCloudShared
	DataResiliency string `json:"dataResiliency,omitempty"`
}

type AerospikeCloudNetworkStorage struct {
	AerospikeCloudShared
	DataResiliency string `json:"dataResiliency,omitempty"`
}

// Aerospike Server Types
type AerospikeServer struct {
	Service    *AerospikeService    `json:"service,omitempty"`
	Network    *AerospikeNetwork    `json:"network,omitempty"`
	Logging    *AerospikeLogging    `json:"logging,omitempty"`
	XDR        *AerospikeXDR        `json:"xdr,omitempty"`
	Namespaces []AerospikeNamespace `json:"namespaces"`
}

type AerospikeNamespace struct {
	Name string `json:"name"`
}

type AerospikeService struct {
	AdvertiseIpv6           bool     `json:"advertise-ipv6,omitempty"`
	AutoPin                 string   `json:"auto-pin,omitempty"`
	BatchIndexThreads       int      `json:"batch-index-threads,omitempty"`
	BatchMaxBuffersPerQueue int      `json:"batch-max-buffers-per-queue,omitempty"`
	BatchMaxRequests        int      `json:"batch-max-requests,omitempty"`
	BatchMaxUnusedBuffers   int      `json:"batch-max-unused-buffers,omitempty"`
	ClusterName             string   `json:"cluster-name,omitempty"`
	DebugAllocations        bool     `json:"debug-allocations,omitempty"`
	DisableUdfExecution     bool     `json:"disable-udf-execution,omitempty"`
	EnableBenchmarksFabric  bool     `json:"enable-benchmarks-fabric,omitempty"`
	EnableHealthCheck       bool     `json:"enable-health-check,omitempty"`
	EnableHistInfo          bool     `json:"enable-hist-info,omitempty"`
	EnforceBestPractices    bool     `json:"enforce-best-practices,omitempty"`
	FeatureKeyFile          string   `json:"feature-key-file,omitempty"`
	FeatureKeyFiles         []string `json:"feature-key-files,omitempty"`
	Group                   string   `json:"group,omitempty"`
	IndentAllocations       bool     `json:"indent-allocations,omitempty"`
	InfoMaxMs               int      `json:"info-max-ms,omitempty"`
	InfoThreads             int      `json:"info-threads,omitempty"`
	// Add more fields as needed
}

type AerospikeNetwork struct {
	// Define based on the actual schema
}

type AerospikeLogging struct {
	// Define based on the actual schema
}

type AerospikeXDR struct {
	// Define based on the actual schema
}

// Complete Database Create Request
type CreateDatabaseRequest struct {
	Name             string           `json:"name"`
	DataPlaneVersion string           `json:"dataPlaneVersion,omitempty"`
	Infrastructure   Infrastructure   `json:"infrastructure"`
	AerospikeCloud   interface{}      `json:"aerospikeCloud"` // Can be AerospikeCloudMemory, AerospikeCloudLocalDisk, or AerospikeCloudNetworkStorage
	AerospikeServer  *AerospikeServer `json:"aerospikeServer,omitempty"`
}

type UpdateDatabaseRequest struct {
	Name             string           `json:"name,omitempty"`
	DataPlaneVersion string           `json:"dataPlaneVersion,omitempty"`
	Infrastructure   *Infrastructure  `json:"infrastructure,omitempty"`
	AerospikeCloud   interface{}      `json:"aerospikeCloud,omitempty"`
	AerospikeServer  *AerospikeServer `json:"aerospikeServer,omitempty"`
}

// Database Credentials Types
type DatabaseCredentials struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Password   string `json:"password,omitempty"`
	Privileges string `json:"privileges"`
}

type DatabaseCredentialsCollection struct {
	Credentials []DatabaseCredentials `json:"credentials"`
	Total       int                   `json:"total"`
}

type CreateDatabaseCredentialsRequest struct {
	Name     string   `json:"name"`
	Password string   `json:"password"`
	Roles    []string `json:"roles"`
}

// VPC Peering Types
type VPCPeering struct {
	ID        string `json:"id"`
	VpcID     string `json:"vpcId"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type VPCPeeringCollection struct {
	VpcPeerings []VPCPeering `json:"vpcPeerings"`
	Total       int          `json:"total"`
}

type CreateVPCPeeringRequest struct {
	VpcID              string `json:"vpcId"`
	CIDRBlock          string `json:"cidrBlock"`
	AccountID          string `json:"accountId"`
	Region             string `json:"region"`
	IsSecureConnection bool   `json:"isSecureConnection"`
}

// Topology Types
type Topology struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type TopologyCollection struct {
	Topologies []Topology `json:"topologies"`
	Total      int        `json:"total"`
}

type CreateTopologyRequest struct {
	Name string `json:"name"`
	// Add other required fields based on the schema
}

// Database Metrics Types
type DatabaseMetrics struct {
	// Define based on the actual metrics schema
	Timestamp string                 `json:"timestamp"`
	Metrics   map[string]interface{} `json:"metrics"`
}

// Common collection structure
type CollectionCommon struct {
	Total int `json:"total"`
}

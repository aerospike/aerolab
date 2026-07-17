package clouds

import "time"

type Credentials struct {
	AWS     AWS     `yaml:"aws" json:"aws"`
	GCP     GCP     `yaml:"gcp" json:"gcp"`
	DOCKER  DOCKER  `yaml:"docker" json:"docker"`
	VAGRANT VAGRANT `yaml:"vagrant" json:"vagrant"`
}

type DOCKER struct {
	EnableDefaultFromEnv bool                    `yaml:"enableDefaultFromEnv" json:"enableDefaultFromEnv"`
	Regions              map[string]DockerRegion `yaml:"regions" json:"regions"` // map[regionName]definition-of-region
}

type DockerRegion struct {
	DockerHost     string        `yaml:"dockerHost" json:"dockerHost"`         // tcp://host:port, unix:///path/to/socket, ssh://user@host:port, http://host:port, https://host:port
	DockerCertPath string        `yaml:"dockerCertPath" json:"dockerCertPath"` // only use with https:// host type
	DockerKeyPath  string        `yaml:"dockerKeyPath" json:"dockerKeyPath"`   // only use with https:// host type
	DockerCaPath   string        `yaml:"dockerCaPath" json:"dockerCaPath"`     // only use with https:// host type
	Timeout        time.Duration `yaml:"timeout" json:"timeout"`               // connection timeout
}

type VAGRANT struct {
	// DefaultProvider is passed as --provider to vagrant up; empty = vagrant's own default resolution.
	DefaultProvider string `yaml:"defaultProvider" json:"defaultProvider"`
	// BinaryPath is the vagrant executable; empty = PATH lookup.
	BinaryPath string `yaml:"binaryPath" json:"binaryPath"`
	// Subnet for static private_network IPs, CIDR. Default 192.168.56.0/24.
	Subnet string `yaml:"subnet" json:"subnet"`
}

type AWS struct {
	AuthMethod AWSAuthMethod   `yaml:"authMethod" json:"authMethod"`
	Static     StaticAWSConfig `yaml:"static" json:"static"`
	Shared     SharedAWSConfig `yaml:"shared" json:"shared"`
	// SkipPricing, when true, disables all cost/pricing lookups (the AWS
	// Pricing API, spot-price history and volume pricing). Instance-type and
	// volume catalogs are still returned, just without prices. Useful when the
	// caller lacks pricing permissions or wants to avoid the extra API calls.
	SkipPricing bool `yaml:"skipPricing" json:"skipPricing"`
}

type AWSAuthMethod string

const (
	AWSAuthMethodShared = "shared"
	AWSAuthMethodStatic = "static"
)

type SharedAWSConfig struct {
	Profile string `yaml:"profile" json:"profile"`
}

type StaticAWSConfig struct {
	KeyID     string `yaml:"keyId" json:"keyId"`
	SecretKey string `yaml:"secretKey" json:"secretKey"`
}

type GCP struct {
	Project    string         `yaml:"project" json:"project"`
	AuthMethod GCPAuthMethod  `yaml:"authMethod" json:"authMethod"`
	Login      LoginGCPConfig `yaml:"login" json:"login"`
	// UseIAP, when true, routes SSH/SFTP traffic to GCE instances through
	// Identity-Aware Proxy TCP forwarding instead of dialing the routable
	// instance IP directly. This is the SOLE trigger for IAP usage; it is
	// intentionally independent of whether instances have public IPs.
	UseIAP bool `yaml:"useIAP" json:"useIAP"`
	// AutoEnableServices, when true, allows aerolab to enable required GCP
	// services (APIs) in the project without prompting, even in
	// non-interactive contexts. When false, aerolab prompts interactively and
	// errors in non-interactive contexts if a required service is not enabled.
	AutoEnableServices bool `yaml:"autoEnableServices" json:"autoEnableServices"`
	// SkipPricing, when true, disables all cost/pricing lookups (the Cloud
	// Billing SKU catalog for instance and volume prices). Instance-type and
	// volume catalogs are still returned, just without prices. Useful under
	// Workload Identity Federation, where the billing API rejects federated
	// tokens, or whenever the caller lacks billing permissions.
	SkipPricing bool `yaml:"skipPricing" json:"skipPricing"`
}

type GCPAuthMethod string

const (
	GCPAuthMethodServiceAccount = "service-account"
	GCPAuthMethodLogin          = "login"
	GCPAuthMethodAny            = "any"
)

type LoginGCPConfig struct {
	Secrets            *LoginGCPSecrets `yaml:"secrets" json:"secrets"`
	Browser            bool             `yaml:"browser" json:"browser"`
	TokenCacheFilePath string           `yaml:"tokenCacheFile" json:"tokenCacheFile"`
}

type LoginGCPSecrets struct {
	ClientID     string `json:"client_id" yaml:"client_id"`
	ClientSecret string `json:"client_secret" yaml:"client_secret"`
}

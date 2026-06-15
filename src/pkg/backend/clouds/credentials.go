package clouds

import "time"

type Credentials struct {
	AWS    AWS    `yaml:"aws" json:"aws"`
	GCP    GCP    `yaml:"gcp" json:"gcp"`
	DOCKER DOCKER `yaml:"docker" json:"docker"`
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

type AWS struct {
	AuthMethod AWSAuthMethod   `yaml:"authMethod" json:"authMethod"`
	Static     StaticAWSConfig `yaml:"static" json:"static"`
	Shared     SharedAWSConfig `yaml:"shared" json:"shared"`
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

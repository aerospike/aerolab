package clouds

type Credentials struct {
	AWS    AWS    `yaml:"aws" json:"aws"`
	GCP    GCP    `yaml:"gcp" json:"gcp"`
	DOCKER DOCKER `yaml:"docker" json:"docker"`
}

type DOCKER struct {
	// TODO: Add docker credentials
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

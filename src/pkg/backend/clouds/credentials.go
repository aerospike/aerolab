package clouds

type Credentials struct {
	AWS AWS `yaml:"aws" json:"aws"`
}

type AWS struct {
	AuthMethod AWSAuthMethod `yaml:"authMethod" json:"authMethod"`
	Static     struct {
		KeyID     string `yaml:"keyId" json:"keyId"`
		SecretKey string `yaml:"secretKey" json:"secretKey"`
	} `yaml:"static" json:"static"`
	Shared struct {
		Profile string `yaml:"profile" json:"profile"`
	} `yaml:"shared" json:"shared"`
}

type AWSAuthMethod string

const (
	AWSAuthMethodShared = "shared"
	AWSAuthMethodStatic = "static"
)

// TODO: add GCP; docker doesn't need credentials, as docker host IPs will be the regions

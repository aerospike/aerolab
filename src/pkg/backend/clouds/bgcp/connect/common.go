package connect

import (
	_ "embed"
	"encoding/json"

	"github.com/aerospike/aerolab/pkg/backend/clouds"
)

//go:embed default_secrets.json
var defaultSecrets []byte

// getSecrets returns the secrets for the Google Cloud Platform.
func getSecrets() (*clouds.LoginGCPSecrets, error) {
	var secrets clouds.LoginGCPSecrets
	if err := json.Unmarshal(defaultSecrets, &secrets); err != nil {
		return nil, err
	}
	return &secrets, nil
}

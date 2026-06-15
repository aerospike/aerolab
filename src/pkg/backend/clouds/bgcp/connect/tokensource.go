package connect

import (
	"fmt"

	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/rglonek/logger"
	"golang.org/x/oauth2"
)

// GetTokenSource returns an oauth2.TokenSource for the given GCP credentials,
// honouring the same auth-method ladder as GetClient/GetCredentials
// (service-account, login, any).
//
// It is intended for downstream consumers that need a TokenSource directly --
// e.g. the IAP TCP-forwarding client in pkg/backend/clouds/bgcp/iap, which
// authenticates each WebSocket dial with a fresh OAuth token.
func GetTokenSource(creds *clouds.GCP, log *logger.Logger) (oauth2.TokenSource, error) {
	gcreds, err := GetCredentials(creds, log)
	if err != nil {
		return nil, err
	}
	if gcreds == nil || gcreds.TokenSource == nil {
		return nil, fmt.Errorf("no usable token source for GCP auth method %q", creds.AuthMethod)
	}
	return gcreds.TokenSource, nil
}

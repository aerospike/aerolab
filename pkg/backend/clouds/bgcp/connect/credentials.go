package connect

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"cloud.google.com/go/auth"
	authcreds "cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/auth/oauth2adapt"
	"github.com/citrusleaf/aerolab/pkg/backend/clouds"
	"github.com/citrusleaf/aerolab/pkg/utils/openbrowser"
	"github.com/google/uuid"
	"github.com/rglonek/logger"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

// cloudPlatformScope is the OAuth2 scope requested for Application Default
// Credentials. It is the superset scope accepted by all GCP APIs aerolab uses.
const cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

func GetCredentials(creds *clouds.GCP, log *logger.Logger) (*google.Credentials, error) {
	if log == nil {
		log = logger.NewLogger()
	}
	if creds == nil {
		return nil, fmt.Errorf("credentials are nil")
	}
	switch creds.AuthMethod {
	case clouds.GCPAuthMethodServiceAccount:
		log.Debug("Attempting to use instance service account credentials")
		return getDefaultCredentials(log)
	case clouds.GCPAuthMethodLogin:
		log.Debug("Attempting to use OAuth2 credentials")
		return getOAuth2Credentials(log, creds.Login.TokenCacheFilePath, creds.Login.Browser, creds.Login.Secrets)
	case clouds.GCPAuthMethodAny:
		log.Debug("Attempting to use instance service account credentials")
		if creds, err := getDefaultCredentials(log); err == nil {
			return creds, nil
		}
		log.Debug("Failed to use instance service account credentials; attempting to use OAuth2 credentials")
		return getOAuth2Credentials(log, creds.Login.TokenCacheFilePath, creds.Login.Browser, creds.Login.Secrets)
	}
	return nil, fmt.Errorf("unsupported auth method: %s", creds.AuthMethod)
}

// getDefaultCredentials gets Application Default Credentials for the Google
// Cloud Platform. log is the logger to use for logging; all logging is done at
// the debug level.
func getDefaultCredentials(log *logger.Logger) (*google.Credentials, error) {
	authCreds, err := detectDefaultAuthCredentials(log)
	if err != nil {
		return nil, err
	}
	// Adapt to *google.Credentials for the token-source based call paths
	// (option.WithCredentials and the IAP token source). option.WithCredentials
	// still routes through the client library's own auth transport, so it
	// already behaves like aerolab v7; only the getDefaultClient path (below)
	// needed to stop wrapping the token in a bare oauth2 client.
	return oauth2adapt.Oauth2CredentialsFromAuthCredentials(authCreds), nil
}

// detectDefaultAuthCredentials resolves Application Default Credentials using the
// modern cloud.google.com/go/auth stack. Unlike the legacy
// golang.org/x/oauth2/google.FindDefaultCredentials path, this fully supports
// Workload Identity Federation (external_account), impersonated service
// accounts, executable/URL/file credential sources and the STS token exchange.
//
// It returns the native *auth.Credentials so getDefaultClient can hand it to the
// client library's own HTTP transport (httptransport.NewClient) -- exactly how
// the GCP client libraries authenticate internally, which is how aerolab v7
// authenticated -- instead of wrapping it in a bare oauth2 client.
func detectDefaultAuthCredentials(log *logger.Logger) (*auth.Credentials, error) {
	authCreds, err := authcreds.DetectDefault(&authcreds.DetectOptions{
		Scopes: []string{cloudPlatformScope},
	})
	if err != nil {
		log.Debug("No default credentials found: %v", err)
		return nil, err
	}
	log.Debug("Using default credentials resolved via cloud.google.com/go/auth (WIF-capable)")
	return authCreds, nil
}

// quotaProjectFor decides which project to send as the X-Goog-User-Project
// (billing / quota) header. It returns configuredProject only when the resolved
// credentials have no project of their own (credProjectID == ""), i.e. Workload
// Identity Federation with direct federation, whose federated access tokens are
// not associated with a project and are otherwise rejected with 401 "Invalid
// Credentials".
//
// Credentials that already carry a project (service-account keys, GCE metadata,
// impersonated WIF) return "" so we do NOT force the quota header on them.
// Forcing it would newly require every such principal to hold
// roles/serviceusage.serviceUsageConsumer on the quota project, turning working
// setups into 403 PermissionDenied failures.
func quotaProjectFor(credProjectID, configuredProject string) string {
	if credProjectID == "" {
		return configuredProject
	}
	return ""
}

// QuotaProjectOption returns a client option setting the X-Goog-User-Project
// (billing / quota) header to configuredProject, but only when cli has no
// project of its own (see quotaProjectFor). Otherwise it returns a no-op
// option.WithQuotaProject("").
//
// Use this at option.WithCredentials call sites so that Workload Identity
// Federation principals get a quota project (required) while ordinary
// service-account principals are left unchanged.
func QuotaProjectOption(cli *google.Credentials, configuredProject string) option.ClientOption {
	credProjectID := ""
	if cli != nil {
		credProjectID = cli.ProjectID
	}
	return option.WithQuotaProject(quotaProjectFor(credProjectID, configuredProject))
}

// getOAuth2Client gets an authenticated client for the Google Cloud Platform.
// log is the logger to use for logging; all logging is done at the debug level.
// tokenCacheFilePath is the file path to cache the token in.
// browser is a flag to enable opening the browser for the OAuth flow.
// secrets is the client ID and client secret for the Google Cloud Platform; it is required, aerolab ships no built-in client ID.
func getOAuth2Credentials(log *logger.Logger, tokenCacheFilePath string, browser bool, secrets *clouds.LoginGCPSecrets) (*google.Credentials, error) {
	if secrets == nil || secrets.ClientID == "" {
		return nil, fmt.Errorf("client ID is required")
	}
	config := &oauth2.Config{
		ClientID:     secrets.ClientID,
		ClientSecret: secrets.ClientSecret,
		Scopes: []string{
			cloudPlatformScope,
		},
		Endpoint: google.Endpoint,
	}

	// Try to load the token from file.
	var token *oauth2.Token
	if tokenCacheFilePath != "" {
		var err error
		token, err = tokenFromFile(tokenCacheFilePath)
		if err == nil {
			log.Debug("Using cached access token: %s", tokenFingerprint(token.AccessToken))
			ts := config.TokenSource(context.Background(), token)
			return &google.Credentials{TokenSource: ts}, nil
		}
	}

	// No valid token found; perform OAuth flow.
	// Start a listener on a random available port on localhost.
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start listener: %v", err)
	}
	defer listener.Close()

	// Extract the allocated port and build the redirect URL.
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://localhost:%d", port)
	config.RedirectURL = redirectURL

	stateToken := uuid.New().String()

	// Build the authorization URL.
	authURL := config.AuthCodeURL(stateToken, oauth2.AccessTypeOffline)
	if !browser {
		fmt.Println("Please navigate to:")
		fmt.Println(authURL)
	} else {
		fmt.Println("Your browser will be opened to visit the Google sign-in page. If it doesn't open automatically, please navigate to:")
		fmt.Println(authURL)
		if err := openbrowser.Open(authURL); err != nil {
			fmt.Printf("Error opening browser: %v\n", err)
		}
	}

	// Channel to receive the token.
	tokenChan := make(chan *oauth2.Token)
	handler := func(w http.ResponseWriter, r *http.Request) {
		requestState := r.URL.Query().Get("state")
		if requestState != stateToken {
			http.Error(w, "State token mismatch", http.StatusBadRequest)
			if requestState != "" {
				log.Debug("Invalid state: expected %q, got %q", stateToken, requestState)
			}
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "No code found in the callback", http.StatusBadRequest)
			log.Debug("No code found in the callback")
			return
		}

		tok, err := config.Exchange(context.Background(), code)
		if err != nil {
			http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
			log.Debug("Token exchange error: %v", err)
			return
		}

		// Notify the user and send the token through the channel.
		fmt.Fprintln(w, "Authentication complete. You may close this window.") //nolint:errcheck
		tokenChan <- tok
	}

	server := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", port),
		Handler: http.HandlerFunc(handler),
	}
	defer server.Close()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Debug("Server terminated: %v", err)
		} else {
			log.Debug("Server closed successfully")
		}
	}()

	// Wait for the token.
	token = <-tokenChan
	log.Debug("Access token obtained: %s", tokenFingerprint(token.AccessToken))

	// Save the token for future use.
	if tokenCacheFilePath != "" {
		if err := saveToken(tokenCacheFilePath, token); err != nil {
			log.Warn("Failed to save token: %v", err)
		}
	}
	// Create a client that automatically refreshes the token.
	ts := config.TokenSource(context.Background(), token)
	return &google.Credentials{TokenSource: ts}, nil
}

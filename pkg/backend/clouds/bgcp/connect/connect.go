package connect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"cloud.google.com/go/auth/httptransport"
	"github.com/aerospike-community/aerolab/pkg/backend/clouds"
	"github.com/aerospike-community/aerolab/pkg/utils/openbrowser"
	"github.com/google/uuid"
	"github.com/rglonek/logger"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func GetClient(creds *clouds.GCP, log *logger.Logger) (*http.Client, error) {
	return getClient(creds, log, false)
}

// GetBillingClient returns an authenticated HTTP client for the Cloud Billing
// catalog API (cloudbilling.googleapis.com). Unlike GetClient it ALWAYS sends
// the X-Goog-User-Project (billing / quota) header.
//
// The SKU catalog is a global, non project-scoped API: a Workload Identity
// Federation access token has no project to bill the request against, so GCP
// rejects it with 401 "Invalid Credentials" unless a quota project is supplied
// explicitly. Project-scoped APIs (compute, serviceusage) derive their project
// from the request URL and therefore work without the header, so GetClient only
// sends it conditionally to avoid newly requiring roles/serviceusage.serviceUsageConsumer.
func GetBillingClient(creds *clouds.GCP, log *logger.Logger) (*http.Client, error) {
	return getClient(creds, log, true)
}

func getClient(creds *clouds.GCP, log *logger.Logger, forceQuotaProject bool) (*http.Client, error) {
	if log == nil {
		log = logger.NewLogger()
	}
	if creds == nil {
		return nil, fmt.Errorf("credentials are nil")
	}
	switch creds.AuthMethod {
	case clouds.GCPAuthMethodServiceAccount:
		log.Debug("Attempting to use instance service account credentials")
		return getDefaultClient(log, creds.Project, forceQuotaProject)
	case clouds.GCPAuthMethodLogin:
		log.Debug("Attempting to use OAuth2 credentials")
		return getOAuth2Client(log, creds.Login.TokenCacheFilePath, creds.Login.Browser, creds.Login.Secrets)
	case clouds.GCPAuthMethodAny:
		log.Debug("Attempting to use instance service account credentials")
		if client, err := getDefaultClient(log, creds.Project, forceQuotaProject); err == nil {
			return client, nil
		}
		log.Debug("Failed to use instance service account credentials; attempting to use OAuth2 credentials")
		return getOAuth2Client(log, creds.Login.TokenCacheFilePath, creds.Login.Browser, creds.Login.Secrets)
	}
	return nil, fmt.Errorf("unsupported auth method: %s", creds.AuthMethod)
}

// getDefaultClient gets an authenticated client for the Google Cloud Platform.
// log is the logger to use for logging; all logging is done at the debug level.
// configuredProject is the project aerolab is configured to use; it is sent as
// the X-Goog-User-Project (billing / quota) header only when the resolved
// credentials have no project of their own (see quotaProjectFor).
//
// Callers inject this client with option.WithHTTPClient, which bypasses the
// Google client library's own credential resolution. To stay faithful to how
// aerolab v7 (and the GCP client libraries) authenticate, we build the client
// through the library's own auth transport (httptransport.NewClient) from
// Application Default Credentials resolved via the modern cloud.google.com/go/auth
// stack. That transport attaches the OAuth2 bearer token and universe-domain
// handling -- unlike a bare oauth2.NewClient, which only sets the Authorization
// header and was insufficient for Workload Identity Federation principals.
//
// It also mirrors the gcloud CLI by sending the configured project as the
// X-Goog-User-Project quota header for Workload Identity Federation principals:
// their federated access token is not associated with a project of its own, so
// GCP APIs reject the request with 401 "Invalid Credentials" unless a
// billing/quota project is supplied. The library only derives this header from
// the credential file's quota_project_id, which the google-github-actions/auth
// external_account file does not populate; gcloud works because it sends the
// project from CLOUDSDK_CORE_PROJECT. For credentials that already carry a
// project the header is omitted, so those principals are not newly required to
// hold roles/serviceusage.serviceUsageConsumer.
func getDefaultClient(log *logger.Logger, configuredProject string, forceQuotaProject bool) (*http.Client, error) {
	authCreds, err := detectDefaultAuthCredentials(log)
	if err != nil {
		return nil, err
	}
	opts := &httptransport.Options{
		Credentials: authCreds,
	}
	quotaProject := ""
	if forceQuotaProject {
		// Global (non project-scoped) APIs such as the Cloud Billing SKU catalog
		// always need an explicit billing/quota project under federated (WIF)
		// tokens; see GetBillingClient.
		quotaProject = configuredProject
	} else {
		credProjectID, _ := authCreds.ProjectID(context.Background())
		quotaProject = quotaProjectFor(credProjectID, configuredProject)
	}
	if quotaProject != "" {
		log.Debug("Setting X-Goog-User-Project quota project to %q", quotaProject)
		opts.Headers = http.Header{}
		opts.Headers.Set("X-Goog-User-Project", quotaProject)
	}
	client, err := httptransport.NewClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build authenticated HTTP client: %w", err)
	}
	return client, nil
}

// getOAuth2Client gets an authenticated client for the Google Cloud Platform.
// log is the logger to use for logging; all logging is done at the debug level.
// tokenCacheFilePath is the file path to cache the token in.
// browser is a flag to enable opening the browser for the OAuth flow.
// secrets is the client ID and client secret for the Google Cloud Platform; it is required, aerolab ships no built-in client ID.
func getOAuth2Client(log *logger.Logger, tokenCacheFilePath string, browser bool, secrets *clouds.LoginGCPSecrets) (*http.Client, error) {
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
			return config.Client(context.Background(), token), nil
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
	return config.Client(context.Background(), token), nil
}

// tokenFromFile retrieves a token from a given file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var token oauth2.Token
	if err := json.NewDecoder(f).Decode(&token); err != nil {
		return nil, err
	}
	return &token, nil
}

// saveToken writes a token to a file.
func saveToken(file string, token *oauth2.Token) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}

// tokenFingerprint renders an OAuth access token as a short digest plus its
// length. Debug logs are routinely pasted into bug reports and shipped to log
// aggregators, and a raw bearer token in one grants whoever reads it the same
// cloud access the user has. The digest is enough to tell two tokens apart or
// confirm that a refresh happened.
func tokenFingerprint(accessToken string) string {
	if accessToken == "" {
		return "<empty>"
	}
	sum := sha256.Sum256([]byte(accessToken))
	return fmt.Sprintf("sha256:%s (len %d)", hex.EncodeToString(sum[:])[:12], len(accessToken))
}

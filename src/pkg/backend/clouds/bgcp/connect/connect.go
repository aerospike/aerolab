package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/google/uuid"
	"github.com/rglonek/logger"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func GetClient(creds *clouds.GCP, log *logger.Logger) (*http.Client, error) {
	if log == nil {
		log = logger.NewLogger()
	}
	if creds == nil {
		return nil, fmt.Errorf("credentials are nil")
	}
	switch creds.AuthMethod {
	case clouds.GCPAuthMethodServiceAccount:
		log.Debug("Attempting to use instance service account credentials")
		return getDefaultClient(log)
	case clouds.GCPAuthMethodLogin:
		log.Debug("Attempting to use OAuth2 credentials")
		return getOAuth2Client(log, creds.Login.TokenCacheFilePath, creds.Login.Browser, creds.Login.Secrets)
	case clouds.GCPAuthMethodAny:
		log.Debug("Attempting to use instance service account credentials")
		if client, err := getDefaultClient(log); err == nil {
			return client, nil
		}
		log.Debug("Failed to use instance service account credentials; attempting to use OAuth2 credentials")
		return getOAuth2Client(log, creds.Login.TokenCacheFilePath, creds.Login.Browser, creds.Login.Secrets)
	}
	return nil, fmt.Errorf("unsupported auth method: %s", creds.AuthMethod)
}

// getDefaultClient gets an authenticated client for the Google Cloud Platform.
// log is the logger to use for logging; all logging is done at the debug level.
func getDefaultClient(log *logger.Logger) (*http.Client, error) {
	ctx := context.Background()
	client, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err == nil {
		log.Debug("Using instance service account credentials")
		return client, nil
	}
	log.Debug("No instance service account found: %v", err)
	return nil, err
}

// getOAuth2Client gets an authenticated client for the Google Cloud Platform.
// log is the logger to use for logging; all logging is done at the debug level.
// tokenCacheFilePath is the file path to cache the token in.
// browser is a flag to enable opening the browser for the OAuth flow.
// secrets is the client ID and client secret for the Google Cloud Platform; if not provided, embedded secrets are used.
func getOAuth2Client(log *logger.Logger, tokenCacheFilePath string, browser bool, secrets *clouds.LoginGCPSecrets) (*http.Client, error) {
	if secrets == nil {
		var err error
		secrets, err = getSecrets()
		if err != nil {
			return nil, fmt.Errorf("failed to get secrets: %v", err)
		}
	}
	config := &oauth2.Config{
		ClientID:     secrets.ClientID,
		ClientSecret: secrets.ClientSecret,
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
		},
		Endpoint: google.Endpoint,
	}

	// Try to load the token from file.
	var token *oauth2.Token
	if tokenCacheFilePath != "" {
		var err error
		token, err = tokenFromFile(tokenCacheFilePath)
		if err == nil {
			log.Debug("Using cached access token: %s", token.AccessToken)
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
		openBrowser(authURL)
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
		fmt.Fprintln(w, "Authentication complete. You may close this window.")
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
	log.Debug("Access Token: %s\n", token.AccessToken)

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

// openBrowser attempts to open the URL in the default browser.
func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Printf("Error opening browser: %v\n", err)
	}
}

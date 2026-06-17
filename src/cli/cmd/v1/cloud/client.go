package cloud

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type Client struct {
	httpClient  *resty.Client
	baseURL     string
	authURL     string
	apiKey      string
	apiSecret   string
	accessToken string
}

// TokenResponse represents the OAuth2 token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func NewClient(version string) (*Client, error) {
	return newClient(version, os.Getenv("AEROSPIKE_CLOUD_ENV") == "dev")
}

func newClient(version string, isDev bool) (*Client, error) {
	apiKey := os.Getenv("AEROSPIKE_CLOUD_KEY")
	apiSecret := os.Getenv("AEROSPIKE_CLOUD_SECRET")

	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("AEROSPIKE_CLOUD_KEY and AEROSPIKE_CLOUD_SECRET environment variables must be set")
	}

	devString := ""
	if isDev {
		devString = "-dev"
	}

	client := &Client{
		httpClient: resty.New(),
		baseURL:    fmt.Sprintf("https://api%s.aerospike.com/%s", devString, version),
		authURL:    fmt.Sprintf("https://auth.control%s.aerospike.cloud/oauth/token", devString),
		apiKey:     apiKey,
		apiSecret:  apiSecret,
	}

	client.httpClient.SetBaseURL(client.baseURL)
	client.httpClient.SetTimeout(30 * time.Second)
	client.httpClient.SetHeader("Content-Type", "application/json")
	client.httpClient.SetHeader("Accept", "application/json")

	// Get access token using OAuth2 client credentials flow
	if err := client.authenticate(); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return client, nil
}

// authenticate gets an OAuth2 access token using client credentials flow
func (c *Client) authenticate() error {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.apiKey)
	data.Set("client_secret", c.apiSecret)

	req, err := http.NewRequest("POST", c.authURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	c.httpClient.SetAuthToken(c.accessToken)

	return nil
}

func (c *Client) Get(path string, result any) error {
	resp, err := c.httpClient.R().
		SetResult(result).
		Get(path)

	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return fmt.Errorf("API error: %s - %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

func (c *Client) Post(path string, body any, result any) error {
	resp, err := c.httpClient.R().
		SetBody(body).
		SetResult(result).
		Post(path)

	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return fmt.Errorf("API error: %s - %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

func (c *Client) Patch(path string, body any, result any) error {
	resp, err := c.httpClient.R().
		SetBody(body).
		SetResult(result).
		Patch(path)

	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return fmt.Errorf("API error: %s - %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

func (c *Client) Delete(path string) error {
	resp, err := c.httpClient.R().
		Delete(path)

	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return fmt.Errorf("API error: %s - %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

func (c *Client) Put(path string, body any, result any) error {
	resp, err := c.httpClient.R().
		SetBody(body).
		SetResult(result).
		Put(path)

	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return fmt.Errorf("API error: %s - %s", resp.Status(), string(resp.Body()))
	}

	return nil
}

// Helper method to pretty print JSON
func (c *Client) PrettyPrint(data any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonData))
	return nil
}

// GetAccessToken returns the current access token
func (c *Client) GetAccessToken() string {
	return c.accessToken
}

// GetStatus performs an authenticated GET and returns the HTTP status code
// along with the raw response body. Unlike Get, it does not synthesize an
// error for 4xx/5xx responses; callers inspect the status code themselves.
// Only network-level failures are surfaced via the returned error.
func (c *Client) GetStatus(path string) (int, []byte, error) {
	resp, err := c.httpClient.R().Get(path)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	return resp.StatusCode(), resp.Body(), nil
}

// OrgID extracts the Aerospike Cloud organization id from the access token.
// The id is published by the auth server as the "dbaas.aerospike.com/org_id"
// JWT claim (see https://api.aerospike.com docs). An empty string is returned
// when the token is unset, malformed, or the claim is absent.
func (c *Client) OrgID() string {
	return parseOrgIDFromJWT(c.accessToken)
}

// parseOrgIDFromJWT decodes the JWT payload (second dot-separated segment) and
// looks for the Aerospike Cloud org-id claim. Returns "" on any parse failure.
func parseOrgIDFromJWT(token string) string {
	if token == "" {
		return ""
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	// JWT payloads use base64url without padding; tolerate both that and
	// padded base64 for robustness against future encoder changes.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	if v, ok := claims["dbaas.aerospike.com/org_id"].(string); ok && v != "" {
		return v
	}
	return ""
}

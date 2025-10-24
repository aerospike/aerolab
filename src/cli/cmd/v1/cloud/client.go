package cloud

import (
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

func NewClient() (*Client, error) {
	apiKey := os.Getenv("AEROSPIKE_CLOUD_KEY")
	apiSecret := os.Getenv("AEROSPIKE_CLOUD_SECRET")

	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("AEROSPIKE_CLOUD_KEY and AEROSPIKE_CLOUD_SECRET environment variables must be set")
	}

	client := &Client{
		httpClient: resty.New(),
		baseURL:    "https://api.aerospike.cloud/v2",
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
	tokenURL := "https://auth.control.aerospike.cloud/oauth/token"

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.apiKey)
	data.Set("client_secret", c.apiSecret)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
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

func (c *Client) Get(path string, result interface{}) error {
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

func (c *Client) Post(path string, body interface{}, result interface{}) error {
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

func (c *Client) Patch(path string, body interface{}, result interface{}) error {
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

func (c *Client) Put(path string, body interface{}, result interface{}) error {
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
func (c *Client) PrettyPrint(data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonData))
	return nil
}

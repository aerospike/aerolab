package jfrog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// httpDo is the seam tests can stub out. Production code goes through
// http.DefaultTransport with the per-call timeout from Config.
var httpDo = func(req *http.Request, timeout time.Duration) (*http.Response, error) {
	c := &http.Client{
		Timeout:   timeout,
		Transport: http.DefaultTransport,
	}
	return c.Do(req)
}

func (c *Config) timeout() time.Duration {
	if c.Timeout <= 0 {
		return 30 * time.Second
	}
	return c.Timeout
}

// AQL POSTs the given AQL query body and returns the raw response body.
// The caller is responsible for parsing the JSON.
func (c *Config) AQL(ctx context.Context, query string) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("jfrog: nil config")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.ArtifactoryURL("/api/search/aql"),
		strings.NewReader(query))
	if err != nil {
		return nil, fmt.Errorf("jfrog: build AQL request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Accept", "application/json")
	c.ApplyAuth(req)

	resp, err := httpDo(req, c.timeout())
	if err != nil {
		return nil, fmt.Errorf("jfrog: AQL request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jfrog: read AQL response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jfrog: AQL HTTP %d: %s", resp.StatusCode, truncate(string(body), 512))
	}
	return body, nil
}

// Get performs an authenticated GET against an absolute URL and streams
// the body to w. Used for artifact downloads.
func (c *Config) Get(ctx context.Context, url string, w io.Writer) (int64, error) {
	if c == nil {
		return 0, fmt.Errorf("jfrog: nil config")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("jfrog: build GET request: %w", err)
	}
	c.ApplyAuth(req)
	resp, err := httpDo(req, c.timeout())
	if err != nil {
		return 0, fmt.Errorf("jfrog: GET %s failed: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("jfrog: GET %s: HTTP %d: %s", url, resp.StatusCode, truncate(string(body), 512))
	}
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, fmt.Errorf("jfrog: stream body: %w", err)
	}
	return n, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

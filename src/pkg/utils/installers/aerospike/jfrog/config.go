// Package jfrog implements an alternative download source for Aerospike
// server install artifacts hosted on a JFrog Artifactory instance. The
// public download.aerospike.com path serves release builds via static HTML
// directory listings; JFrog hosts pre-release / dev builds keyed by build
// name + build number and indexed via the AQL REST API.
//
// All JFrog-specific information (base URL, credentials, build name) is
// supplied by the operator through environment variables so this code can
// ship in an open-source binary without hard-coding any vendor URLs or
// secrets.
package jfrog

import (
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"time"
)

// Environment variable names recognised by the jfrog driver.
const (
	EnvArtifactsURL       = "AEROLAB_ARTIFACTS_URL"
	EnvArtifactsAuth      = "AEROLAB_ARTIFACTS_AUTH"
	EnvArtifactsBuildName = "AEROLAB_ARTIFACTS_BUILD_NAME"

	// DefaultBuildName is what users get if AEROLAB_ARTIFACTS_BUILD_NAME
	// is not set. The JFrog convention used by Aerospike's pipelines is a
	// single build named "aerospike-server" with one build number per CI
	// run.
	DefaultBuildName = "aerospike-server"
)

// Config captures everything needed to talk to a JFrog Artifactory
// instance. Build with FromEnv or compose manually.
type Config struct {
	// BaseURL is the Artifactory base, e.g. https://aerospike.jfrog.io.
	// The "/artifactory" suffix is appended automatically if missing.
	BaseURL string
	// Auth is the raw auth string supplied by the operator. Anything
	// goes - bearer token, basic header value, "user:pass", legacy
	// API key. ApplyAuth() sniffs the format.
	Auth string
	// BuildName defaults to DefaultBuildName.
	BuildName string
	// Timeout per HTTP request. Defaults to 30s.
	Timeout time.Duration
}

// FromEnv reads the configuration out of the environment. Returns nil if
// AEROLAB_ARTIFACTS_URL is not set or does not look like a JFrog URL.
func FromEnv() *Config {
	url := strings.TrimSpace(os.Getenv(EnvArtifactsURL))
	if url == "" {
		return nil
	}
	if !IsJFrogURL(url) {
		return nil
	}
	bn := strings.TrimSpace(os.Getenv(EnvArtifactsBuildName))
	if bn == "" {
		bn = DefaultBuildName
	}
	return &Config{
		BaseURL:   normalizeBaseURL(url),
		Auth:      os.Getenv(EnvArtifactsAuth),
		BuildName: bn,
		Timeout:   30 * time.Second,
	}
}

// IsJFrogURL reports whether the given URL looks like a JFrog Artifactory
// endpoint. The check is intentionally narrow: only ".jfrog.io" triggers
// the JFrog flow; anything else lets the caller fall back to the plain
// HTML mirror path.
func IsJFrogURL(url string) bool {
	return strings.Contains(strings.ToLower(url), ".jfrog.io")
}

// normalizeBaseURL strips trailing slashes and the "/ui/..." or
// "/artifactory/..." suffix users may have copied out of the JFrog
// browser. It returns a clean "<scheme>://<host>" form so callers can
// build "/artifactory/api/..." or "/artifactory/<repo>/..." paths without
// guessing.
func normalizeBaseURL(in string) string {
	u := strings.TrimRight(in, "/")
	// drop the "/ui/..." UI prefix
	if i := strings.Index(u, "/ui/"); i >= 0 {
		u = u[:i]
	}
	// drop "/artifactory[/...]" so we can append it consistently later
	if i := strings.Index(u, "/artifactory"); i >= 0 {
		u = u[:i]
	}
	return strings.TrimRight(u, "/")
}

// ArtifactoryURL returns "<base>/artifactory<suffix>". suffix should start
// with "/", e.g. "/api/search/aql" or "/<repo>/<path>/<name>".
func (c *Config) ArtifactoryURL(suffix string) string {
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	return c.BaseURL + "/artifactory" + suffix
}

// BuildInfoURL returns the JFrog UI link for browsing a build run. This
// is a best-effort human-facing link (the SPA route resolves the build by
// name + number); it lives under "/ui/builds", not "/artifactory".
func (c *Config) BuildInfoURL(name, number string) string {
	return c.BaseURL + "/ui/builds/" + name + "/" + number
}

// ApplyAuth sets the appropriate Authorization or X-JFrog-Art-Api header
// on req based on the format of c.Auth. The sniffing order is:
//
//  1. literal "Bearer ..." / "Basic ..." → used verbatim
//  2. JWT (starts with "eyJ") → Bearer
//  3. JFrog reference / API-key prefixes (AKC, cmVm) → X-JFrog-Art-Api
//  4. contains ':' and no spaces → assumed "user:pass" → Basic
//  5. anything else → assumed bearer token
//
// An empty Auth leaves the request unauthenticated.
func (c *Config) ApplyAuth(req *http.Request) {
	raw := strings.TrimSpace(c.Auth)
	if raw == "" {
		return
	}
	switch {
	case strings.HasPrefix(raw, "Bearer "), strings.HasPrefix(raw, "Basic "):
		req.Header.Set("Authorization", raw)
	case strings.HasPrefix(raw, "eyJ"):
		req.Header.Set("Authorization", "Bearer "+raw)
	case strings.HasPrefix(raw, "AKC"), strings.HasPrefix(raw, "cmVm"):
		req.Header.Set("X-JFrog-Art-Api", raw)
	case strings.Contains(raw, ":") && !strings.ContainsAny(raw, " \t"):
		req.Header.Set("Authorization", "Basic "+
			base64.StdEncoding.EncodeToString([]byte(raw)))
	default:
		req.Header.Set("Authorization", "Bearer "+raw)
	}
}

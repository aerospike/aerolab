package jfrog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsJFrogURL(t *testing.T) {
	cases := map[string]bool{
		"": false,
		"https://download.aerospike.com/artifacts": false,
		"https://my-mirror.example.com/artifacts":  false,
		"https://aerospike.jfrog.io":               true,
		"https://AEROSPIKE.JFROG.IO/ui/builds/...": true, // case-insensitive
		"https://aerospike.jfrog.io/artifactory":   true,
	}
	for in, want := range cases {
		if got := IsJFrogURL(in); got != want {
			t.Errorf("IsJFrogURL(%q) = %v; want %v", in, got, want)
		}
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://aerospike.jfrog.io":                                            "https://aerospike.jfrog.io",
		"https://aerospike.jfrog.io/":                                           "https://aerospike.jfrog.io",
		"https://aerospike.jfrog.io/artifactory":                                "https://aerospike.jfrog.io",
		"https://aerospike.jfrog.io/artifactory/api/search/aql":                 "https://aerospike.jfrog.io",
		"https://aerospike.jfrog.io/ui/builds/aerospike-server":                 "https://aerospike.jfrog.io",
		"https://aerospike.jfrog.io/ui/builds/aerospike-server/8.1.3.0-28/abc/": "https://aerospike.jfrog.io",
	}
	for in, want := range cases {
		if got := normalizeBaseURL(in); got != want {
			t.Errorf("normalizeBaseURL(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestConfigArtifactoryURL(t *testing.T) {
	c := &Config{BaseURL: "https://example.jfrog.io"}
	if got, want := c.ArtifactoryURL("/api/search/aql"), "https://example.jfrog.io/artifactory/api/search/aql"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got, want := c.ArtifactoryURL("foo/bar"), "https://example.jfrog.io/artifactory/foo/bar"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestApplyAuth(t *testing.T) {
	cases := []struct {
		raw     string
		hdrName string
		hdrVal  string
	}{
		{"", "", ""},
		{"Bearer abc", "Authorization", "Bearer abc"},
		{"Basic abc", "Authorization", "Basic abc"},
		{"eyJhbGciOiJI", "Authorization", "Bearer eyJhbGciOiJI"},
		{"AKCp8jQc", "X-JFrog-Art-Api", "AKCp8jQc"},
		{"cmVmdGtuOg", "X-JFrog-Art-Api", "cmVmdGtuOg"},
		{"user:pass", "Authorization", "Basic dXNlcjpwYXNz"},
		{"opaque-bearer-token", "Authorization", "Bearer opaque-bearer-token"},
	}
	for _, tc := range cases {
		c := &Config{Auth: tc.raw}
		req, _ := http.NewRequest("GET", "https://example.jfrog.io/", nil)
		c.ApplyAuth(req)
		if tc.hdrName == "" {
			if len(req.Header) > 0 {
				t.Errorf("auth=%q: expected no header, got %+v", tc.raw, req.Header)
			}
			continue
		}
		if got := req.Header.Get(tc.hdrName); got != tc.hdrVal {
			t.Errorf("auth=%q: header %s=%q; want %q", tc.raw, tc.hdrName, got, tc.hdrVal)
		}
	}
}

func TestAQL_AuthAndContentType(t *testing.T) {
	var sawAuth, sawCT, sawBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawCT = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		sawBody = string(buf)
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	c := &Config{BaseURL: srv.URL, Auth: "Bearer testtoken"}
	body, err := c.AQL(context.TODO(), `items.find({"@build.name":"x"})`)
	if err != nil {
		t.Fatalf("AQL: %v", err)
	}
	if !strings.Contains(string(body), "results") {
		t.Errorf("response not echoed: %s", body)
	}
	if sawAuth != "Bearer testtoken" {
		t.Errorf("auth header = %q", sawAuth)
	}
	if sawCT != "text/plain" {
		t.Errorf("content-type = %q", sawCT)
	}
	if !strings.Contains(sawBody, "@build.name") {
		t.Errorf("body = %q", sawBody)
	}
}

func TestResolveBuild(t *testing.T) {
	c := &Config{BaseURL: "https://example.jfrog.io", BuildName: "aerospike-server"}
	cases := map[string]string{
		"8.1.3.0-28-g302194ebc":           "8.1.3.0-28-g302194ebc-artifacts",
		"8.1.3.0-28-g302194ebc-artifacts": "8.1.3.0-28-g302194ebc-artifacts",
	}
	for in, want := range cases {
		b, err := c.ResolveBuild(in)
		if err != nil {
			t.Errorf("ResolveBuild(%q): %v", in, err)
			continue
		}
		if b.Number != want {
			t.Errorf("ResolveBuild(%q).Number = %q; want %q", in, b.Number, want)
		}
		if b.Name != "aerospike-server" {
			t.Errorf("ResolveBuild(%q).Name = %q", in, b.Name)
		}
	}
	if _, err := c.ResolveBuild("latest"); err == nil {
		t.Errorf("ResolveBuild(latest) should error")
	}
	if _, err := c.ResolveBuild(""); err == nil {
		t.Errorf("ResolveBuild('') should error")
	}
}

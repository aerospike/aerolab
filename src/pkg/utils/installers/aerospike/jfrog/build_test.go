package jfrog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildFiles_ParsesAQLResponse(t *testing.T) {
	body := map[string]any{
		"results": []map[string]any{
			{
				"repo":        "database-rpm-dev-local",
				"path":        "amzn2023/aarch64",
				"name":        "aerospike-server-community-8.1.3.0-28.amzn2023.aarch64.rpm",
				"size":        12345,
				"actual_sha1": "deadbeef",
				"created":     "2026-06-01T00:00:00Z",
			},
			{
				"repo": "database-deb-dev-local",
				"path": "pool/noble/aerospike-server-enterprise",
				"name": "aerospike-server-enterprise_8.1.3.0-28ubuntu24.04_arm64.deb",
				"size": 23456,
			},
			{
				"repo": "database-deb-dev-local",
				"path": "pool/noble/aerospike-server-enterprise",
				"name": "aerospike-server-enterprise_8.1.3.0-28ubuntu24.04_arm64.deb.asc",
				"size": 256,
			},
		},
	}
	var sawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		sawQuery = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	cfg := &Config{BaseURL: srv.URL, BuildName: "aerospike-server"}
	build, err := cfg.ResolveBuild("8.1.3.0-28-g302194ebc")
	if err != nil {
		t.Fatalf("ResolveBuild: %v", err)
	}
	files, err := build.Files(context.Background())
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d files, want 3", len(files))
	}
	if !strings.Contains(sawQuery, `"@build.name":"aerospike-server"`) ||
		!strings.Contains(sawQuery, `"@build.number":"8.1.3.0-28-g302194ebc-artifacts"`) {
		t.Errorf("AQL body did not include expected matchers: %s", sawQuery)
	}
	rpm := files[0]
	wantURL := srv.URL + "/artifactory/database-rpm-dev-local/amzn2023/aarch64/aerospike-server-community-8.1.3.0-28.amzn2023.aarch64.rpm"
	if rpm.DownloadURL != wantURL {
		t.Errorf("download URL = %q; want %q", rpm.DownloadURL, wantURL)
	}
	if rpm.Parts == nil || rpm.Parts.OSName != "amazon" || rpm.Parts.OSVersion != "2023" {
		t.Errorf("rpm parts = %+v", rpm.Parts)
	}

	// .asc should parse to nil parts (matcher ignores it)
	asc := files[2]
	if asc.Parts != nil {
		t.Errorf("asc parts expected nil, got %+v", asc.Parts)
	}
}

func TestFilesMatch(t *testing.T) {
	files := Files{
		{Name: "x.rpm", Parts: &NameParts{Edition: "community", OSName: "amazon", OSVersion: "2023", Arch: "x86_64", Format: "rpm"}},
		{Name: "y.rpm", Parts: &NameParts{Edition: "community", OSName: "amazon", OSVersion: "2023", Arch: "aarch64", Format: "rpm"}},
		{Name: "z.deb", Parts: &NameParts{Edition: "enterprise", OSName: "ubuntu", OSVersion: "24.04", Arch: "aarch64", Format: "deb"}},
		{Name: "junk", Parts: nil},
	}
	got, err := files.Match(MatchCriteria{Edition: "community", OSName: "amazon", OSVersion: "2023", Arch: "aarch64"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.Name != "y.rpm" {
		t.Errorf("got %q want y.rpm", got.Name)
	}
	got, err = files.Match(MatchCriteria{Edition: "enterprise", OSName: "ubuntu", OSVersion: "24.04", Arch: "aarch64"})
	if err != nil {
		t.Fatalf("Match deb: %v", err)
	}
	if got.Name != "z.deb" {
		t.Errorf("got %q want z.deb", got.Name)
	}
	if _, err := files.Match(MatchCriteria{Edition: "federal", OSName: "ubuntu", OSVersion: "24.04", Arch: "aarch64"}); err == nil {
		t.Error("expected error for missing federal edition")
	}
	if _, err := files.Match(MatchCriteria{Edition: "enterprise", OSName: "windows", OSVersion: "11", Arch: "x86_64"}); err == nil {
		t.Error("expected error for unsupported OS")
	}
}

func TestInstallScript_RPM(t *testing.T) {
	f := &File{Name: "aerospike-server-community-8.1.3.0-28.amzn2023.aarch64.rpm", Parts: ParseFileName("aerospike-server-community-8.1.3.0-28.amzn2023.aarch64.rpm")}
	out, err := InstallScript(f, false, false)
	if err != nil {
		t.Fatalf("InstallScript: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "/opt/aerolab/files/aerospike-server-community-8.1.3.0-28.amzn2023.aarch64.rpm") {
		t.Errorf("missing remote path: %s", s)
	}
	if !strings.Contains(s, "rpm -Uvh") && !strings.Contains(s, "yum -y localinstall") && !strings.Contains(s, "dnf install") {
		t.Errorf("no rpm install command found: %s", s)
	}
}

func TestInstallScript_DEB(t *testing.T) {
	f := &File{Name: "aerospike-server-enterprise_8.1.3.0-28ubuntu24.04_arm64.deb", Parts: ParseFileName("aerospike-server-enterprise_8.1.3.0-28ubuntu24.04_arm64.deb")}
	out, err := InstallScript(f, false, true)
	if err != nil {
		t.Fatalf("InstallScript: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "DEBIAN_FRONTEND=noninteractive") || !strings.Contains(s, "apt-get install") {
		t.Errorf("no deb install logic: %s", s)
	}
	if !strings.Contains(s, `UPGRADE="true"`) {
		t.Errorf("upgrade flag not threaded: %s", s)
	}
}

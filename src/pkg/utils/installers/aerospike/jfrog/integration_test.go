package jfrog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEndToEnd_FakeJFrog wires together build resolution, AQL query,
// file matching, package download (to a local cache) and install-script
// generation against an httptest-driven fake JFrog server. It is the
// happy-path smoke test the cluster create / template create flow runs
// at runtime.
func TestEndToEnd_FakeJFrog(t *testing.T) {
	const buildNum = "8.1.3.0-28-g302194ebc-artifacts"
	const fileName = "aerospike-server-enterprise_8.1.3.0-28ubuntu24.04_arm64.deb"
	const filePath = "pool/noble/aerospike-server-enterprise"
	const fileRepo = "database-deb-dev-local"
	pkgBody := []byte("fake-deb-package-bytes")

	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/search/aql"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{
						"repo": fileRepo,
						"path": filePath,
						"name": fileName,
						"size": len(pkgBody),
					},
				},
			})
		case strings.Contains(r.URL.Path, fileName) && r.Method == http.MethodGet:
			_, _ = w.Write(pkgBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := &Config{BaseURL: srv.URL, BuildName: "aerospike-server", Auth: "Bearer testtoken"}

	build, err := cfg.ResolveBuild("8.1.3.0-28-g302194ebc")
	if err != nil {
		t.Fatalf("ResolveBuild: %v", err)
	}
	if build.Number != buildNum {
		t.Fatalf("build number = %q want %q", build.Number, buildNum)
	}

	files, err := build.Files(context.Background())
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}

	match, err := files.Match(MatchCriteria{
		Edition: "enterprise", OSName: "ubuntu", OSVersion: "24.04", Arch: "aarch64",
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}

	cache := t.TempDir()
	local, err := cfg.Download(context.Background(), match, cache)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if filepath.Base(local) != fileName {
		t.Fatalf("local file = %s; want suffix %s", local, fileName)
	}
	got, err := os.ReadFile(local)
	if err != nil || string(got) != string(pkgBody) {
		t.Fatalf("cached file mismatch: %v got=%q want=%q", err, got, pkgBody)
	}

	// Re-download should hit the cache and not error.
	if local2, err := cfg.Download(context.Background(), match, cache); err != nil || local2 != local {
		t.Fatalf("second Download mismatch: %v %s vs %s", err, local2, local)
	}

	script, err := InstallScript(match, false, false)
	if err != nil {
		t.Fatalf("InstallScript: %v", err)
	}
	s := string(script)
	if !strings.Contains(s, "/opt/aerolab/files/"+fileName) {
		t.Fatalf("script missing package path: %s", s)
	}
	if !strings.Contains(s, "DEBIAN_FRONTEND=noninteractive") {
		t.Fatalf("script missing deb install logic")
	}

	if sawAuth != "Bearer testtoken" {
		t.Errorf("auth header not propagated: %q", sawAuth)
	}
}

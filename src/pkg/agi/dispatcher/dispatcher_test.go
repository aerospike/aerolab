package dispatcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverLogDestination(t *testing.T) {
	t.Run("defaultWhenNoLogging", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "aerospike.conf")
		if err := os.WriteFile(p, []byte("namespace test {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		kind, path, err := discoverLogDestination(p)
		if err != nil {
			t.Fatal(err)
		}
		if kind != "file" || path != "/var/log/aerospike/aerospike.log" {
			t.Fatalf("got %q %q", kind, path)
		}
	})
	t.Run("journalFromConsole", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "aerospike.conf")
		body := "logging {\n\tconsole\n}\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		kind, path, err := discoverLogDestination(p)
		if err != nil {
			t.Fatal(err)
		}
		if kind != "journal" || path != "aerospike.service" {
			t.Fatalf("got %q %q", kind, path)
		}
	})
}

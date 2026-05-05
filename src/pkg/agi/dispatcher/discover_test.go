package dispatcher

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDiscoverSourceFromConf_FileOnly covers the most common production
// case: only a `file <path>` destination in the logging stanza. We
// expect a Source{File: <path>} back.
func TestDiscoverSourceFromConf_FileOnly(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "aerospike.conf")
	if err := os.WriteFile(confPath, []byte(`
service {
    user root
}
logging {
    file /var/log/aerospike/aerospike.log {
        context any info
    }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := DiscoverSourceFromConf(confPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !src.IsFile() {
		t.Fatalf("expected file source, got %+v", src)
	}
	if src.File != "/var/log/aerospike/aerospike.log" {
		t.Fatalf("expected file path /var/log/aerospike/aerospike.log, got %q", src.File)
	}
}

// TestDiscoverSourceFromConf_ConsoleOnly covers a console-only conf
// (e.g. a containerized Aerospike). We expect the journald default
// unit back, since systemd captures console output for service
// processes.
func TestDiscoverSourceFromConf_ConsoleOnly(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "aerospike.conf")
	if err := os.WriteFile(confPath, []byte(`
logging {
    console {
        context any info
    }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := DiscoverSourceFromConf(confPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !src.IsJournal() {
		t.Fatalf("expected journal source, got %+v", src)
	}
	if src.Journal != defaultJournalUnit {
		t.Fatalf("expected default journal unit, got %q", src.Journal)
	}
}

// TestDiscoverSourceFromConf_FileWinsOverConsole exercises the rule
// that a `file` destination always wins, even when `console` is also
// listed. Reasoning: file is byte-offset resumable, journald is not.
func TestDiscoverSourceFromConf_FileWinsOverConsole(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "aerospike.conf")
	if err := os.WriteFile(confPath, []byte(`
logging {
    console {
        context any info
    }
    file /var/log/aerospike/aerospike.log {
        context any info
    }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := DiscoverSourceFromConf(confPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !src.IsFile() {
		t.Fatalf("expected file source (file wins over console), got %+v", src)
	}
}

// TestDiscoverSourceFromConf_NoLogging covers a conf with no logging
// stanza at all (or an empty one). The discoverer must surface
// errNoLogging so the caller can fall back to the hardcoded default
// path rather than crash.
func TestDiscoverSourceFromConf_NoLogging(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "aerospike.conf")
	if err := os.WriteFile(confPath, []byte(`
service {
    user root
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverSourceFromConf(confPath); err == nil {
		t.Fatal("expected error when no logging stanza present")
	}
}

// TestResolveSource_ManualOverridesWin verifies the priority order:
// explicit --source-file beats discovery, even when the conf file
// disagrees.
func TestResolveSource_ManualOverridesWin(t *testing.T) {
	src := ResolveSource(Config{
		SourceFile: "/tmp/explicit.log",
	})
	if !src.IsFile() || src.File != "/tmp/explicit.log" {
		t.Fatalf("manual --source-file override not honoured: got %+v", src)
	}

	src = ResolveSource(Config{
		SourceJournal: "myunit.service",
	})
	if !src.IsJournal() || src.Journal != "myunit.service" {
		t.Fatalf("manual --source-journal override not honoured: got %+v", src)
	}
}

// TestResolveSource_FallbackToDefault verifies that with no conf and
// no overrides, the resolver returns the documented default file path
// instead of erroring.
func TestResolveSource_FallbackToDefault(t *testing.T) {
	src := ResolveSource(Config{
		AerospikeConf: "/this/path/does/not/exist",
	})
	if !src.IsFile() || src.File != defaultLogPath {
		t.Fatalf("expected fallback to %q, got %+v", defaultLogPath, src)
	}
}

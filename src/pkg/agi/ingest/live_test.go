package ingest

import (
	"strings"
	"testing"
)

func TestLiveConfigDefaultsAndYAML(t *testing.T) {
	cfg, err := MakeConfigReader(true, strings.NewReader("live:\n  enabled: true\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Live.Enabled {
		t.Fatal("expected live ingest enabled from yaml")
	}
	if cfg.Live.ListenAddr != "127.0.0.1:18080" {
		t.Fatalf("unexpected listen addr %q", cfg.Live.ListenAddr)
	}
	if cfg.Live.Workers != 16 {
		t.Fatalf("unexpected workers %d", cfg.Live.Workers)
	}
	if cfg.Live.MaxStreams != 256 {
		t.Fatalf("unexpected max streams %d", cfg.Live.MaxStreams)
	}
}

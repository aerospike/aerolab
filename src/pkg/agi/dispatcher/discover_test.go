package dispatcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSourceFromAerospikeConf(t *testing.T) {
	dir := t.TempDir()
	fileConf := filepath.Join(dir, "file.conf")
	if err := os.WriteFile(fileConf, []byte(`logging {
	file /var/log/aerospike/aerospike.log {
		context any info
	}
}`), 0644); err != nil {
		t.Fatal(err)
	}
	src, err := New(Config{AerospikeConf: fileConf}).resolveSource()
	if err != nil {
		t.Fatalf("resolve file source: %s", err)
	}
	if src.Kind != SourceKindFile || src.Path != "/var/log/aerospike/aerospike.log" {
		t.Fatalf("unexpected file source: %#v", src)
	}

	consoleConf := filepath.Join(dir, "console.conf")
	if err := os.WriteFile(consoleConf, []byte(`logging {
	console {
		context any info
	}
}`), 0644); err != nil {
		t.Fatal(err)
	}
	src, err = New(Config{AerospikeConf: consoleConf}).resolveSource()
	if err != nil {
		t.Fatalf("resolve console source: %s", err)
	}
	if src.Kind != SourceKindJournal || src.Unit != "aerospike.service" {
		t.Fatalf("unexpected journal source: %#v", src)
	}
}

func TestResolveSourceOverride(t *testing.T) {
	src, err := New(Config{SourceFile: "/tmp/aerospike.log"}).resolveSource()
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind != SourceKindFile || src.Path != "/tmp/aerospike.log" {
		t.Fatalf("unexpected source: %#v", src)
	}
}

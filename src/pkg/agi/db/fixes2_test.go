package db

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// stringSink is a concurrent-safe in-memory logger sink used by tests
// that need to inspect log lines produced during an operation.
type stringSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *stringSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}
func (s *stringSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
func (s *stringSink) logger() *log.Logger {
	return log.New(s, "", 0)
}

// TestBetweenUnknownColumnOnRegisteredSet verifies P3: Query.Between
// on a set that has been RegisterSet'd but where the requested column
// is not in the schema returns an empty iterator with no error. This
// superseded the earlier "returns an error" behavior: the plugin and
// ingest race each other during startup (plugin reads cache,
// ingest's first Put lazily creates the column), and a column-missing
// error bubbles out to Grafana as a visible failure rather than the
// intended "no data yet" empty series. The unknown-but-typo'd case
// is covered by emptiness too and is symmetric with the full-scan
// path (no index entries ⇒ no rows).
func TestBetweenUnknownColumnOnRegisteredSet(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("m", []ColumnSpec{{Name: "ts", Type: TypeInt64, Indexed: true}}); err != nil {
		t.Fatal(err)
	}
	it := d.Query("m").Between("nope", Int(0), Int(100)).Run(context.Background())
	defer it.Close()
	if it.Next() {
		t.Fatalf("Next should not return true on unknown-column Between")
	}
	if err := it.Err(); err != nil {
		t.Fatalf("Err should be nil on unknown-column Between, got %v", err)
	}
}

// TestSetExists verifies D6: SetExists distinguishes "registered" from
// "unknown / dropped" without probing the data plane.
func TestSetExists(t *testing.T) {
	d := openTestDB(t)
	if d.SetExists("missing") {
		t.Fatal("SetExists reported unknown set as present")
	}
	if err := d.Put("live", "k", Row{"v": Int(1)}); err != nil {
		t.Fatal(err)
	}
	if !d.SetExists("live") {
		t.Fatal("SetExists did not recognise newly-created set")
	}
	if err := d.DropSet("live"); err != nil {
		t.Fatal(err)
	}
	if d.SetExists("live") {
		t.Fatal("SetExists reported dropped set as present")
	}
}

// TestBoolDecodeRejectsBadPayload verifies D7: a bool payload byte that
// is neither 0 nor 1 is rejected at decode time instead of coercing to
// true. We round-trip through the public encodePayload so only decode is
// under test.
func TestBoolDecodeRejectsBadPayload(t *testing.T) {
	if _, err := decodePayload(TypeBool, []byte{0x02}); err == nil {
		t.Fatal("bool decode of 0x02 should error")
	}
	if _, err := decodePayload(TypeBool, []byte{0xff}); err == nil {
		t.Fatal("bool decode of 0xff should error")
	}
	v, err := decodePayload(TypeBool, []byte{0x00})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := v.AsBool(); b {
		t.Fatal("0x00 should decode to false")
	}
	v, err = decodePayload(TypeBool, []byte{0x01})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := v.AsBool(); !b {
		t.Fatal("0x01 should decode to true")
	}
}

// TestDropColumn verifies D2: DropColumn retires a column from the
// schema and hides its prior values from Get / Scan / Query without
// requiring a full row rewrite.
func TestDropColumn(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("m", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "tag", Type: TypeString},
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := d.Put("m", fmt.Sprintf("k%d", i), Row{
			"ts":  Int(int64(i)),
			"tag": Str("x"),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.DropColumn("m", "tag"); err != nil {
		t.Fatal(err)
	}
	row, err := d.Get("m", "k0")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := row["tag"]; ok {
		t.Fatalf("tag still visible after DropColumn: %v", row)
	}
	// DropColumn on the indexed column retires the index too.
	if err := d.DropColumn("m", "ts"); err != nil {
		t.Fatal(err)
	}
	// Range query using the old indexed column name should now
	// return an empty iterator (column is gone from the schema,
	// which from the query path's point of view is
	// indistinguishable from "column was never registered"; see
	// P3 and TestBetweenUnknownColumnOnRegisteredSet).
	it := d.Query("m").Between("ts", Int(0), Int(1000)).Run(context.Background())
	defer it.Close()
	if it.Next() {
		t.Fatal("Between on dropped indexed column should yield no rows")
	}
	if err := it.Err(); err != nil {
		t.Fatalf("Between on dropped indexed column should not error, got %v", err)
	}
	// DropColumn on unknown col / set is a no-op.
	if err := d.DropColumn("m", "gone"); err != nil {
		t.Fatalf("DropColumn on missing col should be noop, got %v", err)
	}
	if err := d.DropColumn("never", "x"); err != nil {
		t.Fatalf("DropColumn on missing set should be noop, got %v", err)
	}
}

// TestReadIntoReusesRow verifies X6: ReadInto reuses the caller's Row,
// and that stale keys from a previous iteration are cleared.
func TestReadIntoReusesRow(t *testing.T) {
	d := openTestDB(t)
	if err := d.Put("m", "k1", Row{"a": Int(1), "b": Int(2)}); err != nil {
		t.Fatal(err)
	}
	if err := d.Put("m", "k2", Row{"a": Int(10)}); err != nil {
		t.Fatal(err)
	}
	it := d.Scan("m")
	defer it.Close()
	row := make(Row, 4)
	seen := map[string]Row{}
	for it.Next() {
		k, r := it.ReadInto(row)
		if r == nil {
			t.Fatal("ReadInto returned nil Row")
		}
		cp := make(Row, len(r))
		for kk, vv := range r {
			cp[kk] = vv
		}
		seen[k] = cp
	}
	if it.Err() != nil {
		t.Fatal(it.Err())
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 rows, got %d (%v)", len(seen), seen)
	}
	// k2 has no "b" - the reused map must have cleared it.
	if _, ok := seen["k2"]["b"]; ok {
		t.Fatalf("stale b leaked into k2: %v", seen["k2"])
	}
}

// TestNoBlockCache verifies X5: CacheBytes == NoBlockCache disables the
// block cache. We only check that Open succeeds and the DB functions —
// there is no public surface for cache state, but the Pebble-side
// metrics exposed by Stats let us spot-check that the cache is empty.
func TestNoBlockCache(t *testing.T) {
	opts := DefaultOptions()
	opts.Path = filepath.Join(t.TempDir(), "db")
	opts.CacheBytes = NoBlockCache
	d, err := Open(opts)
	if err != nil {
		t.Fatalf("Open with NoBlockCache: %v", err)
	}
	defer d.Close()
	if err := d.Put("m", "k", Row{"v": Int(1)}); err != nil {
		t.Fatal(err)
	}
	s := d.Stats()
	// With no block cache allocated, the reported size should be zero.
	if s.BlockCacheSize != 0 {
		t.Fatalf("expected BlockCacheSize=0 with NoBlockCache, got %d", s.BlockCacheSize)
	}
}

// TestStatsExposesPebbleMetrics verifies D4: Stats() now surfaces
// basic Pebble subsystem metrics (cache, memtable, flushes, ...).
func TestStatsExposesPebbleMetrics(t *testing.T) {
	d := openTestDB(t)
	for i := 0; i < 100; i++ {
		if err := d.Put("m", fmt.Sprintf("k%d", i), Row{"v": Int(int64(i))}); err != nil {
			t.Fatal(err)
		}
	}
	s := d.Stats()
	if s.Puts == 0 {
		t.Fatal("Puts counter stayed at 0")
	}
	// MemTable size should be non-zero after 100 writes (writes sit in
	// the memtable until flushed).
	if s.MemTableSize == 0 {
		t.Fatalf("MemTableSize is zero after writes: %+v", s)
	}
	if s.DiskUsageBytes == 0 {
		t.Fatalf("DiskUsageBytes is zero after writes: %+v", s)
	}
	// The block cache is populated lazily on read (it caches sstable
	// blocks, not memtable bytes). We don't assert BlockCacheSize > 0
	// here because a pure write workload can leave it empty; we just
	// assert the field is wired up — i.e. it is read without panicking
	// and its type is int64. Any non-negative value is acceptable.
	if s.BlockCacheSize < 0 {
		t.Fatalf("BlockCacheSize negative: %d", s.BlockCacheSize)
	}
}

// TestIteratorLeakCounter verifies D10: iterators that are dropped
// without Close are flagged by the finalizer and surface through the
// LeakedIterators stat. We force two GC cycles to give the finalizer
// goroutine a chance to run.
func TestIteratorLeakCounter(t *testing.T) {
	d := openTestDB(t)
	if err := d.Put("m", "k", Row{"v": Int(1)}); err != nil {
		t.Fatal(err)
	}
	before := d.Stats().LeakedIterators
	// Open an iterator and drop the reference without Close.
	func() {
		it := d.Scan("m")
		// Advance once so the iterator allocates its Pebble iterator.
		it.Next()
		// Intentionally do not Close. Drop reference on return.
		_ = it
	}()
	// Drive GC / finalizers.
	for i := 0; i < 5; i++ {
		runtime.GC()
		runtime.Gosched()
		time.Sleep(20 * time.Millisecond)
		if d.Stats().LeakedIterators > before {
			return
		}
	}
	t.Fatalf("LeakedIterators did not increment after dropped iterator (before=%d, after=%d)", before, d.Stats().LeakedIterators)
}

// TestCloseReportsProgressLogs is a smoke test for X2: Close() should
// emit log lines announcing the flush/close transition. We capture the
// logger's output through a custom Options.Logger and assert the
// expected strings appear.
func TestCloseReportsProgressLogs(t *testing.T) {
	opts := DefaultOptions()
	opts.Path = filepath.Join(t.TempDir(), "db")
	var buf stringSink
	opts.Logger = buf.logger()
	d, err := Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "pebble close starting") {
		t.Fatalf("expected 'pebble close starting' in log, got: %s", got)
	}
	if !strings.Contains(got, "pebble close complete") {
		t.Fatalf("expected 'pebble close complete' in log, got: %s", got)
	}
}

// TestBetweenUnknownReturnsEmpty verifies that Between on a column the
// set's schema does not know yet returns an empty iterator (Err()==nil,
// Next()==false) rather than an error. The sparse-column store creates
// columns lazily on first Put, so "column does not exist in this set"
// is indistinguishable from "no row has written this column yet" and
// callers (notably the plugin's cache-and-query loop that races with
// ingest) must not blow up.
func TestBetweenUnknownReturnsEmpty(t *testing.T) {
	d := openTestDB(t)
	_ = d.Put("m", "k", Row{"v": Int(1)})
	it := d.Query("m").
		Between("missing", Int(0), Int(1)).
		Where(Eq("v", Int(1))).
		Run(context.Background())
	defer it.Close()
	if it.Next() {
		t.Fatal("Next returned true with unknown Between column")
	}
	if err := it.Err(); err != nil {
		t.Fatalf("Err should be nil for missing column, got %v", err)
	}
}

// TestLoadSchemasRepairsMissingIndexedCol: if the persisted schema
// record names an IndexedCol that is not in its column list (corruption
// or partial restore), Open must log a WARN, clear IndexedCol, and
// surface the set with no indexed column instead of silently
// advertising a broken one.
func TestLoadSchemasRepairsMissingIndexedCol(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "db")
	// Pre-seed with a deliberately malformed schema: IndexedCol="ts"
	// but no "ts" column in Cols.
	opts := DefaultOptions()
	opts.Path = tmp
	d, err := Open(opts)
	if err != nil {
		t.Fatalf("Open (seed): %v", err)
	}
	// Write a bogus persisted record directly, bypassing RegisterSet.
	bogus := []byte(`{"id":5,"name":"broken","nextColID":1,"cols":[{"id":0,"name":"v","type":1}],"indexedCol":"ts"}`)
	if err := d.p.Set(metaSchemaKey(5), bogus, d.wopts); err != nil {
		_ = d.Close()
		t.Fatalf("seed Set: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("seed close: %v", err)
	}
	// Reopen with a log sink and assert the repair happens and is
	// logged.
	var sink stringSink
	opts.Logger = sink.logger()
	d2, err := Open(opts)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	logged := sink.String()
	if !strings.Contains(logged, `IndexedCol="ts"`) || !strings.Contains(logged, "clearing") {
		t.Fatalf("expected repair warning in log, got: %s", logged)
	}
	spec, ok := d2.SchemaOf("broken")
	if !ok {
		t.Fatal("repaired set not visible")
	}
	for _, c := range spec {
		if c.Indexed {
			t.Errorf("column %q still marked indexed after repair", c.Name)
		}
	}
}

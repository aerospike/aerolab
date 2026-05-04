package db

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/pebble/v2"
)

// openTestDB opens a fresh DB in a per-test tmp dir. Closes on cleanup.
func openTestDB(t testing.TB) *DB {
	t.Helper()
	opts := DefaultOptions()
	opts.Path = filepath.Join(t.TempDir(), "db")
	opts.CacheBytes = 16 << 20 // smaller cache for tests
	d, err := Open(opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		_ = d.Close()
	})
	return d
}

func TestOpenCloseReopen(t *testing.T) {
	tmp := t.TempDir()
	opts := DefaultOptions()
	opts.Path = filepath.Join(tmp, "db")
	d, err := Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterSet("metrics", []ColumnSpec{
		{Name: "timestamp", Type: TypeInt64, Indexed: true},
		{Name: "value", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.Put("metrics", "row1", Row{"timestamp": Int(1000), "value": Int(7)}); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen and verify data + schema persist.
	d2, err := Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	row, err := d2.Get("metrics", "row1")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := row["value"].AsInt(); v != 7 {
		t.Errorf("persisted value mismatch: got %v", v)
	}
	sets := d2.Sets()
	if len(sets) != 1 || sets[0] != "metrics" {
		t.Errorf("sets persisted mismatch: %v", sets)
	}
}

func TestRegisterSetTypeConflict(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("s", []ColumnSpec{{Name: "x", Type: TypeInt64}}); err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterSet("s", []ColumnSpec{{Name: "x", Type: TypeString}}); err == nil {
		t.Error("expected type conflict error")
	}
}

// TestRegisterSetPromoteNonIndexedRejected asserts that RegisterSet
// refuses to promote an already-registered, non-indexed column to
// indexed. The package does not back-fill index entries for rows that
// pre-date the promotion, so the only consistent behaviour is to
// reject with a clear error — not the legacy "already indexed on \"\""
// message which implied the set was already indexed on nothing.
func TestRegisterSetPromoteNonIndexedRejected(t *testing.T) {
	d := openTestDB(t)
	// Implicit column registration: "ts" is now non-indexed Int64.
	if err := d.Put("m", "k1", Row{"ts": Int(1)}); err != nil {
		t.Fatal(err)
	}
	err := d.RegisterSet("m", []ColumnSpec{{Name: "ts", Type: TypeInt64, Indexed: true}})
	if err == nil {
		t.Fatal("expected promotion-to-indexed error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot promote to indexed") {
		t.Fatalf("error should mention promotion rejection, got: %v", err)
	}
	// Schema must be unchanged: "ts" is still non-indexed.
	specs, ok := d.SchemaOf("m")
	if !ok {
		t.Fatal("SchemaOf: set missing")
	}
	for _, c := range specs {
		if c.Name == "ts" && c.Indexed {
			t.Fatalf("promotion leaked into schema: %+v", c)
		}
	}
}

func TestPutGetBasic(t *testing.T) {
	d := openTestDB(t)
	if err := d.Put("users", "u1", Row{"name": Str("alice"), "age": Int(30)}); err != nil {
		t.Fatal(err)
	}
	row, err := d.Get("users", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := row["name"].AsString(); v != "alice" {
		t.Errorf("name: %q", v)
	}
	if v, _ := row["age"].AsInt(); v != 30 {
		t.Errorf("age: %d", v)
	}
	// missing row
	row, err = d.Get("users", "ghost")
	if err != nil {
		t.Fatal(err)
	}
	if row != nil {
		t.Errorf("expected nil row, got %v", row)
	}
}

func TestPutOverwriteUpdatesIndex(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("metrics", []ColumnSpec{{Name: "timestamp", Type: TypeInt64, Indexed: true}}); err != nil {
		t.Fatal(err)
	}
	if err := d.Put("metrics", "k", Row{"timestamp": Int(100)}); err != nil {
		t.Fatal(err)
	}
	if err := d.Put("metrics", "k", Row{"timestamp": Int(200)}); err != nil {
		t.Fatal(err)
	}
	// Query for [50,150] should yield nothing now (old index entry removed).
	iter := d.Query("metrics").Between("timestamp", Int(50), Int(150)).Run(context.Background())
	defer iter.Close()
	if iter.Next() {
		t.Errorf("stale index entry returned: key=%s row=%v", func() string { k, _ := iter.Record(); return k }(), func() Row { _, r := iter.Record(); return r }())
	}
	if err := iter.Err(); err != nil {
		t.Fatal(err)
	}
	// [150,250] should find the row.
	iter2 := d.Query("metrics").Between("timestamp", Int(150), Int(250)).Run(context.Background())
	defer iter2.Close()
	count := 0
	for iter2.Next() {
		count++
	}
	if count != 1 {
		t.Errorf("expected 1 row after overwrite, got %d", count)
	}
}

func TestDelete(t *testing.T) {
	d := openTestDB(t)
	_ = d.Put("t", "k", Row{"v": Int(1)})
	ok, err := d.Delete("t", "k")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("delete should report true")
	}
	row, _ := d.Get("t", "k")
	if row != nil {
		t.Errorf("row survived delete: %v", row)
	}
	ok, _ = d.Delete("t", "k")
	if ok {
		t.Error("second delete should report false")
	}
}

func TestUpdateReadModifyWrite(t *testing.T) {
	d := openTestDB(t)
	err := d.Update("ctr", "counter", func(old Row) (Row, bool) {
		if old == nil {
			return Row{"n": Int(1)}, true
		}
		v, _ := old["n"].AsInt()
		return Row{"n": Int(v + 1)}, true
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := d.Update("ctr", "counter", func(old Row) (Row, bool) {
			v, _ := old["n"].AsInt()
			return Row{"n": Int(v + 1)}, true
		}); err != nil {
			t.Fatal(err)
		}
	}
	row, _ := d.Get("ctr", "counter")
	if v, _ := row["n"].AsInt(); v != 5 {
		t.Errorf("expected 5 after 5 updates, got %d", v)
	}
}

func TestUpdateDeletePath(t *testing.T) {
	d := openTestDB(t)
	_ = d.Put("t", "k", Row{"v": Int(1)})
	err := d.Update("t", "k", func(old Row) (Row, bool) { return nil, true })
	if err != nil {
		t.Fatal(err)
	}
	row, _ := d.Get("t", "k")
	if row != nil {
		t.Error("row should be deleted by Update")
	}
}

func TestDropSet(t *testing.T) {
	d := openTestDB(t)
	_ = d.RegisterSet("m", []ColumnSpec{{Name: "ts", Type: TypeInt64, Indexed: true}})
	for i := 0; i < 20; i++ {
		_ = d.Put("m", fmt.Sprintf("k%d", i), Row{"ts": Int(int64(i)), "v": Int(int64(i * 2))})
	}
	if err := d.DropSet("m"); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.SchemaOf("m"); ok {
		t.Error("schema still present")
	}
	// Scan after drop should return empty.
	it := d.Scan("m")
	defer it.Close()
	for it.Next() {
		k, r := it.Record()
		t.Errorf("unexpected row post drop: %s %v", k, r)
	}
	// Re-put after drop creates fresh set.
	if err := d.Put("m", "a", Row{"ts": Int(1)}); err != nil {
		t.Fatal(err)
	}
	row, _ := d.Get("m", "a")
	if v, _ := row["ts"].AsInt(); v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
}

func TestScanYieldsAllRowsUnordered(t *testing.T) {
	d := openTestDB(t)
	want := map[string]int64{"a": 1, "b": 2, "c": 3}
	for k, v := range want {
		_ = d.Put("s", k, Row{"v": Int(v)})
	}
	got := map[string]int64{}
	it := d.Scan("s")
	defer it.Close()
	for it.Next() {
		k, r := it.Record()
		v, _ := r["v"].AsInt()
		got[k] = v
	}
	if it.Err() != nil {
		t.Fatal(it.Err())
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("key %q: got %d want %d", k, got[k], v)
		}
	}
}

func TestGetProjectionSkipsNonRequested(t *testing.T) {
	d := openTestDB(t)
	big := bytes.Repeat([]byte{'b'}, 1<<20)
	_ = d.Put("t", "k", Row{"a": Int(1), "b": Str("keep"), "blob": BytesV(big)})
	row, err := d.Get("t", "k", "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := row["blob"]; ok {
		t.Error("blob should not have been decoded under projection")
	}
	if v, _ := row["a"].AsInt(); v != 1 {
		t.Errorf("a: %d", v)
	}
	if v, _ := row["b"].AsString(); v != "keep" {
		t.Errorf("b: %q", v)
	}
}

func TestIndexedTypeRejected(t *testing.T) {
	d := openTestDB(t)
	err := d.RegisterSet("x", []ColumnSpec{{Name: "s", Type: TypeString, Indexed: true}})
	if err == nil {
		t.Error("indexing a non-numeric column should error")
	}
}

func TestPutTypeMismatchRejected(t *testing.T) {
	d := openTestDB(t)
	_ = d.Put("s", "k", Row{"n": Int(1)})
	err := d.Put("s", "k2", Row{"n": Str("oops")})
	if err == nil {
		t.Error("type mismatch should be rejected")
	}
}

func TestEmptyRowPutRejected(t *testing.T) {
	d := openTestDB(t)
	if err := d.Put("s", "k", Row{}); err == nil {
		t.Error("empty Row must be rejected")
	}
}

func TestStorageVersionWrittenOnFreshOpen(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "db")
	opts := DefaultOptions()
	opts.Path = tmp
	d, err := Open(opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	v, closer, err := d.p.Get(metaVersionKey())
	if err != nil {
		_ = d.Close()
		t.Fatalf("version key missing after fresh Open: %v", err)
	}
	if len(v) != 4 || binary.BigEndian.Uint32(v) != currentStorageVersion {
		_ = closer.Close()
		_ = d.Close()
		t.Fatalf("version record mismatch: %x", v)
	}
	_ = closer.Close()
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStorageVersionLazilyWrittenOnLegacyOpen(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "db")
	// Pre-seed a Pebble directory with a schema record but no version
	// key: simulates a legacy (pre-versioning) deployment. Match the
	// DB's WAL-disabled configuration so the subsequent DB Open
	// observes a consistent on-disk layout.
	p, err := pebble.Open(tmp, &pebble.Options{DisableWAL: true})
	if err != nil {
		t.Fatalf("pebble.Open: %v", err)
	}
	schema := []byte(`{"id":0,"name":"legacy","nextColID":0,"cols":[]}`)
	if err := p.Set(metaSchemaKey(0), schema, pebble.NoSync); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	if err := p.Flush(); err != nil {
		t.Fatalf("seed flush: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("seed close: %v", err)
	}
	opts := DefaultOptions()
	opts.Path = tmp
	d, err := Open(opts)
	if err != nil {
		t.Fatalf("legacy Open: %v", err)
	}
	v, closer, err := d.p.Get(metaVersionKey())
	if err != nil {
		_ = d.Close()
		t.Fatalf("version key not written on legacy Open: %v", err)
	}
	if binary.BigEndian.Uint32(v) != currentStorageVersion {
		_ = closer.Close()
		_ = d.Close()
		t.Fatalf("legacy version mismatch: %x", v)
	}
	_ = closer.Close()
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacySetNameKeysPurgedOnOpen(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "db")
	p, err := pebble.Open(tmp, &pebble.Options{DisableWAL: true})
	if err != nil {
		t.Fatalf("pebble.Open: %v", err)
	}
	legacyKey := append([]byte{prefixMeta}, []byte(metaSetNamePref+"oldset")...)
	if err := p.Set(legacyKey, []byte{0, 0, 0, 7}, pebble.NoSync); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	if err := p.Flush(); err != nil {
		t.Fatalf("seed flush: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("seed close: %v", err)
	}
	opts := DefaultOptions()
	opts.Path = tmp
	d, err := Open(opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, _, err := d.p.Get(legacyKey); !errors.Is(err, pebble.ErrNotFound) {
		_ = d.Close()
		t.Fatalf("expected legacy key purged, got err=%v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStorageVersionMismatchRefuses(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "db")
	p, err := pebble.Open(tmp, &pebble.Options{DisableWAL: true})
	if err != nil {
		t.Fatalf("pebble.Open: %v", err)
	}
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, currentStorageVersion+999)
	if err := p.Set(metaVersionKey(), buf, pebble.NoSync); err != nil {
		t.Fatalf("seed version: %v", err)
	}
	if err := p.Flush(); err != nil {
		t.Fatalf("seed flush: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("seed close: %v", err)
	}
	opts := DefaultOptions()
	opts.Path = tmp
	d, err := Open(opts)
	if d != nil {
		_ = d.Close()
	}
	if !errors.Is(err, ErrStorageVersionMismatch) {
		t.Fatalf("expected ErrStorageVersionMismatch, got %v", err)
	}
}

// TestPutBatchAtomicity asserts that a PutBatch either commits every
// row or none of them. We force a mid-batch failure by mixing a row
// that conflicts with an existing column type and verify that no row
// from the failing batch is observable post-commit.
func TestPutBatchAtomicity(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("a", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "n", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}
	// Successful batch first: establishes the schema and the
	// "good" rows we expect to remain visible.
	if err := d.PutBatch("a", []PutItem{
		{Key: "k1", Row: Row{"ts": Int(10), "n": Int(1)}},
		{Key: "k2", Row: Row{"ts": Int(20), "n": Int(2)}},
	}); err != nil {
		t.Fatal(err)
	}
	// Failing batch: second item has wrong type for "n" — the
	// whole batch must reject and leave k3/k4 absent.
	err := d.PutBatch("a", []PutItem{
		{Key: "k3", Row: Row{"ts": Int(30), "n": Int(3)}},
		{Key: "k4", Row: Row{"ts": Int(40), "n": Str("oops")}},
	})
	if err == nil {
		t.Fatal("expected type-conflict error from PutBatch")
	}
	if !errors.Is(err, ErrColumnTypeConflict) {
		t.Fatalf("expected ErrColumnTypeConflict, got %v", err)
	}
	for _, k := range []string{"k1", "k2"} {
		row, err := d.Get("a", k)
		if err != nil {
			t.Fatalf("Get %s: %v", k, err)
		}
		if row == nil {
			t.Errorf("pre-batch row %s missing after failed PutBatch", k)
		}
	}
	for _, k := range []string{"k3", "k4"} {
		row, err := d.Get("a", k)
		if err != nil {
			t.Fatalf("Get %s: %v", k, err)
		}
		if row != nil {
			t.Errorf("failing batch left row %s visible: %v", k, row)
		}
	}
}

// TestPutBatchAssumeNewSkipsPreGet verifies the AssumeNew=true fast
// path does not perform a per-row Pebble Get: with N rows in a single
// batch, the indexed write path must add at most a constant number of
// schema-related Gets (zero on the steady-state path). Compared to
// AssumeNew=false, which always pre-Gets the D/ pointer.
func TestPutBatchAssumeNewSkipsPreGet(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("ts", []ColumnSpec{
		{Name: "timestamp", Type: TypeInt64, Indexed: true},
		{Name: "v", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}

	const rows = 100
	mkBatch := func(prefix string, assumeNew bool) []PutItem {
		out := make([]PutItem, rows)
		for i := 0; i < rows; i++ {
			out[i] = PutItem{
				Key:       fmt.Sprintf("%s-%04d", prefix, i),
				Row:       Row{"timestamp": Int(int64(1_000_000 + i)), "v": Int(int64(i))},
				AssumeNew: assumeNew,
			}
		}
		return out
	}
	// Baseline: AssumeNew=false should issue exactly N pre-Gets
	// (one per row's old D/ pointer).
	before := d.Stats().PebbleGets
	if err := d.PutBatch("ts", mkBatch("safe", false)); err != nil {
		t.Fatal(err)
	}
	safeGets := d.Stats().PebbleGets - before
	if safeGets != rows {
		t.Errorf("AssumeNew=false: want %d pre-Gets, got %d", rows, safeGets)
	}

	// AssumeNew=true: zero pre-Gets.
	before = d.Stats().PebbleGets
	if err := d.PutBatch("ts", mkBatch("fast", true)); err != nil {
		t.Fatal(err)
	}
	fastGets := d.Stats().PebbleGets - before
	if fastGets != 0 {
		t.Errorf("AssumeNew=true: want 0 pre-Gets, got %d", fastGets)
	}
}

// TestPutBatchOrphanGuardSkipsStaleIndex verifies the explicit
// IndexCanHaveOrphans opt-in behaves correctly: with the flag set,
// an AssumeNew=true overwrite on a different indexed value leaves an
// orphan I/ entry behind, and the indexScanIter must skip it. With
// the flag *unset* (the default, exercised by AGI ingest), the
// orphan would leak — that is the contract callers must respect by
// not overwriting different indexed values via AssumeNew.
func TestPutBatchOrphanGuardSkipsStaleIndex(t *testing.T) {
	tmp := t.TempDir()
	opts := DefaultOptions()
	opts.Path = tmp + "/db"
	opts.CacheBytes = 16 << 20
	opts.IndexCanHaveOrphans = true
	d, err := Open(opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.RegisterSet("ts", []ColumnSpec{
		{Name: "timestamp", Type: TypeInt64, Indexed: true},
		{Name: "v", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.Put("ts", "k1", Row{"timestamp": Int(100), "v": Int(1)}); err != nil {
		t.Fatal(err)
	}
	if !d.assumeNewSeen.Load() {
		t.Fatal("IndexCanHaveOrphans=true should pin the orphan-guard on at Open")
	}
	// Overwrite k1 with AssumeNew=true at a NEW indexed value;
	// this leaves the I/ entry at ts=100 as an orphan that the
	// guard must skip on read.
	if err := d.PutBatch("ts", []PutItem{
		{Key: "k1", Row: Row{"timestamp": Int(200), "v": Int(2)}, AssumeNew: true},
	}); err != nil {
		t.Fatal(err)
	}
	// Range scan over [50, 150] must NOT see the orphaned entry
	// at ts=100 (the live D/ pointer now references ts=200).
	ctx := context.Background()
	it := d.Query("ts").Between("timestamp", Int(50), Int(150)).Run(ctx)
	defer it.Close()
	seen := 0
	for it.Next() {
		_, r := it.Record()
		ts, _ := r["timestamp"].AsInt()
		t.Errorf("orphan row leaked into scan at ts=%d: %v", ts, r)
		seen++
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 0 {
		t.Errorf("orphan scan returned %d rows; want 0", seen)
	}
	// The new entry is in [150, 250] — confirm it remains
	// reachable.
	it2 := d.Query("ts").Between("timestamp", Int(150), Int(250)).Run(ctx)
	defer it2.Close()
	seen2 := 0
	for it2.Next() {
		seen2++
	}
	if err := it2.Err(); err != nil {
		t.Fatal(err)
	}
	if seen2 != 1 {
		t.Errorf("live row scan returned %d rows; want 1", seen2)
	}
}

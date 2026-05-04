package ingest

import (
	"path/filepath"
	"testing"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// TestPutDataTwoWayCoercion verifies B2 + B3: putData transparently
// coerces mismatched column types in both directions (int↔string) so
// the same pattern can emit a bin as int for one row and string for
// another without breaking ingest. The coercion relies on the
// db.ErrColumnTypeConflict sentinel (B1) to classify the db's
// response; a stricter error-text match would not compose with
// errors.Wrap chains upstream.
//
// The first-Put race that B3 documents: when RegisterSet has
// declared the indexed timestamp column but NOT the data columns,
// the first Put for a given (col, value) pins the column's type.
// putData's job is to honour the pin for every subsequent Put
// regardless of which type the pattern happens to emit.
func TestPutDataTwoWayCoercion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ingest-db")
	opts := db.DefaultOptions()
	opts.Path = dir
	d, err := db.Open(opts)
	if err != nil {
		t.Fatalf("db.Open: %s", err)
	}
	defer d.Close()

	// Register the set the way ingest's registerSets does: indexed
	// timestamp only, all other columns created lazily on first Put.
	if err := d.RegisterSet("metrics", []db.ColumnSpec{
		{Name: "timestamp", Type: db.TypeInt64, Indexed: true},
	}); err != nil {
		t.Fatalf("RegisterSet: %s", err)
	}

	// Minimal Ingest shell: putData only uses i.db and
	// i.config.TimestampColumnName. Constructing through
	// Init/InitWithDB would also bring patterns + progress + bin list
	// machinery which is out of scope for this unit.
	cfg := new(Config)
	cfg.TimestampColumnName = "timestamp"
	i := &Ingest{
		config: cfg,
		db:     d,
	}

	// First Put pins column types: op_count -> int, node -> string.
	if err := i.putData("metrics", "k1", db.Row{
		"timestamp": db.Int(1),
		"op_count":  db.Int(10),
		"node":      db.Str("node-a"),
	}); err != nil {
		t.Fatalf("first putData: %s", err)
	}

	// Second row sends them with swapped types. putData must coerce
	// op_count's string->int and node's int->string; without
	// coercion db.Put would return ErrColumnTypeConflict and the
	// row would be rejected outright.
	if err := i.putData("metrics", "k2", db.Row{
		"timestamp": db.Int(2),
		"op_count":  db.Str("20"),
		"node":      db.Int(42),
	}); err != nil {
		t.Fatalf("second putData (mixed types): %s", err)
	}

	// Third row: a string that cannot be parsed to int must NOT
	// fail the whole row; coercion drops just that column and the
	// remaining row is persisted. This is B2's "don't fail the
	// whole row" guarantee.
	if err := i.putData("metrics", "k3", db.Row{
		"timestamp": db.Int(3),
		"op_count":  db.Str("not-a-number"),
		"node":      db.Int(7),
	}); err != nil {
		t.Fatalf("third putData (unparseable int): %s", err)
	}

	// k2: op_count must have landed as Int(20), node as Str("42").
	r2, err := d.Get("metrics", "k2")
	if err != nil || r2 == nil {
		t.Fatalf("Get k2: row=%v err=%v", r2, err)
	}
	if v, ok := r2["op_count"].AsInt(); !ok || v != 20 {
		t.Fatalf("k2.op_count: want Int(20), got %v", r2["op_count"])
	}
	if v, ok := r2["node"].AsString(); !ok || v != "42" {
		t.Fatalf("k2.node: want Str(42), got %v", r2["node"])
	}

	// k3: op_count should be absent (dropped by coercion); node
	// should still be coerced to "7".
	r3, err := d.Get("metrics", "k3")
	if err != nil || r3 == nil {
		t.Fatalf("Get k3: row=%v err=%v", r3, err)
	}
	if _, present := r3["op_count"]; present {
		t.Fatalf("k3.op_count should have been dropped after unparseable coercion, got %v", r3["op_count"])
	}
	if v, ok := r3["node"].AsString(); !ok || v != "7" {
		t.Fatalf("k3.node: want Str(7), got %v", r3["node"])
	}

	// Timestamp never gets coerced. Verify that a deliberate
	// timestamp-type mismatch surfaces as a hard error rather than
	// being silently rewritten — the timestamp column is the
	// primary index and must be pinned.
	err = i.putData("metrics", "k4", db.Row{
		"timestamp": db.Str("not-a-ts"),
	})
	if err == nil {
		t.Fatal("putData should reject a non-int timestamp (pinned by RegisterSet)")
	}
}

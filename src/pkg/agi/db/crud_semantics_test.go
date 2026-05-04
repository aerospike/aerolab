package db

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cockroachdb/pebble/v2"
)

// TestGetProjectionAllUnknownReturnsEmptyRow verifies the disambiguated
// return of Get when the projection names exist but none are present in
// the schema: the row is present, so Get must return a non-nil empty
// Row, letting callers distinguish "row absent" (nil) from "row
// present, no projected columns in schema" (empty Row).
func TestGetProjectionAllUnknownReturnsEmptyRow(t *testing.T) {
	d := openTestDB(t)
	if err := d.Put("s", "k", Row{"a": Int(1)}); err != nil {
		t.Fatal(err)
	}
	row, err := d.Get("s", "k", "nope", "stillnope")
	if err != nil {
		t.Fatal(err)
	}
	if row == nil {
		t.Fatal("expected non-nil Row for present row with unknown projection")
	}
	if len(row) != 0 {
		t.Fatalf("expected empty Row, got %v", row)
	}
	// Absent row: still nil.
	row, err = d.Get("s", "ghost", "nope")
	if err != nil {
		t.Fatal(err)
	}
	if row != nil {
		t.Fatalf("expected nil Row for absent key, got %v", row)
	}
}

func TestUpdateDoesNotCreateSet(t *testing.T) {
	d := openTestDB(t)
	const unseen = "never-registered-set-name-xyz"
	if err := d.Update(unseen, "k", func(old Row) (Row, bool) {
		if old != nil {
			t.Error("old row should be nil for missing set")
		}
		return nil, false
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	for _, name := range d.Sets() {
		if name == unseen {
			t.Fatalf("set %q should not exist after skip Update", unseen)
		}
	}
}

func TestUpdateCreatesOnCommit(t *testing.T) {
	d := openTestDB(t)
	const set = "new-from-update-abc"
	if err := d.Update(set, "pk1", func(old Row) (Row, bool) {
		if old != nil {
			t.Error("expected nil old row for missing set")
		}
		return Row{"n": Int(7)}, true
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !slices.Contains(d.Sets(), set) {
		t.Fatalf("expected set %q in Sets(), got %v", set, d.Sets())
	}
	row, err := d.Get(set, "pk1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v, _ := row["n"].AsInt(); v != 7 {
		t.Fatalf("n: got %d want 7", v)
	}
}

func countIndexKeysForPK(t *testing.T, d *DB, setName, pk string) int {
	t.Helper()
	s, ok := d.lookupSet(setName)
	if !ok {
		t.Fatalf("lookupSet(%q): missing", setName)
	}
	s.mu.RLock()
	colID, hasIdx := s.indexedColumn()
	setID := s.ID
	s.mu.RUnlock()
	if !hasIdx {
		t.Fatalf("set %q has no index", setName)
	}
	lower := indexSetLower(setID)
	upper := indexSetUpper(setID)
	it, err := d.p.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		t.Fatalf("NewIter: %v", err)
	}
	defer it.Close()
	n := 0
	for it.First(); it.Valid(); it.Next() {
		_, gotCol, _, gotPK, ok := parseIndexKey(it.Key())
		if !ok {
			continue
		}
		if gotCol != colID || gotPK != pk {
			continue
		}
		n++
	}
	if err := it.Error(); err != nil {
		t.Fatalf("iter error: %v", err)
	}
	return n
}

func TestPutSameIndexedValueNoIndexRewrite(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("m", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "v", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}
	const ts = int64(42)
	if err := d.Put("m", "k1", Row{"ts": Int(ts), "v": Int(1)}); err != nil {
		t.Fatal(err)
	}
	if n := countIndexKeysForPK(t, d, "m", "k1"); n != 1 {
		t.Fatalf("after first Put: want 1 index key for pk, got %d", n)
	}
	if err := d.Put("m", "k1", Row{"ts": Int(ts), "v": Int(2)}); err != nil {
		t.Fatal(err)
	}
	if n := countIndexKeysForPK(t, d, "m", "k1"); n != 1 {
		t.Fatalf("after same-ts Put: want 1 index key for pk, got %d", n)
	}
	row, err := d.Get("m", "k1")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := row["v"].AsInt(); v != 2 {
		t.Fatalf("v: want 2, got %d", v)
	}
	// Query path should still see exactly one row for this key in range.
	it := d.Query("m").Between("ts", Int(ts-1), Int(ts+1)).Run(context.Background())
	defer it.Close()
	var qCount int
	for it.Next() {
		k, _ := it.Record()
		if k == "k1" {
			qCount++
		}
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if qCount != 1 {
		t.Fatalf("query count for k1: want 1, got %d", qCount)
	}
}

// TestUpdateEmptyRowFirstCallRejected: in the set-missing path, an fn
// that returns (empty-non-nil Row, true) must be rejected with the same
// error the set-existing path produces. Before Fix 6 the set-missing
// branch silently treated this as an empty Row and created no set.
func TestUpdateEmptyRowFirstCallRejected(t *testing.T) {
	d := openTestDB(t)
	err := d.Update("brand-new", "k", func(old Row) (Row, bool) {
		return Row{}, true
	})
	if !errors.Is(err, errUpdateEmptyRow) {
		t.Fatalf("expected errUpdateEmptyRow, got %v", err)
	}
	if slices.Contains(d.Sets(), "brand-new") {
		t.Fatal("set should not have been created after empty-row reject")
	}
}

// TestUpdateFnMayBeCalledTwice documents the set-missing bootstrap
// contract: when the set does not exist, fn is invoked once with nil
// to decide whether to create the set. If the caller commits a row AND
// a concurrent writer populates the same key before we re-acquire the
// row lock, fn is invoked a SECOND time with the observed row. Callers
// must write fn to be idempotent.
//
// This test deterministically provokes the double-call by orchestrating
// the race with a sync channel.
func TestUpdateFnMayBeCalledTwice(t *testing.T) {
	d := openTestDB(t)
	const set = "double-call"

	var calls atomic.Int32
	firstCallStarted := make(chan struct{})
	concurrentPutDone := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	var updateErr error
	go func() {
		defer wg.Done()
		updateErr = d.Update(set, "pk", func(old Row) (Row, bool) {
			n := calls.Add(1)
			if n == 1 {
				// Bootstrap call: signal the racer, wait for its Put
				// to commit, then return a row to commit and force
				// the second (under-lock) re-invocation.
				close(firstCallStarted)
				<-concurrentPutDone
				return Row{"n": Int(100)}, true
			}
			// Under-lock call: must see the concurrent Put's row.
			v, _ := old["n"].AsInt()
			return Row{"n": Int(v + 1)}, true
		})
	}()

	<-firstCallStarted
	if err := d.Put(set, "pk", Row{"n": Int(42)}); err != nil {
		t.Fatalf("concurrent Put: %v", err)
	}
	close(concurrentPutDone)
	wg.Wait()

	if updateErr != nil {
		t.Fatalf("Update: %v", updateErr)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected fn to be called exactly 2 times (bootstrap + under-lock); got %d", calls.Load())
	}
	row, err := d.Get(set, "pk")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := row["n"].AsInt(); v != 43 {
		t.Fatalf("final n: expected 43 (42 + 1 from second call), got %d", v)
	}
}

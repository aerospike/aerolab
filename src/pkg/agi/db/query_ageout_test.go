package db

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func TestBetweenTypeMismatchErrors(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("m", []ColumnSpec{{Name: "c", Type: TypeInt64, Indexed: true}}); err != nil {
		t.Fatal(err)
	}
	it := d.Query("m").Between("c", Str("a"), Str("b")).Run(context.Background())
	defer it.Close()
	err := it.Err()
	if err == nil {
		t.Fatal("expected Between type mismatch error")
	}
	s := err.Error()
	if !strings.Contains(s, "int64") || !strings.Contains(s, "string") {
		t.Errorf("error should mention type mismatch, got: %q", s)
	}
}

func TestDeleteBelowLargeAgeOutBounded(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("big", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "v", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}
	const n = 10_000
	for i := int64(0); i < n; i++ {
		if err := d.Put("big", keyOf(i), Row{"ts": Int(i), "v": Int(i)}); err != nil {
			t.Fatal(err)
		}
	}

	var m0 runtime.MemStats
	runtime.ReadMemStats(&m0)
	before := d.Stats().Deletes

	// Remove ts in [0, 9000) i.e. strictly below 9000.
	got, err := d.DeleteBelow(context.Background(), "big", 9000)
	if err != nil {
		t.Fatalf("DeleteBelow: %v", err)
	}
	if got != 9000 {
		t.Errorf("deleted %d, want 9000", got)
	}
	after := d.Stats().Deletes
	if d := int(after - before); d != 9000 {
		t.Errorf("Stats().Deletes +%d, want 9000", d)
	}

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	alloc := m1.TotalAlloc - m0.TotalAlloc
	t.Logf("TotalAlloc delta during test: %d", alloc)
	// Generous cap so this does not depend on exact GC or Pebble pressure.
	if alloc > 50<<20 {
		t.Errorf("alloc delta %d exceeds 50MiB bound (sanity check for chunking)", alloc)
	}

	it := d.Scan("big")
	defer it.Close()
	surv := 0
	for it.Next() {
		_, r := it.Record()
		ts, _ := r["ts"].AsInt()
		if ts < 9000 {
			t.Errorf("row below cutoff should be gone, ts=%d", ts)
		}
		surv++
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if surv != 1000 {
		t.Errorf("survivors %d, want 1000", surv)
	}
}

func keyOf(i int64) string {
	const hex = "0123456789abcdef"
	var b [16]byte
	ii := uint64(i)
	for j := 15; j >= 0; j-- {
		b[j] = hex[ii&0xf]
		ii >>= 4
	}
	return string(b[:])
}

func TestDeleteBelowBatchBoundary(t *testing.T) {
	// Straddle chunk boundaries: 1025 and 2049 deletions to exercise
	// 1024-PK commit batches and partial tail commits.
	cases := []struct {
		hi      int64
		wantDel int
		rows    int
	}{
		{1025, 1025, 2000},
		{2049, 2049, 5000},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("hi=%d", tc.hi), func(t *testing.T) {
			t.Helper()
			d := openTestDB(t)
			if err := d.RegisterSet("x", []ColumnSpec{
				{Name: "ts", Type: TypeInt64, Indexed: true},
			}); err != nil {
				t.Fatal(err)
			}
			for i := int64(0); i < int64(tc.rows); i++ {
				if err := d.Put("x", keyOf(i), Row{"ts": Int(i)}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := d.DeleteBelow(context.Background(), "x", tc.hi)
			if err != nil {
				t.Fatalf("DeleteBelow(%d): %v", tc.hi, err)
			}
			if got != tc.wantDel {
				t.Errorf("hi=%d: deleted %d, want %d", tc.hi, got, tc.wantDel)
			}
			remaining := 0
			it := d.Scan("x")
			for it.Next() {
				_, r := it.Record()
				ts, _ := r["ts"].AsInt()
				if ts < tc.hi {
					t.Errorf("stale row ts=%d below %d", ts, tc.hi)
				}
				remaining++
			}
			if err := it.Err(); err != nil {
				t.Fatal(err)
			}
			_ = it.Close()
			wantRem := tc.rows - tc.wantDel
			if remaining != wantRem {
				t.Errorf("remaining %d, want %d", remaining, wantRem)
			}
		})
	}
}

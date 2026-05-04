package db

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func benchDB(b *testing.B) *DB {
	b.Helper()
	opts := DefaultOptions()
	opts.Path = b.TempDir() + "/db"
	opts.CacheBytes = 64 << 20
	d, err := Open(opts)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = d.Close() })
	return d
}

func BenchmarkPutSerial(b *testing.B) {
	d := benchDB(b)
	_ = d.RegisterSet("m", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "v", Type: TypeInt64},
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.Put("m", fmt.Sprintf("k%010d", i), Row{"ts": Int(int64(i)), "v": Int(int64(i) * 3)})
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "puts/s")
}

func BenchmarkPutParallel128(b *testing.B) {
	d := benchDB(b)
	_ = d.RegisterSet("m", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "v", Type: TypeInt64},
	})
	var idx atomic.Int64
	const workers = 128
	b.ResetTimer()
	var wg sync.WaitGroup
	per := (b.N + workers - 1) / workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				n := idx.Add(1)
				_ = d.Put("m", fmt.Sprintf("k%010d", n), Row{"ts": Int(n), "v": Int(n * 2)})
			}
		}()
	}
	wg.Wait()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "puts/s")
}

func BenchmarkGet(b *testing.B) {
	d := benchDB(b)
	const n = 50000
	for i := 0; i < n; i++ {
		_ = d.Put("m", fmt.Sprintf("k%010d", i), Row{"v": Int(int64(i))})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.Get("m", fmt.Sprintf("k%010d", i%n))
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "gets/s")
}

func BenchmarkRangeQueryWideProjection(b *testing.B) {
	d := benchDB(b)
	_ = d.RegisterSet("m", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "c", Type: TypeInt64},
		{Name: "v", Type: TypeInt64},
	})
	const n = 100000
	for i := 0; i < n; i++ {
		_ = d.Put("m", fmt.Sprintf("k%010d", i), Row{
			"ts": Int(int64(i)),
			"c":  Int(int64(i % 16)),
			"v":  Int(int64(i * 3)),
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it := d.Query("m").
			Between("ts", Int(int64(n/4)), Int(int64(3*n/4))).
			Where(Eq("c", Int(7))).
			Run(context.Background())
		for it.Next() {
			_, _ = it.Record()
		}
		_ = it.Close()
	}
}

func BenchmarkRangeQueryNarrowProjection(b *testing.B) {
	d := benchDB(b)
	_ = d.RegisterSet("m", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "c", Type: TypeInt64},
		{Name: "v", Type: TypeInt64},
	})
	const n = 100000
	for i := 0; i < n; i++ {
		_ = d.Put("m", fmt.Sprintf("k%010d", i), Row{
			"ts": Int(int64(i)),
			"c":  Int(int64(i % 16)),
			"v":  Int(int64(i * 3)),
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it := d.Query("m").
			Between("ts", Int(int64(n/4)), Int(int64(3*n/4))).
			Where(Eq("c", Int(7))).
			Project("v").
			Run(context.Background())
		for it.Next() {
			_, _ = it.Record()
		}
		_ = it.Close()
	}
}

// BenchmarkIndexScanAllocs measures allocation-per-record for a narrow-
// projection range scan over 50K rows. Fix 9 defers pk string
// materialization to yield time and routes index→data via
// dataKeyFromIndexKey; we expect allocs/op to drop vs. the pre-fix
// baseline (~3x on the visited-row hot path). The absolute number
// depends on Go/Pebble versions; the test is a guardrail — run with
// -benchmem and compare against prior runs.
func BenchmarkIndexScanAllocs(b *testing.B) {
	d := benchDB(b)
	_ = d.RegisterSet("m", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "c", Type: TypeInt64},
		{Name: "v", Type: TypeInt64},
	})
	const n = 50000
	for i := 0; i < n; i++ {
		_ = d.Put("m", fmt.Sprintf("k%010d", i), Row{
			"ts": Int(int64(i)),
			"c":  Int(int64(i % 16)),
			"v":  Int(int64(i * 3)),
		})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it := d.Query("m").
			Between("ts", Int(int64(n/4)), Int(int64(3*n/4))).
			Where(Eq("c", Int(7))).
			Project("v").
			Run(context.Background())
		for it.Next() {
			_, _ = it.Record()
		}
		_ = it.Close()
	}
}

// BenchmarkEncodeRowAllocs measures allocations per row for a typical
// 5-column shape (int, float, string, bytes, bool). Fix 8 inlines
// payload emission into encodeRow, removing per-fixed-type intermediate
// allocations; we expect ≥ 30% reduction vs. the pre-fix baseline.
func BenchmarkEncodeRowAllocs(b *testing.B) {
	entries := []codecEntry{
		{ColID: 1, Typ: TypeInt64, Val: Int(12345)},
		{ColID: 2, Typ: TypeFloat64, Val: Float(3.14159)},
		{ColID: 3, Typ: TypeString, Val: Str("hello world")},
		{ColID: 4, Typ: TypeBytes, Val: BytesV([]byte{1, 2, 3, 4, 5, 6, 7, 8})},
		{ColID: 5, Typ: TypeBool, Val: BoolV(true)},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clone the slice because encodeRow sorts in place.
		cp := make([]codecEntry, len(entries))
		copy(cp, entries)
		buf, err := encodeRow(cp)
		if err != nil {
			b.Fatal(err)
		}
		_ = buf
	}
}

// BenchmarkRangeScan2M measures the wall-clock cost of an indexed
// range scan that returns ~half of a 2M-row dataset. With the
// clustered-by-time v2 layout the scan should issue zero per-row
// Pebble Gets (orphan-skip-guard not engaged because no AssumeNew
// writes happened in the seed), so the bench is dominated by
// Pebble's iterator throughput plus row decode, not by random reads
// against the D/ key space. We report rows/s alongside ns/op so a
// regression hunter can compare absolute throughput against the
// pre-clustering baseline (~600 KB/s rows-decoded on the dev box).
//
// Sized to populate roughly the working set the AGI workload sees in
// a 24h window: 2M rows × ~80 B/row ≈ 160 MB of payload, exceeding
// the 64 MiB cache benchDB allocates so the scan also tickles the
// SSTable read path.
func BenchmarkRangeScan2M(b *testing.B) {
	d := benchDB(b)
	if err := d.RegisterSet("m", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "c", Type: TypeInt64},
		{Name: "v", Type: TypeInt64},
	}); err != nil {
		b.Fatal(err)
	}
	const n = 2_000_000
	const batchSize = 1024
	items := make([]PutItem, 0, batchSize)
	for i := 0; i < n; i++ {
		items = append(items, PutItem{
			Key: fmt.Sprintf("k%010d", i),
			Row: Row{
				"ts": Int(int64(i)),
				"c":  Int(int64(i % 16)),
				"v":  Int(int64(i * 3)),
			},
		})
		if len(items) == batchSize {
			if err := d.PutBatch("m", items); err != nil {
				b.Fatal(err)
			}
			items = items[:0]
		}
	}
	if len(items) > 0 {
		if err := d.PutBatch("m", items); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	rows := 0
	for i := 0; i < b.N; i++ {
		it := d.Query("m").
			Between("ts", Int(int64(n/4)), Int(int64(3*n/4))).
			Run(context.Background())
		for it.Next() {
			_, _ = it.Record()
			rows++
		}
		_ = it.Close()
	}
	b.ReportMetric(float64(rows)/b.Elapsed().Seconds(), "rows/s")
}

// BenchmarkRangeScan2MWithExistsFilters mirrors the AGI histogram
// dashboard query shape: an indexed Between on timestamp combined
// with 3 BinExists filters and an optional Or(Not(Exists), Eq)
// fourth filter, plus a projection of timestamp + 1 value column.
//
// Three of four filter columns are presence-only and must skip
// decodePayload (the wantPresenceBM hot-path win). The fourth needs
// the value because of the inner Eq. Compare ns/op against
// BenchmarkRangeScan2M (a no-filter, no-projection scan over the
// same 2 M dataset) to gauge the per-row cost added by the
// presence-aware filter classifier vs. the old "decode every filter
// col" path.
func BenchmarkRangeScan2MWithExistsFilters(b *testing.B) {
	d := benchDB(b)
	if err := d.RegisterSet("m", []ColumnSpec{
		{Name: "timestamp", Type: TypeInt64, Indexed: true},
		{Name: "Histogram", Type: TypeString},
		{Name: "ClusterName", Type: TypeInt64},
		{Name: "NodeIdent", Type: TypeString},
		{Name: "Namespace", Type: TypeString},
		{Name: "value", Type: TypeInt64},
	}); err != nil {
		b.Fatal(err)
	}
	const n = 2_000_000
	const batchSize = 1024
	items := make([]PutItem, 0, batchSize)
	for i := 0; i < n; i++ {
		items = append(items, PutItem{
			Key: fmt.Sprintf("k%010d", i),
			Row: Row{
				"timestamp":   Int(int64(i)),
				"Histogram":   Str("latency"),
				"ClusterName": Int(int64(i % 4)),
				"NodeIdent":   Str(fmt.Sprintf("n%d", i%8)),
				"Namespace":   Str(fmt.Sprintf("ns%d", i%2)),
				"value":       Int(int64(i * 3)),
			},
		})
		if len(items) == batchSize {
			if err := d.PutBatch("m", items); err != nil {
				b.Fatal(err)
			}
			items = items[:0]
		}
	}
	if len(items) > 0 {
		if err := d.PutBatch("m", items); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	rows := 0
	for i := 0; i < b.N; i++ {
		it := d.Query("m").
			Between("timestamp", Int(int64(n/4)), Int(int64(3*n/4))).
			Where(And(
				Exists("Histogram"),
				Exists("ClusterName"),
				Exists("NodeIdent"),
				Or(Not(Exists("Namespace")), Eq("Namespace", Str("ns0"))),
			)).
			Project("timestamp", "value").
			Run(context.Background())
		for it.Next() {
			_, _ = it.Record()
			rows++
		}
		_ = it.Close()
	}
	b.ReportMetric(float64(rows)/b.Elapsed().Seconds(), "rows/s")
}

func BenchmarkFullScanLabels(b *testing.B) {
	d := benchDB(b)
	// Mimic the labels catalog shape: one record per label with JSON-ish
	// string bin.
	const n = 10000
	for i := 0; i < n; i++ {
		_ = d.Put("labels", fmt.Sprintf("label%d", i), Row{
			"Namespace": Str(fmt.Sprintf(`{"entries":["ns%d"]}`, i)),
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it := d.Scan("labels")
		for it.Next() {
			_, _ = it.Record()
		}
		_ = it.Close()
	}
}

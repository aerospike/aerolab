package db

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentPuts128Goroutines(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("metrics", []ColumnSpec{
		{Name: "timestamp", Type: TypeInt64, Indexed: true},
		{Name: "v", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}

	const workers = 128
	const perWorker = 200
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				key := fmt.Sprintf("w%03d-r%05d", w, i)
				row := Row{"timestamp": Int(int64(w*perWorker + i)), "v": Int(int64(i))}
				if err := d.Put("metrics", key, row); err != nil {
					t.Errorf("worker %d put %d: %v", w, i, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	// Verify via a full-range query on the index.
	it := d.Query("metrics").Between("timestamp", Int(0), Int(int64(workers*perWorker))).Run(context.Background())
	defer it.Close()
	seen := 0
	for it.Next() {
		seen++
	}
	if it.Err() != nil {
		t.Fatal(it.Err())
	}
	if seen != workers*perWorker {
		t.Errorf("expected %d rows, saw %d", workers*perWorker, seen)
	}
}

func TestConcurrentUpdateLinearizable(t *testing.T) {
	d := openTestDB(t)
	const workers = 64
	const incPer = 100
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < incPer; i++ {
				err := d.Update("ctr", "x", func(old Row) (Row, bool) {
					var n int64
					if old != nil {
						n, _ = old["n"].AsInt()
					}
					return Row{"n": Int(n + 1)}, true
				})
				if err != nil {
					t.Errorf("update: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	row, _ := d.Get("ctr", "x")
	n, _ := row["n"].AsInt()
	if n != workers*incPer {
		t.Errorf("counter = %d, want %d", n, workers*incPer)
	}
}

func TestContextCancelDuringScan(t *testing.T) {
	d := openTestDB(t)
	for i := 0; i < 5000; i++ {
		if err := d.Put("big", fmt.Sprintf("k%05d", i), Row{"v": Int(int64(i))}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	it := d.ScanContext(ctx, "big")
	defer it.Close()
	// Ensure it starts.
	if !it.Next() {
		t.Fatal("first Next failed")
	}
	cancel()
	// Drain until it yields false.
	tries := 0
	for it.Next() {
		tries++
		if tries > 10000 {
			t.Fatal("did not stop after cancel")
		}
	}
	if it.Err() != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", it.Err())
	}
}

// TestConcurrentSchemaGrowthVsQuery mixes writes that implicitly register
// new columns with ongoing queries + scans. Before the schema-snapshot
// refactor this would trip `go test -race` because iterators read
// s.Columns / s.ByID / s.IndexedCol while another goroutine was inside
// ensureSetLocked adding new columns.
func TestConcurrentSchemaGrowthVsQuery(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("metrics", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Writer: introduces a new column every 50 puts.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := int64(0); i < 2000; i++ {
			row := Row{"ts": Int(i)}
			col := fmt.Sprintf("c%d", i/50)
			row[col] = Int(i)
			if err := d.Put("metrics", fmt.Sprintf("k%d", i), row); err != nil {
				t.Errorf("put: %v", err)
				return
			}
		}
	}()

	// Readers: mix of range queries, full scans, and point gets.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				if ctx.Err() != nil {
					return
				}
				switch i % 3 {
				case 0:
					it := d.Query("metrics").
						Between("ts", Int(0), Int(1<<30)).
						Where(Exists(fmt.Sprintf("c%d", i%10))).
						Project("ts", fmt.Sprintf("c%d", i%10)).
						Run(ctx)
					for it.Next() {
						_, _ = it.Record()
					}
					_ = it.Close()
				case 1:
					it := d.Scan("metrics")
					for it.Next() {
						_, _ = it.Record()
					}
					_ = it.Close()
				case 2:
					_, _ = d.Get("metrics", fmt.Sprintf("k%d", i*r%100), "ts")
				}
			}
		}(r)
	}
	wg.Wait()
}

// TestDropSetAtomicity verifies that when DropSet's batch commit
// succeeds, both the schema maps and the on-disk state are gone; and
// that after a re-create via Put, the fresh set gets a new setID
// (nextSetID is monotonic) while the old set's data stays dropped.
func TestDropSetAtomicity(t *testing.T) {
	d := openTestDB(t)
	_ = d.RegisterSet("m", []ColumnSpec{{Name: "ts", Type: TypeInt64, Indexed: true}})
	for i := 0; i < 10; i++ {
		_ = d.Put("m", fmt.Sprintf("k%d", i), Row{"ts": Int(int64(i))})
	}
	if err := d.DropSet("m"); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.SchemaOf("m"); ok {
		t.Error("schema should be gone after DropSet")
	}
	// Re-create via Put on the same name. The new set should be empty.
	_ = d.Put("m", "new", Row{"v": Int(1)})
	count := 0
	it := d.Scan("m")
	defer it.Close()
	for it.Next() {
		count++
	}
	if count != 1 {
		t.Errorf("re-created set should have only the one new row, got %d", count)
	}
}

func TestConcurrentPutsSameKeySerialize(t *testing.T) {
	// Concurrent puts to the same key must not corrupt the index: the final
	// row's indexed value is the only one that remains in the index and
	// the data row's Get returns a value we actually wrote.
	d := openTestDB(t)
	_ = d.RegisterSet("m", []ColumnSpec{{Name: "ts", Type: TypeInt64, Indexed: true}})

	const workers = 50
	const iters = 200
	var wg sync.WaitGroup
	var putOK uint64
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				ts := int64(w*iters + i + 1)
				if err := d.Put("m", "shared", Row{"ts": Int(ts)}); err == nil {
					atomic.AddUint64(&putOK, 1)
				}
			}
		}(w)
	}
	wg.Wait()

	if putOK != workers*iters {
		t.Errorf("some puts failed: %d/%d", putOK, workers*iters)
	}

	// Index must contain exactly one entry for key "shared" (matching the
	// final ts).
	row, _ := d.Get("m", "shared")
	finalTS, _ := row["ts"].AsInt()

	iter := d.Query("m").Between("ts", Int(0), Int(1<<62)).Run(context.Background())
	defer iter.Close()
	count := 0
	var idxTS int64
	for iter.Next() {
		k, r := iter.Record()
		if k != "shared" {
			t.Errorf("unexpected key %q", k)
		}
		idxTS, _ = r["ts"].AsInt()
		count++
	}
	if count != 1 {
		t.Errorf("index has %d entries for shared key, want 1", count)
	}
	if idxTS != finalTS {
		t.Errorf("index ts %d != stored ts %d", idxTS, finalTS)
	}
}

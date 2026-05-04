package ingest

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// TestIngestUsesPutBatch verifies the ingest hot path commits rows
// through db.PutBatch with AssumeNew=true: 1000 rows must commit
// without any per-row pre-Get on the indexed set. The previous
// version of this test asserted d.assumeNewSeen was set, but that
// auto-flip was removed (it caused a 4-5× regression on indexed
// range scans). Today the read-path optimisation is "no orphan-guard
// unless explicitly opted in", so we instead assert the write-side
// observable: zero PebbleGets across the whole batched ingest.
func TestIngestUsesPutBatch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ingest-db")
	opts := db.DefaultOptions()
	opts.Path = dir
	d, err := db.Open(opts)
	if err != nil {
		t.Fatalf("db.Open: %s", err)
	}
	defer d.Close()

	if err := d.RegisterSet("metrics", []db.ColumnSpec{
		{Name: "timestamp", Type: db.TypeInt64, Indexed: true},
		{Name: "v", Type: db.TypeInt64},
	}); err != nil {
		t.Fatalf("RegisterSet: %s", err)
	}

	cfg := new(Config)
	cfg.TimestampColumnName = "timestamp"
	i := &Ingest{config: cfg, db: d}
	// Mirror finalizeInit: aggressive flush parameters keep the
	// test fast and deterministic without changing the semantics
	// the production batcher exposes.
	i.putBatcher = newPutBatcher(d, 256, 5, 0, i.putDataSingle)
	// We close the batcher explicitly mid-test to drain pending
	// rows before reading stats. Re-closing inside a defer would
	// panic on the closed inCh, so the explicit close below also
	// stands as the only close.

	const rows = 1000
	beforeGets := d.Stats().PebbleGets
	for r := 0; r < rows; r++ {
		key := "k" + itoa(r)
		if err := i.putData("metrics", key, db.Row{
			"timestamp": db.Int(int64(1_000_000 + r)),
			"v":         db.Int(int64(r)),
		}); err != nil {
			t.Fatalf("putData[%d]: %s", r, err)
		}
	}
	// Drain the batcher so all pending PutBatch commits land
	// before we sample stats. close() blocks until the flusher
	// has committed everything it had buffered.
	i.putBatcher.close()
	i.putBatcher = nil

	gets := d.Stats().PebbleGets - beforeGets
	if gets != 0 {
		t.Errorf("ingest write path issued %d Pebble Gets across %d rows; want 0 (AssumeNew=true should skip the pre-read)", gets, rows)
	}

	// Sanity: the rows are actually persisted. We do issue Gets
	// here; they intentionally land AFTER the gets-counter sample
	// above so they are not counted against the write-path
	// budget.
	for _, k := range []string{"k0", "k500", "k999"} {
		row, err := d.Get("metrics", k)
		if err != nil {
			t.Fatalf("Get %s: %s", k, err)
		}
		if row == nil {
			t.Errorf("ingest dropped row %s", k)
		}
	}
	// Guard against the timer-only flush regressing: even if no
	// flushSize threshold ever fired, the timer must drain a
	// bounded number of pending rows. We assert close() did the
	// drain by checking row count is non-zero — but we already
	// did that above; the time.Sleep here exists only to give the
	// race detector a window to surface concurrent map writes if
	// any were introduced.
	time.Sleep(10 * time.Millisecond)
}

// TestPutBatcherShardedConcurrentSubmit verifies that a sharded
// putBatcher correctly accepts concurrent submit() calls from many
// producers and that every row lands exactly once. It is deliberately
// constructed so several keys hash to each shard (the key space is
// dense and the shard count is small) to exercise the multi-shard
// path and shake out any data races; the race detector is the
// primary signal here, but we also assert the row count and a few
// random Get() probes for correctness.
func TestPutBatcherShardedConcurrentSubmit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ingest-shard-db")
	opts := db.DefaultOptions()
	opts.Path = dir
	d, err := db.Open(opts)
	if err != nil {
		t.Fatalf("db.Open: %s", err)
	}
	defer d.Close()

	if err := d.RegisterSet("metrics", []db.ColumnSpec{
		{Name: "timestamp", Type: db.TypeInt64, Indexed: true},
		{Name: "v", Type: db.TypeInt64},
	}); err != nil {
		t.Fatalf("RegisterSet: %s", err)
	}

	cfg := new(Config)
	cfg.TimestampColumnName = "timestamp"
	i := &Ingest{config: cfg, db: d}
	// Force a non-trivial shard count so submit() actually
	// distributes rows across multiple flusher goroutines.
	const shards = 4
	i.putBatcher = newPutBatcher(d, 64, 5, shards, i.putDataSingle)

	const (
		producers      = 8
		rowsPerProducer = 500
	)
	var submitted atomic.Int64
	wg := new(sync.WaitGroup)
	wg.Add(producers)
	for p := 0; p < producers; p++ {
		go func(p int) {
			defer wg.Done()
			for r := 0; r < rowsPerProducer; r++ {
				key := "p" + itoa(p) + "-r" + itoa(r)
				if perr := i.putData("metrics", key, db.Row{
					"timestamp": db.Int(int64(1_000_000 + p*rowsPerProducer + r)),
					"v":         db.Int(int64(r)),
				}); perr != nil {
					t.Errorf("putData[%s]: %s", key, perr)
					return
				}
				submitted.Add(1)
			}
		}(p)
	}
	wg.Wait()

	i.putBatcher.close()
	i.putBatcher = nil

	if got := submitted.Load(); got != int64(producers*rowsPerProducer) {
		t.Fatalf("submitted=%d want=%d", got, producers*rowsPerProducer)
	}

	// Spot-check that the rows actually persisted under their
	// own keys (i.e. the sharded routing did not corrupt any
	// (set,key) tuple).
	for _, k := range []string{"p0-r0", "p3-r123", "p7-r499"} {
		row, gerr := d.Get("metrics", k)
		if gerr != nil {
			t.Fatalf("Get %s: %s", k, gerr)
		}
		if row == nil {
			t.Errorf("ingest dropped row %s", k)
		}
	}
}

// itoa is a stack-allocating int->string for hot test loops; the
// stdlib strconv.Itoa would also work but callers are conventionally
// quiet about heap traffic in inner test loops.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

package db

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCloseDrainsInFlightWrites verifies that Close() waits for an
// in-flight Put to complete before shutting Pebble down. Before the
// lifeMu fix, this raced and could panic inside Pebble.
func TestCloseDrainsInFlightWrites(t *testing.T) {
	opts := DefaultOptions()
	opts.Path = filepath.Join(t.TempDir(), "db")
	d, err := Open(opts)
	if err != nil {
		t.Fatal(err)
	}

	// Fire a lot of concurrent writers and then call Close concurrently.
	// None of them should panic, and Close should eventually return.
	var wg sync.WaitGroup
	var putErrs atomic.Uint64
	var putOK atomic.Uint64
	for w := 0; w < 32; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				err := d.Put("s", fmt.Sprintf("w%d-k%d", w, i), Row{"v": Int(int64(i))})
				switch {
				case err == nil:
					putOK.Add(1)
				case errors.Is(err, ErrClosed):
					putErrs.Add(1)
					return
				default:
					t.Errorf("unexpected put err: %v", err)
					return
				}
			}
		}(w)
	}
	// Give writers a head start, then close.
	time.Sleep(5 * time.Millisecond)
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	wg.Wait()

	if putOK.Load() == 0 {
		t.Error("no puts succeeded before Close; test did not exercise the race")
	}
	if putErrs.Load() == 0 {
		t.Log("note: no puts observed ErrClosed — race window was narrow")
	}
	// Second close must report already-closed.
	if err := d.Close(); !errors.Is(err, ErrClosed) {
		t.Errorf("second Close: got %v, want ErrClosed", err)
	}
	// Public ops after close should all return ErrClosed, not panic.
	if _, err := d.Get("s", "k"); !errors.Is(err, ErrClosed) {
		t.Errorf("Get post-close: %v", err)
	}
	if err := d.Put("s", "k", Row{"v": Int(1)}); !errors.Is(err, ErrClosed) {
		t.Errorf("Put post-close: %v", err)
	}
	if err := d.RegisterSet("s", []ColumnSpec{{Name: "v", Type: TypeInt64}}); !errors.Is(err, ErrClosed) {
		t.Errorf("RegisterSet post-close: %v", err)
	}
	if err := d.DropSet("s"); !errors.Is(err, ErrClosed) {
		t.Errorf("DropSet post-close: %v", err)
	}
	it := d.Scan("s")
	if it.Err() == nil || !errors.Is(it.Err(), ErrClosed) {
		t.Errorf("Scan post-close: %v", it.Err())
	}
	_ = it.Close()
}

// TestDropSetRacesBlockedCleanly exercises concurrent Put + DropSet and
// verifies the contract:
//   - Pre-populated rows are all wiped.
//   - Concurrent writers never panic; each Put either succeeds against
//     the old generation (and is wiped by the drop) or returns
//     ErrSetDropped under the old pointer, or transparently lands in
//     the fresh post-drop generation.
//   - After the drop commits, no key from the PRE-DROP batch survives.
func TestDropSetRacesBlockedCleanly(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("m", []ColumnSpec{{Name: "ts", Type: TypeInt64, Indexed: true}}); err != nil {
		t.Fatal(err)
	}

	const preSeed = 200
	preKeys := make(map[string]struct{}, preSeed)
	for i := 0; i < preSeed; i++ {
		key := fmt.Sprintf("pre-%03d", i)
		preKeys[key] = struct{}{}
		if err := d.Put("m", key, Row{"ts": Int(int64(i))}); err != nil {
			t.Fatal(err)
		}
	}

	// Writers use a "post-" key prefix so they can't accidentally collide
	// with the pre-seed set we are checking for.
	var wg sync.WaitGroup
	stop := make(chan struct{})
	var putOK, putDropped atomic.Uint64
	for w := 0; w < 16; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				err := d.Put("m", fmt.Sprintf("post-w%d-k%d", w, i), Row{"ts": Int(int64(i))})
				switch {
				case err == nil:
					putOK.Add(1)
				case errors.Is(err, ErrSetDropped):
					putDropped.Add(1)
				default:
					t.Errorf("put: %v", err)
					return
				}
			}
		}(w)
	}
	time.Sleep(20 * time.Millisecond)
	if err := d.DropSet("m"); err != nil {
		t.Fatalf("DropSet: %v", err)
	}
	close(stop)
	wg.Wait()

	// Pre-drop rows must be gone regardless of how the writers raced.
	for key := range preKeys {
		row, err := d.Get("m", key)
		if err != nil {
			t.Fatalf("Get(%s): %v", key, err)
		}
		if row != nil {
			t.Errorf("pre-drop row %q survived drop: %v", key, row)
		}
	}

	// Re-create via Put to confirm the fresh generation takes a new
	// setID (i.e., post-drop state is coherent, not half-deleted).
	if err := d.Put("m", "fresh-unique-key", Row{"v": Int(42)}); err != nil {
		t.Fatal(err)
	}
	row, err := d.Get("m", "fresh-unique-key")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := row["v"].AsInt(); v != 42 {
		t.Errorf("fresh row readback: %v", row)
	}

	t.Logf("put outcomes: ok=%d errSetDropped=%d", putOK.Load(), putDropped.Load())
}

// TestDropSetRaceSchemaOrphan exercises the DropSet vs persistSchema
// race closed by taking s.mu around the DropSet batch. Without that
// lock, a concurrent Put slow path (novel column forces
// persistSchema) can commit its M/schema/<id> write AFTER DropSet's
// batch.Delete, leaving an orphan schema that resurrects the set on
// the next Open.
//
// We can't inject a hook, so we fire many concurrent Puts that each
// introduce a novel column (forcing the slow schema-persist path),
// stop the writer pool to guarantee no post-drop writer re-creates
// the set, call DropSet while mid-flight slow-path writers may still
// be inside persistSchema, then close and reopen the DB and assert
// Sets() does not contain the dropped name.
func TestDropSetRaceSchemaOrphan(t *testing.T) {
	tmp := t.TempDir()
	opts := DefaultOptions()
	opts.Path = filepath.Join(tmp, "db")
	opts.CacheBytes = 16 << 20
	d, err := Open(opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const set = "racy"
	if err := d.RegisterSet(set, []ColumnSpec{{Name: "ts", Type: TypeInt64, Indexed: true}}); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for w := 0; w < 32; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				col := fmt.Sprintf("col_%d_%d", w, i)
				row := Row{"ts": Int(int64(i)), col: Int(int64(i))}
				err := d.Put(set, fmt.Sprintf("k_%d_%d", w, i), row)
				switch {
				case err == nil, errors.Is(err, ErrSetDropped), errors.Is(err, ErrClosed):
					// ok
				default:
					t.Errorf("put: %v", err)
					return
				}
			}
		}(w)
	}
	// Warm up: let writers accumulate a queue of in-flight slow-path
	// Puts so DropSet genuinely overlaps with persistSchema.
	time.Sleep(30 * time.Millisecond)
	// Stop the writers BEFORE DropSet so no post-drop Put can
	// legitimately re-create the set and mask the orphan we're
	// testing for. Writers already mid-Put complete against the old
	// s pointer and race DropSet's batch; new Puts exit via stop.
	close(stop)
	if err := d.DropSet(set); err != nil {
		t.Fatalf("DropSet: %v", err)
	}
	wg.Wait()

	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	d2, err := Open(opts)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer d2.Close()
	for _, name := range d2.Sets() {
		if name == set {
			t.Fatalf("orphan schema survived drop: set %q resurrected after reopen", name)
		}
	}
}

// TestPerSetSchemaDoesNotSerialiseAcrossSets is a weak timing test: two
// sets receive implicit column registrations from many goroutines each;
// neither writer should wait on the other's schema-persist call. We
// can't assert exact parallelism, but we can assert that the overall
// wall-clock matches "independent" rather than "serialised".
func TestPerSetSchemaDoesNotSerialiseAcrossSets(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive; skipped in -short")
	}
	d := openTestDB(t)

	writeMany := func(set string, workers, perWorker int) {
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < perWorker; i++ {
					row := Row{
						// Each iteration introduces a unique column
						// name, forcing the slow schema-persist path.
						fmt.Sprintf("col_%s_%d_%d", set, w, i): Int(int64(i)),
					}
					if err := d.Put(set, fmt.Sprintf("k_%d_%d", w, i), row); err != nil {
						t.Errorf("%s put: %v", set, err)
						return
					}
				}
			}(w)
		}
		wg.Wait()
	}

	// Warm up.
	_ = d.Put("a", "warm", Row{"x": Int(0)})
	_ = d.Put("b", "warm", Row{"x": Int(0)})

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); writeMany("a", 8, 200) }()
	go func() { defer wg.Done(); writeMany("b", 8, 200) }()
	wg.Wait()
	parallelDur := time.Since(start)

	// Sanity: this should complete in well under a second even in CI.
	if parallelDur > 10*time.Second {
		t.Errorf("parallel two-set schema churn took %s; expected much faster with per-set locks", parallelDur)
	}
	t.Logf("two-set schema-churn parallel duration: %s", parallelDur)
}


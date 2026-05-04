package db

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestCloseRejectsOpenIterators(t *testing.T) {
	opts := DefaultOptions()
	opts.Path = filepath.Join(t.TempDir(), "db")
	opts.CacheBytes = 16 << 20
	d, err := Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterSet("s", []ColumnSpec{{Name: "k", Type: TypeInt64, Indexed: true}}); err != nil {
		t.Fatal(err)
	}
	if err := d.Put("s", "a", Row{"k": Int(1)}); err != nil {
		t.Fatal(err)
	}
	it := d.Scan("s")
	if err := d.Close(); err == nil {
		it.Close()
		d = nil
		t.Fatal("expected Close to reject open iterator")
	} else if !errors.Is(err, ErrIteratorsOpen) {
		it.Close()
		_ = d.Close()
		t.Fatalf("expected ErrIteratorsOpen, got %v", err)
	}
	if err := it.Close(); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestDropColumnConcurrentPutNoDeadlock(t *testing.T) {
	d := openTestDB(t)
	const setName = "m"
	if err := d.RegisterSet(setName, []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "extra", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}

	// End well before t's deadline so workers can exit and openTestDB's
	// Close can complete before the test process timeout.
	stop := make(chan struct{})
	var stopOnce sync.Once
	armStop := func() { stopOnce.Do(func() { close(stop) }) }
	go func() {
		fin := 8 * time.Second
		if dl, ok := t.Deadline(); ok {
			if u := time.Until(dl) - 500 * time.Millisecond; u > 0 && u < fin {
				fin = u
			}
		}
		time.Sleep(fin)
		armStop()
	}()
	if dl, ok := t.Deadline(); ok {
		d := time.Until(dl) - 400 * time.Millisecond
		if d > 0 {
			time.AfterFunc(d, armStop)
		}
	}

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				pk := fmt.Sprintf("k-%d-%d", id, i)
				if err := d.Put(setName, pk, Row{
					"ts":    Int(int64(i)),
					"extra": Int(1),
				}); err != nil {
					if errors.Is(err, ErrSetDropped) || errors.Is(err, ErrClosed) {
						return
					}
					t.Errorf("Put: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Register a transient column, then drop it (DropColumn+Put
			// interleave per the plan).
			if err := d.RegisterSet(setName, []ColumnSpec{{Name: "aux", Type: TypeInt64}}); err != nil {
				if errors.Is(err, ErrSetDropped) || errors.Is(err, ErrClosed) {
					return
				}
				t.Errorf("RegisterSet: %v", err)
				return
			}
			_ = d.DropColumn(setName, "aux")
		}
	}()
	wg.Wait()
}

func TestDropSetRecreateEnsureSetNotDroppedPointer(t *testing.T) {
	// Commit failure from DropSet is not injectable here; this asserts that
	// a successful drop + recreate leaves Put/RegisterSet working, which
	// exercises ensureSet/lookupSet on a new generation for the name.
	d := openTestDB(t)
	const name = "m"
	if err := d.RegisterSet(name, []ColumnSpec{{Name: "ts", Type: TypeInt64, Indexed: true}}); err != nil {
		t.Fatal(err)
	}
	if err := d.Put(name, "k0", Row{"ts": Int(0)}); err != nil {
		t.Fatal(err)
	}
	if err := d.DropSet(name); err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterSet(name, []ColumnSpec{{Name: "ts", Type: TypeInt64, Indexed: true}}); err != nil {
		t.Fatal(err)
	}
	if err := d.Put(name, "k1", Row{"ts": Int(42)}); err != nil {
		t.Fatal(err)
	}
	row, err := d.Get(name, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if row == nil {
		t.Fatal("row missing after recreate")
	}
	if v, _ := row["ts"].AsInt(); v != 42 {
		t.Errorf("ts: got %d want 42", v)
	}
	_, err = d.Get(name, "k0")
	if err != nil {
		t.Fatal(err)
	}
	// k0 was under the old setID; after drop+recreate it should be absent.
	row, _ = d.Get(name, "k0")
	if row != nil {
		t.Errorf("expected old key gone, got %v", row)
	}
}

// TestFinalizerAfterForceCloseNoPanic simulates the worst-case path for
// the iterator leak finalizer: the caller forgets to Close() and the DB
// is flagged as closed (e.g., process is shutting down) before the
// finalizer runs. The finalizer must bump LeakedIterators and NOT call
// Close() on the iterator, because Pebble resources belonging to a
// closed DB may no longer be safe to touch.
//
// The test does not try to cleanly re-Close the DB afterwards: by
// design the leaked iterator is still outstanding from Pebble's
// perspective and a post-test d.Close() would surface that as an
// ErrIteratorsOpen. The TempDir cleanup handles the on-disk bits.
func TestFinalizerAfterForceCloseNoPanic(t *testing.T) {
	opts := DefaultOptions()
	opts.Path = filepath.Join(t.TempDir(), "db")
	opts.CacheBytes = 16 << 20
	d, err := Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Put("m", "k", Row{"v": Int(1)}); err != nil {
		t.Fatal(err)
	}
	before := d.Stats().LeakedIterators
	// Open an iterator, advance once, then drop the reference without
	// Close.
	func() {
		it := d.Scan("m")
		it.Next()
		_ = it
	}()
	// Flip closed before the finalizer runs so it takes the
	// DB-already-closed branch and does NOT call o.Close().
	d.closed.Store(true)

	// Drive GC / finalizers. Any panic from the finalizer would
	// propagate and fail the test.
	for i := 0; i < 10; i++ {
		runtime.GC()
		runtime.Gosched()
		time.Sleep(20 * time.Millisecond)
		if d.Stats().LeakedIterators > before {
			break
		}
	}
	if d.Stats().LeakedIterators <= before {
		t.Fatalf("LeakedIterators did not increment after forced-close leak (before=%d, after=%d)", before, d.Stats().LeakedIterators)
	}
	// Drop the Pebble handle without going through d.Close() —
	// d.Close() would refuse with ErrIteratorsOpen and tearing down
	// Pebble here would surface the (intentional) iterator leak as a
	// fatal. We trust TempDir to remove the on-disk bits.
	_ = d
}

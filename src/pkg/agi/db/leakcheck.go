package db

import (
	"runtime"
	"sync/atomic"
)

// iterLifecycle is shared bookkeeping for every scan / query iterator
// returned by this package. It tracks whether Close() has been called
// and arranges for a GC finalizer to log a leak and release the
// underlying Pebble resources if the iterator is dropped without Close
// — long-lived unclosed iterators pin a Pebble snapshot, which prevents
// sstable reclamation and compaction.
//
// The finalizer is a safety net, not a substitute for defer Close():
// finalizer timing is non-deterministic and running code inside
// finalizers is discouraged. Callers must still Close() normally; the
// finalizer exists so a buggy caller is loud and non-fatal rather than
// silently pinning disk space.
//
// The zero value is not usable; call newIterLifecycle to install the
// finalizer. closeOnce() is idempotent and safe to call from Close().
type iterLifecycle struct {
	db         *DB
	kind       string // human-readable tag for leak logs ("scan", "query", "index")
	setName    string
	closed     atomic.Bool
	registered atomic.Bool
}

// closable is the minimum shape iterLifecycle needs to release the
// underlying Pebble resources from a GC finalizer.
type closable interface {
	Close() error
}

// newIterLifecycle registers the iterator with the DB's open-iterator
// counter and installs a finalizer on the owning iterator pointer. The
// owner's Close() is called from the finalizer so the Pebble iterator
// and snapshot are released even when the caller forgot to Close().
func newIterLifecycle(d *DB, kind, setName string, owner closable) *iterLifecycle {
	l := &iterLifecycle{db: d, kind: kind, setName: setName}
	d.stats.iterOpen.Add(1)
	l.registered.Store(true)
	runtime.SetFinalizer(owner, func(o closable) {
		if l.closed.Load() {
			return
		}
		// Treat as a leak: bump the leak counter for ops dashboards
		// and log loudly. If the DB has already been Closed we must
		// NOT touch the Pebble iterator: on some Pebble versions
		// Closing an iterator whose parent DB has torn down its
		// background goroutines can panic, and we do not want a
		// late-firing finalizer to crash the process at shutdown.
		// Snapshot reclamation is handled by Pebble when the DB
		// closes, so skipping the Close() here is safe.
		l.db.stats.iterLeaked.Add(1)
		if l.db.closed.Load() {
			l.db.opts.Logger.Printf("WARN: db: %s iterator on set %q leaked (Close() never called); DB already closed, skipping finalizer Close()", l.kind, l.setName)
			return
		}
		l.db.opts.Logger.Printf("WARN: db: %s iterator on set %q leaked (Close() never called); releasing via finalizer", l.kind, l.setName)
		_ = o.Close()
	})
	return l
}

// closeOnce marks the iterator closed and releases the open-iter count.
// Must be called from every iterator's Close() exactly once.
func (l *iterLifecycle) closeOnce() {
	if l == nil {
		return
	}
	if !l.closed.CompareAndSwap(false, true) {
		return
	}
	if l.registered.CompareAndSwap(true, false) {
		l.db.stats.iterOpen.Add(-1)
	}
}

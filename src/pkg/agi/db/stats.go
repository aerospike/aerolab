package db

import "sync/atomic"

// dbStats holds atomic counters for basic instrumentation. Cheap to read.
type dbStats struct {
	puts     atomic.Uint64
	deletes  atomic.Uint64
	getCalls atomic.Uint64
	scans    atomic.Uint64
	queries  atomic.Uint64
	// pebbleGets counts every internal point Get against Pebble (both
	// d.p.Get and snap.Get on a snapshot returned by d.p.NewSnapshot).
	// It is the lower-level cousin of getCalls (which counts public
	// db.Get invocations). Tests use it to assert that the v2
	// indexed-scan path performs zero Pebble point Gets per visited
	// row — the entire payload is read out of the Pebble iterator's
	// Value buffer.
	pebbleGets   atomic.Uint64
	iterOpen     atomic.Int64 // currently-open iterator count
	iterLeaked   atomic.Uint64
	pebbleFatals *atomic.Uint64 // pebbleLoggerAdapter.Fatalf; non-nil after Open
}

// Stats snapshots the DB's runtime counters. Values in the Counters
// block are monotonically increasing since Open. Pebble holds instantaneous
// values (cache size, memtable bytes in use, compaction depth, …).
type Stats struct {
	// Counters
	Puts       uint64
	Deletes    uint64
	Gets       uint64
	Scans      uint64
	Queries    uint64
	PebbleGets uint64

	// Iterators
	OpenIterators   int64  // currently open, pin snapshots/SSTs
	LeakedIterators uint64 // detected by finalizer; see D10 notes
	// PebbleFatals counts invocations of Pebble's logger Fatalf (invariant
	// violations, etc.); the adapter logs and does not exit the process.
	PebbleFatals uint64
	// DiskUsageBytes is the total on-disk space reported by Pebble
	// (sstables + WAL + manifests).
	DiskUsageBytes uint64

	// Pebble subsystem snapshots. Zero-values are fine; callers should
	// treat these as best-effort observability. A production ops
	// dashboard typically wants at least BlockCacheSize/Hits/Misses,
	// MemTableSize, CompactionInProgress, and CompactionEstimatedDebt.
	BlockCacheSize          int64
	BlockCacheCount         int64
	BlockCacheHits          int64
	BlockCacheMisses        int64
	MemTableSize            uint64
	MemTableCount           int64
	MemTableZombieSize      uint64
	MemTableZombieCount     int64
	CompactionsInProgress   int64
	CompactionEstimatedDebt uint64
	CompactionCount         int64
	FlushCount              int64
}

// Stats returns a cheap snapshot of the DB's counters plus a point-in-time
// view of the underlying Pebble metrics. Pebble.Metrics is O(numLevels +
// sstables) and is cheap but not free; if you are polling Stats at very
// high frequency, cache the Pebble side.
func (d *DB) Stats() Stats {
	s := Stats{
		Puts:            d.stats.puts.Load(),
		Deletes:         d.stats.deletes.Load(),
		Gets:            d.stats.getCalls.Load(),
		Scans:           d.stats.scans.Load(),
		Queries:         d.stats.queries.Load(),
		PebbleGets:      d.stats.pebbleGets.Load(),
		OpenIterators:   d.stats.iterOpen.Load(),
		LeakedIterators: d.stats.iterLeaked.Load(),
	}
	if p := d.stats.pebbleFatals; p != nil {
		s.PebbleFatals = p.Load()
	}
	// Pebble can race with Close(); guard against touching a closed
	// handle so Stats() is safe to call at any time.
	if !d.acquire() {
		return s
	}
	defer d.release()
	m := d.p.Metrics()
	if m == nil {
		return s
	}
	s.DiskUsageBytes = m.DiskSpaceUsage()
	s.BlockCacheSize = m.BlockCache.Size
	s.BlockCacheCount = m.BlockCache.Count
	s.BlockCacheHits = m.BlockCache.Hits
	s.BlockCacheMisses = m.BlockCache.Misses
	s.MemTableSize = m.MemTable.Size
	s.MemTableCount = m.MemTable.Count
	s.MemTableZombieSize = m.MemTable.ZombieSize
	s.MemTableZombieCount = m.MemTable.ZombieCount
	s.CompactionsInProgress = m.Compact.NumInProgress
	s.CompactionEstimatedDebt = m.Compact.EstimatedDebt
	s.CompactionCount = m.Compact.Count
	s.FlushCount = m.Flush.Count
	return s
}

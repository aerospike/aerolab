// Package db is an embedded, single-node, fast on-disk database tailored to
// the AGI ingest + plugin workload. It is explicitly throwaway: durability is
// traded for speed. A crash discards unsynced data, matching AGI's "re-ingest
// from logs" recovery model.
//
// Data model:
//
//   - A "set" is a named table. Each row has an opaque string primary key and
//     a sparse map of typed columns (int64, float64, string, bytes, bool).
//   - Each set may declare at most one indexed column, and only numeric
//     (int64) columns may be indexed. The index supports fast range scans by
//     timestamp, which is the dominant read pattern in the plugin. Query.Between
//     uses the index path only when (a) the column being Between'd is the set's
//     indexed column and (b) both lo and hi are TypeInt64 values; otherwise it
//     falls back to a full scan.
//
// Reads are served by:
//
//   - Get(set, key) for point lookups.
//   - Scan(set) for full-set iteration (used by the labels catalog refresh).
//   - Query(set).Between(col, lo, hi).Where(filter).Project(cols...) for
//     time-range filtered scans with pushdown predicate evaluation and
//     per-column projection.
//
// Writes are served by Put (full-row replace), Update (read-modify-write)
// and Delete.
// Concurrent writers are expected; the DB is safe for concurrent use. Puts on
// the same primary key are serialized with a per-key striped lock so that
// index maintenance stays consistent. Writes to distinct keys run in parallel.
//
// Storage is backed by Pebble (LSM). Durability is controlled via Options:
// with EnableWAL=false and SyncWrites=false (the defaults in this package), a
// crash will lose any data not yet persisted to an SSTable. Close() performs
// a clean flush so that a normal shutdown is durable.
package db

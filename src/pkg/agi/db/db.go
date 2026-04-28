package db

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble/v2"
	"github.com/cockroachdb/pebble/v2/sstable"
)

// Exported sentinel errors. Callers that want to classify failures with
// errors.Is should check against these. Wrapped contextual errors returned
// from the package use fmt.Errorf("db: ...: %w", ErrXxx) so Is works.
var (
	// ErrClosed is returned when an operation is attempted on a DB that
	// has been Close()d.
	ErrClosed = errors.New("db: closed")
	// ErrSetDropped is returned when an in-flight operation's target
	// set was concurrently dropped.
	ErrSetDropped = errors.New("db: set dropped")
	// ErrSetNotIndexed is returned by operations that require an
	// indexed column on a set that has none.
	ErrSetNotIndexed = errors.New("db: set is not indexed")
	// ErrIteratorsOpen is returned from Close when iterators are
	// still open; close them first, then Close again.
	ErrIteratorsOpen = errors.New("db: cannot close with open iterators")
	// ErrStorageVersionMismatch is returned from Open when the DB on
	// disk was written with a storage format version that this build
	// does not understand. The caller must choose between discarding
	// the data directory or building against a compatible version;
	// this package does not perform in-place upgrades.
	ErrStorageVersionMismatch = errors.New("db: storage version mismatch")
	// ErrColumnTypeConflict is returned by Put / RegisterSet when a
	// column is written with a type that differs from the type already
	// recorded in the schema. Callers that need to do type coercion
	// (e.g. the ingest path, where a pattern may emit the same column
	// as int for one row and string for another) should test for this
	// with errors.Is and retry with a coerced row.
	ErrColumnTypeConflict = errors.New("db: column type conflict")
)

// DefaultPath is the canonical on-disk location for the shared AGI DB.
// Both ingest and plugin default their Config.DB.Path to this value;
// changing the default here must happen in lockstep with the two
// packages' struct tags or they will diverge and miss each other's
// data.
const DefaultPath = "/opt/agi/db"

// currentStorageVersion is the on-disk format version written by this
// package. It is persisted at metaVersionKey on the first Open that
// finds no version key (lazy migration for pre-versioned databases or
// fresh Opens). Bump this when the row wire format, key layout, or
// schema encoding changes in a way that is not backward-compatible.
//
// Version history:
//   - v1: D/setID/pk → encoded row payload; I/setID/colID/biased-ts/pk → empty.
//   - v2: D/setID/pk → 8-byte biased-ts forward pointer (indexed sets
//     only) or encoded row payload (unindexed sets); I/setID/colID/
//     biased-ts/pk → encoded row payload (indexed sets only). The
//     change clusters indexed-set rows by their indexed value so an
//     indexed Between iterates payload bytes in tight LSM order with
//     zero per-row point Gets — see README.md "Storage layout".
//   - v3: PK semantics change in pkg/agi/ingest. Metrics rows are now
//     keyed by hex(XXH3-128(cluster::/::node::/::logLine)) instead of
//     the raw concatenation; the DB layer is itself unchanged. Bumping
//     the version here forces existing AGI volumes to fail open with
//     ErrStorageVersionMismatch, which the agi exec path handles by
//     wiping and re-ingesting from the source-of-truth log files —
//     necessary because pre-v3 unhashed PKs are unreachable from new
//     ingest writes (the fresh hashed PKs never collide with them, so
//     stale rows would otherwise linger in the LSM).
const currentStorageVersion uint32 = 3

// DB is an embedded, single-node, sparse-column store tuned for the AGI
// ingest + plugin workload.
type DB struct {
	p     *pebble.DB
	opts  Options
	wopts *pebble.WriteOptions

	// lifeMu coordinates the DB's lifecycle with concurrent operations.
	// Every public call takes lifeMu.RLock() for the duration of the
	// actual Pebble interaction; Close() takes lifeMu.Lock() so it
	// waits for in-flight operations to drain before closing the
	// underlying Pebble handle. This removes the TOCTOU race where an
	// op could read closed==false and then segfault on a closed handle.
	lifeMu sync.RWMutex
	closed atomic.Bool

	// Per-row striped locks serialize RMW within a single primary key so
	// index maintenance stays consistent. Locks are keyed by (setID, pk).
	rowLocks stripedLocks

	// setsMu guards the set-name / set-id / nextSetID maps only. The
	// per-set fields (Columns, ByID, IndexedCol, NextColID) are guarded
	// by the per-set setSchema.mu so that implicit column registration
	// on one set does not serialize writes to every other set.
	setsMu     sync.RWMutex
	setsByName map[string]*setSchema
	setsByID   map[uint32]*setSchema
	nextSetID  uint32

	stats dbStats

	// assumeNewSeen, when true, makes indexScanIter consult the D/
	// pointer per row to skip orphaned I/ entries left behind by an
	// AssumeNew=true overwrite. It is set ONCE at Open time from
	// Options.IndexCanHaveOrphans and never mutated thereafter.
	//
	// Why we no longer auto-flip on first AssumeNew=true write: the
	// auto-flip was a 4-5× regression on indexed range scans for
	// any DB whose ingest path used AssumeNew (the AGI workload).
	// Reading 2 M rows turned into 2 M random Pebble Gets, undoing
	// the entire Phase-1 layout win. AGI's PKs embed the source
	// byte-offset, so re-puts at the same offset deterministically
	// produce the same timestamp and cannot create orphans —
	// auto-flipping was paranoia we paid for on every query.
	assumeNewSeen atomic.Bool
}

// Open opens (creating if necessary) a DB at opts.Path. If Path is empty the
// call fails.
func Open(opts Options) (*DB, error) {
	if opts.Path == "" {
		return nil, errors.New("db: Open: Path is required")
	}
	opts.applyDefaults()
	if err := os.MkdirAll(opts.Path, 0o755); err != nil {
		return nil, fmt.Errorf("db: Open: mkdir: %w", err)
	}

	pebbleFatals := new(atomic.Uint64)
	pOpts := &pebble.Options{
		DisableWAL:                  !opts.EnableWAL,
		MemTableSize:                opts.MemTableSizeBytes,
		MemTableStopWritesThreshold: opts.MemTableStopWritesThreshold,
		Logger:                      pebbleLoggerAdapter{l: opts.Logger, fatals: pebbleFatals},
	}
	// Plumb the per-DB compaction parallelism cap through Pebble's
	// CompactionConcurrencyRange callback. Pebble v2 deprecated the
	// scalar MaxConcurrentCompactions in favour of a (lower, upper)
	// range; we model the user-facing knob as a single upper bound
	// (lower stays at 1 so a quiet DB does not waste worker
	// goroutines).
	if opts.MaxConcurrentCompactions > 0 {
		upper := opts.MaxConcurrentCompactions
		pOpts.CompactionConcurrencyRange = func() (int, int) { return 1, upper }
	}
	// Cache semantics:
	//   CacheBytes > 0  → allocate a cache of that size.
	//   CacheBytes < 0  → explicit "no cache" sentinel (NoBlockCache);
	//                     we leave pOpts.Cache nil so Pebble runs
	//                     without a block cache.
	//   CacheBytes == 0 → applyDefaults() promoted it to the default,
	//                     so this branch does not see zero any more.
	if opts.CacheBytes > 0 {
		pOpts.Cache = pebble.NewCache(opts.CacheBytes)
	}
	if opts.MaxOpenFiles > 0 {
		pOpts.MaxOpenFiles = opts.MaxOpenFiles
	}
	// Per-level options: BlockSize and Compression. pebble.Options.Levels
	// is a fixed-size array (NumLevels entries), so we set fields in
	// place rather than reslicing. ApplyCompressionSettings iterates
	// the array and installs a per-level Compression closure, which is
	// exactly the per-level layout we want for AGI on EFS: cheap
	// codecs on the upper levels (flushes / minor compactions on the
	// hot path) and Zstd on the bottom levels where the bulk of bytes
	// settle.
	if opts.BlockSize > 0 {
		for i := range pOpts.Levels {
			pOpts.Levels[i].BlockSize = opts.BlockSize
		}
	}
	if cs, ok := pickCompressionSettings(opts.Compression); ok {
		pOpts.ApplyCompressionSettings(func() pebble.DBCompressionSettings { return cs })
	} else if opts.Compression != "" {
		opts.Logger.Printf("WARN: db: Open: unrecognised Compression %q, falling back to Pebble default", opts.Compression)
	}

	p, err := pebble.Open(opts.Path, pOpts)
	if err != nil {
		return nil, fmt.Errorf("db: Open: pebble: %w", err)
	}
	wopts := pebble.NoSync
	if opts.SyncWrites && opts.EnableWAL {
		wopts = pebble.Sync
	}
	d := &DB{
		p:          p,
		opts:       opts,
		wopts:      wopts,
		setsByName: make(map[string]*setSchema),
		setsByID:   make(map[uint32]*setSchema),
	}
	d.stats.pebbleFatals = pebbleFatals
	if opts.IndexCanHaveOrphans {
		// Pin the orphan-guard ON for this DB. We never auto-flip
		// it from PutBatch any more — the auto-flip path was a
		// 4-5× regression on indexed range scans for any DB that
		// touched AssumeNew=true even once. Operators who actually
		// rely on the orphan-skip (i.e. actively overwrite indexed
		// values via AssumeNew) opt in here and pay the per-row
		// Pebble Get knowingly.
		d.assumeNewSeen.Store(true)
	}
	if err := d.ensureStorageVersion(); err != nil {
		_ = p.Close()
		return nil, err
	}
	if err := d.loadSchemas(); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("db: Open: load schemas: %w", err)
	}
	if err := d.purgeLegacySetNameKeys(); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("db: Open: purge legacy setname keys: %w", err)
	}
	return d, nil
}

// ensureStorageVersion validates the on-disk format version, writing
// currentStorageVersion on fresh / pre-versioned databases and refusing
// to open a database whose version does not match this build.
//
// Behavior:
//   - Key absent (fresh DB or legacy pre-versioning deployment): write
//     currentStorageVersion. A legacy DB is thereby adopted as v1.
//   - Key present and == currentStorageVersion: ok.
//   - Key present and != currentStorageVersion: return
//     ErrStorageVersionMismatch without mutating anything. The caller
//     decides whether to wipe and reopen.
func (d *DB) ensureStorageVersion() error {
	v, closer, err := d.p.Get(metaVersionKey())
	switch {
	case err == nil:
		defer closer.Close()
		if len(v) != 4 {
			return fmt.Errorf("%w: malformed version record (len=%d)", ErrStorageVersionMismatch, len(v))
		}
		got := binary.BigEndian.Uint32(v)
		if got != currentStorageVersion {
			return fmt.Errorf("%w: on-disk=%d, build=%d", ErrStorageVersionMismatch, got, currentStorageVersion)
		}
		return nil
	case errors.Is(err, pebble.ErrNotFound):
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, currentStorageVersion)
		if err := d.p.Set(metaVersionKey(), buf, d.wopts); err != nil {
			return fmt.Errorf("db: Open: persist version: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("db: Open: read version: %w", err)
	}
}

// purgeLegacySetNameKeys deletes the M/setname/ namespace left behind by
// older builds. The keys are never read (loadSchemas only walks
// M/schema/), so this is a best-effort cleanup on Open. Called before
// any public API is reachable so no acquire/release is needed.
func (d *DB) purgeLegacySetNameKeys() error {
	lower := metaSetNameLower()
	upper := metaSetNameUpper()
	it, err := d.p.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return err
	}
	hasAny := it.First()
	_ = it.Close()
	if !hasAny {
		return nil
	}
	return d.p.DeleteRange(lower, upper, d.wopts)
}

// acquire pins the DB against concurrent Close for the duration of one
// operation. Returns false if the DB is closed. Callers MUST call
// release() on success.
func (d *DB) acquire() bool {
	d.lifeMu.RLock()
	if d.closed.Load() {
		d.lifeMu.RUnlock()
		return false
	}
	return true
}

// release returns the lifecycle read lock taken by acquire().
func (d *DB) release() {
	d.lifeMu.RUnlock()
}

// Close flushes memtables and closes the DB. It waits for in-flight
// operations to complete before touching the underlying Pebble handle so
// there is no window in which a caller can observe a half-closed DB.
// A non-nil return from Flush signals potential data loss (memtable did
// not fully persist); the underlying Pebble close is always attempted.
func (d *DB) Close() error {
	// Drain in-flight ops. Any op that already passed acquire() runs
	// to completion here; any op that arrives during/after Close sees
	// closed==true and returns ErrClosed.
	d.lifeMu.Lock()
	defer d.lifeMu.Unlock()
	if d.closed.Load() {
		return ErrClosed
	}
	if n := d.stats.iterOpen.Load(); n > 0 {
		return fmt.Errorf("%w (%d open)", ErrIteratorsOpen, n)
	}
	d.closed.Store(true)
	var firstErr error
	flushStart := time.Now()
	if err := d.p.Flush(); err != nil {
		d.opts.Logger.Printf("WARN: flush on close failed after %s: %s", time.Since(flushStart), err)
		firstErr = fmt.Errorf("db: Close: flush: %w", err)
	} else {
		d.opts.Logger.Printf("INFO: pebble flush on close complete in %s", time.Since(flushStart))
	}
	// Log before handing off to Pebble.Close so an operator watching logs
	// can tell whether a hang is inside Flush, Close, or a stuck
	// compaction blocking Close. Without this line the two calls are
	// indistinguishable from the outside.
	d.opts.Logger.Printf("INFO: pebble close starting")
	closeStart := time.Now()
	if err := d.p.Close(); err != nil {
		d.opts.Logger.Printf("WARN: pebble close failed after %s: %s", time.Since(closeStart), err)
		if firstErr == nil {
			firstErr = fmt.Errorf("db: Close: %w", err)
		}
	} else {
		d.opts.Logger.Printf("INFO: pebble close complete in %s", time.Since(closeStart))
	}
	return firstErr
}

// Path returns the on-disk path this DB was opened with.
func (d *DB) Path() string { return d.opts.Path }

// loadSchemas reads the M/schema/ namespace into the in-memory caches.
// Called from Open before the DB is published, so no locks are needed.
func (d *DB) loadSchemas() error {
	if v, closer, err := d.p.Get(metaNextSetIDKeyBytes()); err == nil {
		if len(v) == 4 {
			d.nextSetID = binary.BigEndian.Uint32(v)
		}
		_ = closer.Close()
	} else if !errors.Is(err, pebble.ErrNotFound) {
		return err
	}

	lower := metaSchemaLower()
	upper := metaSchemaUpper()
	it, err := d.p.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return err
	}
	defer it.Close()
	for it.First(); it.Valid(); it.Next() {
		raw := it.Value()
		s, warnings, err := decodeSchema(raw)
		for _, w := range warnings {
			d.opts.Logger.Printf("WARN: %s", w)
		}
		if err != nil {
			// A single corrupt schema record must not wedge Open. Log
			// loudly, skip the entry, and carry on; the set will be
			// invisible (it is not in setsByName / setsByID) so no
			// caller can accidentally target it. An operator can drop
			// the bad key out of band, or the set will be re-created
			// on the next RegisterSet/Put that references its name —
			// which will allocate a fresh setID. Note: we still
			// attempt to extract the setID from the key so nextSetID
			// stays monotonic.
			d.opts.Logger.Printf("WARN: skipping undecodable schema at key %x: %s", it.Key(), err)
			if key := it.Key(); len(key) >= 4 {
				badID := binary.BigEndian.Uint32(key[len(key)-4:])
				if badID+1 > d.nextSetID {
					d.nextSetID = badID + 1
				}
			}
			continue
		}
		d.setsByName[s.Name] = s
		d.setsByID[s.ID] = s
		if s.ID+1 > d.nextSetID {
			d.nextSetID = s.ID + 1
		}
	}
	if err := it.Error(); err != nil {
		return err
	}
	return nil
}

// RegisterSet ensures a set exists with the given column specs. Existing
// columns are validated (type match); new columns are appended. If any spec
// has Indexed=true, the named column becomes the indexed column. It is an
// error to change an existing column's type or to re-index an already
// indexed set on a different column.
//
// RegisterSet is safe to call repeatedly and is how a caller declares a
// schema up front. Writers may also add columns implicitly via Put; see
// the package documentation.
func (d *DB) RegisterSet(name string, cols []ColumnSpec) error {
	if !d.acquire() {
		return ErrClosed
	}
	defer d.release()
	if name == "" {
		return errors.New("db: RegisterSet: name required")
	}
	s, err := d.ensureSet(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dropped.Load() {
		return ErrSetDropped
	}
	cp := s.checkpoint()
	changed := false
	for _, c := range cols {
		if c.Name == "" {
			s.restore(cp)
			return errors.New("db: RegisterSet: empty column name")
		}
		if c.Type == TypeInvalid {
			s.restore(cp)
			return fmt.Errorf("db: RegisterSet: column %q has TypeInvalid", c.Name)
		}
		_, ch, err := s.addColumn(c.Name, c.Type, c.Indexed)
		if err != nil {
			s.restore(cp)
			return err
		}
		if ch {
			changed = true
		}
	}
	if changed {
		if err := d.persistSchema(s); err != nil {
			s.restore(cp)
			return err
		}
	}
	return nil
}

// ensureSet returns the setSchema for name, creating a fresh one and
// persisting the setID/name mapping if it does not yet exist.
//
// The in-memory maps are only mutated after the Pebble batch commits
// successfully. If the commit fails we return the error with the maps
// untouched so a subsequent retry can proceed from a consistent state.
func (d *DB) ensureSet(name string) (*setSchema, error) {
	// Fast path: the set already exists and is not a dropped shell.
	d.setsMu.RLock()
	if s, ok := d.setsByName[name]; ok && !s.dropped.Load() {
		d.setsMu.RUnlock()
		return s, nil
	}
	d.setsMu.RUnlock()

	// Slow path: may need to create.
	d.setsMu.Lock()
	defer d.setsMu.Unlock()
	if s, ok := d.setsByName[name]; ok {
		if !s.dropped.Load() {
			return s, nil
		}
		// A pre-drop schema may still be indexed by name; unmap it so
		// a fresh setID is minted (symmetry with lookupSet / fast path).
		delete(d.setsByName, name)
		delete(d.setsByID, s.ID)
	}
	id := d.nextSetID
	s := &setSchema{
		ID:      id,
		Name:    name,
		Columns: make(map[string]columnInfo),
		ByID:    make(map[uint32]string),
	}

	batch := d.p.NewBatch()
	defer batch.Close()
	nextBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(nextBuf, id+1)
	if err := batch.Set(metaNextSetIDKeyBytes(), nextBuf, nil); err != nil {
		return nil, err
	}
	payload, err := encodeSchema(s)
	if err != nil {
		return nil, err
	}
	if err := batch.Set(metaSchemaKey(id), payload, nil); err != nil {
		return nil, err
	}
	if err := batch.Commit(d.wopts); err != nil {
		return nil, err
	}
	d.setsByName[name] = s
	d.setsByID[id] = s
	d.nextSetID = id + 1
	return s, nil
}

// persistSchema writes s to disk. Caller must hold s.mu for writing.
func (d *DB) persistSchema(s *setSchema) error {
	payload, err := encodeSchema(s)
	if err != nil {
		return err
	}
	return d.p.Set(metaSchemaKey(s.ID), payload, d.wopts)
}

// DropColumn removes a column from the named set's schema. The column
// name and its colID are retired; any on-disk row data carrying that
// colID becomes garbage that the codec's "unknown colID" branch already
// ignores, so there is no need to rewrite every row — the data simply
// stops being surfaced through Get / Scan / Query.
//
// If the column is the set's indexed column, every index entry for it
// is also removed via a ranged delete and the set becomes unindexed.
//
// Behavior / limitations:
//   - Columns can be re-added later with RegisterSet or implicit Put;
//     the new registration will allocate a fresh colID, it does not
//     resurrect the old one (so the previously-stored values stay
//     invisible even if the same name is reintroduced).
//   - Calling DropColumn on a non-existent set or a non-existent
//     column is a no-op (returns nil). Use SchemaOf / SetExists if you
//     need to distinguish these cases.
//   - Concurrent Put/Update/Delete on the same set are serialized by
//     the per-key row locks; DropColumn drains them via lockAll so
//     no writer can observe a half-dropped schema.
//
// This is intended as the escape valve for long-lived sets with
// churning label cardinality (schema entries that would otherwise
// accumulate forever).
func (d *DB) DropColumn(setName, colName string) error {
	if !d.acquire() {
		return ErrClosed
	}
	defer d.release()
	if setName == "" || colName == "" {
		return errors.New("db: DropColumn: set and column required")
	}
	s, ok := d.lookupSet(setName)
	if !ok {
		return nil
	}
	// lockAll before s.mu: matches Put (stripe first) and avoids
	// AB-BA deadlock with concurrent writers. We intentionally do not
	// call lookupSet again here (it would take setsMu while holding
	// lockAll and can deadlock with ensureSet/RegisterSet). Re-checks
	// for dropped and column presence happen under s.mu.
	d.rowLocks.lockAll()
	defer d.rowLocks.unlockAll()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dropped.Load() {
		return ErrSetDropped
	}
	info, present := s.Columns[colName]
	if !present {
		return nil
	}
	cp := s.checkpoint()

	wasIndexed := s.IndexedCol == colName
	delete(s.Columns, colName)
	delete(s.ByID, info.ID)
	if wasIndexed {
		s.IndexedCol = ""
	}

	batch := d.p.NewBatch()
	defer batch.Close()
	if wasIndexed {
		lower := indexPrefix(s.ID, info.ID)
		upper := indexPrefix(s.ID, info.ID+1)
		if info.ID == ^uint32(0) {
			// Stepping colID wraps; use the full-set index upper bound.
			upper = indexSetUpper(s.ID)
		}
		if err := batch.DeleteRange(lower, upper, nil); err != nil {
			s.restore(cp)
			return err
		}
	}
	payload, err := encodeSchema(s)
	if err != nil {
		s.restore(cp)
		return err
	}
	if err := batch.Set(metaSchemaKey(s.ID), payload, nil); err != nil {
		s.restore(cp)
		return err
	}
	if err := batch.Commit(d.wopts); err != nil {
		s.restore(cp)
		return fmt.Errorf("db: DropColumn: commit: %w", err)
	}
	return nil
}

// DropSet removes every data row, every index entry and the schema for the
// named set. The operation is atomic on disk (the Pebble batch either
// fully commits or nothing changes). Concurrent Put/Update/Delete calls
// that are already holding a per-key row lock are drained before the drop
// commits; new operations after the drop see ErrSetDropped (if they already
// cached the old *setSchema pointer) or a transparent "set not found"
// miss on the next lookupSet.
func (d *DB) DropSet(name string) error {
	if !d.acquire() {
		return ErrClosed
	}
	defer d.release()

	d.setsMu.Lock()
	s, ok := d.setsByName[name]
	if !ok {
		d.setsMu.Unlock()
		return nil
	}
	// Remove from the name/id maps so new callers can't see this
	// schema. Still held under setsMu for the rest of the critical
	// section so that ensureSet() for the same name can't race.
	delete(d.setsByName, name)
	delete(d.setsByID, s.ID)
	d.setsMu.Unlock()

	// Flip the dropped flag BEFORE acquiring row locks. Any Put/Update
	// that already holds a stripe lock will complete first; after they
	// release we acquire the stripes, and any writer that acquires a
	// stripe AFTER us sees dropped=true under the stripe.
	s.dropped.Store(true)
	d.rowLocks.lockAll()
	defer d.rowLocks.unlockAll()

	// Hold s.mu for the batch so a concurrent prepareEntriesForSet slow
	// path that is mid-persistSchema cannot write M/schema/<id> after
	// we Delete it. Lock ordering matches DropColumn:
	// stripes (lockAll) → s.mu → Pebble batch.
	s.mu.Lock()
	defer s.mu.Unlock()

	batch := d.p.NewBatch()
	defer batch.Close()
	if err := batch.DeleteRange(dataLowerBound(s.ID), dataUpperBound(s.ID), nil); err != nil {
		return err
	}
	if err := batch.DeleteRange(indexSetLower(s.ID), indexSetUpper(s.ID), nil); err != nil {
		return err
	}
	if err := batch.Delete(metaSchemaKey(s.ID), nil); err != nil {
		return err
	}
	if err := batch.Commit(d.wopts); err != nil {
		// Restore the maps so the caller's retry sees consistent
		// state. Clear dropped before reinsert so the set is observed
		// as live (on-disk state was not dropped). Restore only
		// touches setsByName / setsByID / dropped, so it is safe to
		// run while still holding s.mu.
		d.setsMu.Lock()
		s.dropped.Store(false)
		d.setsByName[name] = s
		d.setsByID[s.ID] = s
		d.setsMu.Unlock()
		return fmt.Errorf("db: DropSet: commit: %w", err)
	}
	return nil
}

// SetExists reports whether a set with the given name is currently
// registered. This is the strict counterpart to the ergonomic "unknown
// set is a miss" behaviour of Get / Scan / Query, and is intended for
// callers that want to distinguish "the set is empty" from "the set is
// not here at all" without probing the data plane.
//
// Sets that have been dropped are reported as non-existent.
func (d *DB) SetExists(name string) bool {
	_, ok := d.lookupSet(name)
	return ok
}

// Sets returns the names of all known sets.
func (d *DB) Sets() []string {
	d.setsMu.RLock()
	defer d.setsMu.RUnlock()
	out := make([]string, 0, len(d.setsByName))
	for name, s := range d.setsByName {
		if s.dropped.Load() {
			continue
		}
		out = append(out, name)
	}
	return out
}

// SchemaOf returns the ColumnSpecs registered for the named set. The order is
// not specified. Returns false if the set does not exist.
func (d *DB) SchemaOf(name string) ([]ColumnSpec, bool) {
	s, ok := d.lookupSet(name)
	if !ok {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ColumnSpec, 0, len(s.Columns))
	for n, info := range s.Columns {
		out = append(out, ColumnSpec{
			Name:    n,
			Type:    info.Type,
			Indexed: s.IndexedCol == n,
		})
	}
	return out, true
}

// lookupSet returns the cached setSchema for name, or (nil, false). Also
// reports false if the set exists in memory but has been flagged as
// dropped by a concurrent DropSet.
func (d *DB) lookupSet(name string) (*setSchema, bool) {
	d.setsMu.RLock()
	defer d.setsMu.RUnlock()
	s, ok := d.setsByName[name]
	if !ok {
		return nil, false
	}
	if s.dropped.Load() {
		return nil, false
	}
	return s, true
}

// Striped row-lock table. 1024 shards keeps same-set same-stripe
// collisions rare even under the expected 128-writer workload (~8 keys
// per stripe on average at that concurrency).

const stripeCount = 1024

type stripedLocks struct {
	m [stripeCount]sync.Mutex
}

// mutexFor returns the mutex protecting (setID, pk). The mapping is a
// cheap inline FNV-1a over the 4-byte setID prefix + pk bytes; this
// avoids the allocation of hash/fnv.New32a() in the hot path.
func (s *stripedLocks) mutexFor(setID uint32, pk string) *sync.Mutex {
	return &s.m[stripeIndex(setID, pk)]
}

// stripeIndex computes the same FNV-1a stripe index that mutexFor uses,
// without needing the *sync.Mutex pointer. PutBatch uses this to sort
// the locks it must acquire in stripe-index order — that gives a global
// total ordering on row locks (any two PutBatches that touch overlapping
// PKs acquire their shared stripes in the same order) and so cannot
// deadlock against each other.
func stripeIndex(setID uint32, pk string) uint32 {
	const (
		fnvOffset uint32 = 2166136261
		fnvPrime  uint32 = 16777619
	)
	h := fnvOffset
	h = (h ^ (setID >> 24 & 0xff)) * fnvPrime
	h = (h ^ (setID >> 16 & 0xff)) * fnvPrime
	h = (h ^ (setID >> 8 & 0xff)) * fnvPrime
	h = (h ^ (setID & 0xff)) * fnvPrime
	for i := 0; i < len(pk); i++ {
		h = (h ^ uint32(pk[i])) * fnvPrime
	}
	return h % stripeCount
}

// lockAll acquires every stripe in order. Used by DropSet to drain any
// in-flight writer on the set being dropped.
func (s *stripedLocks) lockAll() {
	for i := 0; i < stripeCount; i++ {
		s.m[i].Lock()
	}
}

// unlockAll is the mirror of lockAll.
func (s *stripedLocks) unlockAll() {
	for i := 0; i < stripeCount; i++ {
		s.m[i].Unlock()
	}
}

// pickCompressionSettings maps the string knob in Options.Compression
// to one of Pebble's predefined DBCompressionSettings. The second return
// is false when the input is empty (caller should leave Pebble's default
// in place) or not recognised (caller should warn and fall back).
//
// "balanced" / "good" deliberately use Pebble's curated per-level
// profiles instead of a uniform Zstd: data settles at the bottom levels
// where Zstd amortises well, while flushes and minor compactions stay
// on the cheap codec so the foreground write path is not slowed.
func pickCompressionSettings(name string) (pebble.DBCompressionSettings, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		return pebble.DBCompressionSettings{}, false
	case "default", "snappy":
		return pebble.UniformDBCompressionSettings(sstable.SnappyCompression), true
	case "fastest":
		return pebble.DBCompressionFastest, true
	case "balanced":
		return pebble.DBCompressionBalanced, true
	case "good":
		return pebble.DBCompressionGood, true
	case "zstd":
		return pebble.UniformDBCompressionSettings(sstable.ZstdCompression), true
	case "none":
		return pebble.DBCompressionNone, true
	default:
		return pebble.DBCompressionSettings{}, false
	}
}

// pebbleLoggerAdapter bridges the package Logger to Pebble's logger
// interface. Infof is intentionally dropped to keep the caller's logs
// quiet; errors and fatal messages always propagate.
type pebbleLoggerAdapter struct {
	l interface {
		Printf(format string, v ...any)
	}
	fatals *atomic.Uint64
}

func (a pebbleLoggerAdapter) Infof(format string, args ...interface{}) {
}

func (a pebbleLoggerAdapter) Errorf(format string, args ...interface{}) {
	a.l.Printf("pebble ERR: "+format, args...)
}

// Fatalf logs Pebble's "fatal" messages without tearing the process down.
// Pebble calls Fatalf from a few internal invariant-violation paths; the
// default logger's Fatalf calls os.Exit, which is inappropriate for an
// embedded library (it would take the whole agent down). We log loudly
// instead and let the nearest public call surface the real error.
func (a pebbleLoggerAdapter) Fatalf(format string, args ...interface{}) {
	if a.fatals != nil {
		a.fatals.Add(1)
	}
	a.l.Printf("pebble FATAL: "+format, args...)
}

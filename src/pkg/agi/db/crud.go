package db

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Put stores row under (set, key). An existing row at the same key is
// fully replaced. New columns are registered implicitly; their types are
// taken from their Value. Type conflicts (e.g. storing a string into an
// existing int64 column) return an error and the Put is rejected.
//
// Put is a thin wrapper over PutBatch with a single item and
// AssumeNew=false; both APIs share the same row-lock and Pebble-batch
// machinery so single-row writers and bulk ingest behave identically
// under concurrency.
func (d *DB) Put(set, key string, row Row) error {
	if set == "" {
		return errors.New("db: Put: set is required")
	}
	if len(row) == 0 {
		return errors.New("db: Put: empty row")
	}
	return d.PutBatch(set, []PutItem{{Key: key, Row: row}})
}

// PutItem is a single entry in a PutBatch. AssumeNew=true tells the
// write path that the caller has external knowledge that the row does
// not currently exist (the typical ingest-pipeline case: a freshly
// parsed log line whose primary key is built from monotonic ts +
// random suffix). The pre-write Pebble Get that resolves the old
// indexed value is then skipped — this is the main read-path speedup
// for bulk ingest. AssumeNew is silently ignored on unindexed sets
// (no pre-Get there to skip).
//
// AssumeNew is a soft hint; if the caller is wrong, the new I/ entry
// is written but the old I/ entry at the prior indexed value is left
// behind as an orphan. indexScanIter has an orphan-skip guard
// (compares the I/ key's biased ts against the current D/ pointer)
// that drops the stale I/ entry on the next read. The orphan is
// reclaimed on the next overwrite of the same pk with AssumeNew=false.
type PutItem struct {
	Key       string
	Row       Row
	AssumeNew bool
}

// PutBatch atomically stores multiple rows in a single Pebble.Batch
// commit. It resolves the schema once (so a batch that introduces a
// new column persists the schema upgrade just once instead of once
// per row), acquires every row lock the batch needs in stripe-index
// order (so concurrent PutBatch calls cannot deadlock against each
// other), then stages all data + index writes into one batch and
// commits with a single fsync (or memtable append, when WAL is off).
//
// Per-item AssumeNew skips the pre-write Pebble Get on indexed sets;
// see the PutItem doc for the orphan-tolerance contract that makes
// that safe.
//
// Errors:
//   - The whole batch fails atomically (no partial commits) on
//     ErrColumnTypeConflict, ErrSetDropped, encode failure, or the
//     Pebble commit error. Callers that want per-row retry on type
//     conflict (e.g. ingest's coercion fallback) should fall back to
//     single-row Put on ErrColumnTypeConflict.
//   - An empty items slice is a no-op (no error).
//   - Empty Key, empty Row, or zero Value is rejected before any
//     lock or Pebble work.
func (d *DB) PutBatch(set string, items []PutItem) error {
	if !d.acquire() {
		return ErrClosed
	}
	defer d.release()
	if set == "" {
		return errors.New("db: PutBatch: set is required")
	}
	if len(items) == 0 {
		return nil
	}
	for i, it := range items {
		if len(it.Row) == 0 {
			return fmt.Errorf("db: PutBatch[%d]: empty row", i)
		}
	}
	s, err := d.ensureSet(set)
	if err != nil {
		return err
	}
	// Resolve schema once per item. prepareEntriesForSet's fast path
	// runs under s.mu's read lock, so all items whose columns are
	// already known proceed without contention; the slow path
	// (column add / type check) takes s.mu.Lock once per batch where
	// every previously-unknown column is added in a single shared
	// upgrade pass.
	preps := make([]struct {
		entries []codecEntry
		payload []byte
	}, len(items))
	for i := range items {
		entries, perr := d.prepareEntriesForSet(s, items[i].Row)
		if perr != nil {
			return perr
		}
		payload, perr := encodeRow(entries)
		if perr != nil {
			return perr
		}
		preps[i].entries = entries
		preps[i].payload = payload
	}

	// Skip the row-stripe lock pool when every item is AssumeNew.
	//
	// The stripe locks exist to serialise concurrent writers to the
	// same PK so the read-modify-write of (D/ pointer, old I/ entry,
	// new I/ entry) is atomic. The AGI ingest hot path — the only
	// AssumeNew=true caller in this repo — routes rows through the
	// putBatcher with maphash(key) % nShards, so any given PK is
	// only ever handled by one shard goroutine, and the shard
	// processes batches sequentially. Two concurrent PutBatch calls
	// can therefore never see the same PK; the cross-shard stripe
	// lock acquisition was guarding a race that the routing layer
	// already makes structurally impossible.
	//
	// In production runs (mutex.pprof) ≈99% of all mutex contention
	// across the ingest pipeline came from this very lock pool —
	// shards waiting on each other for stripe overlap during their
	// batch.Commit windows. The skip turns that contention into
	// zero for the ingest hot path while leaving the locked
	// behaviour exactly as before for non-AssumeNew callers
	// (single-row Put, label-set writes, mixed batches).
	//
	// CONTRACT: the safety of skipping the stripe locks rests on
	// "no other writer to the same set is concurrent with an
	// AssumeNew=true PutBatch". This was previously also enforced
	// by the legacy DeleteBelow age-out primitive (removed because
	// AGI did not use it). Any future caller that re-introduces
	// concurrent same-set deletion or non-AssumeNew writers MUST
	// coordinate at a higher layer (e.g. quiesce ingest before
	// running the sweep) — the db layer no longer arbitrates that
	// race for AssumeNew batches.
	allAssumeNew := true
	for i := range items {
		if !items[i].AssumeNew {
			allAssumeNew = false
			break
		}
	}
	if !allAssumeNew {
		// Acquire row locks in stripe-index order. Dedup hash-
		// collided PKs in this batch so we don't double-lock the
		// same mutex (sync.Mutex is non-reentrant). Cardinality
		// is bounded by min(len(items), stripeCount) so a small
		// slice is fine.
		type held struct {
			idx uint32
			mu  *sync.Mutex
		}
		locks := make([]held, 0, len(items))
		seen := make(map[uint32]struct{}, len(items))
		for i := range items {
			idx := stripeIndex(s.ID, items[i].Key)
			if _, ok := seen[idx]; ok {
				continue
			}
			seen[idx] = struct{}{}
			locks = append(locks, held{idx: idx, mu: &d.rowLocks.m[idx]})
		}
		sort.Slice(locks, func(i, j int) bool { return locks[i].idx < locks[j].idx })
		for i := range locks {
			locks[i].mu.Lock()
		}
		defer func() {
			for i := range locks {
				locks[i].mu.Unlock()
			}
		}()
	}
	// dropped.Load is atomic and safe to read without holding any
	// stripe lock; the AssumeNew skip does not change its
	// correctness.
	if s.dropped.Load() {
		return ErrSetDropped
	}

	colID, hasIndex := d.indexedColumnSnapshot(s)

	batch := d.p.NewBatch()
	defer batch.Close()

	// AssumeNew=true skips the pre-Get of the old D/ pointer. This
	// is correct only when the caller can prove that either no row
	// exists at this PK, or the row that exists has the same
	// indexed value as the new row (so no orphan I/ entry can be
	// created). AGI's ingest satisfies the contract because PKs are
	// XXH3-128 hashes of (cluster, node, log line) and the parsed
	// timestamp embedded in a given log line is deterministic.
	// Callers that violate the contract leak orphaned I/ entries
	// that will surface as duplicate rows in indexed scans; such
	// callers must Open() with IndexCanHaveOrphans=true and accept
	// the per-row Pebble Get penalty in indexScanIter.

	for i := range items {
		item := &items[i]
		dkey := dataKey(s.ID, item.Key)
		var (
			oldVal    int64
			hasOldVal bool
		)
		if hasIndex && !item.AssumeNew {
			ov, _, hov, _, rerr := d.readOldIndexedValue(dkey, hasIndex, colID)
			if rerr != nil && !errors.Is(rerr, errCorruptIndexedValue) {
				return rerr
			}
			if errors.Is(rerr, errCorruptIndexedValue) {
				// Treat a corrupt D/ pointer as "no old index
				// entry"; an orphaned I/ from the corrupt prior
				// state is harmless because indexScanIter's
				// pointer re-check skips orphans.
				hov = false
			}
			oldVal, hasOldVal = ov, hov
		}

		if hasIndex {
			newVal, newHasIndexed := int64(0), false
			for _, e := range preps[i].entries {
				if e.ColID == colID {
					newVal = e.Val.i
					newHasIndexed = true
					break
				}
			}
			if !newHasIndexed {
				return fmt.Errorf("db: PutBatch on indexed set %q is missing indexed column %q at item %d", s.Name, s.IndexedCol, i)
			}
			if hasOldVal && oldVal != newVal {
				if err := stageIndexWrite(batch, s.ID, colID, item.Key, true, oldVal, true, newVal, preps[i].payload); err != nil {
					return err
				}
			} else {
				if err := stageIndexWrite(batch, s.ID, colID, item.Key, false, 0, true, newVal, preps[i].payload); err != nil {
					return err
				}
			}
			if err := batch.Set(dkey, encodePointer(biasInt64(newVal)), nil); err != nil {
				return err
			}
		} else {
			if err := batch.Set(dkey, preps[i].payload, nil); err != nil {
				return err
			}
		}
	}

	if err := batch.Commit(d.wopts); err != nil {
		return err
	}
	d.stats.puts.Add(uint64(len(items)))
	return nil
}

// Get returns the row at (set, key). If project is non-empty, only the named
// columns are decoded; missing columns are simply absent in the returned
// row.
//
// Returns nil if the row does not exist. Returns an empty (non-nil) Row
// when the row exists but the projection names no columns present in the
// schema; this lets callers distinguish "absent" from "no projected cols".
func (d *DB) Get(set, key string, project ...string) (Row, error) {
	if !d.acquire() {
		return nil, ErrClosed
	}
	defer d.release()
	s, ok := d.lookupSet(set)
	if !ok {
		return nil, nil
	}
	return d.getFromSet(s, key, project)
}

// getFromSet implements the inner Get path with a resolved schema.
//
// Storage layout (v2):
//   - Unindexed sets: D/ holds the encoded row directly (one Get).
//   - Indexed sets: D/ holds an 8-byte biased-ts pointer; we then
//     do a second Get on the I/ key (one extra point Get, but the
//     indexed-scan hot path uses a single iterator with no extra
//     Get per row — this 2-Get path only fires on the rare random
//     point-Get).
func (d *DB) getFromSet(s *setSchema, key string, project []string) (Row, error) {
	d.stats.pebbleGets.Add(1)
	raw, closer, err := d.p.Get(dataKey(s.ID, key))
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	buf := make([]byte, len(raw))
	copy(buf, raw)
	_ = closer.Close()

	colID, hasIndex := d.indexedColumnSnapshot(s)
	if hasIndex {
		biased, ok := decodePointer(buf)
		if !ok {
			return nil, fmt.Errorf("db: Get %q/%q: %w", s.Name, key, errCorruptIndexedValue)
		}
		ikey := indexKeyFromBiased(s.ID, colID, biased, key)
		d.stats.pebbleGets.Add(1)
		raw, closer, err = d.p.Get(ikey)
		if err != nil {
			if isNotFound(err) {
				// Pointer says there should be a row at this I/
				// key but it's gone. Treat as "not found": this
				// can happen if a stale snapshot still has the
				// D/ pointer for an orphaned I/ entry that an
				// overwrite already replaced. The pointer is
				// harmless orphan state until the next overwrite.
				return nil, nil
			}
			return nil, err
		}
		buf = make([]byte, len(raw))
		copy(buf, raw)
		_ = closer.Close()
	}

	var wantIDs map[uint32]struct{}
	if len(project) > 0 {
		wantIDs = make(map[uint32]struct{}, len(project))
		s.mu.RLock()
		for _, name := range project {
			if info, ok := s.Columns[name]; ok {
				wantIDs[info.ID] = struct{}{}
			}
		}
		s.mu.RUnlock()
		// Row exists but none of the projected names are in the
		// schema. Return a non-nil empty Row so callers can still
		// distinguish "row absent" (nil) from "row present, no
		// projected columns in schema" (empty Row).
		if len(wantIDs) == 0 {
			return Row{}, nil
		}
	}

	decoded := make(map[uint32]Value)
	if err := decodeWanted(buf, wantIDs, decoded); err != nil {
		return nil, err
	}
	s.mu.RLock()
	out := rowFromColIDs(s, decoded)
	s.mu.RUnlock()
	d.stats.getCalls.Add(1)
	return out, nil
}

// Delete removes the row at (set, key). If the row did not exist the call is
// a no-op. Returns true if a row was actually removed.
func (d *DB) Delete(set, key string) (bool, error) {
	if !d.acquire() {
		return false, ErrClosed
	}
	defer d.release()
	s, ok := d.lookupSet(set)
	if !ok {
		return false, nil
	}
	mu := d.rowLocks.mutexFor(s.ID, key)
	mu.Lock()
	defer mu.Unlock()
	if s.dropped.Load() {
		return false, ErrSetDropped
	}

	dkey := dataKey(s.ID, key)
	colID, hasIndex := d.indexedColumnSnapshot(s)
	oldVal, rowExists, hasOldVal, _, err := d.readOldIndexedValue(dkey, hasIndex, colID)
	if err != nil && !errors.Is(err, errCorruptIndexedValue) {
		return false, err
	}
	if !rowExists {
		return false, nil
	}
	if errors.Is(err, errCorruptIndexedValue) {
		// Data row exists but its indexed column is corrupt. We
		// cannot stage the matching index delete, but we can still
		// clear the data row; any orphaned index entry becomes a
		// stale entry that the indexed-scan path already skips.
		d.opts.Logger.Printf("WARN: delete on corrupt indexed row %q/%q: %s", s.Name, key, err)
	}
	batch := d.p.NewBatch()
	defer batch.Close()
	if hasIndex && hasOldVal {
		if err := stageIndexWrite(batch, s.ID, colID, key, true, oldVal, false, 0, nil); err != nil {
			return false, err
		}
	}
	if err := batch.Delete(dkey, nil); err != nil {
		return false, err
	}
	if err := batch.Commit(d.wopts); err != nil {
		return false, err
	}
	d.stats.deletes.Add(1)
	return true, nil
}

// errUpdateEmptyRow is the canonical error returned from Update when fn
// yields a non-nil but empty Row. Centralising the message keeps both
// call sites symmetric: whether the set is missing or present, a
// non-nil empty Row is rejected with the same string.
var errUpdateEmptyRow = errors.New("db: Update: fn returned empty row (use nil to delete)")

// validateUpdateProposal enforces the shared rules between fn call #1
// (set-missing bootstrap) and fn call #2 (under the row lock). Returns
// (shouldCommit, shouldDelete, error).
//
//	commit=false         -> Update is a no-op (nil, nil)
//	next=nil, commit=true -> delete path (nil, true) returned
//	next=empty Row       -> errUpdateEmptyRow
//	next=populated Row   -> write path (true, false)
func validateUpdateProposal(next Row, commit bool) (shouldCommit, shouldDelete bool, err error) {
	if !commit {
		return false, false, nil
	}
	if next == nil {
		return true, true, nil
	}
	if len(next) == 0 {
		return false, false, errUpdateEmptyRow
	}
	return true, false, nil
}

// Update atomically replaces the row at (set, key). fn is called with
// the old row (nil if absent) and must return the new row and a commit
// flag.
//
// Semantics:
//   - commit=false         -> Update is a no-op.
//   - next=nil, commit=true -> the row is deleted (if it exists).
//   - next=empty Row       -> error (use nil to delete).
//   - next=populated Row   -> the row is replaced.
//
// fn invocation contract:
//
//	fn MAY be invoked more than once per Update call. In particular,
//	when the target set does not yet exist, fn is first invoked with
//	nil to decide whether the set should be created at all; if the
//	caller commits a non-nil row, the set is created and fn is then
//	re-invoked under the row lock with the actual old row observed at
//	that moment. fn MUST therefore be idempotent and free of side
//	effects outside the Row it returns — do not use fn to increment
//	external counters, send messages, or mutate shared state.
func (d *DB) Update(set, key string, fn func(old Row) (Row, bool)) error {
	if !d.acquire() {
		return ErrClosed
	}
	defer d.release()
	if set == "" {
		return errors.New("db: Update: set is required")
	}
	if fn == nil {
		return errors.New("db: Update: fn is nil")
	}
	s, ok := d.lookupSet(set)
	if !ok {
		// No ensureSet here: a missing set must not be created unless the
		// user commits a non-nil row. Run fn once with nil; only then may
		// we create the set. If another writer populated the key before we
		// take the row lock, re-run fn with the real old row (same as
		// the existing-set path) so concurrent bootstraps stay linear.
		next, commit := fn(nil)
		shouldCommit, shouldDelete, err := validateUpdateProposal(next, commit)
		if err != nil {
			return err
		}
		if !shouldCommit {
			return nil
		}
		if shouldDelete {
			// The set does not exist and the caller asked to delete;
			// there is nothing to do.
			return nil
		}
		s, err = d.ensureSet(set)
		if err != nil {
			return err
		}
		mu := d.rowLocks.mutexFor(s.ID, key)
		mu.Lock()
		defer mu.Unlock()
		if s.dropped.Load() {
			return ErrSetDropped
		}
		old, oldIndexedVal, oldHasIndexed, oldExists, err := d.getLockedWithIndex(s, key)
		if err != nil {
			return err
		}
		if oldExists {
			next, commit = fn(old)
			shouldCommit, shouldDelete, err = validateUpdateProposal(next, commit)
			if err != nil {
				return err
			}
			if !shouldCommit {
				return nil
			}
			if shouldDelete {
				return d.deleteLocked(s, key, oldExists, true, oldHasIndexed, oldIndexedVal)
			}
		}
		entries, err := d.prepareEntriesForSet(s, next)
		if err != nil {
			return err
		}
		return d.applyPutLockedWithOld(s, key, entries, true, oldHasIndexed, oldIndexedVal)
	}

	mu := d.rowLocks.mutexFor(s.ID, key)
	mu.Lock()
	defer mu.Unlock()
	if s.dropped.Load() {
		return ErrSetDropped
	}

	old, oldIndexedVal, oldHasIndexed, oldExists, err := d.getLockedWithIndex(s, key)
	if err != nil {
		return err
	}
	next, commit := fn(old)
	shouldCommit, shouldDelete, err := validateUpdateProposal(next, commit)
	if err != nil {
		return err
	}
	if !shouldCommit {
		return nil
	}
	if shouldDelete {
		// deleteLocked knows the old indexed state from the RMW read,
		// so it will not re-Get.
		return d.deleteLocked(s, key, oldExists, true, oldHasIndexed, oldIndexedVal)
	}
	// Use the existing s pointer; do NOT call ensureSet again, otherwise
	// a concurrent DropSet + re-create would retarget the RMW at a
	// different setID.
	entries, err := d.prepareEntriesForSet(s, next)
	if err != nil {
		return err
	}
	_ = oldExists // derived state; we only need oldHasIndexed/oldIndexedVal for the index edit
	return d.applyPutLockedWithOld(s, key, entries, true, oldHasIndexed, oldIndexedVal)
}

// --- helpers below ---

// prepareEntriesForSet resolves a Row into codec entries against an
// already-ensured set. The fast path runs entirely under s.mu's read
// lock when every column already exists with a matching type; the
// slow path takes s.mu.Lock to add unknown columns and persist the
// schema upgrade. Used by both PutBatch (which calls ensureSet itself
// once for the whole batch) and Update (which must not re-ensure the
// set mid-RMW).
func (d *DB) prepareEntriesForSet(s *setSchema, row Row) ([]codecEntry, error) {
	// Fast path under read lock: build entries on the fly while
	// validating that every column is already known with a matching
	// type. The pre-merge form walked the row twice — once to
	// validate ("allKnown"), then again to materialise entries —
	// which on AGI's ingest profile cost 26.24s + 24.55s of map
	// lookups against s.Columns for every fast-path row (the
	// duplicated `s.Columns[name]` calls). Folding the two passes
	// halves the lookups and removes a row-sized iteration.
	//
	// On a column miss / type mismatch we discard the partial
	// entries slice, drop the read lock, and fall through to the
	// slow path which re-walks the row under s.mu.Lock. The slow
	// path's correctness is independent of any work the fast pass
	// did, so the discarded slice is harmless.
	s.mu.RLock()
	entries := make([]codecEntry, 0, len(row))
	allKnown := true
	for name, v := range row {
		if v.IsZero() {
			s.mu.RUnlock()
			return nil, fmt.Errorf("db: column %q has zero Value", name)
		}
		info, present := s.Columns[name]
		if !present || info.Type != v.t {
			allKnown = false
			break
		}
		entries = append(entries, codecEntry{ColID: info.ID, Typ: info.Type, Val: v})
	}
	if allKnown {
		s.mu.RUnlock()
		return entries, nil
	}
	s.mu.RUnlock()
	entries = nil

	// Slow path: add unknown columns / detect type conflicts under
	// s.mu exclusively. This only serializes writers to THIS set; other
	// sets' writers run unaffected.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dropped.Load() {
		return nil, ErrSetDropped
	}
	cp := s.checkpoint()
	changed := false
	entries = make([]codecEntry, 0, len(row))
	for name, v := range row {
		if v.IsZero() {
			s.restore(cp)
			return nil, fmt.Errorf("db: column %q has zero Value", name)
		}
		colID, ch, err := s.addColumn(name, v.t, false)
		if err != nil {
			s.restore(cp)
			return nil, err
		}
		if ch {
			changed = true
		}
		entries = append(entries, codecEntry{ColID: colID, Typ: v.t, Val: v})
	}
	if changed {
		if err := d.persistSchema(s); err != nil {
			s.restore(cp)
			return nil, err
		}
	}
	return entries, nil
}

// applyPutLockedWithOld performs the data+index batched write. The caller
// MUST hold the row lock for (s.ID, key) and MUST have verified
// s.dropped == false under that lock. If oldKnown is true, the caller
// has already resolved the old indexed state (typically from an RMW
// read in Update) and the pre-write Pebble Get is skipped; oldHasIndexed
// and oldIndexedVal are then used directly.
//
// Storage layout (v2):
//   - Indexed sets: the encoded row payload is stored at the I/ key
//     (covering index, clustered by indexed value); D/ stores the
//     8-byte forward pointer biased(newVal). Updating the row at a
//     new indexed value writes the new I/ entry, deletes the old
//     I/ entry, and overwrites D/ with the new pointer in one
//     batch.
//   - Indexed sets, but the new row has no indexed column: cannot
//     happen via this path because RegisterSet rejects a row missing
//     the indexed column; defensive code below returns an error in
//     that case rather than orphaning data.
//   - Unindexed sets: the encoded row payload is stored at D/; no
//     I/ entries are ever written.
func (d *DB) applyPutLockedWithOld(s *setSchema, key string, entries []codecEntry, oldKnown, oldHasIndexed bool, oldIndexedVal int64) error {
	payload, err := encodeRow(entries)
	if err != nil {
		return err
	}
	dkey := dataKey(s.ID, key)

	colID, hasIndex := d.indexedColumnSnapshot(s)

	oldVal := oldIndexedVal
	hasOldVal := oldHasIndexed
	if hasIndex && !oldKnown {
		oldVal, _, hasOldVal, _, err = d.readOldIndexedValue(dkey, hasIndex, colID)
		if err != nil && !errors.Is(err, errCorruptIndexedValue) {
			return err
		}
		if errors.Is(err, errCorruptIndexedValue) {
			// Treat a corrupt D/ pointer as "no old index entry".
			// We will still overwrite D/ with the fresh pointer
			// below; an orphaned I/ entry from the corrupt prior
			// state is harmless because indexScanIter's pointer
			// re-check skips orphans.
			hasOldVal = false
		}
	}

	newVal, newHasIndexed := int64(0), false
	if hasIndex {
		for _, e := range entries {
			if e.ColID == colID {
				newVal = e.Val.i
				newHasIndexed = true
				break
			}
		}
		if !newHasIndexed {
			return fmt.Errorf("db: Put on indexed set %q is missing indexed column %q", s.Name, s.IndexedCol)
		}
	}

	batch := d.p.NewBatch()
	defer batch.Close()
	if hasIndex {
		// Always write the new covering I/ entry. If the indexed
		// value did not change we still rewrite the same key so
		// the payload reflects any non-indexed column updates.
		if hasOldVal && oldVal != newVal {
			if err := stageIndexWrite(batch, s.ID, colID, key, true, oldVal, true, newVal, payload); err != nil {
				return err
			}
		} else {
			if err := stageIndexWrite(batch, s.ID, colID, key, false, 0, true, newVal, payload); err != nil {
				return err
			}
		}
		// D/ is the 8-byte forward pointer.
		if err := batch.Set(dkey, encodePointer(biasInt64(newVal)), nil); err != nil {
			return err
		}
	} else {
		if err := batch.Set(dkey, payload, nil); err != nil {
			return err
		}
	}
	if err := batch.Commit(d.wopts); err != nil {
		return err
	}
	d.stats.puts.Add(1)
	return nil
}

// getLockedWithIndex is a specialised Get that also extracts the indexed
// column's prior value (for Update's RMW + delete path). Avoids a third
// Pebble Get when the Update fn elects to delete the row.
//
// On v2 indexed sets, D/ is an 8-byte pointer; the row payload lives at
// the I/ key. We read both (one for the pointer / existence check, one
// for the row) under the row lock so the value we hand to fn is
// consistent with the index entry that the subsequent write will
// delete. On unindexed sets D/ is the row payload, same as before.
func (d *DB) getLockedWithIndex(s *setSchema, key string) (row Row, oldIndexedVal int64, oldHasIndexed bool, oldExists bool, err error) {
	d.stats.pebbleGets.Add(1)
	raw, closer, gerr := d.p.Get(dataKey(s.ID, key))
	if gerr != nil {
		if isNotFound(gerr) {
			return nil, 0, false, false, nil
		}
		return nil, 0, false, false, gerr
	}
	dbuf := make([]byte, len(raw))
	copy(dbuf, raw)
	_ = closer.Close()

	s.mu.RLock()
	colID, hasIndex := s.indexedColumn()
	s.mu.RUnlock()

	if !hasIndex {
		decoded, derr := decodeRowAll(dbuf)
		if derr != nil {
			return nil, 0, false, false, derr
		}
		s.mu.RLock()
		row = rowFromColIDs(s, decoded)
		s.mu.RUnlock()
		return row, 0, false, true, nil
	}

	biased, okPtr := decodePointer(dbuf)
	if !okPtr {
		return nil, 0, false, true, errCorruptIndexedValue
	}
	oldIndexedVal = unbiasUint64(biased)
	oldHasIndexed = true

	ikey := indexKeyFromBiased(s.ID, colID, biased, key)
	d.stats.pebbleGets.Add(1)
	raw, closer, gerr = d.p.Get(ikey)
	if gerr != nil {
		if isNotFound(gerr) {
			// D/ pointer exists but I/ row is gone — treat as
			// "row absent" for the RMW path; the caller's fn
			// will see nil and either no-op or write a fresh
			// row that overwrites the orphan pointer cleanly.
			return nil, 0, false, false, nil
		}
		return nil, 0, false, false, gerr
	}
	ibuf := make([]byte, len(raw))
	copy(ibuf, raw)
	_ = closer.Close()

	decoded, derr := decodeRowAll(ibuf)
	if derr != nil {
		return nil, 0, false, false, derr
	}
	s.mu.RLock()
	row = rowFromColIDs(s, decoded)
	s.mu.RUnlock()
	return row, oldIndexedVal, oldHasIndexed, true, nil
}

// deleteLocked assumes the row lock is held AND s.dropped has been checked.
// If oldIndexedKnown is true, the caller has already resolved the old
// indexed value (typically from the RMW read in Update) so we can skip
// the point Get.
func (d *DB) deleteLocked(s *setSchema, key string, oldExists, oldIndexedKnown, oldHasIndexed bool, oldIndexedVal int64) error {
	dkey := dataKey(s.ID, key)
	colID, hasIndex := d.indexedColumnSnapshot(s)

	var (
		oldVal    = oldIndexedVal
		hasOldVal = oldHasIndexed
		rowExists = oldExists
		err       error
	)
	if !oldIndexedKnown {
		oldVal, rowExists, hasOldVal, _, err = d.readOldIndexedValue(dkey, hasIndex, colID)
		if err != nil && !errors.Is(err, errCorruptIndexedValue) {
			return err
		}
		if errors.Is(err, errCorruptIndexedValue) {
			d.opts.Logger.Printf("WARN: delete on corrupt indexed row %q/%q: %s", s.Name, key, err)
		}
	}
	if !rowExists {
		return nil
	}
	batch := d.p.NewBatch()
	defer batch.Close()
	if hasIndex && hasOldVal {
		if err := stageIndexWrite(batch, s.ID, colID, key, true, oldVal, false, 0, nil); err != nil {
			return err
		}
	}
	if err := batch.Delete(dkey, nil); err != nil {
		return err
	}
	if err := batch.Commit(d.wopts); err != nil {
		return err
	}
	d.stats.deletes.Add(1)
	return nil
}

// rowFromColIDs translates a decoded colID->Value map into the public name
// keyed Row. Columns whose ID has been removed from the schema are skipped;
// columns retired via DropColumn leave orphaned colIDs in on-disk rows; this
// branch quietly skips them, which is exactly what the drop semantics promise.
// Caller must hold s.mu read lock.
func rowFromColIDs(s *setSchema, decoded map[uint32]Value) Row {
	out := make(Row, len(decoded))
	for id, v := range decoded {
		name, ok := s.ByID[id]
		if !ok {
			continue
		}
		out[name] = v
	}
	return out
}

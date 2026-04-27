package db

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/cockroachdb/pebble/v2"
)

// indexEntry is one (ts, pk) staged for deletion by the age-out path.
// On v2 the index iterator already yields ts directly (it's part of
// the I/ key) so we don't need a second Get to find it; capturing
// (ts, pk) lets us issue point Deletes on both I/ and D/ from the
// per-chunk batch without re-reading any payload.
type indexEntry struct {
	ts int64
	pk string
}

// Number of row deletes committed together in a single Pebble batch when
// using DeleteBelow (see that method’s docstring).
const deleteBelowBatchSize = 1024

// DeleteBelow deletes every row in set whose indexed column is strictly
// less than hi. It is intended as the age-out / retention primitive for
// time-series sets (where the indexed column is typically a timestamp).
//
// The set MUST have an indexed column (TypeInt64). On an unindexed set
// DeleteBelow returns ErrSetNotIndexed without touching any data.
//
// Implementation notes:
//   - The primary index is walked under a Pebble snapshot so the set of
//     candidate PKs is stable, even while writers are adding new rows.
//   - Unique PKs are discovered in on-disk order and processed in chunks of
//     up to deleteBelowBatchSize (1024); each chunk is applied as one
//     Pebble batch (Commit per chunk) to cap memtable and batch size.
//   - All distinct row-lock stripes a chunk needs are acquired up-front
//     in stripe-index order (matching PutBatch's ordering) so the two
//     paths cannot AB-BA deadlock against each other. The row is then
//     re-checked under those locks, deletes are staged, and the stripes
//     are released only after batch.Commit returns (or after an error
//     path abandons the batch). This closes a lost-write race where a
//     concurrent Put committed between the stripe-release and the
//     batch-commit could be wiped out by the batch.
//   - Stripe count is bounded by the number of distinct stripes in a
//     chunk (≤ stripeCount); since chunks are 1024 PKs we may briefly
//     hold every stripe, serializing all writers against the running
//     age-out chunk. This is intentional: the write path already
//     tolerates full drain via DropColumn/DropSet.
//   - This matches standard delete semantics: concurrent Put/Update on
//     the same PK are serialized with the age-out.
//
// Returns the number of rows actually removed. If ctx is cancelled
// mid-sweep, DeleteBelow stops and returns the partial count along with
// ctx.Err(); rows already committed stay deleted.
func (d *DB) DeleteBelow(ctx context.Context, set string, hi int64) (int, error) {
	if !d.acquire() {
		return 0, ErrClosed
	}
	defer d.release()

	s, ok := d.lookupSet(set)
	if !ok {
		return 0, nil
	}

	s.mu.RLock()
	idxName := s.IndexedCol
	var colID uint32
	var haveCol bool
	if idxName != "" {
		if info, present := s.Columns[idxName]; present {
			colID = info.ID
			haveCol = true
		}
	}
	s.mu.RUnlock()

	if idxName == "" || !haveCol {
		return 0, fmt.Errorf("%w: %s", ErrSetNotIndexed, set)
	}

	return d.deleteBelowChunked(ctx, s, colID, hi)
}

// deleteBelowChunked walks the index under a snapshot, groups PKs into
// chunks of at most deleteBelowBatchSize, and commits one Pebble batch
// per chunk.
//
// Idempotency: we deliberately do NOT carry a sweep-wide "seen" set
// here. Per the index-maintenance contract (stageIndexWrite atomically
// deletes the old index entry and inserts the new one), a given PK has
// at most one index entry at any snapshot, so the iterator should not
// yield the same pk twice under this snapshot. Even if it did (e.g. a
// synthetic duplicate from a prior corruption), `deleteBelowCommitChunk`
// re-checks under the row lock: a duplicate that was already deleted in
// an earlier chunk hits `!rowExists` and is a cheap no-op, and a
// duplicate with a now-newer indexed value hits `oldVal >= hi`. So a
// sweep-wide dedupe map is unnecessary and only cost memory proportional
// to total rows deleted.
func (d *DB) deleteBelowChunked(ctx context.Context, s *setSchema, colID uint32, hi int64) (int, error) {
	// indexRangeBounds is inclusive on both ends; we want strictly less
	// than hi. Use hi-1 as the inclusive upper, unless hi is MinInt64 in
	// which case there is nothing to delete.
	if hi == minInt64 {
		return 0, nil
	}
	lower, upper := indexRangeBounds(s.ID, colID, minInt64, hi-1)

	snap := d.p.NewSnapshot()
	defer snap.Close()
	it, err := snap.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return 0, err
	}
	defer it.Close()

	chunk := make([]indexEntry, 0, deleteBelowBatchSize)
	deleted := 0

	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		n, err := d.deleteBelowCommitChunk(ctx, s, colID, hi, chunk)
		deleted += n
		return err
	}

	for it.First(); it.Valid(); it.Next() {
		if err := ctxErr(ctx); err != nil {
			return deleted, err
		}
		_, _, val, pk, ok := parseIndexKey(it.Key())
		if !ok {
			_ = flush()
			return deleted, errors.New("db: DeleteBelow: unexpected key in index iterator")
		}
		chunk = append(chunk, indexEntry{ts: val, pk: pk})
		if len(chunk) < deleteBelowBatchSize {
			continue
		}
		if err := flush(); err != nil {
			return deleted, err
		}
		chunk = chunk[:0]
	}
	if err := it.Error(); err != nil {
		return deleted, err
	}
	if err := flush(); err != nil {
		return deleted, err
	}
	return deleted, nil
}

// deleteBelowCommitChunk deletes all rows in chunk that still qualify
// under the row lock, using a single Pebble batch. Acquired stripes are
// retained until after batch.Commit so a concurrent Put landing between
// stage and commit cannot be silently overwritten by this batch. chunk
// may be retained by the caller for reuse; this function does not read
// chunk after return.
//
// On v2 the iterator already gave us (ts, pk) for each candidate index
// entry, so we no longer need a per-row readOldIndexedValue point Get
// to figure out which I/ key to delete. Instead we compare the D/
// pointer's biased-ts to the iterator's ts under the row lock — if a
// concurrent writer has bumped the row to a newer timestamp ≥ hi, the
// pointer mismatches and we skip it (the old I/ entry it referenced
// was deleted by that writer's batch atomically with the new I/
// insert). This collapses the per-row 1 Get + 2 Deletes to a single
// 8-byte Get + 2 Deletes, and keeps DeleteBelow safe against
// concurrent writers.
func (d *DB) deleteBelowCommitChunk(ctx context.Context, s *setSchema, colID uint32, hi int64, chunk []indexEntry) (int, error) {
	if len(chunk) == 0 {
		return 0, nil
	}

	batch := d.p.NewBatch()
	staged := 0

	// Acquire all distinct row-lock stripes this chunk touches in
	// stripe-index order. PutBatch acquires its locks in stripe-index
	// order too, so this guarantees the two paths cannot AB-BA
	// deadlock against each other when their PK sets overlap on
	// shared stripes (the rowLocks pool is shared across all sets).
	// Cardinality is ≤ min(len(chunk), stripeCount).
	type stripe struct {
		idx uint32
		mu  *sync.Mutex
	}
	seen := make(map[uint32]struct{}, len(chunk))
	stripes := make([]stripe, 0, len(chunk))
	for _, ent := range chunk {
		idx := stripeIndex(s.ID, ent.pk)
		if _, ok := seen[idx]; ok {
			continue
		}
		seen[idx] = struct{}{}
		stripes = append(stripes, stripe{idx: idx, mu: &d.rowLocks.m[idx]})
	}
	sort.Slice(stripes, func(i, j int) bool { return stripes[i].idx < stripes[j].idx })
	held := make(map[*sync.Mutex]struct{}, len(stripes))
	for _, st := range stripes {
		st.mu.Lock()
		held[st.mu] = struct{}{}
	}
	unlockAll := func() {
		for mu := range held {
			mu.Unlock()
		}
	}

	for _, ent := range chunk {
		if err := ctxErr(ctx); err != nil {
			batch.Close()
			unlockAll()
			return 0, err
		}
		if s.dropped.Load() {
			batch.Close()
			unlockAll()
			return 0, ErrSetDropped
		}
		dkey := dataKey(s.ID, ent.pk)
		// Read the D/ pointer to decide whether the index entry we
		// observed in the snapshot is still the live one. A
		// concurrent Put on this pk overwrites D/ with a fresh
		// pointer and (in the same batch) inserts a new I/ entry
		// at the new ts and deletes the old one; if we still saw
		// the old I/ entry in our snapshot, the pointer here will
		// be different (or the row will be gone entirely).
		d.stats.pebbleGets.Add(1)
		raw, closer, gerr := d.p.Get(dkey)
		if gerr != nil {
			if isNotFound(gerr) {
				// Row already gone (a peer DeleteBelow chunk
				// or a concurrent Delete picked it up); the
				// I/ entry our snapshot showed is harmless
				// orphan state. Issue a defensive I/ Delete
				// so we don't leave the orphan around.
				if derr := batch.Delete(indexKey(s.ID, colID, ent.ts, ent.pk), nil); derr != nil {
					batch.Close()
					unlockAll()
					return 0, derr
				}
				continue
			}
			batch.Close()
			unlockAll()
			return 0, gerr
		}
		ptrCopy := make([]byte, len(raw))
		copy(ptrCopy, raw)
		_ = closer.Close()
		biased, ok := decodePointer(ptrCopy)
		if !ok {
			d.opts.Logger.Printf("WARN: DeleteBelow: skip corrupt indexed row %q/%q (D/ pointer len=%d)", s.Name, ent.pk, len(ptrCopy))
			continue
		}
		curVal := unbiasUint64(biased)
		if curVal != ent.ts {
			// Row has been bumped to a different ts since the
			// snapshot. If the new ts is still < hi a future
			// iteration of DeleteBelow will catch it; otherwise
			// it's correctly retained.
			continue
		}
		if curVal >= hi {
			// Defensive: the snapshot bound made this impossible
			// but we recheck under the lock anyway.
			continue
		}
		if derr := batch.Delete(indexKey(s.ID, colID, ent.ts, ent.pk), nil); derr != nil {
			batch.Close()
			unlockAll()
			return 0, derr
		}
		if derr := batch.Delete(dkey, nil); derr != nil {
			batch.Close()
			unlockAll()
			return 0, derr
		}
		staged++
	}

	if staged == 0 {
		batch.Close()
		unlockAll()
		return 0, nil
	}
	if cerr := batch.Commit(d.wopts); cerr != nil {
		batch.Close()
		unlockAll()
		return 0, cerr
	}
	batch.Close()
	unlockAll()
	for i := 0; i < staged; i++ {
		d.stats.deletes.Add(1)
	}
	return staged, nil
}

// ctxErr returns ctx.Err() when ctx is non-nil. Centralised helper so
// hot loops don't repeat the nil guard.
func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// minInt64 avoids importing math into this file just to get one const.
const minInt64 int64 = -1 << 63

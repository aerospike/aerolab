package db

import (
	"context"
	"errors"

	"github.com/cockroachdb/pebble/v2"
)

// Iter is the common iterator type returned by Scan and Query.Run. Call
// Next() to advance; use Record() to access the current key and row; always
// Close() the iterator, ideally with defer, even after completion.
//
// Concurrency:
//   - Next / Record / Err MUST all be called from a single goroutine. They
//     share unsynchronised per-iterator state and are not safe to interleave
//     across goroutines.
//   - Close is the exception: it is safe to call from a different goroutine
//     (e.g. to cancel a long scan from a supervisor), PROVIDED the goroutine
//     running Next has already returned or has observed Next() == false.
//     Calling Close concurrently with an active Next is a data race against
//     Pebble's iterator state.
//   - Close is idempotent and is safe to call multiple times.
type Iter interface {
	// Next advances to the next matching record. Returns false at end of
	// iteration or on error; callers should check Err() after a final
	// false.
	Next() bool
	// Record returns the current key and row. The returned slices are
	// owned by the caller. Valid only while Next() == true.
	//
	// Record allocates a fresh Row on every call. In very high-rate
	// streaming loops (100K+ records/sec) that can dominate the cost;
	// use ReadInto if allocation shows up in the pprof.
	Record() (key string, row Row)
	// ReadInto writes the current record into the caller-supplied Row
	// and returns the primary key. If dst is nil a fresh Row is
	// allocated for the caller's convenience. If dst is non-nil it is
	// reused: any keys already present are cleared before the new
	// record is written, so a single Row may be recycled across an
	// entire iteration.
	//
	// ReadInto shares the same validity window as Record: the returned
	// key and the values inside dst are valid only while Next() == true.
	// Byte / string values are still copied by the codec so the row
	// remains safe to retain once Next() has advanced.
	ReadInto(dst Row) (key string, out Row)
	// Err returns the first error encountered, if any.
	Err() error
	// Close releases the underlying resources. Safe to call multiple
	// times. See the concurrency notes on Iter for cross-goroutine use.
	Close() error
}

// Scan returns an iterator over every row in the set. If project is empty,
// every column is decoded. The iteration order is unspecified but stable for
// a given set contents (it is Pebble's key order).
func (d *DB) Scan(set string, project ...string) Iter {
	if !d.acquire() {
		return errIter(ErrClosed)
	}
	defer d.release()
	s, ok := d.lookupSet(set)
	if !ok {
		return emptyIter{}
	}
	d.stats.scans.Add(1)
	return d.newDataScan(s, project, nil)
}

// ScanContext is like Scan but honors ctx. When ctx is cancelled mid-scan,
// subsequent Next() calls return false and Err() returns ctx.Err().
func (d *DB) ScanContext(ctx context.Context, set string, project ...string) Iter {
	if !d.acquire() {
		return errIter(ErrClosed)
	}
	defer d.release()
	s, ok := d.lookupSet(set)
	if !ok {
		return emptyIter{}
	}
	d.stats.scans.Add(1)
	return d.newDataScan(s, project, ctx)
}

// newDataScan constructs an iterator bound to the data key range for a set.
// The Pebble iterator streams rows in primary-key order (unindexed sets)
// or in indexed-value order (indexed sets, since v2 clusters the row
// payload at the I/ key). A Pebble snapshot is pinned for the lifetime
// of the iterator so the scan observes a consistent point-in-time view
// even while writers are active.
func (d *DB) newDataScan(s *setSchema, project []string, ctx context.Context) Iter {
	s.mu.RLock()
	idxColID, hasIndex := s.indexedColumn()
	plan := buildPlan(s, project, nil, nil, false)
	s.mu.RUnlock()

	snap := d.p.NewSnapshot()
	var (
		pit  *pebble.Iterator
		err  error
		lo   []byte
		hi   []byte
		kind string
	)
	if hasIndex {
		// v2: the row payload lives at I/setID/colID/biased-ts/pk.
		// Walk just the indexed colID range so foreign columns
		// (none today, since we only ever have one index per set,
		// but indexPrefix is per-(setID,colID)) stay disjoint.
		lo = indexPrefix(s.ID, idxColID)
		hi = indexPrefix(s.ID, idxColID+1)
		if idxColID == ^uint32(0) {
			hi = indexSetUpper(s.ID)
		}
		kind = "scan(indexed)"
	} else {
		lo = dataLowerBound(s.ID)
		hi = dataUpperBound(s.ID)
		kind = "scan"
	}
	pit, err = snap.NewIter(&pebble.IterOptions{LowerBound: lo, UpperBound: hi})
	if err != nil {
		_ = snap.Close()
		return errIter(err)
	}
	it := &dataIter{
		db:        d,
		s:         s,
		snap:      snap,
		pit:       pit,
		ctx:       ctx,
		plan:      plan,
		clustered: hasIndex,
	}
	it.decoded.resize(plan.bufCap)
	it.life = newIterLifecycle(d, kind, s.Name, it)
	return it
}

// dataIter walks the data keyspace for a single set. On unindexed sets
// it streams D/ keys in primary-key order; on indexed sets it streams
// I/ keys in indexed-value order (the row payload is clustered there
// since v2). The two forms share this struct because the per-record
// work (decode value, build row) is identical; only the key parser
// differs.
type dataIter struct {
	db   *DB
	s    *setSchema
	snap *pebble.Snapshot
	pit  *pebble.Iterator
	ctx  context.Context

	plan    queryPlan
	decoded decodedRow

	clustered bool // true ⇒ pit walks I/ keys; false ⇒ D/ keys

	started bool
	err     error

	curKey string
	curRow Row

	life *iterLifecycle
}

func (it *dataIter) Next() bool {
	if it.err != nil {
		return false
	}
	if it.ctx != nil {
		select {
		case <-it.ctx.Done():
			it.err = it.ctx.Err()
			return false
		default:
		}
	}
	var ok bool
	if !it.started {
		it.started = true
		ok = it.pit.First()
	} else {
		ok = it.pit.Next()
	}
	if !ok {
		if e := it.pit.Error(); e != nil {
			it.err = e
		}
		return false
	}
	key := it.pit.Key()
	var pkBytes []byte
	if it.clustered {
		_, _, _, pkb, isIdx := parseIndexKeyBytes(key)
		if !isIdx {
			it.err = errors.New("db: unexpected key prefix in clustered scan iterator")
			return false
		}
		pkBytes = pkb
	} else {
		_, pkb, isData := parseDataKeyBytes(key)
		if !isData {
			it.err = errors.New("db: unexpected key prefix in data iterator")
			return false
		}
		pkBytes = pkb
	}
	// Pebble's Value buffer is stable until pit.Next() / pit.Close(); we
	// decode into our reusable `decoded` map before advancing, so no
	// copy is needed. See the identical invariant in query.go's
	// fullScanIter and indexScanIter.
	val := it.pit.Value()
	it.decoded.Reset()
	if e := decodeRow(val, &it.plan.mask, &it.decoded); e != nil {
		it.err = e
		return false
	}
	it.curRow = shapeRowFromPlan(&it.plan, &it.decoded)
	// pkBytes aliases into the Pebble iterator's key buffer which is
	// only valid until the next Next(); string() copies it.
	it.curKey = string(pkBytes)
	return true
}

func (it *dataIter) Record() (string, Row) {
	return it.curKey, it.curRow
}

// ReadInto populates dst with the current record, allocating a Row if dst
// is nil. See Iter.ReadInto for the semantics.
func (it *dataIter) ReadInto(dst Row) (string, Row) {
	if dst == nil {
		dst = make(Row, len(it.plan.outIDs))
	}
	out := shapeRowIntoPlan(&it.plan, &it.decoded, dst)
	return it.curKey, out
}

func (it *dataIter) Err() error { return it.err }

func (it *dataIter) Close() error {
	var firstErr error
	if it.pit != nil {
		if err := it.pit.Close(); err != nil {
			firstErr = err
		}
		it.pit = nil
	}
	if it.snap != nil {
		if err := it.snap.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		it.snap = nil
	}
	it.life.closeOnce()
	return firstErr
}

// --- static iterators ---

type emptyIter struct{}

func (emptyIter) Next() bool                      { return false }
func (emptyIter) Record() (string, Row)           { return "", nil }
func (emptyIter) ReadInto(dst Row) (string, Row) { return "", dst }
func (emptyIter) Err() error                      { return nil }
func (emptyIter) Close() error                    { return nil }

type staticErrIter struct{ err error }

func (e staticErrIter) Next() bool                      { return false }
func (e staticErrIter) Record() (string, Row)           { return "", nil }
func (e staticErrIter) ReadInto(dst Row) (string, Row) { return "", dst }
func (e staticErrIter) Err() error                      { return e.err }
func (e staticErrIter) Close() error                    { return nil }

func errIter(err error) Iter { return staticErrIter{err: err} }

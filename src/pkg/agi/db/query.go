package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/cockroachdb/pebble/v2"
)

// QueryBuilder accumulates predicates, range, and projection for a Query.
// Call Run to execute it.
//
// The hot path in the plugin is:
//
//	db.Query(set).Between("timestamp", fromMs, toMs).
//	  Where(And(Eq("ClusterName", Int(0)), Exists("latency_p99"))).
//	  Project("timestamp", "latency_p99").
//	  Run(ctx)
//
// Between on the set's indexed column uses the primary-index range scan.
// Between on any other column (or no Between at all) falls back to a full
// set scan with the predicate evaluated in memory.
//
// Note: an indexed Between only visits rows that carry the indexed
// column. Rows where the indexed column is absent have no index entry
// and are therefore skipped by this path. If you need to see all rows
// regardless of the indexed column, use Where(Between(...)) without the
// Between shortcut (or use Scan).
type QueryBuilder struct {
	db      *DB
	set     string
	err     error
	between *betweenClause
	filter  Expr
	project []string
}

type betweenClause struct {
	col string
	lo  Value
	hi  Value
}

// Query starts a new query for the named set. If the set does not exist,
// Run() will return an iterator that yields no records and no error.
func (d *DB) Query(set string) *QueryBuilder {
	return &QueryBuilder{db: d, set: set}
}

// Between restricts results to rows whose col is in [lo, hi] inclusive.
// lo and hi must have matching types. Calling Between twice is an error.
// Only one indexed Between is used; the rest fall through to filter
// evaluation.
func (q *QueryBuilder) Between(col string, lo, hi Value) *QueryBuilder {
	if q.err != nil {
		return q
	}
	if q.between != nil {
		q.err = errors.New("db: Between called more than once")
		return q
	}
	if lo.t != hi.t {
		q.err = fmt.Errorf("db: Between: lo (%s) and hi (%s) types differ", lo.t, hi.t)
		return q
	}
	if lo.IsZero() || hi.IsZero() {
		q.err = errors.New("db: Between: zero value")
		return q
	}
	q.between = &betweenClause{col: col, lo: lo, hi: hi}
	return q
}

// Where applies a pushdown filter evaluated during the scan.
func (q *QueryBuilder) Where(e Expr) *QueryBuilder {
	if q.err != nil {
		return q
	}
	q.filter = e
	return q
}

// Project limits returned rows to the named columns.
func (q *QueryBuilder) Project(cols ...string) *QueryBuilder {
	if q.err != nil {
		return q
	}
	q.project = cols
	return q
}

// queryPlan is a snapshot of the schema-derived state needed to run an
// iterator. Every field is resolved under s.mu in Run() so the per-
// record loop can run without any schema lock. Adding new columns after
// Run() does NOT retro-affect this iterator; unknown columns referenced
// by the filter simply evaluate to "not present", matching the semantics
// of a sparse-column store.
type queryPlan struct {
	// mask drives the codec's per-row decode. When the caller didn't
	// project, mask.decodeAll is true and every column up to bufCap is
	// decoded; otherwise the mask carries the wantValueBM/
	// wantPresenceBM bitmaps built from valueIDs / presenceIDs.
	mask decodeMask
	// outNames maps colID -> output name for columns that should appear
	// in the returned Row. When projIDs was explicit, this is the exact
	// projection; when it wasn't, this is a snapshot of s.ByID so we
	// never touch the live schema from the decode hot path.
	outNames map[uint32]string
	// outIDs is a sorted slice of the keys of outNames, used by
	// shapeRow to walk the projection deterministically without paying
	// a map iteration on every record.
	outIDs []uint32
	// projectionExplicit is true when the caller supplied Project(...).
	// When false, rows are shaped from outNames covering every known
	// column; when true, only projected columns are emitted.
	projectionExplicit bool
	// filterCols maps each column name referenced by the filter (or by
	// a non-indexed Between fallback) to its colID. Names that do not
	// appear in the schema are absent from the map and evaluate to
	// "not present" during filter eval.
	filterCols map[string]uint32
	// bufCap is the s.NextColID snapshot used to size the decodedRow
	// scratch buffer for this iterator.
	bufCap uint32
}

// Run executes the query and returns an Iter. Cancelling ctx will cause
// subsequent Next() calls to return false and Err() to return ctx.Err().
func (q *QueryBuilder) Run(ctx context.Context) Iter {
	if q.err != nil {
		return errIter(q.err)
	}
	if !q.db.acquire() {
		return errIter(ErrClosed)
	}
	defer q.db.release()
	s, ok := q.db.lookupSet(q.set)
	if !ok {
		return emptyIter{}
	}

	// Resolve every piece of schema-derived state under a single s.mu
	// RLock so the iterator loop runs without touching any schema lock
	// on the hot path. Concurrent column additions after this point are
	// simply invisible to the iterator (which matches the snapshot
	// semantics we already provide at the data level via Pebble
	// snapshots).
	s.mu.RLock()
	var useIndex bool
	var idxColID uint32
	if q.between != nil {
		// Validate the column exists. A sparse-column store lets a
		// column come into existence lazily on first Put, so the
		// column-missing case is indistinguishable from "set exists
		// but has zero rows for this column yet". That happens in
		// practice when a query lands before ingest has written its
		// first row to a newly-registered set. Return an empty
		// iterator rather than an error; a typo'd column will also
		// return empty, but that is symmetric with the full-scan
		// path's behavior (no index entry ⇒ no results).
		info, present := s.Columns[q.between.col]
		if !present {
			s.mu.RUnlock()
			return emptyIter{}
		}
		if info.Type != q.between.lo.t {
			s.mu.RUnlock()
			return errIter(fmt.Errorf("db: Between: column %q has type %s, not %s", q.between.col, info.Type.String(), q.between.lo.t.String()))
		}
		if idxName := s.IndexedCol; idxName != "" && idxName == q.between.col {
			if info, present := s.Columns[idxName]; present && info.Type == TypeInt64 {
				_, okLo := q.between.lo.AsInt()
				_, okHi := q.between.hi.AsInt()
				if okLo && okHi {
					useIndex = true
					idxColID = info.ID
				}
			}
		}
	}
	plan := buildPlan(s, q.project, q.filter, q.between, useIndex)
	s.mu.RUnlock()

	q.db.stats.queries.Add(1)
	if useIndex {
		return q.db.newIndexScan(s, idxColID, q.between.lo.i, q.between.hi.i, ctx, q.filter, plan)
	}
	return q.db.newFullScanQuery(s, ctx, q.filter, q.between, plan)
}

// buildPlan resolves the projection, decode-column set, filter-column name
// map, and output name map for an iterator. Must be called with s.mu
// held for reading.
//
// Filter columns are split into two classes:
//
//   - "value-needed":  Eq/In/Between (and any non-presence expr) references
//     the column's value, so the codec must decodePayload it.
//   - "presence-only": Exists / Not(Exists) only ask whether the column is
//     present on the row; the codec just sets a bit and skips
//     decodePayload, saving ~300 ns/row per such column at the AGI
//     dashboard's 4-Exists shape.
//
// A column promoted into the value set on any path is removed from the
// presence set (value supersedes presence). The two sets, together
// with the projection's outNames, are turned into a decodeMask whose
// two bitmaps drive the codec inner loop.
func buildPlan(s *setSchema, project []string, filter Expr, bt *betweenClause, indexedPath bool) queryPlan {
	plan := queryPlan{bufCap: s.NextColID}

	// Filter column name -> colID. Names missing from the schema are
	// still represented; we just omit them so the getter reports them
	// as "not present".
	var filterNames []string
	if filter != nil {
		filterNames = filter.columns(filterNames)
	}
	if bt != nil && !indexedPath {
		filterNames = append(filterNames, bt.col)
	}
	if len(filterNames) > 0 {
		plan.filterCols = make(map[string]uint32, len(filterNames))
		for _, n := range filterNames {
			if info, ok := s.Columns[n]; ok {
				plan.filterCols[n] = info.ID
			}
		}
	}

	// Classify the filter's column references into presence-only vs
	// value-needed. The non-indexed Between fallback always needs the
	// value.
	presenceCols := map[string]struct{}{}
	valueCols := map[string]struct{}{}
	if filter != nil {
		classifyFilterCols(filter, presenceCols, valueCols)
	}
	if bt != nil && !indexedPath {
		valueCols[bt.col] = struct{}{}
	}
	// value supersedes presence (we already pay decodePayload for it).
	for col := range valueCols {
		delete(presenceCols, col)
	}

	// Projection handling.
	if len(project) == 0 {
		// No projection: emit every known column. outNames is a
		// snapshot of s.ByID so the decode loop never touches the
		// live schema.
		plan.projectionExplicit = false
		if len(s.ByID) > 0 {
			plan.outNames = make(map[uint32]string, len(s.ByID))
			plan.outIDs = make([]uint32, 0, len(s.ByID))
			for id, name := range s.ByID {
				plan.outNames[id] = name
				plan.outIDs = append(plan.outIDs, id)
			}
			sortUint32s(plan.outIDs)
		}
		// All filter columns are still classified — even on the no-
		// projection path the iterator decodes every column anyway,
		// so we use decodeAll. The classification matters only for
		// the explicit-projection branch below.
		plan.mask = decodeAllMask(plan.bufCap)
		return plan
	}

	plan.projectionExplicit = true
	plan.outNames = make(map[uint32]string, len(project))
	plan.outIDs = make([]uint32, 0, len(project))
	for _, name := range project {
		if info, ok := s.Columns[name]; ok {
			plan.outNames[info.ID] = name
			plan.outIDs = append(plan.outIDs, info.ID)
		}
	}
	sortUint32s(plan.outIDs)

	// Build sorted valueIDs (projection ∪ value-needed filter cols)
	// and presenceIDs (presence-only filter cols not promoted by the
	// projection or by the value set).
	valueSet := map[uint32]struct{}{}
	for id := range plan.outNames {
		valueSet[id] = struct{}{}
	}
	for col := range valueCols {
		if id, ok := plan.filterCols[col]; ok {
			valueSet[id] = struct{}{}
		}
	}
	presenceSet := map[uint32]struct{}{}
	for col := range presenceCols {
		id, ok := plan.filterCols[col]
		if !ok {
			continue
		}
		if _, inValue := valueSet[id]; inValue {
			continue
		}
		presenceSet[id] = struct{}{}
	}
	valueIDs := make([]uint32, 0, len(valueSet))
	for id := range valueSet {
		valueIDs = append(valueIDs, id)
	}
	sortUint32s(valueIDs)
	presenceIDs := make([]uint32, 0, len(presenceSet))
	for id := range presenceSet {
		presenceIDs = append(presenceIDs, id)
	}
	sortUint32s(presenceIDs)
	plan.mask = buildDecodeMask(valueIDs, presenceIDs, plan.bufCap)
	return plan
}

// sortUint32s is an in-place ascending sort. Copying sort.Slice's tiny
// closure overhead per iterator-build matters here because buildPlan
// runs once per query — not in the hot loop — but the dependency-free
// helper keeps the file from importing sort just for this.
func sortUint32s(a []uint32) {
	for i := 1; i < len(a); i++ {
		v := a[i]
		j := i
		for j > 0 && a[j-1] > v {
			a[j] = a[j-1]
			j--
		}
		a[j] = v
	}
}

// --- index-range scan iterator ---

// indexScanIter walks the primary-index range [lo, hi] and, for every
// candidate pk, fetches the data row to evaluate the filter and shape the
// projection. A Pebble snapshot is pinned for the lifetime of the iterator
// so the index walk and the data lookups share a single consistent view,
// even while writers are active.
type indexScanIter struct {
	db   *DB
	s    *setSchema
	snap *pebble.Snapshot
	pit  *pebble.Iterator
	ctx  context.Context

	filter Expr

	plan queryPlan

	// Reusable per-record state to keep the hot loop allocation-free.
	decoded decodedRow
	// getter is bound once at iterator construction so the per-row
	// filter eval doesn't allocate a closure. It closes over &decoded
	// and plan.filterCols.
	getter colGetter

	started bool
	err     error

	curKey string
	curRow Row

	life *iterLifecycle
}

func (d *DB) newIndexScan(s *setSchema, colID uint32, lo, hi int64, ctx context.Context, filter Expr, plan queryPlan) Iter {
	lower, upper := indexRangeBounds(s.ID, colID, lo, hi)
	snap := d.p.NewSnapshot()
	pit, err := snap.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		_ = snap.Close()
		return errIter(err)
	}
	it := &indexScanIter{
		db:     d,
		s:      s,
		snap:   snap,
		pit:    pit,
		ctx:    ctx,
		filter: filter,
		plan:   plan,
	}
	it.decoded.resize(plan.bufCap)
	it.getter = makeDecodedGetter(plan.filterCols, &it.decoded)
	it.life = newIterLifecycle(d, "query(index)", s.Name, it)
	return it
}

func (it *indexScanIter) Next() bool {
	if it.err != nil {
		return false
	}
	for {
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
		idxKey := it.pit.Key()
		_, _, _, pkBytes, isIdx := parseIndexKeyBytes(idxKey)
		if !isIdx {
			it.err = errors.New("db: unexpected key prefix in index iterator")
			return false
		}
		// Orphan-skip guard: opt-in via Options.IndexCanHaveOrphans.
		// When enabled, every scanned I/ entry is verified against
		// the D/ pointer to skip orphans left by an AssumeNew=true
		// overwrite that landed on a different indexed value. The
		// check costs one Pebble Get per row, well-served by the
		// block cache but a 4-5× tax on cold ranges, so it is OFF
		// by default and the AGI ingest workload (whose PKs cannot
		// orphan) keeps it off. Callers that genuinely overwrite
		// indexed values via AssumeNew opt in at Open time and pay
		// the tax knowingly.
		if it.db.assumeNewSeen.Load() {
			idxBiased := indexKeyBiasedRaw(idxKey)
			it.db.stats.pebbleGets.Add(1)
			ptrRaw, ptrCloser, perr := it.snap.Get(dataKeyBytes(it.s.ID, pkBytes))
			if perr != nil {
				if !isNotFound(perr) {
					it.err = perr
					return false
				}
				// D/ gone but I/ still present: the row was
				// deleted after this snapshot was pinned (or
				// the writer is mid-batch). Either way the I/
				// entry is no longer live; skip.
				continue
			}
			ptrBiased, ok := decodePointer(ptrRaw)
			_ = ptrCloser.Close()
			if !ok || ptrBiased != idxBiased {
				continue
			}
		}
		// Storage version 2: the row payload lives directly at the
		// I/ key (covering index, clustered by indexed value), so
		// pit.Value() is the row bytes. No second snap.Get is
		// required — this is the read-path win that motivated the
		// layout change. Pebble's Value buffer is stable until the
		// next pit.Next() / pit.Close(); we decode into the
		// reusable `decoded` map before advancing.
		val := it.pit.Value()
		it.decoded.Reset()
		if e := decodeRow(val, &it.plan.mask, &it.decoded); e != nil {
			it.err = e
			return false
		}
		if it.filter != nil {
			if !it.filter.eval(it.getter) {
				continue
			}
		}
		it.curRow = shapeRowFromPlan(&it.plan, &it.decoded)
		// pkBytes aliases into the Pebble iterator's key buffer which
		// is only valid until the next Next(); string() copies it.
		it.curKey = string(pkBytes)
		return true
	}
}

func (it *indexScanIter) Record() (string, Row) { return it.curKey, it.curRow }
func (it *indexScanIter) ReadInto(dst Row) (string, Row) {
	if dst == nil {
		dst = make(Row, len(it.plan.outIDs))
	}
	out := shapeRowIntoPlan(&it.plan, &it.decoded, dst)
	return it.curKey, out
}
func (it *indexScanIter) Err() error { return it.err }
func (it *indexScanIter) Close() error {
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

// --- full set scan with filter ---

type fullScanIter struct {
	db   *DB
	s    *setSchema
	snap *pebble.Snapshot
	pit  *pebble.Iterator
	ctx  context.Context

	filter  Expr
	between *betweenExpr // pre-built, non-nil only for non-indexed Between fallback

	plan queryPlan

	decoded decodedRow
	getter  colGetter

	// clustered=true ⇒ pit walks I/ keys (indexed sets, v2); pit.Value()
	// is the row payload and the pk lives at offset 17. clustered=false
	// ⇒ pit walks D/ keys (unindexed sets); pit.Value() is the row
	// payload and the pk lives at offset 5.
	clustered bool

	started bool
	err     error

	curKey string
	curRow Row

	life *iterLifecycle
}

func (d *DB) newFullScanQuery(s *setSchema, ctx context.Context, filter Expr, bt *betweenClause, plan queryPlan) Iter {
	s.mu.RLock()
	idxColID, hasIndex := s.indexedColumn()
	s.mu.RUnlock()

	snap := d.p.NewSnapshot()
	var (
		lo, hi []byte
		kind   string
	)
	if hasIndex {
		// v2: row payload is clustered at I/ keys for indexed sets.
		// A non-indexed Between (or no Between) on an indexed set
		// still has to walk every row, but it does so in indexed-
		// value order rather than pk order — this is observable
		// to callers that did not request a Between, but the
		// iteration order is documented as unspecified.
		lo = indexPrefix(s.ID, idxColID)
		hi = indexPrefix(s.ID, idxColID+1)
		if idxColID == ^uint32(0) {
			hi = indexSetUpper(s.ID)
		}
		kind = "query(scan-indexed)"
	} else {
		lo = dataLowerBound(s.ID)
		hi = dataUpperBound(s.ID)
		kind = "query(scan)"
	}
	pit, err := snap.NewIter(&pebble.IterOptions{LowerBound: lo, UpperBound: hi})
	if err != nil {
		_ = snap.Close()
		return errIter(err)
	}
	var btExpr *betweenExpr
	if bt != nil {
		btExpr = &betweenExpr{col: bt.col, lo: bt.lo, hi: bt.hi}
	}
	it := &fullScanIter{
		db:        d,
		s:         s,
		snap:      snap,
		pit:       pit,
		ctx:       ctx,
		filter:    filter,
		between:   btExpr,
		plan:      plan,
		clustered: hasIndex,
	}
	it.decoded.resize(plan.bufCap)
	it.getter = makeDecodedGetter(plan.filterCols, &it.decoded)
	it.life = newIterLifecycle(d, kind, s.Name, it)
	return it
}

func (it *fullScanIter) Next() bool {
	if it.err != nil {
		return false
	}
	for {
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
		var pkBytes []byte
		if it.clustered {
			_, _, _, pkb, isIdx := parseIndexKeyBytes(it.pit.Key())
			if !isIdx {
				it.err = errors.New("db: unexpected key prefix in clustered scan iterator")
				return false
			}
			pkBytes = pkb
		} else {
			_, pkb, isData := parseDataKeyBytes(it.pit.Key())
			if !isData {
				it.err = errors.New("db: unexpected key prefix in data iterator")
				return false
			}
			pkBytes = pkb
		}
		// Pebble's Value buffer is stable until the next pit.Next() (or
		// pit.Close()); we decode into our reusable `decoded` map
		// before advancing the iterator, so no copy is needed. If you
		// change this loop to retain any byte slice produced by decode
		// past the next Next() call, copy it first — the codec already
		// copies string/bytes payloads, so only the raw Value() slice
		// has this restriction.
		val := it.pit.Value()
		it.decoded.Reset()
		if e := decodeRow(val, &it.plan.mask, &it.decoded); e != nil {
			it.err = e
			return false
		}
		// Between that didn't hit the index is evaluated here.
		if it.between != nil {
			if !it.between.eval(it.getter) {
				continue
			}
		}
		if it.filter != nil {
			if !it.filter.eval(it.getter) {
				continue
			}
		}
		it.curRow = shapeRowFromPlan(&it.plan, &it.decoded)
		// pkBytes aliases into the Pebble iterator's key buffer which
		// is only valid until the next Next(); string() copies it.
		it.curKey = string(pkBytes)
		return true
	}
}

func (it *fullScanIter) Record() (string, Row) { return it.curKey, it.curRow }
func (it *fullScanIter) ReadInto(dst Row) (string, Row) {
	if dst == nil {
		dst = make(Row, len(it.plan.outIDs))
	}
	out := shapeRowIntoPlan(&it.plan, &it.decoded, dst)
	return it.curKey, out
}
func (it *fullScanIter) Err() error { return it.err }
func (it *fullScanIter) Close() error {
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

// makeDecodedGetter returns a colGetter backed by the filter-column
// snapshot and the iterator's decodedRow scratch buffer. It is bound
// once at iterator construction so the per-row filter eval path
// doesn't allocate a fresh closure. The closure captures both
// filterCols and the decodedRow pointer, so Reset() between rows is
// observed transparently.
//
// Presence-only filter columns (existsExpr) get a (zero Value, true)
// reading because the codec marked the bit but never decoded the
// payload — exactly what existsExpr.eval consumes. The planner
// guarantees value-using exprs (eqExpr/inExpr/betweenExpr) only refer
// to columns whose colIDs landed in mask.wantValueBM, so they never
// observe the zero-Value-with-true sentinel.
func makeDecodedGetter(filterCols map[string]uint32, decoded *decodedRow) colGetter {
	return func(name string) (Value, bool) {
		id, ok := filterCols[name]
		if !ok {
			return Value{}, false
		}
		return decoded.Get(id)
	}
}

// shapeRowFromPlan builds the public Row from the decodedRow buffer,
// using the per-iterator outNames / outIDs snapshot so we never touch
// s.ByID from the hot path. Allocates a fresh Row on every call; see
// shapeRowIntoPlan for the zero-alloc variant used by Iter.ReadInto.
func shapeRowFromPlan(plan *queryPlan, decoded *decodedRow) Row {
	out := make(Row, len(plan.outIDs))
	return shapeRowIntoPlan(plan, decoded, out)
}

// shapeRowIntoPlan is the in-place form of shapeRowFromPlan. dst must be
// non-nil; any keys already present are cleared before the new record is
// written. Returns dst so callers can chain. Used by the ReadInto path
// to avoid the per-record Row allocation.
func shapeRowIntoPlan(plan *queryPlan, decoded *decodedRow, dst Row) Row {
	for k := range dst {
		delete(dst, k)
	}
	for _, id := range plan.outIDs {
		if v, ok := decoded.Get(id); ok {
			dst[plan.outNames[id]] = v
		}
	}
	return dst
}

package ingest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"log"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

type MetaEntries map[string]*metaEntries

type metaEntries struct {
	Entries          []string
	ByCluster        map[string][]int
	StaticEntriesIdx []int

	// Unexported derived state, never serialised. Built lazily in
	// ensureIdx() from Entries / ByCluster the first time a
	// metaEntries is observed (after json.Unmarshal of an existing
	// label row, or right at &metaEntries{} construction). Kept in
	// sync with the slice fields on every mutation. Lower-case
	// names => the json package skips them, so the wire format
	// stays compatible with the plugin's reader in
	// backendQueryAndCache.go.
	mu           sync.Mutex                  // per-entry hot-path lock
	entriesIdx   map[string]int              // sv -> index into Entries
	byClusterSet map[string]map[int]struct{} // clusterName -> set of idx
}

// ensureIdx populates entriesIdx and byClusterSet from the JSON-loaded
// slice fields. Called once when an entry is loaded from disk in
// ProcessLogsPrep and once when first allocated in the hot path; safe
// to call repeatedly (idempotent).
func (m *metaEntries) ensureIdx() {
	if m.entriesIdx == nil {
		m.entriesIdx = make(map[string]int, len(m.Entries))
		for i, e := range m.Entries {
			// On a duplicate (shouldn't happen with the current
			// pipeline, but be defensive), keep the first
			// index — that matches what slices.Index returned
			// pre-fix.
			if _, ok := m.entriesIdx[e]; !ok {
				m.entriesIdx[e] = i
			}
		}
	}
	if m.byClusterSet == nil {
		m.byClusterSet = make(map[string]map[int]struct{}, len(m.ByCluster))
		for cl, idxs := range m.ByCluster {
			s := make(map[int]struct{}, len(idxs))
			for _, idx := range idxs {
				s[idx] = struct{}{}
			}
			m.byClusterSet[cl] = s
		}
	}
}

// metaShards wraps the parent meta map with a single RWMutex that
// guards structural mutations (first-sighting insertion). Per-entry
// mutations on the hot path use metaEntries.mu instead — this RWMutex
// is held only for the brief get-or-create branch, so first sightings
// of a never-seen key (a few dozen times per ingest) are the only
// readers blocked behind the exclusive lock.
type metaShards struct {
	parentMu sync.RWMutex
	meta     map[string]*metaEntries
}

// getOrCreate returns the metaEntries for key, allocating one (with
// pre-populated index maps) on first sighting. The two-phase
// RLock-then-Lock dance is the standard double-checked init pattern:
// the hot read path takes the cheap RLock once the key exists; only
// the rare insertion takes the exclusive Lock.
func (s *metaShards) getOrCreate(key string) *metaEntries {
	s.parentMu.RLock()
	if m, ok := s.meta[key]; ok {
		s.parentMu.RUnlock()
		return m
	}
	s.parentMu.RUnlock()
	s.parentMu.Lock()
	defer s.parentMu.Unlock()
	if m, ok := s.meta[key]; ok {
		return m
	}
	m := &metaEntries{
		ByCluster:    make(map[string][]int),
		entriesIdx:   make(map[string]int),
		byClusterSet: make(map[string]map[int]struct{}),
	}
	s.meta[key] = m
	return m
}

func (i *Ingest) ProcessLogsPrep() (foundLogs map[string]*LogFile, meta map[string]*metaEntries, err error) {
	i.progress.Lock()
	i.progress.LogProcessor.Finished = false
	i.progress.LogProcessor.running = true
	i.progress.LogProcessor.wasRunning = true
	i.progress.LogProcessor.StartTime = time.Now()
	i.progress.Unlock()
	// find node prefix->nodeID from log files
	log.Printf("DEBUG: Process Logs: enumerating log files")
	foundLogs = make(map[string]*LogFile) //cluster,nodeid,prefix
	err = filepath.Walk(i.config.Directories.Logs, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		fn := strings.Split(info.Name(), "_")
		if len(fn) != 3 {
			return nil
		}
		fdir, _ := path.Split(filePath)
		_, fcluster := path.Split(strings.TrimSuffix(fdir, "/"))
		foundLogs[filePath] = &LogFile{
			ClusterName: fcluster,
			NodePrefix:  fn[0],
			NodeID:      fn[1],
			NodeSuffix:  fn[2],
			Size:        info.Size(),
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing logs: %s", err)
	}
	// merge list
	log.Printf("DEBUG: ProcessLogs: merging lists")
	i.progress.Lock()
	maps.Copy(foundLogs, i.progress.LogProcessor.Files)
	i.progress.LogProcessor.Files = make(map[string]*LogFile)
	maps.Copy(i.progress.LogProcessor.Files, foundLogs)
	i.progress.LogProcessor.changed = true
	meta = i.loadMetaEntries()
	i.progress.Unlock()
	err = i.saveProgress()
	return foundLogs, meta, err
}

func (i *Ingest) loadMetaEntries() map[string]*metaEntries {
	meta := make(map[string]*metaEntries)
	// The labels set uses <row-key = label-name, column-name = "json">
	// (see init.go's labelsValueCol). Keys like "BINLIST" / "cfName" /
	// "sources" / "timerange" are not metaEntries; skip them. Every
	// other row is a meta entry for the label named by its primary
	// key.
	skipKeys := map[string]struct{}{
		"BINLIST":   {},
		"cfName":    {},
		"sources":   {},
		"timerange": {},
	}
	it := i.db.Scan(i.patterns.LabelsSetName, labelsValueCol)
	for it.Next() {
		key, row := it.Record()
		if _, skip := skipKeys[key]; skip {
			continue
		}
		v, ok := row[labelsValueCol]
		if !ok {
			continue
		}
		s, ok := v.AsString()
		if !ok {
			continue
		}
		metaItem := &metaEntries{}
		if uerr := json.Unmarshal([]byte(s), metaItem); uerr != nil {
			log.Printf("WARN: Failed to unmarshal existing label data for %s: %s", key, uerr)
			continue
		}
		// Pre-build the index maps so the hot path's per-row
		// lookups never have to lazy-init under a lock.
		metaItem.ensureIdx()
		meta[key] = metaItem
	}
	if serr := it.Err(); serr != nil {
		log.Printf("WARN: Could not read existing labels: %s", serr)
	}
	_ = it.Close()
	return meta
}

func (i *Ingest) ProcessLogs(foundLogs map[string]*LogFile, meta map[string]*metaEntries) error {
	if foundLogs == nil || meta == nil {
		var err error
		foundLogs, meta, err = i.ProcessLogsPrep()
		if err != nil {
			i.progress.Lock()
			i.progress.LogProcessor.running = false
			i.progress.Unlock()
			return err
		}
	}
	defer func() {
		i.progress.Lock()
		i.progress.LogProcessor.running = false
		i.progress.Unlock()
	}()
	// metaShards replaces the single global metaLock with one
	// sync.Mutex per meta key (sitting on metaEntries.mu) plus a
	// RWMutex over the parent map. The hot path holds only the
	// per-key mu while it performs the Index lookup, append,
	// JSON marshal, db.Put, rollback for that key. Different rows
	// touching disjoint key sets proceed in parallel; rows that
	// collide on the always-shared keys (ClusterName, NodeIdent)
	// only serialize on those, not on every label.
	shards := &metaShards{meta: meta}
	// resultsChan is buffered so producers can push multiple rows
	// without a per-row scheduler rendezvous against the receivers.
	// The buffer is a smoothing window, not a parallelism
	// multiplier; correctness is independent of the size.
	//
	// Its capacity is decoupled from the worker count: the buffer
	// exists to absorb short bursts where parsers temporarily
	// outrun workers (e.g. a worker mid-PutBatch on a slow
	// per-shard commit), and the right size is "enough rows so a
	// single batcher commit window does not propagate
	// back-pressure all the way up to the parser goroutines",
	// not "one slot per worker". Tying it to MaxPutThreads
	// silently shrank this buffer 8x when the worker default was
	// reduced from 128 -> GOMAXPROCS*2, which surfaced as more
	// upstream chansend1 blocking in the parser path with no
	// throughput improvement to justify it. 128 slots restores
	// the historical buffer depth while keeping the smaller
	// worker pool's CPU benefits.
	const resultsChanBuf = 128
	workers := i.config.MaxPutThreads
	if workers <= 0 {
		// Auto branch (opt-in via maxPutThreads: 0). Scales with
		// the box's GOMAXPROCS for constrained deployments that
		// genuinely benefit from a smaller pool. The default
		// path bypasses this branch entirely (Config.MaxPutThreads
		// defaults to 128); see struct.go for the rationale on
		// why 128 was chosen as the default rather than this
		// auto formula.
		workers = runtime.GOMAXPROCS(0) * 2
		if workers < 4 {
			workers = 4
		}
		if workers > 32 {
			workers = 32
		}
	}
	resultsChan := make(chan *processResult, resultsChanBuf)
	go i.processLogsFeed(foundLogs, resultsChan, shards)

	// feed results to backend DB.
	//
	// Pre-pool design spawned one goroutine per row from a single
	// dispatcher loop, gated by a 128-slot semaphore. At AGI's
	// typical throughput of ~1M rows/s the per-row goroutine
	// creation (~1-3µs each) saturated the dispatcher's single
	// goroutine and capped the entire pipeline at ~2 cores busy
	// regardless of how parallel the producers, batcher, or worker
	// body actually were. The fixed worker pool below removes
	// that ceiling: N persistent goroutines pull directly from
	// resultsChan with no per-row spawn and no semaphore.
	//
	// Label resolution USED to live inside the worker body too —
	// each worker resolved every meta key per row via
	// shards.getOrCreate + upsertMetaEntry, holding metaEntries.mu
	// each time. Labels with low cardinality (ClusterName,
	// NodeIdent) had a single mutex shared by every one of the 128
	// workers; the resulting contention serialised the pipeline
	// onto roughly the cardinality of the labels (~10 effective
	// parallel workers) regardless of how many goroutines or cores
	// were available, and showed up as ~50% idle CPU on 8-vCPU
	// boxes with throughput stuck around 27 MiB/s on EFS / EBS /
	// RAMdisk alike (the bottleneck was in-process mutex
	// contention, not storage). Resolution now happens parser-side
	// in processLogFile via a per-file resolveCache: the contended
	// metaEntries.mu path runs O(distinct values per file) instead
	// of O(rows), and only the 6 parser goroutines (not 128
	// workers) ever take it. The worker body below is reduced to
	// stamping pre-resolved indices into rowData.
	i.runWorkerPool(resultsChan, workers)

	// Final flush of the bin list, in case the last Put inside the
	// worker-goroutine loop saw changed=false but a later goroutine
	// re-set it. If this fails the next ingest cycle's storeBinList
	// will retry, so we log and continue rather than aborting the
	// whole pipeline.
	if serr := i.storeBinList(); serr != nil {
		log.Printf("ERROR: Log Processor: could not store bin list: %s", serr)
	}

	// Optional post-ingest manual compaction. Runs synchronously
	// before we mark LogProcessor.Finished so that "ingest done"
	// implies "DB is in its optimised steady state" — the first
	// plugin query after this point sees a single dense bottom
	// level instead of L0+L1+L2 merges. Failure is logged and
	// swallowed: the data is queryable either way; the only loss
	// is that the on-disk layout stays in its un-compacted post-
	// ingest shape until Pebble's background compactions catch up
	// naturally.
	//
	// Compact already implies a memtable flush (Pebble waits for
	// any overlapping memtable before walking the levels), so a
	// successful Compact is also our durability barrier. When
	// PostIngestCompact is off (Docker default) we fall through
	// to an explicit db.Flush() below — the dirty marker can
	// only be cleared after we know every Put is on disk.
	durable := false
	if i.config.DB.PostIngestCompact {
		if i.db == nil {
			log.Printf("WARN: post-ingest compaction skipped: db handle not initialised")
		} else {
			compactStart := time.Now()
			log.Printf("INFO: post-ingest compaction starting; this may take several minutes on large EFS-backed DBs")
			if cerr := i.db.Compact(context.TODO()); cerr != nil {
				log.Printf("WARN: post-ingest compaction failed after %s: %s", time.Since(compactStart), cerr)
			} else {
				log.Printf("INFO: post-ingest compaction complete in %s", time.Since(compactStart))
				durable = true
			}
		}
	}
	if !durable && i.db != nil {
		// No Compact (or Compact failed). Force a memtable
		// flush so the dirty-marker clear below is honest about
		// disk state. Cheap (single Pebble Flush call), bounded
		// by the size of the active memtable.
		flushStart := time.Now()
		if ferr := i.db.Flush(); ferr != nil {
			log.Printf("WARN: post-ingest db.Flush failed after %s: %s; leaving dirty marker in place", time.Since(flushStart), ferr)
		} else {
			log.Printf("INFO: post-ingest db.Flush complete in %s", time.Since(flushStart))
			durable = true
		}
	}
	// Only clear the dirty marker once we have a real durability
	// guarantee. If both Compact and Flush failed we leave the
	// marker behind so the next startup wipes-and-reingests
	// (WAL=off) rather than trusting a half-flushed DB.
	if durable {
		if cerr := ClearDirtyMarker(i.config.ProgressFile.OutputFilePath); cerr != nil {
			log.Printf("WARN: could not clear ingest dirty marker: %s", cerr)
		}
	}

	// done
	i.progress.Lock()
	i.progress.LogProcessor.Finished = true
	i.progress.Unlock()
	return nil
}

func (i *Ingest) runWorkerPool(resultsChan <-chan *processResult, workers int) {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0) * 2
		if workers < 4 {
			workers = 4
		}
		if workers > 32 {
			workers = 32
		}
	}
	wg := new(sync.WaitGroup)
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for data := range resultsChan {
				if data.Error != nil {
					log.Printf("ERROR: Log Processor: error encountered processing %s: %s", data.FileName, data.Error)
				}
				if data.Data == nil || data.SetName == "" || data.LogLine == "" {
					continue
				}
				rowData := data.Data
				fn := data.FileName
				logLine := data.LogLine
				setName := data.SetName
				nodeIdentifier := data.UniqNodeString
				// Stamp pre-resolved label indices. The parser-side
				// processLogFile already paid the metaShards /
				// metaEntries.mu cost (typically once per (file,
				// key, value) combination via the per-file
				// resolveCache), so this loop is pure map writes.
				// Keys absent from data.Resolved are keys that
				// resolveLabelsCached refused (non-string value or
				// upsertMetaEntry persist failure) — the row goes
				// through with one less label rather than dumping
				// every other label too, matching the legacy
				// per-key error semantics.
				for k, idx := range data.Resolved {
					rowData[k] = idx
				}
				// Hot path: lock-free probe against the bin-list
				// snapshot. After warmup every column is already
				// known and missingNames returns nil with zero
				// allocations; the rare miss takes b.lock once
				// and publishes a copy-on-written replacement.
				// We deliberately do NOT call storeBinList here.
				// Persistence is folded into saveProgress (see
				// trackprogress.go) so the on-disk pair
				// (BINLIST, progress) is consistent at every
				// checkpoint, which is the invariant the
				// crash-resume path relies on.
				if missing := i.binList.missingNames(rowData); len(missing) > 0 {
					i.binList.addNames(missing)
				}
				row, rerr := buildDataRow(rowData, i.config.TimestampColumnName)
				if rerr != nil {
					log.Printf("ERROR: Log Processor: %s: %s", fn, rerr)
					continue
				}
				// PK = XXH3-128(clusterName::/::nodeIdent::/::logLine).
				// See pk.go for the rationale; pre-v3 builds wrote the
				// raw concatenation as the key, which cost ~150 bytes
				// per row in two LSM keyspaces (D/ pointer + I/ index
				// suffix) and bought no functionality the live read
				// path uses.
				pk := MetricsRowKeyFromCombined(nodeIdentifier, logLine)
				if perr := i.putData(setName, pk, row); perr != nil {
					log.Printf("ERROR: Log Processor: could not insert data for %s: %s", fn, perr)
				}
			}
		}()
	}
	wg.Wait()
}

// resolveCache is a per-file (key -> value -> idx) memoiser that
// short-circuits the contended metaEntries.mu / metaShards.parentMu
// path on the hot row-emit loop in processLogFile. It is owned by
// exactly one parser goroutine (one per log file), so it requires no
// synchronisation of its own. Cache misses fall through to the
// shared metaShards / upsertMetaEntry path, paying the lock cost
// once per (file, key, value) combination instead of once per row.
//
// The legacy design did the resolve in the 128-strong worker pool
// (one upsertMetaEntry per row per key), which serialised the entire
// pipeline on a small set of metaEntries.mu mutexes — labels like
// ClusterName have one distinct value across the whole ingest, so
// every worker contended on the same mutex for every row. After the
// switch every parser sees a cache hit on the second-and-onwards row
// for a given (key, value), so the contended path runs O(distinct
// values) times per file instead of O(rows).
type resolveCache map[string]map[string]int

// resolveLabelsCached resolves every (k, v) pair in metadata to a
// metaEntries idx using the per-file cache first and the shared
// metaShards path on miss. The returned map is freshly allocated and
// owned by the caller; callers stamp it into rowData on the consumer
// side (or into a processResult that the worker copies). Non-string
// values and per-key persistence failures are dropped from the
// resulting map (they were dropped from rowData by the legacy worker
// loop too) so a single bad label never poisons the whole row.
//
// clusterName is the cluster context for the row (for the per-cluster
// reverse-index). It is the file-scope ClusterName passed to
// processLogFile; the legacy worker fished the same value out of
// metadata["ClusterName"] every row.
func (i *Ingest) resolveLabelsCached(metadata map[string]any, cache resolveCache, shards *metaShards, clusterName, fn string) map[string]int {
	if len(metadata) == 0 {
		return nil
	}
	resolved := make(map[string]int, len(metadata))
	for k, v := range metadata {
		sv, ok := v.(string)
		if !ok {
			log.Printf("ERROR: Log Processor: metadata %q has non-string value %T; skipping", k, v)
			continue
		}
		if vmap, ok := cache[k]; ok {
			if idx, ok := vmap[sv]; ok {
				resolved[k] = idx
				continue
			}
		}
		me := shards.getOrCreate(k)
		idx, persisted := i.upsertMetaEntry(me, k, sv, clusterName, fn)
		if !persisted {
			continue
		}
		resolved[k] = idx
		vmap := cache[k]
		if vmap == nil {
			vmap = make(map[string]int)
			cache[k] = vmap
		}
		vmap[sv] = idx
	}
	return resolved
}

// mergeResolved is the row-emit fast path: if perLine is empty the
// caller's resolvedLabels (file-scope, allocated once per file) is
// returned by reference and the worker reads it directly — no copy
// per row. Otherwise we resolve the per-line keys via the cache and
// merge with the file-scope labels into a fresh map. Merge order
// matches the legacy `meta := d.Metadata; maps.Copy(meta, labels)`
// semantics: file-scope labels win on collision.
//
// Returning resolvedLabels by reference is safe because the worker
// pool only reads from processResult.Resolved (it stamps idx values
// into rowData, which is per-row); resolvedLabels is never mutated
// after the file-scope pre-resolve in processLogFile.
func (i *Ingest) mergeResolved(resolvedLabels map[string]int, perLine map[string]any, cache resolveCache, shards *metaShards, clusterName, fn string) map[string]int {
	if len(perLine) == 0 {
		return resolvedLabels
	}
	resolved := i.resolveLabelsCached(perLine, cache, shards, clusterName, fn)
	if len(resolved) == 0 {
		return resolvedLabels
	}
	if len(resolvedLabels) == 0 {
		return resolved
	}
	merged := make(map[string]int, len(resolvedLabels)+len(resolved))
	for k, v := range resolved {
		merged[k] = v
	}
	for k, v := range resolvedLabels {
		merged[k] = v
	}
	return merged
}

// upsertMetaEntry performs the previously-monolithic per-key body of
// ProcessLogs's worker loop under the per-key metaEntries.mu lock.
// Returns (idx, true) when sv has a stable index in me.Entries (either
// because it was already present, or because the append + db.Put
// succeeded); returns (_, false) when the key could not be persisted
// and the metric row should NOT carry a translated idx for it (the row
// goes through with one less label rather than dumping every other
// label too, matching the multi-key resilience the pre-fix code
// already had).
//
// Rollback is kept symmetric with the slice append: if anything fails
// after we've appended, we shrink Entries / ByCluster AND the
// entriesIdx / byClusterSet maps in lockstep so the in-memory state
// stays consistent with the on-disk JSON.
func (i *Ingest) upsertMetaEntry(me *metaEntries, k, sv, clusterName, fn string) (int, bool) {
	me.mu.Lock()
	defer me.mu.Unlock()
	// Defensive: ensureIdx is also called at load time and at
	// allocation time, so the maps should already be non-nil. Be
	// permissive in case a future caller bypasses metaShards.
	me.ensureIdx()
	const (
		mutNone        = 0
		mutAppendEntry = 1 // appended to Entries (and possibly ByCluster)
		mutAppendCl    = 2 // only appended to ByCluster[cluster]
	)
	mutKind := mutNone
	var mutCluster string
	saveMeta := false
	idx, present := me.entriesIdx[sv]
	if !present {
		idx = len(me.Entries)
		saveMeta = true
		me.Entries = append(me.Entries, sv)
		me.entriesIdx[sv] = idx
		if me.ByCluster == nil {
			me.ByCluster = make(map[string][]int)
		}
		if clusterName != "" {
			me.ByCluster[clusterName] = append(me.ByCluster[clusterName], idx)
			if me.byClusterSet[clusterName] == nil {
				me.byClusterSet[clusterName] = make(map[int]struct{})
			}
			me.byClusterSet[clusterName][idx] = struct{}{}
			mutCluster = clusterName
		}
		mutKind = mutAppendEntry
	} else if clusterName != "" {
		if _, inSet := me.byClusterSet[clusterName][idx]; !inSet {
			me.ByCluster[clusterName] = append(me.ByCluster[clusterName], idx)
			if me.byClusterSet[clusterName] == nil {
				me.byClusterSet[clusterName] = make(map[int]struct{})
			}
			me.byClusterSet[clusterName][idx] = struct{}{}
			saveMeta = true
			mutKind = mutAppendCl
			mutCluster = clusterName
		}
	}
	rollback := func() {
		switch mutKind {
		case mutAppendEntry:
			if n := len(me.Entries); n > 0 {
				last := me.Entries[n-1]
				me.Entries = me.Entries[:n-1]
				// Only delete from entriesIdx if it still
				// points at the index we just appended; a
				// concurrent goroutine cannot reach here
				// because we hold me.mu, but the map could
				// still have a stale entry from a previous
				// rollback if the same sv was re-introduced.
				if cur, ok := me.entriesIdx[last]; ok && cur == n-1 {
					delete(me.entriesIdx, last)
				}
			}
			if mutCluster != "" {
				if cl := me.ByCluster[mutCluster]; len(cl) > 0 {
					popped := cl[len(cl)-1]
					me.ByCluster[mutCluster] = cl[:len(cl)-1]
					delete(me.byClusterSet[mutCluster], popped)
					if len(me.ByCluster[mutCluster]) == 0 {
						delete(me.ByCluster, mutCluster)
						delete(me.byClusterSet, mutCluster)
					}
				}
			}
		case mutAppendCl:
			if cl := me.ByCluster[mutCluster]; len(cl) > 0 {
				popped := cl[len(cl)-1]
				me.ByCluster[mutCluster] = cl[:len(cl)-1]
				delete(me.byClusterSet[mutCluster], popped)
				if len(me.ByCluster[mutCluster]) == 0 {
					delete(me.ByCluster, mutCluster)
					delete(me.byClusterSet, mutCluster)
				}
			}
		}
	}
	if saveMeta {
		metajson, merr := json.Marshal(me)
		if merr != nil {
			log.Printf("ERROR: Log Processor: could not jsonify metadata for %s key %s: %s; rolling back in-memory meta", fn, k, merr)
			rollback()
			return 0, false
		}
		// First-sighting bin list bookkeeping. Lock-free probe;
		// addNames takes the per-list mutex once on a true miss
		// and publishes a copy-on-written snapshot. The hot
		// ProcessLogs worker uses the same helpers, so this path
		// shares cache lines and never contends with itself.
		if !i.binList.containsName(k) {
			i.binList.addNames([]string{k})
		}
		if perr := i.db.Put(i.patterns.LabelsSetName, k, db.Row{labelsValueCol: db.Str(string(metajson))}); perr != nil {
			log.Printf("ERROR: Log Processor: could not store metadata for %s key %s: %s; rolling back in-memory meta", fn, k, perr)
			rollback()
			return 0, false
		}
	}
	return idx, true
}

// storeBinList persists the canonical BinNames slice into the BINLIST
// row of the labels set. It is called (a) from the head of every
// saveProgress() so the on-disk pair (BINLIST, progress) is
// consistent at every persisted checkpoint — the crash-resume
// contract relies on this — and (b) once at the end of ProcessLogs
// as a clean-shutdown final flush.
//
// We intentionally do NOT hold binList.lock across the db.Put. A
// concurrent addNames (the COW writer that publishes a new snapshot)
// would otherwise block for the whole Pebble round-trip, briefly
// stalling the hot-path workers that actually need to add a new
// column. Instead we snapshot BinNames under the lock, drop the
// lock, do the Put, then re-take the lock and only clear `changed`
// if BinNames did not grow during the Put — otherwise the next
// caller re-flushes.
func (i *Ingest) storeBinList() error {
	if i.binList == nil {
		return nil
	}
	// No labels set configured (test fixtures and minimal Init
	// paths can leave patterns nil or LabelsSetName empty). Treat
	// as "nothing to persist" rather than failing — the in-memory
	// state is still correct and saveProgress() will keep calling
	// us, this just makes the call cheap.
	if i.patterns == nil || i.patterns.LabelsSetName == "" {
		return nil
	}
	i.binList.lock.Lock()
	if !i.binList.changed {
		i.binList.lock.Unlock()
		return nil
	}
	namesCopy := make([]string, len(i.binList.BinNames))
	copy(namesCopy, i.binList.BinNames)
	i.binList.lock.Unlock()
	binListJson, err := json.Marshal(namesCopy)
	if err != nil {
		return err
	}
	if err := i.db.Put(i.patterns.LabelsSetName, "BINLIST", db.Row{labelsValueCol: db.Str(string(binListJson))}); err != nil {
		return err
	}
	i.binList.lock.Lock()
	if len(i.binList.BinNames) == len(namesCopy) {
		i.binList.changed = false
	}
	i.binList.lock.Unlock()
	return nil
}

// buildDataRow translates the dynamic map[string]any shape produced by the
// log stream into a typed db.Row. The timestamp column is forced to int64
// because that set's indexed column requires it. Every other Go scalar
// type is mapped to its native db.Value so that floats keep their
// precision and bools/bytes round-trip without going through fmt.Sprint.
//
// Anything truly unrecognised (struct, slice, map) falls through to
// db.Str(fmt.Sprint(v)) — that's a last-resort coercion, not a
// normal path.
//
// Returns an error when the timestamp column is missing or carries a
// type we cannot sensibly cast to int64. Silently writing a row with
// timestamp=0 (the old behaviour) created a phantom point at the unix
// epoch that Grafana rendered on every timeseries panel; the log
// pipeline already treats errNoTimestamp rows as non-stat, so this
// only ever triggers on malformed pattern output and the line should
// be dropped rather than injected.
func buildDataRow(data map[string]any, timestampCol string) (db.Row, error) {
	row := make(db.Row, len(data))
	tsSeen := false
	for k, v := range data {
		if k == timestampCol {
			tsSeen = true
			switch vt := v.(type) {
			case int64:
				row[k] = db.Int(vt)
			case int:
				row[k] = db.Int(int64(vt))
			case int32:
				row[k] = db.Int(int64(vt))
			case uint64:
				row[k] = db.Int(int64(vt))
			case uint32:
				row[k] = db.Int(int64(vt))
			default:
				return nil, fmt.Errorf("timestamp column %q has non-integer type %T (value=%v); dropping row", timestampCol, v, v)
			}
			continue
		}
		switch vt := v.(type) {
		case int:
			row[k] = db.Int(int64(vt))
		case int64:
			row[k] = db.Int(vt)
		case int32:
			row[k] = db.Int(int64(vt))
		case uint64:
			row[k] = db.Int(int64(vt))
		case uint32:
			row[k] = db.Int(int64(vt))
		case float64:
			row[k] = db.Float(vt)
		case float32:
			row[k] = db.Float(float64(vt))
		case bool:
			row[k] = db.BoolV(vt)
		case []byte:
			row[k] = db.BytesV(vt)
		case string:
			row[k] = db.Str(vt)
		default:
			row[k] = db.Str(fmt.Sprint(v))
		}
	}
	if !tsSeen {
		return nil, fmt.Errorf("timestamp column %q missing from row", timestampCol)
	}
	return row, nil
}

// putData writes a data row, retrying on a type-conflict error after
// coercing every offending column to the type the schema already has
// recorded. The embedded db enforces per-column types; this helper
// reconciles the case where a pattern legitimately emits the same
// column with two compatible representations (e.g. int once and a
// string-of-int later) without failing the whole row.
//
// Coercion rules (incoming → existing column):
//
//   - any → existing string : format via strconv (int / float / bool)
//   - string → existing int : strconv.ParseInt; drop column on failure
//   - float → existing int  : refuse (float→int is lossy and signals a
//     pattern bug); drop column instead of silently truncating
//   - any → existing float  : strconv.ParseFloat for strings, widen for
//     ints; drop column otherwise
//
// The timestamp column is never coerced; its schema type is pinned to
// int64 at RegisterSet time. Error classification uses the
// db.ErrColumnTypeConflict sentinel via errors.Is so we don't rely on
// the exact wording of the db layer's error messages.
//
// Dropped columns mean the row goes through with one less field,
// which is preferable to losing every other bin in the row.
//
// Retry policy: on a conflict we re-read SchemaOf and coerce. Two
// goroutines writing the same previously-unseen column concurrently
// can race — worker B's Put can miss A's column-addition and trip
// its own ErrColumnTypeConflict on the retry. We bound the retry
// count (putDataMaxRetries) so the helper always terminates; each
// iteration re-fetches the schema so we eventually observe the
// stable resolved type.
//
// 5 retries (was 3) absorbs first-batch storms where MaxPutThreads
// goroutines all race to introduce the same brand-new columns: each
// retry re-reads SchemaOf, so once any one writer commits the type
// the rest converge in a single pass. Beyond ~5 the pathology is no
// longer "concurrent first-write" but a real bug, and looping
// forever would hide it.
const putDataMaxRetries = 5

// putData hands the row to the per-set putBatcher (AssumeNew=true) on
// the ingest hot path. The batcher accumulates rows and flushes them
// via db.PutBatch in PutBatchSize chunks; on a column-type conflict
// the batcher falls back to putDataSingle on each affected row.
//
// Returns nil eagerly: the batcher commits asynchronously and surfaces
// its own errors via log.Printf. The pre-batching contract (return nil
// on success, error on type-conflict-after-retries) is preserved for
// callers that expect the function to return — but with batching the
// error visibility moved into the flusher's log output. ProcessLogs's
// callers only used the return value to log; they did not gate
// downstream work on it.
func (i *Ingest) putData(set, key string, row db.Row) error {
	if i.putBatcher == nil {
		// Tests construct Ingest without finalizeInit and call
		// putData directly; fall back to the synchronous path.
		return i.putDataSingle(set, key, row)
	}
	i.putBatcher.submit(set, key, row)
	return nil
}

// putDataSingle is the legacy synchronous path: one row, with retry on
// db.ErrColumnTypeConflict. The batcher uses it as the per-row
// fallback when a PutBatch fails atomically due to a type conflict.
// Tests that construct an Ingest without finalizeInit also reach this
// path via putData.
func (i *Ingest) putDataSingle(set, key string, row db.Row) error {
	current := row
	var lastErr error
	for attempt := 0; attempt < putDataMaxRetries; attempt++ {
		err := i.db.Put(set, key, current)
		if err == nil {
			return nil
		}
		if !errors.Is(err, db.ErrColumnTypeConflict) {
			return err
		}
		lastErr = err
		schema, ok := i.db.SchemaOf(set)
		if !ok {
			// Set may have been dropped mid-flight, or the db was
			// closed. Either way we can't coerce without schema
			// knowledge; surface the original error.
			return err
		}
		current = i.coerceRow(current, schema)
	}
	log.Printf("WARN: putData: %q key %q: type conflict persisted after %d retries: %s", set, key, putDataMaxRetries, lastErr)
	return lastErr
}

// coerceRow applies the per-column coercion rules documented on
// putData. It is a pure function of (row, schema) so repeated calls
// on a row whose schema has stabilised are idempotent.
func (i *Ingest) coerceRow(row db.Row, schema []db.ColumnSpec) db.Row {
	types := make(map[string]db.ColumnType, len(schema))
	for _, col := range schema {
		types[col.Name] = col.Type
	}
	ts := i.config.TimestampColumnName
	coerced := make(db.Row, len(row))
	for k, v := range row {
		if k == ts {
			coerced[k] = v
			continue
		}
		want, known := types[k]
		if !known {
			coerced[k] = v
			continue
		}
		if v.Type() == want {
			coerced[k] = v
			continue
		}
		switch want {
		case db.TypeString:
			if iv, ok := v.AsInt(); ok {
				coerced[k] = db.Str(strconv.FormatInt(iv, 10))
				continue
			}
			if fv, ok := v.AsFloat(); ok {
				coerced[k] = db.Str(strconv.FormatFloat(fv, 'f', -1, 64))
				continue
			}
			if bv, ok := v.AsBool(); ok {
				coerced[k] = db.Str(strconv.FormatBool(bv))
				continue
			}
			if sv, ok := v.AsString(); ok {
				coerced[k] = db.Str(sv)
				continue
			}
		case db.TypeInt64:
			// Refuse float→int: silently truncating real-valued
			// metrics into an int column is the kind of corruption
			// that takes weeks to notice. Drop instead.
			if v.Type() == db.TypeFloat64 {
				log.Printf("WARN: putData: column %q is int but pattern emitted float; dropping", k)
				continue
			}
			if sv, ok := v.AsString(); ok {
				if iv, perr := strconv.ParseInt(sv, 10, 64); perr == nil {
					coerced[k] = db.Int(iv)
					continue
				}
			}
		case db.TypeFloat64:
			if iv, ok := v.AsInt(); ok {
				coerced[k] = db.Float(float64(iv))
				continue
			}
			if sv, ok := v.AsString(); ok {
				if fv, perr := strconv.ParseFloat(sv, 64); perr == nil {
					coerced[k] = db.Float(fv)
					continue
				}
			}
		}
	}
	return coerced
}

func (i *Ingest) processLogsFeed(foundLogs map[string]*LogFile, resultsChan chan *processResult, shards *metaShards) {
	// resultsChan MUST be closed exactly once, even if a per-file
	// goroutine or this dispatcher itself panics. Otherwise the
	// consumer in ProcessLogs (`for data := range resultsChan`)
	// blocks forever, the WaitGroup never returns, and Ingest.Close
	// is never reached. The outer recover catches a panic in the
	// dispatch loop or the wg.Wait path; the per-goroutine recover
	// catches a panic in processLogFile so the WaitGroup count still
	// drains. Both are logged so the failure remains visible.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: Log Processor: processLogsFeed panic: %v", r)
		}
		close(resultsChan)
	}()
	wg := new(sync.WaitGroup)
	// Resolve MaxConcurrentLogFiles. 0 means auto-scale with
	// GOMAXPROCS (which respects cgroup CPU limits in Go 1.21+,
	// so this works correctly inside both bare-metal and Docker
	// AGI deployments). See struct.go for the rationale on the
	// 4..16 clamp.
	maxConcurrent := i.config.Processor.MaxConcurrentLogFiles
	if maxConcurrent <= 0 {
		maxConcurrent = runtime.GOMAXPROCS(0)
		if maxConcurrent < 4 {
			maxConcurrent = 4
		}
		if maxConcurrent > 16 {
			maxConcurrent = 16
		}
	}
	threads := make(chan bool, maxConcurrent)
	for n, f := range foundLogs {
		if f.Finished {
			continue
		}
		threads <- true
		wg.Add(1)
		go func(n string, f *LogFile) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("ERROR: Log Processor: processLogFile panic on %s: %v", n, r)
				}
				<-threads
				wg.Done()
			}()
			labels := map[string]any{
				"ClusterName": f.ClusterName,
				"NodeIdent":   f.NodePrefix + "_" + f.NodeID,
			}
			fd, err := os.Open(n)
			if err != nil {
				resultsChan <- &processResult{
					FileName: n,
					Error:    err,
				}
				return
			}
			defer fd.Close()
			if f.Processed > 0 && f.Processed < f.Size {
				move := f.Processed - int64(i.config.Processor.LogReadBufferSizeKb*1024*2)
				if move > 0 {
					//nolint:errcheck
					fd.Seek(move, 0)
				}
			}
			nprefix, _ := strconv.Atoi(f.NodePrefix)
			i.processLogFile(n, fd, resultsChan, shards, labels, nprefix, f.ClusterName+"::/::"+f.NodePrefix+"_"+f.NodeID)
		}(n, f)
	}
	wg.Wait()
}

type processResult struct {
	FileName string
	Data     map[string]any
	// Resolved carries pre-translated label indices (key -> idx into
	// metaEntries.Entries). The parser-side processLogFile populates
	// this via resolveLabelsCached so the per-row worker pool can
	// stamp indices into Data without touching metaShards or
	// metaEntries.mu — the single biggest source of lock contention
	// in the legacy worker loop. Keys with translation failures (or
	// non-string values that resolveLabelsCached refused) are simply
	// absent from Resolved; the worker behaves identically to the
	// pre-fix per-key error path (skip translation, keep going).
	Resolved       map[string]int
	Error          error
	SetName        string
	LogLine        string
	UniqNodeString string
}

func (i *Ingest) processLogFile(fileName string, fd *os.File, resultsChan chan *processResult, shards *metaShards, labels map[string]any, nodePrefix int, uniqNodeString string) {
	i.progress.Lock()
	i.progress.LogProcessor.Files[fileName].StartTime = time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"
	i.progress.LogProcessor.changed = true
	i.progress.Unlock()

	_, fn := path.Split(fileName)
	var unmatched *os.File
	var err error
	s := bufio.NewScanner(fd)
	buffer := make([]byte, i.config.Processor.LogReadBufferSizeKb*1024)
	s.Buffer(buffer, i.config.Processor.LogReadBufferSizeKb*1024)
	loc := int64(0)
	timer := time.Now()
	stepper := i.config.ProgressPrint.UpdateInterval / 2
	logDir, fileNameOnly := path.Split(fileName)
	_, clusterName := path.Split(strings.Trim(logDir, "/"))
	stream := newLogStream(clusterName, i.patterns, &i.config.IngestTimeRanges, i.config.TimestampColumnName)
	// Per-file (key, value) -> idx cache. Owned by this goroutine, no
	// synchronisation needed. The label resolution path (formerly run
	// once per row in every one of the 128 worker goroutines) becomes
	// "miss once per (file, key, value) combination" — for AGI's
	// label cardinality (~10 keys with a handful of distinct values
	// each) the cache hits >99% of resolves after the first dozen
	// rows, taking the contended metaEntries.mu out of the steady-
	// state hot path entirely.
	cache := make(resolveCache, len(labels)+4)
	// File-scope labels are constant for the duration of this file;
	// resolve them ONCE here so the per-line emit loop doesn't have
	// to even re-walk the cache for them. resolvedLabels is shared
	// (read-only after this point) by every emit below; the worker
	// pool reads it via processResult.Resolved.
	resolvedLabels := i.resolveLabelsCached(labels, cache, shards, clusterName, fn)
	for s.Scan() {
		if err = s.Err(); err != nil {
			resultsChan <- &processResult{
				Error: fmt.Errorf("could not read input file: %s", err),
			}
			return
		}
		line := s.Text()
		out, err := stream.Process(line, nodePrefix)
		if err != nil && err != errNotMatched && err != errNoTimestamp && !strings.HasPrefix(err.Error(), "TIME PARSE:") {
			log.Printf("ERROR: Stream Processor for line: %s", err)
			i.progress.LogProcessor.LineErrors.add(nodePrefix, err.Error())
			continue
		}
		if len(out) == 0 && err != nil && (err == errNotMatched || err == errNoTimestamp || strings.HasPrefix(err.Error(), "TIME PARSE:")) {
			if unmatched == nil {
				noStatDir := path.Join(i.config.Directories.NoStatLogs, labels["ClusterName"].(string))
				if mkErr := os.MkdirAll(noStatDir, 0755); mkErr != nil {
					log.Printf("ERROR: Could not create no-stat directory %s: %s", noStatDir, mkErr)
				}
				unmatched, err = os.Create(path.Join(noStatDir, fn))
				if err != nil {
					log.Printf("ERROR: Could not create file for non-stat: %s", err)
				} else {
					defer unmatched.Close()
				}
			}
			if unmatched != nil {
				_, err = unmatched.WriteString(line + "\n")
				if err != nil {
					log.Printf("ERROR: Could not write no-stat: %s", err)
				}
			}
			continue
		}
		for _, d := range out {
			// Int-coercion of d.Data now happens inside
			// lineProcess (see logstream.go) so the per-row map
			// allocation + copy that lived here is gone. d.Data
			// is owned by this row; no other goroutine reads it.
			resultsChan <- &processResult{
				FileName:       fileName,
				Data:           d.Data,
				Resolved:       i.mergeResolved(resolvedLabels, d.Metadata, cache, shards, clusterName, fn),
				Error:          d.Error,
				SetName:        d.SetName,
				LogLine:        d.Line,
				UniqNodeString: uniqNodeString,
			}
		}
		if time.Since(timer) > stepper {
			newloc, _ := fd.Seek(0, 1)
			if newloc > 0 && newloc != loc {
				loc = newloc
				i.progress.Lock()
				i.progress.LogProcessor.Files[fileName].Processed = loc
				i.progress.LogProcessor.changed = true
				i.progress.Unlock()
			}
			timer = time.Now()
		}
	}
	out, startTime, endTime := stream.Close()
	for _, d := range out {
		resultsChan <- &processResult{
			FileName:       fileName,
			Data:           d.Data,
			Resolved:       i.mergeResolved(resolvedLabels, d.Metadata, cache, shards, clusterName, fn),
			Error:          d.Error,
			SetName:        d.SetName,
			LogLine:        d.Line,
			UniqNodeString: uniqNodeString,
		}
	}
	// store startTime and endTime of logs
	for _, point := range []time.Time{startTime, endTime} {
		if point.IsZero() {
			continue
		}
		nodePrefix, err := strconv.Atoi(strings.Split(fileNameOnly, "_")[0])
		if err != nil {
			continue
		}
		// Synthetic "fileName" label per-emit; cache hits on the
		// second timestamp emit for this file.
		extra := map[string]any{"fileName": clusterName + "/" + fileNameOnly}
		resultsChan <- &processResult{
			FileName: fileName,
			Data: map[string]any{
				"nodePrefix":                 nodePrefix,
				i.config.TimestampColumnName: point.UnixMilli(),
			},
			Resolved:       i.mergeResolved(resolvedLabels, extra, cache, shards, clusterName, fn),
			Error:          nil,
			SetName:        i.config.LogFileRangesSetName,
			LogLine:        fmt.Sprintf("%s:%d", fileName, point.UnixMilli()),
			UniqNodeString: uniqNodeString,
		}
	}

	// done
	i.progress.Lock()
	i.progress.LogProcessor.Files[fileName].Processed = i.progress.LogProcessor.Files[fileName].Size
	i.progress.LogProcessor.Files[fileName].Finished = true
	i.progress.LogProcessor.Files[fileName].FinishTime = time.Now().UTC().Format("2006-01-02 15:04:05") + " UTC"
	i.progress.LogProcessor.changed = true
	i.progress.Unlock()
}

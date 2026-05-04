package ingest

import (
	"errors"
	"hash/maphash"
	"log"
	"runtime"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// putBatcher streams ingest metric rows into per-set buffers and flushes
// each buffer via db.PutBatch (with AssumeNew=true) when it reaches
// flushSize entries OR ages past flushAge. The batcher fans out to N
// shard goroutines (see newPutBatcher) so the Pebble commit pipeline
// is no longer gated by a single flusher; each shard maintains its
// own pending map and runs db.PutBatch independently. db.PutBatch is
// concurrency-safe (acquire/release uses an RWMutex; row mutations
// are protected by stripedLocks keyed by (setID, PK), and shards
// route by maphash(key) so two shards essentially never contend on
// the same stripe).
//
// AssumeNew is correct here because metric primary keys are constructed
// as <nodeIdentifier>::/::<logLine> — the log line includes a byte
// offset which is unique per (cluster, node, file). Even if a previous
// ingest session processed the same byte offset and orphaned an old
// I/ entry, indexScanIter's orphan-skip guard (gated by
// d.assumeNewSeen) drops the stale entry on read. Age-out reclaims the
// orphan permanently when its ts falls below the retention window.
//
// On db.ErrColumnTypeConflict the affected shard's batch fails
// atomically; we then fall back to the legacy single-row putData
// path, which retries with type-coercion. Different shards commit
// independent batches, so a type conflict on one shard never
// rolls back rows that already committed on another shard — that
// matches the pre-sharding semantics where flushes for different
// sets were already independent.
type putBatcher struct {
	db        *db.DB
	flushSize int
	flushAge  time.Duration
	fallback  func(set, key string, row db.Row) error

	// shards owns N flusher goroutines. Producers route into one
	// of them via maphash(key) % nShards; the seed is randomised
	// per process so adversarial key distributions cannot pin all
	// traffic to one shard across runs.
	shards  []*batcherShard
	nShards uint32
	seed    maphash.Seed
}

type batcherShard struct {
	inCh chan putReq
	done chan struct{}
}

type putReq struct {
	set string
	key string
	row db.Row
}

// newPutBatcher constructs and starts the flusher goroutines. flushSize
// and flushAge defaults are applied when the caller passes 0/<=0; that
// preserves the "leave config zero to take the package default" idiom
// the rest of the ingest config uses. shardCount<=0 selects an auto
// value of min(GOMAXPROCS, 8) which matches the parallelism the
// upstream worker pool can sustain on AGI's typical hardware without
// adding scheduler churn for small boxes.
func newPutBatcher(d *db.DB, flushSize, flushAgeMs, shardCount int, fallback func(string, string, db.Row) error) *putBatcher {
	if flushSize <= 0 {
		// Mirrors the yaml default on Config.PutBatchSize. Kept
		// in sync there so the in-code fallback (callers that
		// pass 0) and the config-file default produce identical
		// behaviour.
		flushSize = 1024
	}
	if flushAgeMs <= 0 {
		flushAgeMs = 50
	}
	if shardCount <= 0 {
		// Auto: align with the GOMAXPROCS budget so the batcher
		// can keep up with the parallel worker fan-in. Capped at
		// 16 because beyond that Pebble's commit pipeline starts
		// to serialise the prepare phase across writers and the
		// scheduler cost of additional flusher goroutines stops
		// paying for itself.
		//
		// The cap was 8 historically, sized when the upstream
		// AssumeNew lock-skip in db.PutBatch had not yet landed
		// and a single shard's commit window was several ms long.
		// With the lock-skip shrinking the per-batch CPU cost,
		// the batcher became the new pipeline gate (workers spent
		// ~90% of wall time blocked on per-shard inCh sends in
		// the post-AssumeNew block profile). Doubling the auto
		// cap lets the batcher absorb the fan-in from the parser
		// pool without back-pressuring all the way upstream on
		// 16+ vCPU hosts.
		shardCount = runtime.GOMAXPROCS(0)
		if shardCount < 1 {
			shardCount = 1
		}
		if shardCount > 16 {
			shardCount = 16
		}
	}
	// Per-shard backlog mirrors the legacy single-shard depth
	// (flushSize*4) so each shard absorbs the same steady-state
	// burst the old single inCh did. Aggregate memory is N×4×
	// flushSize×row_size which is sub-MB at default settings.
	perShard := flushSize * 4
	b := &putBatcher{
		db:        d,
		flushSize: flushSize,
		flushAge:  time.Duration(flushAgeMs) * time.Millisecond,
		fallback:  fallback,
		nShards:   uint32(shardCount),
		seed:      maphash.MakeSeed(),
	}
	b.shards = make([]*batcherShard, shardCount)
	for i := 0; i < shardCount; i++ {
		sh := &batcherShard{
			inCh: make(chan putReq, perShard),
			done: make(chan struct{}),
		}
		b.shards[i] = sh
		go b.run(sh)
	}
	return b
}

// submit queues a row for batched insertion. Producers must NOT mutate
// row after calling submit — the row is forwarded by reference into a
// db.PutItem inside the flusher and read concurrently with the
// producer's next iteration.
//
// Routing: maphash(key) % nShards. We hash the key (not set+key)
// because (a) AGI's primary metrics keys are already XXH3-128 hex
// strings so they're uniformly distributed on their own, and (b)
// keying purely on PK keeps a given row's writes always on the same
// shard, which preserves the original "single-flusher per row"
// ordering guarantee the legacy batcher relied on for type-conflict
// recovery semantics.
func (b *putBatcher) submit(set, key string, row db.Row) {
	var idx uint32
	if b.nShards == 1 {
		idx = 0
	} else {
		h := uint32(maphash.String(b.seed, key))
		idx = h % b.nShards
	}
	b.shards[idx].inCh <- putReq{set: set, key: key, row: row}
}

// close signals every shard to drain its in-flight batches and exit.
// Returns once all shards have committed everything they had buffered.
// It is safe to call close multiple times only via sync.Once at the
// caller; this method does not idempotency-guard close(inCh) on the
// per-shard channels.
func (b *putBatcher) close() {
	for _, sh := range b.shards {
		close(sh.inCh)
	}
	for _, sh := range b.shards {
		<-sh.done
	}
}

func (b *putBatcher) run(sh *batcherShard) {
	defer close(sh.done)
	pending := make(map[string][]db.PutItem)
	timer := time.NewTimer(b.flushAge)
	defer timer.Stop()

	flush := func(set string) {
		items := pending[set]
		if len(items) == 0 {
			return
		}
		delete(pending, set)
		err := b.db.PutBatch(set, items)
		if err == nil {
			return
		}
		if errors.Is(err, db.ErrColumnTypeConflict) {
			// Type conflict is rare and requires per-row
			// coercion; fall back to the legacy putData path,
			// which loops with coerceRow. We deliberately do
			// not re-issue PutBatch after coercion: a single
			// row's type may differ between adjacent items in
			// the same batch, and the fallback resolves each
			// row's coercion against the latest schema.
			for _, it := range items {
				if ferr := b.fallback(set, it.Key, it.Row); ferr != nil {
					log.Printf("ERROR: putBatcher: per-row fallback %q/%q: %s", set, it.Key, ferr)
				}
			}
			return
		}
		log.Printf("ERROR: putBatcher: PutBatch on %q (%d rows): %s", set, len(items), err)
	}
	flushAll := func() {
		for set := range pending {
			flush(set)
		}
	}

	for {
		select {
		case req, ok := <-sh.inCh:
			if !ok {
				flushAll()
				return
			}
			pending[req.set] = append(pending[req.set], db.PutItem{
				Key:       req.key,
				Row:       req.row,
				AssumeNew: true,
			})
			if len(pending[req.set]) >= b.flushSize {
				flush(req.set)
			}
		case <-timer.C:
			flushAll()
			timer.Reset(b.flushAge)
		}
	}
}

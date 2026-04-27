package ingest

import (
	"errors"
	"log"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// putBatcher streams ingest metric rows into per-set buffers and flushes
// each buffer via db.PutBatch (with AssumeNew=true) when it reaches
// flushSize entries OR ages past flushAge. The single flusher goroutine
// is the only writer to the metric sets during ingest, so per-batch
// commits run without contention against the per-row Pebble Batch
// commits the legacy putData hot path used to produce.
//
// AssumeNew is correct here because metric primary keys are constructed
// as <nodeIdentifier>::/::<logLine> — the log line includes a byte
// offset which is unique per (cluster, node, file). Even if a previous
// ingest session processed the same byte offset and orphaned an old
// I/ entry, indexScanIter's orphan-skip guard (gated by
// d.assumeNewSeen) drops the stale entry on read. Age-out reclaims the
// orphan permanently when its ts falls below the retention window.
//
// On db.ErrColumnTypeConflict the whole batch fails atomically; we then
// fall back to the legacy single-row putData path, which retries with
// type-coercion. This keeps the type-conflict recovery semantics
// identical to the pre-batching behaviour even though the happy path
// commits in batches of putBatchSize rows.
type putBatcher struct {
	db        *db.DB
	inCh      chan putReq
	flushSize int
	flushAge  time.Duration
	done      chan struct{}
	fallback  func(set, key string, row db.Row) error
}

type putReq struct {
	set string
	key string
	row db.Row
}

// newPutBatcher constructs and starts the flusher goroutine. flushSize
// and flushAge defaults are applied when the caller passes 0/<=0; that
// preserves the "leave config zero to take the package default" idiom
// the rest of the ingest config uses.
func newPutBatcher(d *db.DB, flushSize, flushAgeMs int, fallback func(string, string, db.Row) error) *putBatcher {
	if flushSize <= 0 {
		flushSize = 256
	}
	if flushAgeMs <= 0 {
		flushAgeMs = 50
	}
	b := &putBatcher{
		db: d,
		// 4× flushSize backlog: when 128 putData callers fan in,
		// the channel absorbs a steady-state burst without
		// blocking the producers. Larger backlogs delay shutdown
		// (drain on close) and consume memory but cannot cause
		// correctness issues because ProcessLogs waits for both
		// the producer wg and putBatcher.close() before declaring
		// the step complete.
		inCh:      make(chan putReq, flushSize*4),
		flushSize: flushSize,
		flushAge:  time.Duration(flushAgeMs) * time.Millisecond,
		done:      make(chan struct{}),
		fallback:  fallback,
	}
	go b.run()
	return b
}

// submit queues a row for batched insertion. Producers must NOT mutate
// row after calling submit — the row is forwarded by reference into a
// db.PutItem inside the flusher and read concurrently with the
// producer's next iteration.
func (b *putBatcher) submit(set, key string, row db.Row) {
	b.inCh <- putReq{set: set, key: key, row: row}
}

// close signals the flusher to drain the in-flight batches and exit.
// Returns once the flusher has committed everything it had buffered.
// It is safe to call close multiple times only via sync.Once at the
// caller; this method does not idempotency-guard close(inCh).
func (b *putBatcher) close() {
	close(b.inCh)
	<-b.done
}

func (b *putBatcher) run() {
	defer close(b.done)
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
		case req, ok := <-b.inCh:
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

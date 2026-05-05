package ingest

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"
)

// Live mode contract for callers (typically pkg/agi/livelisten):
//
//   1. AllocLiveStream(cluster, nodeID) returns a stable nodePrefix int.
//      Allocation reuses the same monotonic prefix counter as the
//      batch pre-processor so a (cluster, node) seen by both paths
//      gets the same prefix. Stream from a freshly-restarted AGI
//      keeps its prefix when reconnecting because the counter and
//      nodeToPrefix map are persisted via saveProgress.
//
//   2. NewLogStream(cluster) constructs a parser bound to the
//      patterns Defs entry that matches the cluster name (or the
//      default Defs[0] when no entry matches). Each connection
//      owns its own logStream — never share between goroutines.
//
//   3. NewResolveCache() / ResolveLiveLabels: per-connection memo
//      that short-circuits the metaShards path on the hot row-emit
//      loop. The first occurrence of (key, value) takes the parent
//      lock once; every subsequent row is an O(1) map probe.
//
//   4. StartLiveWorkers(ctx, n) spawns N goroutines that drain
//      LiveResultsChan() (constructed lazily on first call). The
//      worker body is the shared runWorkerPool used by the batch
//      pipeline. Submit *ProcessResult pointers via
//      SubmitLiveResult.
//
//   5. PutBatcherRetain / PutBatcherRelease guard the putBatcher
//      teardown so the batch path's deferred Close cannot tear
//      down the batcher while live streams are still submitting
//      rows.
//
// All public functions in this file are safe for concurrent use
// unless documented otherwise.

// ErrLiveStreamLimit is returned when the in-flight stream count
// would exceed Config.Live.MaxStreams. Returned through the
// listener's HTTP layer as 429.
var ErrLiveStreamLimit = errors.New("ingest: live stream limit reached")

// AllocLiveStream returns the stable nodePrefix for (cluster, nodeID).
// First sighting takes the progress lock and increments
// PreProcessor.LastUsedPrefix; subsequent sightings hit the
// NodeToPrefix map under RLock. The same allocator backs the batch
// PreProcess path, so mixing batch + live ingest produces consistent
// prefixes across both paths.
//
// The returned prefix is the integer used in the
// "<prefix>_<nodeID>" NodeIdent label that ProcessLogs writes.
func (i *Ingest) AllocLiveStream(cluster, nodeID string) int {
	key := cluster + "_" + nodeID
	i.progress.Lock()
	if i.progress.PreProcessor.NodeToPrefix == nil {
		i.progress.PreProcessor.NodeToPrefix = make(map[string]int)
	}
	if i.progress.PreProcessor.LastUsedSuffixForPrefix == nil {
		i.progress.PreProcessor.LastUsedSuffixForPrefix = make(map[int]int)
	}
	prefix, ok := i.progress.PreProcessor.NodeToPrefix[key]
	if !ok {
		i.progress.PreProcessor.LastUsedPrefix++
		prefix = i.progress.PreProcessor.LastUsedPrefix
		i.progress.PreProcessor.NodeToPrefix[key] = prefix
		i.progress.PreProcessor.LastUsedSuffixForPrefix[prefix] = 1
		i.progress.PreProcessor.changed = true
	}
	i.progress.Unlock()
	return prefix
}

// LiveStream couples a *logStream parser with the per-connection
// resolveCache and the file-scope (cluster, NodeIdent) labels needed
// to drive the row-emit fast path. The returned object is owned by
// exactly one goroutine and must not be shared across connections.
type LiveStream struct {
	stream         *logStream
	cache          resolveCache
	resolved       map[string]int
	clusterName    string
	sourceLabel    string
	nodePrefix     int
	uniqNodeString string
	shards         *metaShards
	ingest         *Ingest
}

// NewLiveStream allocates a LiveStream for one (cluster, nodeID,
// source) tuple. Pre-resolves the file-scope ClusterName / NodeIdent
// labels so the per-line emit loop never has to even re-walk the
// resolveCache for them.
//
// shards is shared across every active live stream in this Ingest
// (constructed in cmdAgiExecService and passed into the listener).
// Sharing the metaShards across streams is correct because the
// underlying meta entries map is keyed by label key, not source —
// two streams writing the same ClusterName resolve to the same
// metaEntries entry and both see the same idx for it, which
// matches the batch path's behaviour where every parser
// goroutine resolves into the same shared metaShards.
func (i *Ingest) NewLiveStream(shards *metaShards, cluster, nodeID, source string) *LiveStream {
	prefix := i.AllocLiveStream(cluster, nodeID)
	uniq := cluster + "::/::" + strconv.Itoa(prefix) + "_" + nodeID
	cache := make(resolveCache, 6)
	labels := map[string]any{
		"ClusterName": cluster,
		"NodeIdent":   strconv.Itoa(prefix) + "_" + nodeID,
	}
	resolved := i.resolveLabelsCached(labels, cache, shards, cluster, source)
	return &LiveStream{
		stream:         newLogStream(cluster, i.patterns, &i.config.IngestTimeRanges, i.config.TimestampColumnName),
		cache:          cache,
		resolved:       resolved,
		clusterName:    cluster,
		sourceLabel:    source,
		nodePrefix:     prefix,
		uniqNodeString: uniq,
		shards:         shards,
		ingest:         i,
	}
}

// NodePrefix returns the stable per-(cluster,node) prefix this stream
// uses in the NodeIdent label.
func (s *LiveStream) NodePrefix() int { return s.nodePrefix }

// Process feeds one input line through the underlying parser and
// emits zero or more rows onto the live results channel of the
// owning Ingest. Errors are logged and absorbed (matching the batch
// path's per-line error policy: a single bad line never tears down
// the stream).
func (s *LiveStream) Process(line string) error {
	out, err := s.stream.Process(line, s.nodePrefix)
	if err != nil && err != errNotMatched && err != errNoTimestamp {
		// Time-parse errors (the "TIME PARSE: ..." family) are
		// logged but not returned: the dispatcher would just
		// reconnect on a returned error and we'd lose the
		// remainder of the chunk.
		s.ingest.progress.Lock()
		if s.ingest.progress.LogProcessor.LineErrors == nil {
			s.ingest.progress.LogProcessor.LineErrors = &lineErrors{}
		}
		s.ingest.progress.Unlock()
		s.ingest.progress.LogProcessor.LineErrors.add(s.nodePrefix, err.Error())
	}
	for _, d := range out {
		s.ingest.SubmitLiveResult(&processResult{
			FileName:       s.sourceLabel,
			Data:           d.Data,
			Resolved:       s.ingest.mergeResolved(s.resolved, d.Metadata, s.cache, s.shards, s.clusterName, s.sourceLabel),
			Error:          d.Error,
			SetName:        d.SetName,
			LogLine:        d.Line,
			UniqNodeString: s.uniqNodeString,
		})
	}
	return nil
}

// Close flushes the parser's tail emit (any still-buffered multiline
// or aggregator state) onto the live results channel. Should be
// called when the underlying connection ends.
func (s *LiveStream) Close() {
	out, _, _ := s.stream.Close()
	for _, d := range out {
		s.ingest.SubmitLiveResult(&processResult{
			FileName:       s.sourceLabel,
			Data:           d.Data,
			Resolved:       s.ingest.mergeResolved(s.resolved, d.Metadata, s.cache, s.shards, s.clusterName, s.sourceLabel),
			Error:          d.Error,
			SetName:        d.SetName,
			LogLine:        d.Line,
			UniqNodeString: s.uniqNodeString,
		})
	}
}

// NewLiveMetaShards constructs the shared metaShards passed into
// NewLiveStream. The listener should construct one shards object
// per Ingest and re-use it across every connection.
func (i *Ingest) NewLiveMetaShards() *metaShards {
	return &metaShards{meta: make(map[string]*metaEntries)}
}

// LiveResultsChan returns (and lazily allocates) the shared
// results channel that all live streams submit to. The channel
// capacity matches the batch pipeline's resultsChan buffer.
func (i *Ingest) LiveResultsChan() chan *processResult {
	i.putBatcherMu.Lock()
	defer i.putBatcherMu.Unlock()
	if i.liveResultsChan == nil {
		i.liveResultsChan = make(chan *processResult, 128)
	}
	return i.liveResultsChan
}

// SubmitLiveResult pushes one row onto the live results channel.
// Blocks if the channel is full; the channel buffer (128) is sized
// to absorb single-shard putBatcher commit windows without
// propagating back-pressure all the way up to the network reader.
func (i *Ingest) SubmitLiveResult(r *processResult) {
	if r == nil {
		return
	}
	ch := i.LiveResultsChan()
	ch <- r
}

// StartLiveWorkers spawns n worker goroutines that drain the live
// results channel through runWorkerPool. The workers stop when the
// channel is closed (via StopLiveWorkers) and return; ctx is honored
// only as a coarse "best effort" cancellation signal — once a row
// has been submitted to the channel it WILL be drained, even on
// ctx cancel, so the dirty marker invariant the batch path relies
// on (every row queued is either committed or visible-as-loss in
// the LineErrors tracker) holds for the live path too.
//
// Returns an error if Close has already drained the batcher (live
// mode cannot start once batch shutdown is complete).
func (i *Ingest) StartLiveWorkers(ctx context.Context, n int) error {
	if n <= 0 {
		n = 16
	}
	i.putBatcherMu.Lock()
	if i.putBatcher == nil {
		i.putBatcherMu.Unlock()
		return errors.New("ingest: StartLiveWorkers: putBatcher not initialised (Close already ran?)")
	}
	if i.liveResultsChan == nil {
		i.liveResultsChan = make(chan *processResult, 128)
	}
	ch := i.liveResultsChan
	i.putBatcherMu.Unlock()
	i.liveWG.Add(1)
	go func() {
		defer i.liveWG.Done()
		i.runWorkerPool(ch, n)
	}()
	_ = ctx
	return nil
}

// StopLiveWorkers closes the live results channel and waits for the
// worker goroutines to drain. After this call the live channel is
// nil; a fresh LiveResultsChan / StartLiveWorkers cycle is required
// to re-arm.
func (i *Ingest) StopLiveWorkers() {
	i.putBatcherMu.Lock()
	ch := i.liveResultsChan
	i.liveResultsChan = nil
	i.putBatcherMu.Unlock()
	if ch != nil {
		close(ch)
	}
	i.liveWG.Wait()
}

// PutBatcherRetain pins the putBatcher alive across an additional
// shutdown path. Live mode calls this on Serve start so the batch
// path's deferred Close cannot tear down the batcher while live
// rows are still being submitted. Returns false (and does nothing)
// if the batcher has already been torn down.
func (i *Ingest) PutBatcherRetain() bool {
	i.putBatcherMu.Lock()
	defer i.putBatcherMu.Unlock()
	if i.putBatcher == nil {
		return false
	}
	if i.putBatcherRefs <= 0 {
		i.putBatcherRefs = 1
	}
	i.putBatcherRefs++
	return true
}

// PutBatcherRelease decrements the refcount; when it reaches zero
// the putBatcher is closed (drained + flushed) and set to nil. The
// next caller that tries to submit a row sees a nil batcher and
// falls back to putDataSingle, which is the same behaviour
// putData already used for tests that bypassed finalizeInit.
func (i *Ingest) PutBatcherRelease() {
	var toClose *putBatcher
	i.putBatcherMu.Lock()
	if i.putBatcherRefs > 0 {
		i.putBatcherRefs--
	}
	if i.putBatcherRefs == 0 && i.putBatcher != nil {
		toClose = i.putBatcher
		i.putBatcher = nil
	}
	i.putBatcherMu.Unlock()
	if toClose != nil {
		toClose.close()
	}
}

// HasLiveStreams reports whether any live worker goroutines are
// currently draining. Used by the listener to decide whether to
// refuse new connections (per Config.Live.MaxStreams).
func (i *Ingest) HasLiveStreams() bool {
	i.putBatcherMu.Lock()
	defer i.putBatcherMu.Unlock()
	return i.liveResultsChan != nil
}

// LiveStreamWatchdog logs an info line every interval while the
// listener is running. Pure observability; no side effects on the
// ingest pipeline. Returns when ctx is cancelled.
func (i *Ingest) LiveStreamWatchdog(ctx context.Context, interval time.Duration, count func() int) {
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n := 0
			if count != nil {
				n = count()
			}
			log.Printf("DEBUG: live ingest: %d active stream(s)", n)
		}
	}
}

// EnableWAL reports whether the configured Pebble DB has WAL
// enabled. Live mode requires WAL=on (otherwise the dirty-marker
// mechanism wipes the DB on next start with no source files to
// re-ingest from). The caller is the merged service, which refuses
// to start the listener if this returns false even when
// Config.Live.Enabled=true.
func (i *Ingest) EnableWAL() bool {
	if i.config == nil {
		return false
	}
	return i.config.DB.EnableWAL
}

// LiveSourceLabel returns a human-readable label describing the
// configured live ingest endpoint. Folded into the "sources" label
// in init.go (next to S3 / SFTP / local entries) so Grafana shows
// it alongside the static sources.
func (c *Config) LiveSourceLabel() string {
	if c == nil || !c.Live.Enabled {
		return ""
	}
	return fmt.Sprintf("live %s (max %d streams)", c.Live.ListenAddr, c.Live.MaxStreams)
}

package ingest

import (
	"errors"
	"fmt"
	"log"
	"strings"
)

const liveResultsChanBuf = 128

// StartLiveWorkers starts a worker pool for live log ingestion. The workers
// consume rows produced by LiveStream instances and submit them through the
// same putBatcher path as batch ProcessLogs.
//
// Parameters:
//   - workers: number of worker goroutines; values <=0 use the ingest auto
//     sizing formula.
//
// Returns:
//   - *LiveWorkers: worker pool handle that must be closed during shutdown
//   - error: nil on success, or an error describing why workers could not start
func (i *Ingest) StartLiveWorkers(workers int) (*LiveWorkers, error) {
	if i == nil || i.db == nil {
		return nil, errors.New("ingest: live workers require an initialized ingest")
	}
	w := &LiveWorkers{
		i:       i,
		results: make(chan *processResult, liveResultsChanBuf),
		shards:  &metaShards{meta: i.loadMetaEntries()},
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		i.runWorkerPool(w.results, workers)
	}()
	return w, nil
}

// Close stops the live worker pool and flushes any final bin-list changes.
// It is safe to call multiple times.
func (w *LiveWorkers) Close() {
	if w == nil {
		return
	}
	w.once.Do(func() {
		close(w.results)
		w.wg.Wait()
		if err := w.i.storeBinList(); err != nil {
			log.Printf("ERROR: Live Log Processor: could not store bin list: %s", err)
		}
	})
}

// AllocLiveStream returns the stable numeric node prefix for a live stream's
// cluster/node pair, allocating it if this is the first sighting.
func (i *Ingest) AllocLiveStream(cluster, nodeID string) int {
	i.progress.Lock()
	defer i.progress.Unlock()
	if i.progress.PreProcessor.NodeToPrefix == nil {
		i.progress.PreProcessor.NodeToPrefix = make(map[string]int)
	}
	if i.progress.PreProcessor.LastUsedSuffixForPrefix == nil {
		i.progress.PreProcessor.LastUsedSuffixForPrefix = make(map[int]int)
	}
	key := cluster + "_" + nodeID
	if prefix, ok := i.progress.PreProcessor.NodeToPrefix[key]; ok {
		return prefix
	}
	i.progress.PreProcessor.LastUsedPrefix++
	prefix := i.progress.PreProcessor.LastUsedPrefix
	i.progress.PreProcessor.NodeToPrefix[key] = prefix
	i.progress.PreProcessor.LastUsedSuffixForPrefix[prefix] = 1
	i.progress.PreProcessor.changed = true
	return prefix
}

// NewLiveStream creates a per-connection live parser using the same patterns,
// label cache, and row emission contract as batch log processing.
func (w *LiveWorkers) NewLiveStream(cluster, nodeID, source string) *LiveStream {
	prefix := w.i.AllocLiveStream(cluster, nodeID)
	labels := map[string]any{
		"ClusterName": cluster,
		"NodeIdent":   fmt.Sprintf("%d_%s", prefix, nodeID),
	}
	cache := make(resolveCache, len(labels)+4)
	fileName := source
	if fileName == "" {
		fileName = "live"
	}
	return &LiveStream{
		i:              w.i,
		workers:        w,
		stream:         newLogStream(cluster, w.i.patterns, &w.i.config.IngestTimeRanges, w.i.config.TimestampColumnName),
		cache:          cache,
		resolvedLabels: w.i.resolveLabelsCached(labels, cache, w.shards, cluster, fileName),
		clusterName:    cluster,
		fileName:       fileName,
		nodePrefix:     prefix,
		uniqNodeString: fmt.Sprintf("%s::/::%d_%s", cluster, prefix, nodeID),
	}
}

// ProcessLine parses one live log line and submits every emitted metric row to
// the live worker pool. Non-metric and timestamp-missing lines are ignored the
// same way batch ingest ignores them after writing to no-stat files.
func (s *LiveStream) ProcessLine(line string) error {
	out, err := s.stream.Process(line, s.nodePrefix)
	if err != nil && err != errNotMatched && err != errNoTimestamp && !strings.HasPrefix(err.Error(), "TIME PARSE:") {
		s.i.progress.LogProcessor.LineErrors.add(s.nodePrefix, err.Error())
		return err
	}
	if len(out) == 0 {
		return nil
	}
	for _, d := range out {
		s.workers.results <- &processResult{
			FileName:       s.fileName,
			Data:           d.Data,
			Resolved:       s.i.mergeResolved(s.resolvedLabels, d.Metadata, s.cache, s.workers.shards, s.clusterName, s.fileName),
			Error:          d.Error,
			SetName:        d.SetName,
			LogLine:        d.Line,
			UniqNodeString: s.uniqNodeString,
		}
	}
	return nil
}

// Close flushes multiline and aggregate state from the live parser.
func (s *LiveStream) Close() {
	if s == nil || s.stream == nil {
		return
	}
	out, _, _ := s.stream.Close()
	for _, d := range out {
		s.workers.results <- &processResult{
			FileName:       s.fileName,
			Data:           d.Data,
			Resolved:       s.i.mergeResolved(s.resolvedLabels, d.Metadata, s.cache, s.workers.shards, s.clusterName, s.fileName),
			Error:          d.Error,
			SetName:        d.SetName,
			LogLine:        d.Line,
			UniqNodeString: s.uniqNodeString,
		}
	}
}

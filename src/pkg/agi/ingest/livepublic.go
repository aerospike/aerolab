package ingest

import (
	"time"
)

// ResolveCache is a per-connection (key -> value -> idx) memoiser for
// label resolution on the live ingest path. See resolveLabelsCached.
type ResolveCache map[string]map[string]int

// ResolveLabelsForLive wraps resolveLabelsCached for external callers.
func (i *Ingest) ResolveLabelsForLive(metadata map[string]any, cache ResolveCache, shards *MetaShards, clusterName, fn string) map[string]int {
	var rc resolveCache
	if cache != nil {
		rc = resolveCache(cache)
	}
	return i.resolveLabelsCached(metadata, rc, shards, clusterName, fn)
}

// MergeResolvedForLive wraps mergeResolved for live streaming.
func (i *Ingest) MergeResolvedForLive(resolvedLabels map[string]int, perLine map[string]any, cache ResolveCache, shards *MetaShards, clusterName, fn string) map[string]int {
	var rc resolveCache
	if cache != nil {
		rc = resolveCache(cache)
	}
	return i.mergeResolved(resolvedLabels, perLine, rc, shards, clusterName, fn)
}

// AllocNodePrefixForLive assigns a stable numeric prefix for (cluster, nodeID),
// matching the batch preprocessor's NodeToPrefix allocator.
func (i *Ingest) AllocNodePrefixForLive(clusterName, nodeID string) int {
	i.progress.Lock()
	defer i.progress.Unlock()
	if i.progress.PreProcessor.NodeToPrefix == nil {
		i.progress.PreProcessor.NodeToPrefix = make(map[string]int)
	}
	if i.progress.PreProcessor.LastUsedSuffixForPrefix == nil {
		i.progress.PreProcessor.LastUsedSuffixForPrefix = make(map[int]int)
	}
	key := clusterName + "_" + nodeID
	if pfx, ok := i.progress.PreProcessor.NodeToPrefix[key]; ok {
		return pfx
	}
	i.progress.PreProcessor.LastUsedPrefix++
	pfx := i.progress.PreProcessor.LastUsedPrefix
	i.progress.PreProcessor.NodeToPrefix[key] = pfx
	i.progress.PreProcessor.LastUsedSuffixForPrefix[pfx] = 1
	i.progress.PreProcessor.changed = true
	return pfx
}

// LiveLogStream parses newline-delimited log lines for one cluster on
// the live ingest path.
type LiveLogStream struct {
	s *logStream
}

// NewLiveLogStream constructs a parser for clusterName using configured patterns.
func (i *Ingest) NewLiveLogStream(clusterName string) *LiveLogStream {
	return &LiveLogStream{
		s: newLogStream(clusterName, i.patterns, &i.config.IngestTimeRanges, i.config.TimestampColumnName),
	}
}

// Process parses one log line.
func (l *LiveLogStream) Process(line string, nodePrefix int) ([]*LogStreamOutput, error) {
	return l.s.Process(line, nodePrefix)
}

// Close flushes multiline / aggregate state at end of stream.
func (l *LiveLogStream) Close() ([]*LogStreamOutput, time.Time, time.Time) {
	return l.s.Close()
}

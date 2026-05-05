package ingest

// MetaShards is the public alias for the per-Ingest metaShards
// allocator passed into NewLiveStream. Construct one per Ingest
// (via NewLiveMetaShards) and reuse across every connection — the
// allocator is concurrency-safe and shared metaEntries entries are
// what give live and batch streams matching label idx values.
type MetaShards = metaShards

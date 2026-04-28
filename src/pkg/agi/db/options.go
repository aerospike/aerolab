package db

import (
	"log"
	"os"
)

// Options configure an embedded DB. The zero value and DefaultOptions() both
// yield EnableWAL=false; applyDefaults only promotes unset numeric fields and
// installs a default Logger when nil. For other fields, start from
// DefaultOptions() and override what you need.
type Options struct {
	// Path is the directory on disk where DB files are stored. It will be
	// created if it does not exist.
	Path string

	// CacheBytes is the size of the shared block cache.
	//
	//   CacheBytes == 0  → apply the default (512 MiB).
	//   CacheBytes > 0   → exact cache size in bytes.
	//   CacheBytes < 0   → disable the block cache entirely.
	//
	// A negative value is the explicit "no cache" sentinel
	// (NoBlockCache). Zero is reserved for "Options was not filled in",
	// so applyDefaults can promote it to the standard default.
	CacheBytes int64

	// MemTableSizeBytes is the size of each memtable (write buffer) before
	// it flushes to an SSTable. Larger values absorb more bursty writes at
	// the cost of memory and recovery time. Default 256 MiB — chosen to
	// match the AGI ingest profile, where a single batch flush easily
	// fills 64 MiB and Pebble would otherwise stall the writer waiting
	// for L0 compactions to drain. 256 MiB amortises one flush over
	// many more PutBatch commits and keeps the ingest pipeline running
	// at memtable speed for longer bursts.
	MemTableSizeBytes uint64

	// MemTableStopWritesThreshold caps the number of queued memtables
	// before Pebble starts blocking the writer. With ingest's batched
	// writes hitting MemTableSize quickly, the default (2) makes
	// writers stall whenever a flush is in flight. Default 4: lets
	// the next batch land into a fresh memtable while the previous one
	// is being written, doubling the burst capacity at the cost of
	// 2× memtable memory.
	MemTableStopWritesThreshold int

	// MaxConcurrentCompactions caps simultaneous compactions. Pebble
	// defaults to 1, which on AGI's many-set workload becomes a
	// bottleneck because age-out and ingest write to disjoint sets.
	// Default 4: matches typical NVMe parallelism without starving the
	// foreground I/O the indexed scans need.
	MaxConcurrentCompactions int

	// MaxOpenFiles caps the number of open file handles Pebble may use.
	// Zero leaves Pebble's default.
	MaxOpenFiles int

	// BlockSize is the target uncompressed size of each SSTable data
	// block in bytes. Larger blocks raise the compression ratio (more
	// cross-row redundancy per block) and reduce per-block metadata
	// overhead at a small cost in point-Get amplification (each Get
	// decompresses one block). 0 leaves Pebble's default of 4 KiB.
	// AGI's hot path is range scans where successive reads share a
	// block, so larger blocks (e.g. 32 KiB) are nearly free on the
	// read side and useful on the write side.
	BlockSize int

	// Compression selects the per-level SSTable compression profile.
	// The empty string leaves Pebble's default (uniform Snappy on
	// every level). Recognised values are case-insensitive:
	//
	//   ""        / "default" — uniform Snappy on every level (Pebble default).
	//   "snappy"             — uniform Snappy on every level (explicit).
	//   "fastest"            — uniform FastestCompression (MinLZ on x86_64,
	//                          Snappy on arm64). Slightly better ratio than
	//                          Snappy on x86_64 at the same CPU cost.
	//   "balanced"           — Pebble's DBCompressionBalanced: FastestCompression
	//                          on the upper LSM levels (cheap flushes / minor
	//                          compactions) and Zstd level 1 on the bottom 1-2
	//                          levels where the bulk of bytes settle. Targets
	//                          the EFS / network-FS profile where on-disk
	//                          bytes dominate cost and CPU has slack.
	//   "good"               — Pebble's DBCompressionGood: like "balanced"
	//                          but with Zstd level 3 on the bottom levels.
	//                          Tighter ratio, ~2x the CPU per byte compacted.
	//   "zstd"               — uniform Zstd level 3 on every level. Heaviest
	//                          CPU; only useful when L0 flush throughput is
	//                          not on any hot path.
	//   "none"               — no compression. For benchmarking only.
	//
	// Unrecognised values are treated as "" and a warning is logged.
	Compression string

	// EnableWAL turns on the Pebble write-ahead log. The WAL is OFF by
	// default, matching the throwaway-fast posture of this package: a
	// crash loses any still-in-memtable writes, which is acceptable
	// because the source of truth for AGI is log files on disk and
	// re-ingest is cheap. Set EnableWAL=true when you need crash
	// recovery of unflushed writes.
	EnableWAL bool

	// SyncWrites forces an fsync at the end of every write batch. Only
	// meaningful when EnableWAL=true. Default false.
	SyncWrites bool

	// IndexCanHaveOrphans turns on the per-row pointer check in
	// indexScanIter that detects and skips orphaned I/ entries left
	// behind by a PutBatch with AssumeNew=true that overwrote an
	// existing row with a different indexed value. The default
	// (false) trusts the PutBatch contract: AssumeNew=true is a
	// caller's promise that either no row exists at the PK, or the
	// row that exists has the same indexed value — in which case no
	// orphan can ever be produced. Workloads that genuinely overwrite
	// indexed values via AssumeNew (a hypothetical write-once-correct
	// then write-once-revised pattern) must set this to true; doing
	// so adds one Pebble Get per scanned row, which on a 2M-row
	// indexed scan costs ~6× the steady-state scan time.
	//
	// AGI's ingest workload uses AssumeNew=true on every metric-row
	// put but cannot produce orphans by construction (the row PK
	// embeds the source byte-offset, and the same offset always
	// parses to the same timestamp), so it leaves this option at
	// its default.
	IndexCanHaveOrphans bool

	// Logger receives informational and error messages. If nil, a logger
	// writing to stderr is used.
	Logger *log.Logger
}

// NoBlockCache is the sentinel value for Options.CacheBytes meaning "do
// not allocate a block cache at all". Pass this when the workload is
// scan-heavy and the cache would only waste memory.
const NoBlockCache int64 = -1

// DefaultOptions returns Options with the throwaway-fast defaults filled in.
// Path is left empty and must be set by the caller.
//
// CacheBytes (1 GiB) and MemTableSizeBytes (256 MiB) are tuned for the
// AGI ingest workload: large indexed scans benefit linearly from cache
// (every avoided block fetch is an avoided syscall + decompression),
// and a 256 MiB memtable keeps the post-batching writer running at
// memory speed rather than stalling on L0 flushes. The trade-off is
// ~1.25 GiB of resident memory per DB at steady state, which is
// acceptable for the deployed AGI host shape.
func DefaultOptions() Options {
	return Options{
		CacheBytes:                  1024 << 20,
		MemTableSizeBytes:           256 << 20,
		MemTableStopWritesThreshold: 4,
		MaxConcurrentCompactions:    4,
		EnableWAL:                   false,
		SyncWrites:                  false,
	}
}

func (o *Options) applyDefaults() {
	if o.CacheBytes == 0 {
		o.CacheBytes = 1024 << 20
	}
	if o.MemTableSizeBytes == 0 {
		o.MemTableSizeBytes = 256 << 20
	}
	if o.MemTableStopWritesThreshold == 0 {
		o.MemTableStopWritesThreshold = 4
	}
	if o.MaxConcurrentCompactions == 0 {
		o.MaxConcurrentCompactions = 4
	}
	if o.Logger == nil {
		o.Logger = log.New(os.Stderr, "agidb ", log.LstdFlags)
	}
}

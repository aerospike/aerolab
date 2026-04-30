package plugin

import (
	"fmt"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// DBOptionsFromConfig translates the plugin Config's DB sub-struct into
// a db.Options. Callers that share a DB handle across packages (see
// cmdAgiExecService) use this to build the same options without
// duplicating field-by-field mapping. Mirrors
// ingest.DBOptionsFromConfig: start from db.DefaultOptions() and
// override only the explicitly-set numeric fields so the two packages
// stay in lockstep if db.DefaultOptions() changes.
//
// Numeric fields use 0 as a sentinel for "not set". Bool fields
// (EnableWAL, SyncWrites) cannot do that — Go bools are tri-stateless
// — so they ARE operator-authoritative: yaml false beats DefaultOptions
// true. That mirrors the user expectation ("if I write enableWAL:
// false, WAL is off") and matches what cmdAgiExecService does
// explicitly via --no-force-wal when service mode wants to override.
func DBOptionsFromConfig(cfg *Config) db.Options {
	opts := db.DefaultOptions()
	opts.Path = cfg.DB.Path
	if cfg.DB.CacheBytes != 0 {
		opts.CacheBytes = cfg.DB.CacheBytes
	}
	if cfg.DB.MemTableSizeBytes != 0 {
		opts.MemTableSizeBytes = cfg.DB.MemTableSizeBytes
	}
	if cfg.DB.MemTableStopWritesThreshold != 0 {
		opts.MemTableStopWritesThreshold = cfg.DB.MemTableStopWritesThreshold
	}
	if cfg.DB.MaxConcurrentCompactions != 0 {
		opts.MaxConcurrentCompactions = cfg.DB.MaxConcurrentCompactions
	}
	if cfg.DB.MaxOpenFiles != 0 {
		opts.MaxOpenFiles = cfg.DB.MaxOpenFiles
	}
	if cfg.DB.BlockSize != 0 {
		opts.BlockSize = cfg.DB.BlockSize
	}
	if cfg.DB.Compression != "" {
		opts.Compression = cfg.DB.Compression
	}
	if cfg.DB.TargetFileSizeL0 != 0 {
		opts.TargetFileSizeL0 = cfg.DB.TargetFileSizeL0
	}
	// BytesPerSync: 0 = leave Pebble's default; <0 = disable
	// periodic sync; >0 = exact bytes. Pass through verbatim — the
	// db package interprets the sentinels.
	opts.BytesPerSync = cfg.DB.BytesPerSync
	if cfg.DB.LBaseMaxBytes != 0 {
		opts.LBaseMaxBytes = cfg.DB.LBaseMaxBytes
	}
	if cfg.DB.L0StopWritesThreshold != 0 {
		opts.L0StopWritesThreshold = cfg.DB.L0StopWritesThreshold
	}
	opts.EnableBloomFilter = cfg.DB.EnableBloomFilter
	opts.EnableWAL = cfg.DB.EnableWAL
	opts.SyncWrites = cfg.DB.SyncWrites
	return opts
}

// dbConnect opens the embedded db store at the configured path. Only
// used by the backward-compatible single-process Init() path. The merged
// service uses InitWithDB and opens the db itself.
func (p *Plugin) dbConnect() error {
	d, err := db.Open(DBOptionsFromConfig(p.config))
	if err != nil {
		return fmt.Errorf("db.Open: %w", err)
	}
	p.db = d
	p.ownsDB = true
	return nil
}

package ingest

import (
	"fmt"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// DBOptionsFromConfig translates the ingest Config's DB sub-struct
// into a db.Options. Callers that share a DB handle across packages
// (see cmdAgiExecService) use this to build the same options without
// duplicating field-by-field mapping. Mirrors
// plugin.DBOptionsFromConfig: numeric zero is the "not set" sentinel
// (preserve db.DefaultOptions); bool fields are operator-authoritative
// because Go bools cannot represent "not set".
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

// dbConnect opens the embedded db store at the configured path and then
// registers the well-known sets. Only used by the backward-compatible
// single-process Init() path.
func (i *Ingest) dbConnect() error {
	opts := DBOptionsFromConfig(i.config)
	d, err := db.Open(opts)
	if err != nil {
		return fmt.Errorf("db.Open(%s): %w", opts.Path, err)
	}
	i.db = d
	i.ownsDB = true
	return i.registerSets()
}

// registerSets pre-declares every set the ingest pipeline will write
// to. Metric-shaped sets (pattern outputs, the default set, and
// logRanges) get an indexed timestamp column. The collectinfo set is
// declared with its fixed string-typed columns so that a schema
// conflict surfaces at Init time rather than halfway through a long
// ingest — processcf.go writes it directly via i.db.Put (no putData
// coercion path), so a mid-run column-type mismatch would abort a
// collectinfo file rather than silently round-trip.
//
// Sets are deduped before RegisterSet so the same pattern name
// appearing under both `regex` and `exportAdvanced` is only registered
// once.
func (i *Ingest) registerSets() error {
	ts := i.config.TimestampColumnName
	indexed := []db.ColumnSpec{{Name: ts, Type: db.TypeInt64, Indexed: true}}

	seen := make(map[string]struct{})
	add := func(name string, out *[]string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		*out = append(*out, name)
	}
	var setsWithIndex []string
	add(i.config.DefaultSetName, &setsWithIndex)
	add(i.config.LogFileRangesSetName, &setsWithIndex)
	// Guard against a nil patterns block. The labels-set block
	// below already does this; the metric-pattern loop did not,
	// so any code path that opened the DB before patterns were
	// loaded (a future schema-only migration tool, a unit test
	// constructing Ingest directly, etc.) would nil-deref here.
	if i.patterns != nil {
		for _, def := range i.patterns.Defs {
			for _, pattern := range def.Patterns {
				add(pattern.Name, &setsWithIndex)
				for _, adv := range pattern.RegexAdvanced {
					add(adv.SetName, &setsWithIndex)
				}
			}
		}
	}
	for _, name := range setsWithIndex {
		if err := i.db.RegisterSet(name, indexed); err != nil {
			return fmt.Errorf("RegisterSet(%s): %w", name, err)
		}
	}
	// Collectinfo set: declare the full column set processcf.go will
	// write. All columns are strings; the set is intentionally not
	// indexed (collectinfo rows have no meaningful numeric order).
	if i.config.CollectInfoSetName != "" {
		cfCols := []db.ColumnSpec{
			{Name: "sysinfo", Type: db.TypeString},
			{Name: "conffile", Type: db.TypeString},
			{Name: "health", Type: db.TypeString},
			{Name: "summary", Type: db.TypeString},
			{Name: "cfName", Type: db.TypeString},
			{Name: "build", Type: db.TypeString},
			{Name: "clientConns", Type: db.TypeString},
			{Name: "ip", Type: db.TypeString},
			{Name: "migrations", Type: db.TypeString},
			{Name: "nodeId", Type: db.TypeString},
			{Name: "uptime", Type: db.TypeString},
			{Name: "integrity", Type: db.TypeString},
			{Name: "clusterKey", Type: db.TypeString},
			{Name: "principal", Type: db.TypeString},
			{Name: "clusterSize", Type: db.TypeString},
			{Name: "clusterName", Type: db.TypeString},
		}
		if err := i.db.RegisterSet(i.config.CollectInfoSetName, cfCols); err != nil {
			return fmt.Errorf("RegisterSet(%s): %w", i.config.CollectInfoSetName, err)
		}
	}
	// Labels set: a plain string-valued bag keyed by label name. It's
	// non-indexed so cacheMetadataList's full Scan stays bounded by
	// the small number of label names (<10 control rows plus one row
	// per categorical column the patterns emit). Declaring the column
	// here makes the labels-set/labelsValueCol contract visible in
	// one place instead of "first writer wins".
	if i.patterns != nil && i.patterns.LabelsSetName != "" {
		if err := i.db.RegisterSet(i.patterns.LabelsSetName, []db.ColumnSpec{{Name: labelsValueCol, Type: db.TypeString}}); err != nil {
			return fmt.Errorf("RegisterSet(%s): %w", i.patterns.LabelsSetName, err)
		}
	}
	return nil
}

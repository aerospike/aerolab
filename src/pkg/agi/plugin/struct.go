package plugin

import (
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

type Plugin struct {
	config *Config
	// pprofMu serialises StartCPUProfile, StopCPUProfile,
	// RotateCPUProfile and Close. Without it: a deferred coordinator
	// (cmdAgiExecService starts plugin pprof when ingest completes)
	// could race shutdown and leave a profile running against a
	// plugin that has already torn down, or a SIGUSR1-driven rotate
	// could re-truncate a file Close is about to flush. Hold this
	// for the entire stop-and-rename window in RotateCPUProfile so
	// no other goroutine can recreate the file before the rename.
	pprofMu      sync.Mutex
	cpuProfile   *os.File
	pprofRunning bool
	db           *db.DB
	// ownsDB is true when this Plugin opened the db handle itself via
	// Init(); Close() is then responsible for db.Close. When ownsDB is
	// false (handle was injected via InitWithDB from a caller that
	// co-owns the db with ingest), Close() does NOT close the db —
	// the caller does.
	ownsDB bool
	srv    *http.Server
	mux    *http.ServeMux
	cache  struct {
		lock     *sync.RWMutex
		setNames []string
		binNames []string
		metadata map[string]*metaEntries
		// warnedNonIndexed tracks sets for which we have already
		// emitted the "no indexed timestamp column" warning. Keyed
		// by set name; presence means warned. Cleared only by
		// restart, which matches the lifecycle: a set can only
		// become non-indexed if ingest created it without the
		// registerSets path.
		warnedNonIndexed map[string]bool
	}
	requests chan bool
	jobs     chan bool
	// done is closed by Close to stop background goroutines
	// (queryAndCache, stats). Goroutines select on it and bail.
	done chan struct{}
	// bg tracks the background goroutines (queryAndCache, stats).
	// Close() must Wait on it BEFORE calling p.db.Close(), otherwise
	// an in-flight p.db.Scan() iterator inside the refresher trips
	// db.ErrIteratorsOpen and leaves Pebble open (file lock leak).
	bg sync.WaitGroup
	// handlers tracks in-flight HTTP handlers. Close() waits on it
	// after srv.Shutdown so a Query that already entered
	// p.db.Query(...).Run(ctx) is fully drained before Close returns.
	// When the plugin is driven purely via httptest (no srv), the
	// handlers still register themselves here so tests that don't
	// go through srv.Shutdown still see a correctly-ordered Close.
	handlers  sync.WaitGroup
	closeOnce sync.Once
}

type Config struct {
	Service struct {
		ListenAddress string `yaml:"listenAddress" default:"127.0.0.1" envconfig:"PLUGIN_LISTEN_ADDR"`
		ListenPort    int    `yaml:"listenPort" default:"8851" envconfig:"PLUGIN_LISTEN_PORT"`
	} `yaml:"service"`
	AddNoneToLabels            []string      `yaml:"addNoneToLabels"`
	TimeseriesLegendSeparator  string        `yaml:"timeseriesLegendSeparator" default:" : " envconfig:"PLUGIN_SEPARATOR"`
	TimeseriesDisplayNameFirst bool          `yaml:"timeseriesDisplayNameFirst" default:"false" envconfig:"PLUGIN_DISPLAYNAME_FIRST"` // should the display name come first in the legend
	MaxSeriesPerGraph          int           `yaml:"maxSeriesPerGraph" default:"1000" envconfig:"PLUGIN_MAX_SERIES"`
	MaxDataPointsReceived      int           `yaml:"maxDataPointsReceived" default:"34560000" envconfig:"PLUGIN_MAX_DP_RECV"` // 8640000 is about 1 GiB for concurrent 4 graphs, covering 1000 series in each graph, for a day; default max 4 GiB before reduction
	// Defaults bumped post-Pebble migration: snapshot-isolated reads
	// no longer share an in-memory primary index, so fanning out is
	// almost free. Operators on tiny hosts can still pin these back
	// to 4/4 in plugin.yaml; cmdAgiCreate writes 16/8 (cloud) or
	// 4/4 (Docker) into the deployed yaml on instance creation.
	MaxConcurrentRequests      int           `yaml:"maxConcurrentRequests" default:"16" envconfig:"PLUGIN_MAX_REQUESTS"`
	MaxConcurrentJobs          int           `yaml:"maxConcurrentJobs" default:"8" envconfig:"PLUGIN_MAX_JOBS"`
	CacheRefreshInterval       time.Duration `yaml:"cacheRefreshInterval" default:"30s" envconfig:"PLUGIN_CACHE_REFRESH"`
	LabelsSetName              string        `yaml:"labelsSetName" default:"labels" envconfig:"PLUGIN_LABELS_SETNAME"`
	LogLevel                   int           `yaml:"logLevel" default:"4" envconfig:"PLUGIN_LOGLEVEL"` // 0=NO_LOGGING 1=CRITICAL, 2=ERROR, 3=WARNING, 4=INFO, 5=DEBUG, 6=DETAIL
	DB                         struct {
		// Path is the on-disk directory Pebble writes to. The default
		// is db.DefaultPath; keep ingest and plugin in lockstep or
		// they will open separate stores and never see each other's
		// data.
		Path string `yaml:"path" default:"/opt/agi/db" envconfig:"PLUGIN_DB_PATH"`
		// CacheBytes/MemTableSizeBytes/MemTableStopWritesThreshold/
		// MaxConcurrentCompactions default to 0 so the unset path
		// falls through to db.DefaultOptions(); operators that
		// previously pinned the legacy 64 MiB / 512 MiB sizes via
		// yaml will keep getting those values (override still wins).
		CacheBytes                  int64         `yaml:"cacheBytes" default:"0" envconfig:"PLUGIN_DB_CACHE_BYTES"`
		MemTableSizeBytes           uint64        `yaml:"memTableSizeBytes" default:"0" envconfig:"PLUGIN_DB_MEMTABLE_BYTES"`
		MemTableStopWritesThreshold int           `yaml:"memTableStopWritesThreshold" default:"0" envconfig:"PLUGIN_DB_MEMTABLE_STOP_THRESHOLD"`
		MaxConcurrentCompactions    int           `yaml:"maxConcurrentCompactions" default:"0" envconfig:"PLUGIN_DB_MAX_COMPACTIONS"`
		MaxOpenFiles                int           `yaml:"maxOpenFiles" default:"0" envconfig:"PLUGIN_DB_MAX_OPEN_FILES"`
		BlockSize                   int           `yaml:"blockSize" default:"0" envconfig:"PLUGIN_DB_BLOCK_SIZE"`
		Compression                 string        `yaml:"compression" default:"" envconfig:"PLUGIN_DB_COMPRESSION"`
		// EFS / NFS-shape Pebble tuning knobs. See db.Options docs
		// for full semantics. 0 = leave Pebble's default; for
		// BytesPerSync a negative value (db.BytesPerSyncDisabled)
		// explicitly disables the periodic sync_file_range cadence
		// that becomes a NFS COMMIT round-trip on EFS.
		TargetFileSizeL0      int64 `yaml:"targetFileSizeL0" default:"0" envconfig:"PLUGIN_DB_TARGET_FILE_SIZE_L0"`
		BytesPerSync          int   `yaml:"bytesPerSync" default:"0" envconfig:"PLUGIN_DB_BYTES_PER_SYNC"`
		LBaseMaxBytes         int64 `yaml:"lBaseMaxBytes" default:"0" envconfig:"PLUGIN_DB_LBASE_MAX_BYTES"`
		L0StopWritesThreshold int   `yaml:"l0StopWritesThreshold" default:"0" envconfig:"PLUGIN_DB_L0_STOP_WRITES_THRESHOLD"`
		EnableBloomFilter     bool  `yaml:"enableBloomFilter" default:"false" envconfig:"PLUGIN_DB_ENABLE_BLOOM_FILTER"`
		EnableWAL             bool  `yaml:"enableWAL" default:"false" envconfig:"PLUGIN_DB_ENABLE_WAL"`
		SyncWrites            bool  `yaml:"syncWrites" default:"false" envconfig:"PLUGIN_DB_SYNC_WRITES"`
		ShutdownTimeout       time.Duration `yaml:"shutdownTimeout" default:"60s" envconfig:"PLUGIN_DB_SHUTDOWN_TIMEOUT"`
	} `yaml:"db"`
	TimestampBinName       string `yaml:"timestampBinName" default:"timestamp" envconfig:"PLUGIN_TIMESTAMP_BIN"`
	CPUProfilingOutputFile string `yaml:"cpuProfilingOutputFile" envconfig:"PLUGIN_CPUPROFILE_FILE"`
}

package ingest

import (
	_ "embed"
	"encoding/json"
	"maps"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
	"github.com/gabriel-vasile/mimetype"
)

type Ingest struct {
	config       *Config
	patterns     *patterns
	cpuProfile   *os.File
	pprofRunning bool
	progress     *Progress
	db           *db.DB
	// ownsDB is true when this Ingest opened the db handle itself via
	// Init(); Close() is then responsible for db.Close. When ownsDB is
	// false (handle was injected via InitWithDB from a caller that
	// co-owns the db with the plugin), Close() does NOT close the db —
	// the caller does.
	ownsDB  bool
	end     bool
	endLock *sync.Mutex
	binList *binList
	// bg tracks saveProgressInterval and printProgressInterval so
	// Close() can Wait for them. They don't touch the db themselves
	// (they write the progress file) but leaving them alive past
	// Close() leaks goroutines in tests that spawn many Ingests and
	// masks lifecycle bugs in longer-running deployments.
	bg        sync.WaitGroup
	closeOnce sync.Once

	// putBatcher accumulates per-set metric rows on the
	// ProcessLogs hot path and flushes them in db.PutBatch chunks
	// with AssumeNew=true. Initialised in finalizeInit (so both
	// Init and InitWithDB get one) and closed by Ingest.Close
	// after the per-row workers have drained. nil only between
	// newIngest and finalizeInit; the hot path always sees a
	// non-nil batcher.
	putBatcher *putBatcher
	// putBatcherRefs gates putBatcher teardown across the batch
	// and live ingest paths. Both batch (Close) and live
	// (livelisten.Listener.Shutdown) hold a ref so the batcher
	// only drains when the LAST holder releases. Initialised to
	// 1 in finalizeInit (the batch holder); live mode adds 1 on
	// Serve and releases on Shutdown via PutBatcherRetain /
	// PutBatcherRelease. When the count reaches zero, putBatcher
	// is closed.
	putBatcherRefs   int
	putBatcherMu     sync.Mutex
	// liveResultsChan is the per-process input channel for the
	// live worker pool. Allocated lazily by StartLiveWorkers; nil
	// when live mode is not active. The handler in
	// pkg/agi/livelisten submits *processResult onto this channel,
	// the worker pool drains it via runWorkerPool (the same code
	// path the batch pipeline uses).
	liveResultsChan chan *processResult
	liveWG          sync.WaitGroup
}

// binList is the catalog of every column ever produced by ingest.
// It is published two ways:
//
//   - BinNames: the canonical, ordered slice that is JSON-serialised
//     into the BINLIST row of the labels set. Mutated only under
//     lock.
//   - snapshot: an atomic.Pointer to an immutable presence-set map.
//     The ProcessLogs hot path reads this with NO lock; rare
//     additions take lock and publish a copy-on-written replacement.
//
// The two views are kept in step: a single mutator goroutine takes
// lock, appends to BinNames, builds a new map = old map ∪ new keys,
// atomically swaps snapshot, sets changed=true, releases lock.
//
// Persistence is decoupled from the hot path. storeBinList() is
// invoked (a) at the head of every saveProgress() so the on-disk
// (BINLIST, progress) pair is consistent at every persisted
// checkpoint — the crash-resume contract relies on this — and (b)
// at the end of ProcessLogs as a clean-shutdown final flush.
type binList struct {
	lock     sync.Mutex
	BinNames []string `json:"binNames"`
	// snapshot holds the read-only presence map used by the
	// lock-free hot path. The pointed-at map is treated as
	// immutable: writers always allocate a fresh map and atomic
	// Store, never mutate the existing one. Concurrent readers
	// holding a stale pointer keep working safely against the old
	// map until they reload.
	snapshot atomic.Pointer[map[string]struct{}]
	changed  bool
}

// loadSnapshot returns the current immutable presence map; nil only
// before seedSnapshot has been called (i.e. only in tests that
// bypass newIngest / finalizeInit).
func (b *binList) loadSnapshot() *map[string]struct{} {
	return b.snapshot.Load()
}

// seedSnapshot publishes a snapshot built from BinNames. Called at
// the end of newIngest and finalizeInit so the very first hot-path
// read sees a non-nil map. Caller must hold b.lock or be running
// during single-threaded init.
func (b *binList) seedSnapshot() {
	m := make(map[string]struct{}, len(b.BinNames))
	for _, n := range b.BinNames {
		m[n] = struct{}{}
	}
	b.snapshot.Store(&m)
}

// missingNames returns the subset of keys in data that are not
// present in the current snapshot. Lock-free; safe to call from any
// goroutine. The returned slice is freshly allocated only when at
// least one key is missing (the steady-state hot path returns nil).
func (b *binList) missingNames(data map[string]any) []string {
	snap := b.snapshot.Load()
	if snap == nil {
		out := make([]string, 0, len(data))
		for k := range data {
			out = append(out, k)
		}
		return out
	}
	var out []string
	m := *snap
	for k := range data {
		if _, present := m[k]; !present {
			out = append(out, k)
		}
	}
	return out
}

// containsName is the single-key probe used by the upsertMetaEntry
// first-sighting path. Lock-free.
func (b *binList) containsName(k string) bool {
	snap := b.snapshot.Load()
	if snap == nil {
		return false
	}
	_, ok := (*snap)[k]
	return ok
}

// addNames appends every key from `keys` that is not already in the
// snapshot. Returns the slice of names actually added (so callers
// can log or react). Acquires b.lock for the duration of the COW;
// the lock is held for microseconds (one map copy + one slice
// append) and is uncontested in steady state because the hot path
// never enters this branch after the first ~thousand rows.
//
// addNames re-checks presence under the lock to absorb races where
// two goroutines saw the same missing key in their own snapshot
// reads.
func (b *binList) addNames(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	cur := b.snapshot.Load()
	var base map[string]struct{}
	if cur != nil {
		base = *cur
	}
	var added []string
	var nextMap map[string]struct{}
	for _, k := range keys {
		if _, present := base[k]; present {
			continue
		}
		if nextMap != nil {
			if _, present := nextMap[k]; present {
				continue
			}
		}
		if nextMap == nil {
			nextMap = make(map[string]struct{}, len(base)+len(keys))
			for kk := range base {
				nextMap[kk] = struct{}{}
			}
		}
		nextMap[k] = struct{}{}
		b.BinNames = append(b.BinNames, k)
		added = append(added, k)
	}
	if len(added) > 0 {
		b.snapshot.Store(&nextMap)
		b.changed = true
	}
	return added
}

type lineErrors struct {
	sync.Mutex
	errors  map[int]map[string]int
	changed bool
}

func (l *lineErrors) add(nodeIdent int, err string) {
	l.Lock()
	l.changed = true
	if l.errors == nil {
		l.errors = make(map[int]map[string]int)
	}
	if _, ok := l.errors[nodeIdent]; !ok {
		l.errors[nodeIdent] = make(map[string]int)
		l.errors[nodeIdent][err] = 1
		l.Unlock()
		return
	}
	l.errors[nodeIdent][err]++
	l.Unlock()
}

func (l *lineErrors) isChanged() bool {
	l.Lock()
	c := l.changed
	l.Unlock()
	return c
}

// provide node prefix, and get a map[error-string]repeat-count
func (l *lineErrors) Get(nodePrefix int) map[string]int {
	l.Lock()
	a := make(map[string]int)
	if _, ok := l.errors[nodePrefix]; ok {
		maps.Copy(a, l.errors[nodePrefix])
	}
	l.Unlock()
	return a
}

func (l *lineErrors) MarshalJSON() ([]byte, error) {
	l.Lock()
	defer l.Unlock()
	return json.Marshal(l.errors)
}

func (l *lineErrors) UnmarshalJSON(v []byte) error {
	l.Lock()
	defer l.Unlock()
	l.errors = make(map[int]map[string]int)
	return json.Unmarshal(v, &l.errors)
}

type Config struct {
	LogLevel int `yaml:"logLevel" default:"4" envconfig:"LOGINGEST_LOGLEVEL"` // 0=NO_LOGGING 1=CRITICAL, 2=ERROR, 3=WARNING, 4=INFO, 5=DEBUG, 6=DETAIL
	// DB holds embedded-db tuning knobs only. Pipeline knobs that used
	// to live here (DefaultSetName, LogFileRangesSetName,
	// TimestampColumnName, MaxPutThreads) were moved to top-level
	// fields below — they are not properties of the storage engine.
	DB struct {
		// Path is the on-disk directory Pebble writes to. The default
		// is db.DefaultPath; keep ingest and plugin in lockstep or
		// they will open separate stores and never see each other's
		// data.
		Path                        string `yaml:"path" default:"/opt/agi/db" envconfig:"LOGINGEST_DB_PATH"`
		CacheBytes                  int64  `yaml:"cacheBytes" default:"0"`                  // 0 -> db default
		MemTableSizeBytes           uint64 `yaml:"memTableSizeBytes" default:"0"`           // 0 -> db default
		MemTableStopWritesThreshold int    `yaml:"memTableStopWritesThreshold" default:"0"` // 0 -> db default
		MaxConcurrentCompactions    int    `yaml:"maxConcurrentCompactions" default:"0"`    // 0 -> db default
		MaxOpenFiles                int    `yaml:"maxOpenFiles" default:"0"`
		BlockSize                   int    `yaml:"blockSize" default:"0"`     // 0 -> db default (Pebble default = 4 KiB)
		Compression                 string `yaml:"compression" default:""`    // "" -> db default (Pebble default = uniform Snappy); see db.Options.Compression for valid values
		// EFS / NFS-shape Pebble tuning knobs. See db.Options docs
		// for full semantics. 0 = leave Pebble's default for every
		// numeric field here (consistent with the rest of this
		// struct). BytesPerSync also accepts a negative value
		// (db.BytesPerSyncDisabled) to explicitly disable the
		// periodic sync_file_range cadence, which is the
		// EFS-friendly setting.
		TargetFileSizeL0      int64 `yaml:"targetFileSizeL0" default:"0"`      // 0 -> Pebble default 2 MiB
		BytesPerSync          int   `yaml:"bytesPerSync" default:"0"`          // 0 -> Pebble default 512 KiB; <0 -> disabled (EFS-friendly)
		LBaseMaxBytes         int64 `yaml:"lBaseMaxBytes" default:"0"`         // 0 -> Pebble default 64 MiB
		L0StopWritesThreshold int   `yaml:"l0StopWritesThreshold" default:"0"` // 0 -> Pebble default 12
		EnableBloomFilter     bool  `yaml:"enableBloomFilter" default:"false"` // false -> Pebble default (no bloom)
		EnableWAL             bool  `yaml:"enableWAL" default:"false"`
		SyncWrites            bool  `yaml:"syncWrites" default:"false"`
		// PostIngestCompact, when true, triggers a synchronous
		// full-keyspace db.Compact() at the end of ProcessLogs
		// (just before LogProcessor.Finished is set). The
		// compaction collapses L0 sublevel overlap, GC's
		// tombstones, and re-encodes the LSM under the bottom-
		// level compression profile — typically shrinking the
		// on-disk DB by 30-50% and making subsequent indexed
		// range scans (the plugin's hot path) noticeably faster
		// because they touch a single dense level instead of
		// merging across L0/L1/L2. The cost is a one-time
		// post-ingest pause whose duration is bounded by total
		// DB bytes ÷ EFS bandwidth (typically a few minutes on
		// AGI-shaped runs). Default false to preserve the legacy
		// "ingest finished == LogProcessor.Finished == queryable"
		// timing; cmdAgiCreate flips this on for cloud
		// (AWS / GCP) deploys where the post-compaction layout
		// pays for itself immediately on the first plugin query.
		PostIngestCompact bool `yaml:"postIngestCompact" default:"false"`
	} `yaml:"db"`
	// Pipeline tuning. These were under `db:` in earlier versions; they
	// are not engine-level knobs and were moved here for clarity. AGI
	// instances are short-lived and not migrated, so the rename is safe.
	DefaultSetName       string `yaml:"defaultSetName" default:"default"`
	LogFileRangesSetName string `yaml:"logFileRangesSetName" default:"logRanges"`
	TimestampColumnName  string `yaml:"timestampColumnName" default:"timestamp"`
	// MaxPutThreads is the size of the worker goroutine pool that
	// drains resultsChan and forwards rows into the putBatcher
	// shards. Workers do per-row label stamping, missing-bin
	// probe, row materialisation, and submit. They do NOT write
	// to Pebble themselves — that work happens on the batcher
	// shards (see PutBatchShards). Workers are essentially
	// row-prep + submit goroutines.
	//
	// 0 selects auto = clamp(GOMAXPROCS*2, 4, 32). Default 128 on
	// the assumption that a deep pool insulates the upstream
	// resultsChan against single-shard commit-window stalls (each
	// parked worker effectively holds one extra row of in-flight
	// buffer past resultsChan). The cost of the deeper pool is
	// minimal: parked goroutines consume zero CPU, the wakeup
	// path is O(1) regardless of pool size, and 128 stacks add
	// ~256 KiB of resident memory.
	//
	// The auto branch (set this to 0 explicitly) is preserved for
	// constrained deployments that genuinely benefit from a
	// smaller pool — but it is NOT the default because the
	// "deeper pool wastes CPU" hypothesis did not hold up under
	// measurement: in head-to-head pprofs at the same throughput
	// the 128-worker and 16-worker runs had indistinguishable CPU
	// profiles, with the only observable difference being the
	// 16-worker run's smaller resultsChan buffer (now decoupled,
	// so the buffer side is no longer tied to this knob).
	MaxPutThreads int `yaml:"maxPutThreads" default:"128"`
	// PutBatchSize is the number of metric rows accumulated per set
	// before the ingest hot path flushes via db.PutBatch. Larger
	// batches amortise the Pebble.Batch commit overhead and the
	// schema-resolution fast-path's lock churn at the cost of
	// holding more rows in memory between flushes (each row is a
	// few hundred bytes) and a longer worst-case end-to-end
	// latency before a row is queryable.
	//
	// Default 1024. Was 256, raised after pprofs taken with the
	// AssumeNew lock-skip in db.PutBatch showed the pipeline's
	// new gate was per-shard putBatcher.submit blocking — workers
	// stalled because each shard's commit window kept its inCh
	// full. Quadrupling the batch size cuts the number of commit
	// calls per row 4x and gives each shard a longer drain phase
	// between commits, which directly widens the back-pressure
	// window before submit blocks. Per-shard inCh capacity is
	// flushSize*4 = 4096 entries at this default, so a single
	// commit window has to outlast ~4 batches' worth of
	// production before submit becomes the bottleneck.
	//
	// Memory cost at peak (16 shards × 4096 entries × ~150 B/row)
	// is on the order of 10 MiB resident, which is trivial next
	// to the 256 MiB Pebble memtable.
	PutBatchSize int `yaml:"putBatchSize" default:"1024"`
	// PutBatchFlushMs is the maximum age of an in-flight batch
	// before the flusher commits it even if it is below
	// PutBatchSize. Bounds the staleness window between log-line
	// arrival and queryability; 50ms keeps the staleness lower
	// than a typical Grafana refresh interval. Set to 0 to use the
	// 50ms default; very small values (<5ms) defeat the batching
	// benefit because the flusher trips before any meaningful
	// number of rows accumulate.
	PutBatchFlushMs int `yaml:"putBatchFlushMs" default:"50"`
	// PutBatchShards is the number of parallel flusher goroutines
	// behind putBatcher. The pre-sharded batcher used a single
	// flusher whose db.PutBatch throughput became the ingest
	// pipeline's hard ceiling once the upstream metaLock was
	// removed; sharding by maphash(key) lets independent batches
	// commit through Pebble in parallel because db.PutBatch is
	// concurrency-safe and stripedLocks never collide across
	// shards.
	//
	// 0 selects auto = min(GOMAXPROCS, 8). The upper cap reflects
	// that Pebble's commit pipeline saturates well before 8 active
	// writers on typical AGI hardware; raising this knob beyond
	// the available cores adds scheduler churn without raising
	// throughput.
	PutBatchShards int `yaml:"putBatchShards" default:"0"`
	Dedup          struct {
		Enabled   bool `yaml:"enabled" default:"true"`
		ReadBytes int  `yaml:"readBytesCount" default:"1048576"`
	} `yaml:"dedup"`
	Processor struct {
		// MaxConcurrentLogFiles caps how many log files are
		// parsed in parallel (one parser goroutine per file in
		// flight). 0 selects auto = clamp(GOMAXPROCS, 4, 16);
		// the resolution lives in processLogsFeed so the same
		// formula applies whether the value comes from the yaml
		// default, an envvar, or a CLI override. Cloud boxes
		// with 16+ vCPU therefore parse 16 files in parallel by
		// default; small Docker containers (and Docker
		// Desktop's GOMAXPROCS-respected cgroup) get 4-8.
		//
		// 16 is the upper cap because past that the resultsChan
		// buffer (128 slots) and the batcher fan-in saturate;
		// adding more parsers just blocks them on chansend
		// without raising throughput.
		MaxConcurrentLogFiles int `yaml:"maxConcurrentLogFiles" default:"0"`
		LogReadBufferSizeKb   int `yaml:"logReadBufferSizeKb" default:"1024"`
	} `yaml:"processor"`
	PreProcess struct {
		FileThreads         int `yaml:"fileThreads" default:"6"`
		UnpackerFileThreads int `yaml:"unpackerFileThreads" default:"4"`
	} `yaml:"preProcessor"`
	ProgressFile struct {
		DisableWrite   bool          `yaml:"disableWrite" default:"false"`
		OutputFilePath string        `yaml:"outputFilePath" default:"ingest/progress/"`
		WriteInterval  time.Duration `yaml:"writeInterval" default:"10s"`
		Compress       bool          `yaml:"compress" default:"true"`
	} `yaml:"progressFile"`
	ProgressPrint struct {
		Enable               bool          `yaml:"enable" default:"true"`
		UpdateInterval       time.Duration `yaml:"updateInterval" default:"10s"`
		PrintOverallProgress bool          `yaml:"printOverallProgress" default:"true"`
		PrintDetailProgress  bool          `yaml:"printDetailProgress" default:"true"`
	} `yaml:"progressPrint"`
	PatternsFile            string        `yaml:"patternsFile"`
	IngestTimeRanges        TimeRanges    `yaml:"ingestTimeRanges"`
	CollectInfoAsadmTimeout time.Duration `yaml:"collectInfoCommandTimeout" default:"150s"`
	CollectInfoMaxSize      int64         `yaml:"collectInfoMaxSize" default:"20971520"` // files over 20MiB will be considered not collectinfo
	CollectInfoSetName      string        `yaml:"collectInfoSetName" default:"collectinfos"`
	Directories             struct {
		CollectInfo   string `yaml:"collectInfo" default:"ingest/files/collectinfo"`
		Logs          string `yaml:"logs" default:"ingest/files/logs"`
		DirtyTmp      string `yaml:"dirtyTemp" default:"ingest/files/input"`
		NoStatLogs    string `yaml:"noStatOut" default:"ingest/files/logs-cut"`
		OtherFiles    string `yaml:"otherFiles" default:"ingest/files/other"`
		ReadOnlyInput bool   `yaml:"readOnlyInput" default:"false" envconfig:"LOGINGEST_READONLY_INPUT"` // When true, input directory is read-only (e.g., bind mount); copy files instead of moving, don't delete after unpack
	} `yaml:"directories"`
	Downloader struct {
		ConcurrentSources bool        `yaml:"concurrentSources" default:"true"`
		S3Source          *S3Source   `yaml:"s3Source"`
		SftpSource        *SftpSource `yaml:"sftpSource"`
	} `yaml:"downloader"`
	CustomSourceName           string `yaml:"customSourceName" default:"" envconfig:"LOGINGEST_CUSTOM_SRCNAME"`
	FindClusterNameNodeIdRegex string `yaml:"findClusterNameNodeIdRegex" default:"NODE-ID (?P<NodeId>[^ ]+) CLUSTER-SIZE (?P<ClusterSize>\\d+)( CLUSTER-NAME (?P<ClusterName>[^$]+))*"`
	findClusterNameNodeIdRegex *regexp.Regexp
	CPUProfilingOutputFile     string `yaml:"cpuProfilingOutputFile" envconfig:"LOGINGEST_CPUPROFILE_FILE"`
	SendClusterInfo            string `yaml:"sendClusterInfo" envconfig:"LOGINGEST_SEND_CLUSTER_INFO"`
	// Live ingest is an additive "live tail" path on top of the
	// existing batch ingest pipeline. When enabled, the merged AGI
	// service starts an in-process listener (see pkg/agi/livelisten)
	// that accepts streamed log lines from a sidecar dispatcher
	// running on each Aerospike node. The listener feeds rows
	// through the same logStream / putBatcher / Pebble pipeline the
	// batch path uses.
	//
	// Live mode REQUIRES EnableWAL=true: the dirty-marker mechanism
	// in init.go wipes the DB on the next start when WAL=off, which
	// is correct for batch ingest (the source files re-populate the
	// DB on restart) but not for live ingest (the source lines have
	// already been consumed and the dispatcher has no replay path).
	// cmdAgiCreate enforces this in plugin.yaml when --enable-live-
	// ingest is set, and the merged service refuses to start the
	// listener at runtime when dbOpts.EnableWAL=false.
	Live struct {
		Enabled    bool   `yaml:"enabled" default:"false" envconfig:"LOGINGEST_LIVE_ENABLED"`
		ListenAddr string `yaml:"listenAddr" default:"127.0.0.1:18080" envconfig:"LOGINGEST_LIVE_ADDR"`
		Workers    int    `yaml:"workers" default:"16"`
		MaxStreams int    `yaml:"maxStreams" default:"256"` // safety cap
	} `yaml:"live"`
}

type TimeRanges struct {
	Enabled bool      `yaml:"enabled" envconfig:"LOGINGEST_TIMERANGE_ENABLE" default:"false"`
	From    time.Time `yaml:"from" envconfig:"LOGINGEST_TIMERANGE_FROM"`
	To      time.Time `yaml:"to" envconfig:"LOGINGEST_TIMERANGE_TO"`
}

type S3Source struct {
	Enabled     bool   `yaml:"enabled" envconfig:"LOGINGEST_S3SOURCE_ENABLED"`
	Threads     int    `yaml:"threads" envconfig:"LOGINGEST_S3SOURCE_THREADS" default:"4"`
	Region      string `yaml:"region" envconfig:"LOGINGEST_S3SOURCE_REGION"`
	BucketName  string `yaml:"bucketName" envconfig:"LOGINGEST_S3SOURCE_BUCKET"`
	KeyID       string `yaml:"keyID" envconfig:"LOGINGEST_S3SOURCE_KEYID"`
	SecretKey   string `yaml:"secretKey" envconfig:"LOGINGEST_S3SOURCE_SECRET"`
	PathPrefix  string `yaml:"pathPrefix" envconfig:"LOGINGEST_S3SOURCE_PATH"`
	SearchRegex string `yaml:"searchRegex" envconfig:"LOGINGEST_S3SOURCE_REGEX"`
	Endpoint    string `yaml:"endpoint" envconfig:"LOGINGEST_S3SOURCE_ENDPOINT"`
	searchRegex *regexp.Regexp
}

type SftpSource struct {
	Enabled     bool   `yaml:"enabled" envconfig:"LOGINGEST_SFTPSOURCE_ENABLED"`
	Threads     int    `yaml:"threads" envconfig:"LOGINGEST_SFTPSOURCE_THREADS" default:"4"`
	Host        string `yaml:"host" envconfig:"LOGINGEST_SFTPSOURCE_HOST"`
	Port        int    `yaml:"port" envconfig:"LOGINGEST_SFTPSOURCE_PORT"`
	Username    string `yaml:"username" envconfig:"LOGINGEST_SFTPSOURCE_USER"`
	Password    string `yaml:"password" envconfig:"LOGINGEST_SFTPSOURCE_PASSWORD"`
	KeyFile     string `yaml:"keyFile" envconfig:"LOGINGEST_SFTPSOURCE_KEYFILE"`
	PathPrefix  string `yaml:"pathPrefix" envconfig:"LOGINGEST_SFTPSOURCE_PATH"`
	SearchRegex string `yaml:"searchRegex" envconfig:"LOGINGEST_SFTPSOURCE_REGEX"`
	searchRegex *regexp.Regexp
}

//go:embed patterns.yml
var patternEmbed []byte

type patterns struct {
	GenericLogs []*struct {
		ContainsStrings  []string `yaml:"fileContainsStrings"`
		ApplyClusterName string   `yaml:"applyClusterName"`
	} `yaml:"genericLogs"`
	Timestamps []*struct {
		ClusterName string `yaml:"clusterName"`
		Defs        []*struct {
			Definition string `yaml:"definition"`
			Regex      string `yaml:"regex"`
			regex      *regexp.Regexp
		} `yaml:"defs"`
	} `yaml:"timestamps"`
	Multiline []*struct {
		StartLineSearch string `yaml:"startLineSearch"`
		ReMatchLines    string `yaml:"reMatchLines"`
		reMatchLines    *regexp.Regexp
		ReMatchJoin     []struct {
			Re       string `yaml:"re"`
			re       *regexp.Regexp
			MatchSeq int `yaml:"matchSeq"`
		} `yaml:"reMatchJoin"`
	} `yaml:"multilineJoins"`
	GlobalLabels  []string `yaml:"labels"`
	LabelsSetName string   `yaml:"labelsSetName"`
	Defs          []*struct {
		ClusterName string `yaml:"clusterName"`
		Patterns    []*struct {
			Name    string `yaml:"setName"`
			Search  string `yaml:"search"`
			Replace []*struct {
				Regex string `yaml:"regex"`
				regex *regexp.Regexp
				Sub   string `yaml:"sub"`
			} `yaml:"replace"`
			Regex         []string `yaml:"export"`
			regex         []*regexp.Regexp
			RegexAdvanced []struct {
				Regex   string `yaml:"regex"`
				regex   *regexp.Regexp
				SetName string `yaml:"setName"`
			} `yaml:"exportAdvanced"`
			StoreNodePrefix     string            `yaml:"storeNodePrefix"`
			Labels              []string          `yaml:"labels"` // used to define which regex matches are labels (to be stuck in metadata)
			DefaultValuePadding map[string]string `yaml:"defaultValuePadding"`
			Histogram           *struct {
				Buckets       []string `yaml:"buckets"`
				GenCumulative bool     `yaml:"generateCumulative"`
			} `yaml:"histogram"`
			Aggregate *struct {
				Every     time.Duration `yaml:"every"`     // aggregation time window
				Field     string        `yaml:"field"`     // field to use for aggregation counts
				On        string        `yaml:"on"`        // aggregate on this result value being the unique value
				Increment bool          `yaml:"increment"` // increment the field value counts by 1 as part of aggregation; this is for lines that have (repeated:X) where the repeated stat does not contain the first argument, only repeat counts
			} `yaml:"aggregate"`
		} `yaml:"patterns"`
		// matcher is the Aho-Corasick automaton over every
		// Patterns[i].Search string for this Defs entry. Built
		// once in (*patterns).compile() and used by lineProcess to
		// pick the first matching pattern in O(len(line)) instead
		// of N strings.Contains scans. Lower-case name => yaml
		// decoder skips it.
		matcher *acMatcher
	} `yaml:"defs"`
}

type Progress struct {
	sync.RWMutex
	Downloader           *ProgressDownloader
	Unpacker             *ProgressUnpacker
	PreProcessor         *ProgressPreProcessor
	LogProcessor         *ProgressLogProcessor
	CollectinfoProcessor *ProgressCollectProcessor
}

type ProgressDownloader struct {
	S3Files    map[string]*DownloaderFile // map[key]*details
	SftpFiles  map[string]*DownloaderFile // map[path]*details
	Finished   bool
	running    bool
	wasRunning bool
	changed    bool
}

type ProgressUnpacker struct {
	Files      map[string]*EnumFile // map[path]*details
	Finished   bool
	running    bool
	wasRunning bool
	changed    bool
}

type ProgressPreProcessor struct {
	Files                     map[string]*EnumFile // map[path]*details
	CollectInfoUniquePrefixes int
	Finished                  bool
	running                   bool
	wasRunning                bool
	changed                   bool
	LastUsedPrefix            int
	LastUsedSuffixForPrefix   map[int]int
	NodeToPrefix              map[string]int
}

type ProgressLogProcessor struct {
	Files      map[string]*LogFile
	Finished   bool
	running    bool
	wasRunning bool
	changed    bool
	StartTime  time.Time
	LineErrors *lineErrors
}

type LogFile struct {
	ClusterName string
	NodePrefix  string
	NodeID      string
	NodeSuffix  string
	Size        int64
	Processed   int64
	Finished    bool
	StartTime   string
	FinishTime  string
}

type ProgressCollectProcessor struct {
	Files      map[string]*CfFile
	Finished   bool
	running    bool
	wasRunning bool
	changed    bool
}

type CfFile struct {
	Size                int64
	NodeID              string
	RenameAttempted     bool
	Renamed             bool
	OriginalName        string
	ProcessingAttempted bool
	Processed           bool
	Errors              []string
	StartTime           string
	FinishTime          string
}

type DownloaderFile struct {
	Size         int64
	LastModified time.Time
	IsDownloaded bool
	Error        string
	StartTime    string
	FinishTime   string
}

type EnumFile struct {
	Size                  int64
	mimeType              *mimetype.MIME
	ContentType           string
	IsCollectInfo         bool
	IsArchive             bool
	IsText                bool
	IsTarGz               bool
	IsTarBz               bool
	UnpackFailed          bool
	Errors                []string
	PreProcessDuplicateOf []string
	StartAt               int64 // workaround for log files starting at binary 000s
	PreProcessOutPaths    []string
}

type jsonPayload struct {
	JsonPayload struct {
		Log          string `json:"log"`           // type 1: full log line in here
		Level        string `json:"level"`         // type 2: INFO
		Module       string `json:"module"`        // type 2: info
		ModuleDetail string `json:"module_detail"` // type 2: ticker.c:497
		Message      string `json:"message"`       // type 2: {test} memory-usage: total-bytes 0 index-bytes 0 sindex-bytes 0 data-bytes 0 used-pct 0.00
	} `json:"jsonPayload"`
	Timestamp   string   `json:"timestamp"`   // type 2: 2021-06-18T10:00:00Z
	TextPayload string   `json:"textPayload"` // type 3
	Resource    struct { // type 3
		Labels struct { // type 3
			PodName string `json:"pod_name"` // type 3
		} `json:"labels"` // type 3
	} `json:"resource"` // type 3
	Log string `json:"log"` // type 4
}

type IngestStatusStruct struct {
	Ingest struct {
		Running                  bool
		CompleteSteps            *IngestSteps
		DownloaderCompletePct    int
		DownloaderTotalSize      int64
		DownloaderCompleteSize   int64
		LogProcessorCompletePct  int
		LogProcessorTotalSize    int64
		LogProcessorCompleteSize int64
		Errors                   []string
		ErrorCount               int
	}
	PluginRunning        bool
	GrafanaHelperRunning bool
	System               struct {
		DiskTotalBytes   uint64
		DiskFreeBytes    uint64
		MemoryTotalBytes int
		MemoryFreeBytes  int
	}
}

type IngestSteps struct {
	Init                        bool
	Download                    bool
	Unpack                      bool
	PreProcess                  bool
	ProcessLogs                 bool
	ProcessCollectInfo          bool
	CriticalError               string
	InitStartTime               time.Time
	InitEndTime                 time.Time
	DownloadStartTime           time.Time
	DownloadEndTime             time.Time
	UnpackStartTime             time.Time
	UnpackEndTime               time.Time
	PreProcessStartTime         time.Time
	PreProcessEndTime           time.Time
	ProcessLogsStartTime        time.Time
	ProcessLogsEndTime          time.Time
	ProcessCollectInfoStartTime time.Time
	ProcessCollectInfoEndTime   time.Time
}

type NotifyEvent struct {
	AGIName                    string
	Event                      string
	EventDetail                string
	IsDataInMemory             bool
	IngestStatus               *IngestStatusStruct
	DeploymentJsonGzB64        string
	SSHAuthorizedKeysFileGzB64 string
	Owner                      string
	S3Source                   string
	SftpSource                 string
	LocalSource                string
	Label                      string
}

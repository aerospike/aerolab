package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"

	"github.com/aerospike/aerolab/pkg/agi/db"
	"github.com/creasty/defaults"
	"github.com/rglonek/envconfig"
	"gopkg.in/yaml.v3"
)

func MakeConfigReader(setDefaults bool, configYaml io.Reader, parseEnv bool) (*Config, error) {
	config := new(Config)
	config.Downloader.S3Source = &S3Source{}
	config.Downloader.SftpSource = &SftpSource{}
	if setDefaults {
		if err := defaults.Set(config); err != nil {
			return nil, fmt.Errorf("could not set defaults: %s", err)
		}
	}
	if configYaml != nil {
		err := yaml.NewDecoder(configYaml).Decode(config)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %s", err)
		}
	}
	if parseEnv {
		err := envconfig.Process("LOGINGEST_", config)
		if err != nil {
			return nil, fmt.Errorf("could not process environment variables: %s", err)
		}
	}
	return config, nil
}

func MakeConfig(setDefaults bool, configFile string, parseEnv bool) (*Config, error) {
	if configFile != "" {
		cf, err := os.Open(configFile)
		if err != nil {
			return nil, fmt.Errorf("could not open config file: %s", err)
		}
		defer cf.Close()
		return MakeConfigReader(setDefaults, cf, parseEnv)
	}
	return MakeConfigReader(setDefaults, nil, parseEnv)
}

// Init opens its own embedded db handle and initialises the ingest
// service. Use InitWithDB instead when ingest needs to share a handle
// with the plugin in the same process — Init's exclusive Pebble lock
// would otherwise block the co-resident plugin from opening the same
// directory.
func Init(config *Config) (*Ingest, error) {
	i, err := newIngest(config)
	if err != nil {
		return nil, err
	}
	log.Printf("DEBUG: INIT: Connect to backend")
	if err := i.dbConnect(); err != nil {
		// Use %w (not %s) so callers can errors.Is(err,
		// db.ErrStorageVersionMismatch) and trigger the
		// wipe-and-retry path on a v1→v2 upgrade.
		return nil, fmt.Errorf("could not connect to the database: %w", err)
	}
	log.Printf("DEBUG: INIT: Backend connected")
	return finalizeInit(i)
}

// InitWithDB is like Init but uses an externally-owned db handle. The
// caller retains ownership: Close() on the returned Ingest will NOT
// close d. This is the entry-point used by the merged agi service
// (cmdAgiExecService) where ingest and plugin share a single Pebble
// store opened once at process start.
//
// The caller is responsible for closing d after both ingest and plugin
// have shut down.
func InitWithDB(config *Config, d *db.DB) (*Ingest, error) {
	if d == nil {
		return nil, errors.New("ingest: InitWithDB: db handle is required")
	}
	i, err := newIngest(config)
	if err != nil {
		return nil, err
	}
	i.db = d
	i.ownsDB = false
	if err := i.registerSets(); err != nil {
		return nil, fmt.Errorf("registerSets: %s", err)
	}
	return finalizeInit(i)
}

// newIngest builds the Ingest struct from config, compiles patterns and
// regexes, and populates the default bin list. It does NOT open a db
// handle — the caller (Init or InitWithDB) does that.
func newIngest(config *Config) (*Ingest, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}
	if config.LogLevel >= 5 {
		log.Printf("DEBUG: ==== CONFIG ====")
		//nolint:errcheck
		yaml.NewEncoder(os.Stdout).Encode(config)
	}
	p := new(patterns)
	if config.PatternsFile == "" {
		log.Printf("DEBUG: INIT: Loading embedded patterns")
		err := yaml.Unmarshal(patternEmbed, p)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal patterns: %s", err)
		}
	} else {
		log.Printf("DEBUG: INIT: Loading %s", config.PatternsFile)
		f, err := os.Open(config.PatternsFile)
		if err != nil {
			return nil, fmt.Errorf("failed to open specified patterns file: %s", err)
		}
		defer f.Close()
		err = yaml.NewDecoder(f).Decode(p)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal patterns: %s", err)
		}
	}
	log.Printf("DEBUG: INIT: Compiling patterns")
	if err := p.compile(); err != nil {
		return nil, err
	}
	log.Printf("DEBUG: INIT: Compiling config regexes")
	if config.Downloader.S3Source.SearchRegex != "" {
		regex, err := regexp.Compile(config.Downloader.S3Source.SearchRegex)
		if err != nil {
			return nil, fmt.Errorf("failed to compile %s: %s", config.Downloader.S3Source.SearchRegex, err)
		}
		config.Downloader.S3Source.searchRegex = regex
	}
	if config.Downloader.SftpSource.SearchRegex != "" {
		regex, err := regexp.Compile(config.Downloader.SftpSource.SearchRegex)
		if err != nil {
			return nil, fmt.Errorf("failed to compile %s: %s", config.Downloader.SftpSource.SearchRegex, err)
		}
		config.Downloader.SftpSource.searchRegex = regex
	}
	regex, err := regexp.Compile(config.FindClusterNameNodeIdRegex)
	if err != nil {
		return nil, fmt.Errorf("failed to compile %s: %s", config.FindClusterNameNodeIdRegex, err)
	}
	config.findClusterNameNodeIdRegex = regex
	defaultBins := []string{
		"BINLIST",
		"cfName",
		"summary",
		"health",
		"conffile",
		"sysinfo",
		"build",
		"clientConns",
		"ip",
		"migrations",
		"nodeId",
		"uptime",
		"integrity",
		"clusterKey",
		"principal",
		"clusterSize",
		"clusterName",
	}
	bl := &binList{
		BinNames: defaultBins,
		// Force the first storeBinList() to actually persist the
		// row. Without this the plugin's cacheBinList() never
		// finds BINLIST until a pattern emits a previously-unseen
		// bin name, which can leave Grafana with an empty bin
		// cache for an entire ingest cycle on a fresh AGI.
		changed: true,
	}
	bl.seedSnapshot()
	return &Ingest{
		config:   config,
		patterns: p,
		progress: new(Progress),
		binList:  bl,
	}, nil
}

// finalizeInit runs the post-dbConnect steps shared between Init and
// InitWithDB: load the persisted BINLIST, start pprof (if configured),
// load progress, publish initial label rows, and spin up the background
// progress goroutines.
func finalizeInit(i *Ingest) (*Ingest, error) {
	// labelsSet is "" when the patterns YAML omits labelsSetName or
	// when patterns is nil (Init paths that never load a patterns
	// file). Every db.Put/Get in this function targets the labels
	// set, so a single resolve up front lets us skip the entire
	// metadata block instead of hitting "db: Put: set is required"
	// errors row-by-row.
	labelsSet := ""
	if i.patterns != nil {
		labelsSet = i.patterns.LabelsSetName
	}
	// load the bin list
	if labelsSet != "" {
		row, err := i.db.Get(labelsSet, "BINLIST", labelsValueCol)
		if err != nil {
			log.Printf("DEBUG: INIT: could not get bin list: %s", err)
		} else if row != nil {
			if s, ok := row[labelsValueCol].AsString(); ok {
				aBinList := []string{}
				if jerr := json.Unmarshal([]byte(s), &aBinList); jerr != nil {
					log.Printf("DEBUG: INIT: could not unmarshal bin list: %s", jerr)
				} else {
					i.binList.BinNames = aBinList
					// Re-seed the presence snapshot from the loaded
					// BINLIST so the hot path's lock-free probe in
					// upsertMetaEntry / ProcessLogs sees every
					// previously-persisted bin name on first sight
					// and avoids spurious BINLIST updates.
					i.binList.seedSnapshot()
					log.Printf("DEBUG: INIT: Existing bin list loaded")
				}
			}
		}
	}
	if i.config.CPUProfilingOutputFile != "" {
		log.Printf("DEBUG: INIT: Enabling CPU profiling")
		var err error
		i.cpuProfile, err = os.Create(i.config.CPUProfilingOutputFile)
		if err != nil {
			return nil, fmt.Errorf("could not create file %s: %s", i.config.CPUProfilingOutputFile, err)
		}
		err = pprof.StartCPUProfile(i.cpuProfile)
		if err != nil {
			return nil, fmt.Errorf("could not start CPU profiling: %s", err)
		}
		i.pprofRunning = true
		// Arm the block + mutex profilers alongside the CPU
		// profile. cpu.pprof only sees on-CPU work; off-CPU
		// stalls (channel waits, mutex contention) are invisible
		// there. AGI ingest pipelines are dominated by goroutines
		// parked on resultsChan / per-shard channels / the
		// progress mutex during EFS flushes — all silent in
		// cpu.pprof. The block+mutex profiles plus a goroutine
		// snapshot at Close are what locate those gates. We arm
		// them here (rather than in cmdAgiCreate) so every entry
		// point that sets CPUProfilingOutputFile (the CLI flag,
		// AgiRetrigger, the LOGINGEST_CPUPROFILE_FILE env var)
		// gets the full debug bundle from one switch.
		//
		// Block profile rate is in nanoseconds: events that block
		// for >= rate ns are sampled. 1 means "every event" and
		// produces a lot of noise from sub-microsecond channel
		// ops; 1000 (1µs) keeps the long-tail signal we care
		// about while keeping overhead bounded. Mutex fraction
		// of 1 captures every contended unlock — volume is
		// naturally bounded because uncontended unlocks are not
		// sampled.
		runtime.SetBlockProfileRate(1000)
		runtime.SetMutexProfileFraction(1)
	}
	if err := i.loadProgress(); err != nil {
		return nil, err
	}
	log.Printf("DEBUG: INIT: Update DB labels")
	sources := ""
	if i.config.Downloader.S3Source.Enabled {
		sources = "s3 " + i.config.Downloader.S3Source.BucketName + ":/" + i.config.Downloader.S3Source.PathPrefix + i.config.Downloader.S3Source.SearchRegex
	}
	if i.config.Downloader.SftpSource.Enabled {
		if sources != "" {
			sources = sources + "\n"
		}
		sources = sources + "sftp " + i.config.Downloader.SftpSource.Host + ":" + i.config.Downloader.SftpSource.PathPrefix + i.config.Downloader.SftpSource.SearchRegex
	}
	if i.config.CustomSourceName != "" {
		if sources != "" {
			sources = sources + "\n"
		}
		sources = sources + "local " + i.config.CustomSourceName
	}
	if i.config.Live.Enabled {
		if sources != "" {
			sources = sources + "\n"
		}
		sources = sources + "live streams"
	}
	if labelsSet != "" {
		metajson, _ := json.Marshal(&metaEntries{
			Entries: []string{sources},
		})
		if perr := i.db.Put(labelsSet, "sources", db.Row{labelsValueCol: db.Str(string(metajson))}); perr != nil {
			log.Printf("ERROR: Could not insert sources label: %s", perr)
		}
		if i.config.IngestTimeRanges.Enabled {
			metajson, _ := json.Marshal(&metaEntries{
				Entries: []string{i.config.IngestTimeRanges.From.String() + " - " + i.config.IngestTimeRanges.To.String()},
			})
			if perr := i.db.Put(labelsSet, "timerange", db.Row{labelsValueCol: db.Str(string(metajson))}); perr != nil {
				log.Printf("ERROR: Could not insert timerange label: %s", perr)
			}
		}
	} else {
		log.Printf("DEBUG: INIT: labels set name is empty; skipping label writes")
	}
	i.endLock = new(sync.Mutex)
	// Drop the dirty-ingest sentinel ("we have written rows that
	// may not be flushed to disk yet") into the progress dir
	// BEFORE we start the metric-row batcher. The orchestrating
	// CLI consults the same marker on next startup: with WAL=off
	// the marker triggers a soft wipe (DB + log-processor.json
	// reset; Downloader / Unpacker / PreProcessor progress is
	// preserved so logs/ that already landed on disk are reused).
	// Failure here is logged but not fatal — at worst a future
	// crash-on-WAL-off run misses the wipe and re-ingests once.
	if merr := WriteDirtyMarker(i.config.ProgressFile.OutputFilePath); merr != nil {
		log.Printf("WARN: INIT: could not write dirty marker: %s", merr)
	}
	// Spin up the metric-row batcher BEFORE the progress goroutines
	// so a SIGTERM right after Init still has a defined batcher to
	// drain in Close (the close() helper handles a never-used
	// batcher cleanly). The fallback handles type-conflict
	// retries the same way the pre-batcher putData hot path did.
	i.putBatcher = newPutBatcher(i.db, i.config.PutBatchSize, i.config.PutBatchFlushMs, i.config.PutBatchShards, i.putDataSingle)
	i.bg.Add(2)
	go func() {
		defer i.bg.Done()
		i.saveProgressInterval()
	}()
	go func() {
		defer i.bg.Done()
		i.printProgressInterval()
	}()
	return i, nil
}

// labelsValueCol is the single, fixed column name used by every row in
// the labels set (BINLIST, sources, timerange, cfName, and the
// per-metric meta rows written during ProcessLogs). The plugin's
// cache refresher reads the labels set with project=[labelsValueCol],
// so ingest and plugin must agree on the column name or the plugin
// silently observes an empty metadata map. Previously ingest wrote
// using the key-name as the column-name (e.g. Row{"BINLIST":...},
// Row{"sources":...}) while the plugin read column "json" — the
// plugin never saw any label data. Using a single constant in both
// packages (plugin also has its own symmetric constant) makes the
// contract explicit and enforced at compile time.
const labelsValueCol = "json"

// Close releases the resources owned by this Ingest. It is safe to call
// multiple times and from concurrent goroutines (e.g. a SIGTERM
// handler racing the normal-completion deferred call). Close only
// closes the underlying db handle when ownsDB=true (i.e. Init opened
// the handle itself); when the handle was injected via InitWithDB, the
// caller retains ownership and must close it.
func (i *Ingest) Close() {
	i.closeOnce.Do(func() {
		log.Printf("DEBUG: CLOSE: Saving progress")
		// Signal the interval goroutines to exit on their next
		// wake-up, then Wait so they can't start a new
		// saveProgress/printProgress after we've closed the db.
		if i.endLock != nil {
			i.endLock.Lock()
			i.end = true
			i.endLock.Unlock()
		}
		i.bg.Wait()
		// Drain the metric-row batcher BEFORE saving progress
		// or closing the db: any rows still buffered by the
		// flusher must commit so saveProgress/printProgress
		// observe a consistent state, and (when ownsDB=true)
		// db.Close can flush an empty memtable. close() blocks
		// until the flusher's run loop returns, which guarantees
		// every queued PutBatch has been issued.
		if i.putBatcher != nil {
			i.putBatcher.close()
			i.putBatcher = nil
		}
		if err := i.saveProgress(); err != nil {
			log.Printf("ERROR: Could not save progress: %s", err)
		}
		if err := i.printProgress(); err != nil {
			log.Printf("ERROR: Could not print progress: %s", err)
		}
		if i.pprofRunning {
			log.Printf("DEBUG: CLOSE: Stopping CPU profiling")
			pprof.StopCPUProfile()
			i.pprofRunning = false
		}
		if i.cpuProfile != nil {
			log.Printf("DEBUG: CLOSE: Closing CPU profiling file")
			i.cpuProfile.Close()
			i.cpuProfile = nil
		}
		// Drain the block / mutex / goroutine profiles into
		// sibling files next to cpu.<...>.pprof. Done after
		// StopCPUProfile so the final samples include any
		// shutdown-time stalls (drainage of putBatcher, last
		// saveProgress, db.Close). Errors here never fail the
		// close — these are observability artefacts; the ingest
		// itself succeeded if we got this far.
		if i.config.CPUProfilingOutputFile != "" {
			i.writeRuntimeProfiles(i.config.CPUProfilingOutputFile)
			runtime.SetBlockProfileRate(0)
			runtime.SetMutexProfileFraction(0)
		}
		if i.ownsDB && i.db != nil {
			log.Printf("DEBUG: CLOSE: Closing db store")
			if err := i.db.Close(); err != nil {
				log.Printf("ERROR: db.Close: %s", err)
			}
			i.db = nil
		}
	})
}

// runtimeProfileSiblingPath derives the output path for a non-CPU
// runtime profile (block / mutex / goroutine) next to the configured
// CPU profile. The canonical AGI layout is "/opt/agi/cpu.ingest.pprof",
// for which this returns "/opt/agi/<kind>.ingest.pprof". When the base
// name does not start with "cpu." we fall back to "<stem>-<kind><ext>"
// so operators using a non-default naming scheme still get
// recognisable sibling files instead of a silent collision with the
// CPU profile path.
func runtimeProfileSiblingPath(cpuPath, kind string) string {
	dir, base := filepath.Split(cpuPath)
	if strings.HasPrefix(base, "cpu.") {
		return filepath.Join(dir, kind+"."+strings.TrimPrefix(base, "cpu."))
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+"-"+kind+ext)
}

// writeRuntimeProfiles dumps the block, mutex, and goroutine profiles
// into sibling files of cpuPath. Each profile is independent — a
// failure on one does not abort the others — because the operator
// usually wants whatever profiles did succeed even if one filesystem
// op fails (e.g. EFS hiccup on the final write). The goroutine
// profile uses debug=2 so the output is the human-readable goroutine
// dump (every goroutine's full stack), which is what we actually
// want for "what is everything parked on at shutdown?"; the block
// and mutex profiles use debug=0 (binary pprof) so they round-trip
// through `go tool pprof` directly.
func (i *Ingest) writeRuntimeProfiles(cpuPath string) {
	type profile struct {
		kind  string
		debug int
	}
	for _, p := range []profile{
		{kind: "block", debug: 0},
		{kind: "mutex", debug: 0},
		{kind: "goroutine", debug: 2},
	} {
		path := runtimeProfileSiblingPath(cpuPath, p.kind)
		f, err := os.Create(path)
		if err != nil {
			log.Printf("WARN: CLOSE: could not create %s profile %q: %s", p.kind, path, err)
			continue
		}
		prof := pprof.Lookup(p.kind)
		if prof == nil {
			log.Printf("WARN: CLOSE: %s profile is not registered; skipping", p.kind)
			f.Close()
			continue
		}
		if werr := prof.WriteTo(f, p.debug); werr != nil {
			log.Printf("WARN: CLOSE: could not write %s profile %q: %s", p.kind, path, werr)
		}
		if cerr := f.Close(); cerr != nil {
			log.Printf("WARN: CLOSE: could not close %s profile %q: %s", p.kind, path, cerr)
		}
		log.Printf("DEBUG: CLOSE: wrote %s profile to %s", p.kind, path)
	}
}

func (p *patterns) compile() error {
	for j := range p.Timestamps {
		for i := range p.Timestamps[j].Defs {
			log.Printf("DETAIL: REGEX: compiling timestamps:%s", p.Timestamps[j].Defs[i].Regex)
			regex, err := regexp.Compile(p.Timestamps[j].Defs[i].Regex)
			if err != nil {
				return fmt.Errorf("failed to compile %s: %s", p.Timestamps[j].Defs[i].Regex, err)
			}
			p.Timestamps[j].Defs[i].regex = regex
		}
	}
	for i := range p.Multiline {
		log.Printf("DETAIL: REGEX: compiling multiline:%s", p.Multiline[i].ReMatchLines)
		regex, err := regexp.Compile(p.Multiline[i].ReMatchLines)
		if err != nil {
			return fmt.Errorf("failed to compile %s: %s", p.Multiline[i].ReMatchLines, err)
		}
		p.Multiline[i].reMatchLines = regex
		for j := range p.Multiline[i].ReMatchJoin {
			log.Printf("DETAIL: REGEX: compiling multiline-join:%s", p.Multiline[i].ReMatchJoin[j].Re)
			regex, err := regexp.Compile(p.Multiline[i].ReMatchJoin[j].Re)
			if err != nil {
				return fmt.Errorf("failed to compile %s: %s", p.Multiline[i].ReMatchJoin[j].Re, err)
			}
			p.Multiline[i].ReMatchJoin[j].re = regex
		}
	}
	for k := range p.Defs {
		for i := range p.Defs[k].Patterns {
			for j := range p.Defs[k].Patterns[i].Regex {
				log.Printf("DETAIL: REGEX: compiling pattern:%s", p.Defs[k].Patterns[i].Regex[j])
				regex, err := regexp.Compile(p.Defs[k].Patterns[i].Regex[j])
				if err != nil {
					return fmt.Errorf("failed to compile %s: %s", p.Defs[k].Patterns[i].Regex[j], err)
				}
				p.Defs[k].Patterns[i].regex = append(p.Defs[k].Patterns[i].regex, regex)
			}
			for j := range p.Defs[k].Patterns[i].RegexAdvanced {
				log.Printf("DETAIL: REGEX: compiling pattern:%s", p.Defs[k].Patterns[i].RegexAdvanced[j].Regex)
				regex, err := regexp.Compile(p.Defs[k].Patterns[i].RegexAdvanced[j].Regex)
				if err != nil {
					return fmt.Errorf("failed to compile %s: %s", p.Defs[k].Patterns[i].RegexAdvanced[j].Regex, err)
				}
				p.Defs[k].Patterns[i].RegexAdvanced[j].regex = regex
			}
			for j := range p.Defs[k].Patterns[i].Replace {
				log.Printf("DETAIL: REGEX: compiling pattern-replace:%s", p.Defs[k].Patterns[i].Replace[j].Regex)
				regex, err := regexp.Compile(p.Defs[k].Patterns[i].Replace[j].Regex)
				if err != nil {
					return fmt.Errorf("failed to compile %s: %s", p.Defs[k].Patterns[i].Replace[j].Regex, err)
				}
				p.Defs[k].Patterns[i].Replace[j].regex = regex
			}
		}
		// Build the Aho-Corasick matcher over every pattern's
		// `search` substring for this Defs entry. This replaces
		// the linear `for _, p := range Patterns { if
		// !strings.Contains(line, p.Search) continue }` walk in
		// lineProcess with a single O(len(line)) traversal.
		// Empty `search` strings are rejected at compile time:
		// today's embedded patterns.yml has none, but a custom
		// PatternsFile could, and an empty needle would degenerate
		// the AC trie into matching at every input position.
		if len(p.Defs[k].Patterns) > 0 {
			needles := make([]string, len(p.Defs[k].Patterns))
			for i := range p.Defs[k].Patterns {
				needles[i] = p.Defs[k].Patterns[i].Search
				if needles[i] == "" {
					return fmt.Errorf("patterns: defs[%d].patterns[%d] (setName=%q) has empty `search`",
						k, i, p.Defs[k].Patterns[i].Name)
				}
			}
			m, err := newACMatcher(needles)
			if err != nil {
				return fmt.Errorf("patterns: defs[%d]: build AC matcher: %w", k, err)
			}
			p.Defs[k].matcher = m
		}
	}
	return nil
}

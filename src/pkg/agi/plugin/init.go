package plugin

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
	"github.com/creasty/defaults"
	"github.com/rglonek/envconfig"
	"gopkg.in/yaml.v3"
)

func MakeConfigReader(setDefaults bool, configYaml io.Reader, parseEnv bool) (*Config, error) {
	config := new(Config)
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
		err := envconfig.Process("PLUGIN_", config)
		if err != nil {
			return nil, fmt.Errorf("could not process environment variables: %s", err)
		}
	}
	return config, nil
}

func MakeConfig(setDefaults bool, configFile string, parseEnv bool) (*Config, error) {
	var cf *os.File
	var err error
	if configFile != "" {
		cf, err = os.Open(configFile)
		if err != nil {
			return nil, fmt.Errorf("could not open config file: %s", err)
		}
		defer cf.Close()
	}
	return MakeConfigReader(setDefaults, cf, parseEnv)
}

// Init opens its own embedded db handle and initialises the plugin
// service. Use InitWithDB instead when plugin needs to share a handle
// with ingest in the same process — Init's exclusive Pebble lock would
// otherwise block the co-resident ingest from opening the same
// directory.
//
// Init also auto-starts the configured CPU profile (if any) as part of
// finalizeInit. The standalone path owns the whole process, so capturing
// from t=0 is the right default. The merged-service path (InitWithDB)
// must NOT auto-start: ingest's own pprof would clash with it (pprof is
// process-global) and the operator wants plugin samples without ingest
// noise — see StartCPUProfile.
func Init(config *Config) (*Plugin, error) {
	p, err := newPlugin(config)
	if err != nil {
		return nil, err
	}
	log.Printf("DEBUG: INIT: Connect to backend")
	if err := p.dbConnect(); err != nil {
		return nil, fmt.Errorf("could not connect to the database: %s", err)
	}
	log.Printf("DEBUG: INIT: Backend connected")
	if _, err := finalizeInit(p); err != nil {
		return nil, err
	}
	if p.config.CPUProfilingOutputFile != "" {
		if err := p.StartCPUProfile(); err != nil {
			p.Close()
			return nil, err
		}
	}
	return p, nil
}

// InitWithDB is like Init but uses an externally-owned db handle. The
// caller retains ownership: Close() on the returned Plugin will NOT
// close d. This is the entry-point used by the merged agi service
// (cmdAgiExecService) where ingest and plugin share a single Pebble
// store opened once at process start.
//
// Unlike Init, InitWithDB does NOT auto-start CPU profiling even when
// CPUProfilingOutputFile is configured. The merged service runs ingest
// and plugin in the same process and Go's runtime/pprof CPU profiler is
// process-global; the orchestrator (cmdAgiExecService) calls
// StartCPUProfile explicitly once ingest has finished and released the
// profiler, which gives a clean plugin-only profile.
func InitWithDB(config *Config, d *db.DB) (*Plugin, error) {
	if d == nil {
		return nil, errors.New("plugin: InitWithDB: db handle is required")
	}
	p, err := newPlugin(config)
	if err != nil {
		return nil, err
	}
	p.db = d
	p.ownsDB = false
	return finalizeInit(p)
}

// labelsValueCol is the single, fixed column name used by every row in
// the labels set (BINLIST, sources, timerange, cfName, and the
// per-metric meta rows written during ingest). The plugin's cache
// refresher reads the labels set with project=[labelsValueCol]; ingest
// has its own symmetric constant. They MUST agree or the plugin
// silently observes an empty metadata map.
const labelsValueCol = "json"

// ClusterNameLabel is the well-known metadata key that ingest writes for
// every metric row carrying a cluster-name label. It shows up as both a
// row in the labels set (p.cache.metadata["ClusterName"]) and as a
// column on the metric sets that filter by cluster. Hoisting it into a
// constant keeps the histogram handler and any other per-cluster lookup
// from drifting if we ever rename the pattern output (a quiet rename
// would otherwise break histograms without any test failure).
const ClusterNameLabel = "ClusterName"

func newPlugin(config *Config) (*Plugin, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}
	if config.LogLevel >= 5 {
		log.Printf("DEBUG: ==== CONFIG ====")
		yaml.NewEncoder(os.Stdout).Encode(config) //nolint:errcheck
	}
	p := &Plugin{
		config:   config,
		requests: make(chan bool, config.MaxConcurrentRequests),
		jobs:     make(chan bool, config.MaxConcurrentJobs),
		done:     make(chan struct{}),
	}
	p.cache.lock = new(sync.RWMutex)
	p.cache.metadata = make(map[string]*metaEntries)
	return p, nil
}

func finalizeInit(p *Plugin) (*Plugin, error) {
	p.bg.Add(2)
	go func() {
		defer p.bg.Done()
		p.stats()
	}()
	go func() {
		defer p.bg.Done()
		p.queryAndCache()
	}()
	return p, nil
}

// StartCPUProfile starts a runtime/pprof CPU profile and writes samples
// to the path configured in Config.CPUProfilingOutputFile. It is safe to
// call multiple times: the second and subsequent calls are no-ops.
//
// Init() calls StartCPUProfile automatically because the standalone
// plugin owns the whole process. The merged-service path (InitWithDB)
// deliberately does NOT call it, because Go's CPU profiler is
// process-global and would clash with ingest's profile. cmdAgiExecService
// calls this explicitly once ingest has finished, so the resulting
// profile contains plugin work only.
//
// Returns:
//   - error: nil on success; nil also when CPUProfilingOutputFile is empty,
//     when the plugin is already shutting down, or when a profile is already
//     running. Returns a wrapped error if the output file cannot be created
//     or pprof.StartCPUProfile rejects it.
func (p *Plugin) StartCPUProfile() error {
	if p == nil || p.config == nil || p.config.CPUProfilingOutputFile == "" {
		return nil
	}
	p.pprofMu.Lock()
	defer p.pprofMu.Unlock()
	if p.pprofRunning {
		return nil
	}
	// Don't start a profile against a plugin that's already (or
	// concurrently) shutting down: Close() drains pprof under the
	// same lock, so without this guard we could create a file and
	// start sampling that nothing ever stops.
	select {
	case <-p.done:
		return nil
	default:
	}
	log.Printf("DEBUG: INIT: Enabling CPU profiling")
	f, err := os.Create(p.config.CPUProfilingOutputFile)
	if err != nil {
		return fmt.Errorf("could not create file %s: %s", p.config.CPUProfilingOutputFile, err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return fmt.Errorf("could not start CPU profiling: %s", err)
	}
	p.cpuProfile = f
	p.pprofRunning = true
	return nil
}

// StopCPUProfile stops the in-flight CPU profile (if any) and closes
// the output file, but does NOT tear down the rest of the plugin.
// After it returns, the file at the returned path is a complete,
// parseable pprof profile suitable for `go tool pprof`. Safe to call
// when no profile is running (no-op, returns ""). Safe to call
// concurrently with StartCPUProfile / RotateCPUProfile / Close — all
// four serialise on p.pprofMu.
//
// Returns:
//   - string: the on-disk path of the file that was just flushed, or
//     "" when there was nothing running to flush.
//
// Note: StartCPUProfile creates the output file with O_TRUNC, so the
// file at this path will be re-truncated to 0 bytes the moment a
// subsequent StartCPUProfile runs. Callers that want to keep the
// flushed bytes around must rename the file before re-arming —
// RotateCPUProfile does exactly that.
func (p *Plugin) StopCPUProfile() string {
	if p == nil {
		return ""
	}
	p.pprofMu.Lock()
	defer p.pprofMu.Unlock()
	return p.stopCPUProfileLocked()
}

// stopCPUProfileLocked is the body of StopCPUProfile and the matching
// block in Close. The caller MUST hold p.pprofMu. Returns the on-disk
// path of the file that was just flushed, or "" when no profile was
// running.
func (p *Plugin) stopCPUProfileLocked() string {
	if !p.pprofRunning {
		return ""
	}
	log.Printf("DEBUG: PPROF: Stopping CPU profiling")
	pprof.StopCPUProfile()
	p.pprofRunning = false
	path := ""
	if p.cpuProfile != nil {
		path = p.cpuProfile.Name()
		if err := p.cpuProfile.Close(); err != nil {
			log.Printf("WARN: PPROF: closing profile file %s: %s", path, err)
		}
		p.cpuProfile = nil
	}
	return path
}

// RotateCPUProfile flushes the in-flight CPU profile to a timestamped
// sibling of the configured output path, then starts a fresh profile
// at that configured path. Use this from a SIGUSR1 handler to obtain
// a complete profile dump from a long-lived plugin without restarting
// the service: Go's runtime/pprof buffers the entire profile in memory
// until StopCPUProfile is called, so the only way to get bytes on disk
// short of process exit is to stop-and-restart.
//
// On entry a profile may or may not already be running. If one was,
// it is flushed and the file is renamed to
// "<CPUProfilingOutputFile>.<UTC-stamp>" before a new profile is
// started at the original path. The "current" file therefore always
// sits at the configured path and is always 0 bytes (it is the live
// profile); each rotation produces a complete, parseable pprof file
// with a timestamp suffix.
//
// Returns:
//   - rotated: the path of the just-flushed, timestamp-suffixed file.
//     Empty when there was no prior profile to flush (e.g. the very
//     first SIGUSR1 fires before the deferred coordinator goroutine
//     in cmdAgiExecService got around to starting one). A non-empty
//     value is suitable for `go tool pprof <rotated>`.
//   - err: a non-nil error means re-arming the next profile failed
//     (or, much more rarely, the rename failed). The just-flushed
//     file is still safely on disk in either case.
//
// No-op (returns "", nil) when CPUProfilingOutputFile is unset or the
// plugin is nil.
func (p *Plugin) RotateCPUProfile() (string, error) {
	if p == nil || p.config == nil || p.config.CPUProfilingOutputFile == "" {
		return "", nil
	}
	// Stop+rename under the lock so a concurrent StartCPUProfile
	// (e.g. the deferred coordinator in cmdAgiExecService) cannot
	// race in between the flush and the rename and re-truncate the
	// file we are about to preserve. StartCPUProfile is then called
	// outside the lock because it acquires the same mutex itself —
	// holding it across that call would deadlock.
	p.pprofMu.Lock()
	flushed := p.stopCPUProfileLocked()
	var rotated string
	if flushed != "" {
		// Nanosecond precision in the suffix so two rotates
		// within the same wall-clock second (operator script
		// fans out, test harness, etc.) cannot collide on
		// rename. os.Rename is silent-overwrite on POSIX, so a
		// colliding suffix would clobber the older capture.
		rotated = flushed + "." + time.Now().UTC().Format("20060102-150405.000000000Z")
		if err := os.Rename(flushed, rotated); err != nil {
			p.pprofMu.Unlock()
			// Do not re-arm: the next StartCPUProfile would
			// O_TRUNC the unrenamed flushed file and lose the
			// data we just collected. Surface the error so the
			// operator can inspect the un-rotated file at its
			// original path.
			return flushed, fmt.Errorf("rotate cpu profile: rename %s -> %s: %w", flushed, rotated, err)
		}
	}
	p.pprofMu.Unlock()

	if err := p.StartCPUProfile(); err != nil {
		return rotated, fmt.Errorf("rotate cpu profile: restart: %w", err)
	}
	return rotated, nil
}

// Close releases the resources owned by this Plugin. It is safe to call
// multiple times and from concurrent goroutines (e.g. SIGTERM racing
// the deferred call after Listen returns). Close only closes the
// underlying db handle when ownsDB=true (i.e. Init opened the handle
// itself); when the handle was injected via InitWithDB, the caller
// retains ownership and must close it.
func (p *Plugin) Close() {
	p.closeOnce.Do(func() {
		// Order matters and is load-bearing:
		//   1. Stop the HTTP server so no new requests land. Existing
		//      requests continue until srv.Shutdown's deadline.
		//   2. Close p.done so the cache refresher exits its sleep
		//      between cycles and doesn't start a new db.Scan.
		//   3. Wait for in-flight HTTP handlers (p.handlers) AND the
		//      refresher/stats goroutines (p.bg) to fully return.
		//      Both may hold open db.Iter handles.
		//   4. Only now close the db. Calling p.db.Close() with an
		//      open iterator returns ErrIteratorsOpen, leaves the db
		//      open, and leaks the Pebble file lock into the next
		//      process lifetime.
		if p.srv != nil {
			log.Printf("DEBUG: CLOSE: Shutting down HTTP server")
			p.Shutdown()
		}
		if p.done != nil {
			close(p.done)
		}
		p.handlers.Wait()
		p.bg.Wait()
		// Take pprofMu so a coordinator goroutine racing
		// StartCPUProfile (e.g. cmdAgiExecService firing it as
		// ingest finishes) can't open a file and start sampling
		// that no one stops. Close-after-done guards inside
		// StartCPUProfile complete the contract; close p.done
		// before this block runs (above). The same lock is
		// taken by SIGUSR1-driven RotateCPUProfile, so a
		// half-rotated flush cannot race teardown either.
		p.pprofMu.Lock()
		p.stopCPUProfileLocked()
		p.pprofMu.Unlock()
		if p.ownsDB && p.db != nil {
			log.Printf("DEBUG: CLOSE: Closing embedded db")
			if err := p.db.Close(); err != nil {
				log.Printf("WARN: CLOSE: db close: %s", err)
			}
			p.db = nil
		}
	})
}

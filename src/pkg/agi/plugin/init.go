package plugin

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/pprof"
	"sync"

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
	return finalizeInit(p)
}

// InitWithDB is like Init but uses an externally-owned db handle. The
// caller retains ownership: Close() on the returned Plugin will NOT
// close d. This is the entry-point used by the merged agi service
// (cmdAgiExecService) where ingest and plugin share a single Pebble
// store opened once at process start.
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
	if p.config.CPUProfilingOutputFile != "" {
		log.Printf("DEBUG: INIT: Enabling CPU profiling")
		var err error
		p.cpuProfile, err = os.Create(p.config.CPUProfilingOutputFile)
		if err != nil {
			return nil, fmt.Errorf("could not create file %s: %s", p.config.CPUProfilingOutputFile, err)
		}
		err = pprof.StartCPUProfile(p.cpuProfile)
		if err != nil {
			return nil, fmt.Errorf("could not start CPU profiling: %s", err)
		}
		p.pprofRunning = true
	}
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
		if p.pprofRunning {
			log.Printf("DEBUG: CLOSE: Stopping CPU profiling")
			pprof.StopCPUProfile()
			p.pprofRunning = false
		}
		if p.cpuProfile != nil {
			log.Printf("DEBUG: CLOSE: Closing CPU profiling file")
			p.cpuProfile.Close()
			p.cpuProfile = nil
		}
		if p.ownsDB && p.db != nil {
			log.Printf("DEBUG: CLOSE: Closing embedded db")
			if err := p.db.Close(); err != nil {
				log.Printf("WARN: CLOSE: db close: %s", err)
			}
			p.db = nil
		}
	})
}

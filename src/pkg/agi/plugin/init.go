package plugin

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/pprof"
	"sync"

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

func Init(config *Config) (*Plugin, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}
	if config.LogLevel >= 5 {
		log.Printf("DEBUG: ==== CONFIG ====")
		yaml.NewEncoder(os.Stdout).Encode(config)
	}
	log.Printf("DEBUG: INIT: Connect to backend")
	p := &Plugin{
		config:   config,
		requests: make(chan bool, config.MaxConcurrentRequests),
		jobs:     make(chan bool, config.MaxConcurrentJobs),
	}
	p.cache.lock = new(sync.RWMutex)
	p.cache.metadata = make(map[string]*metaEntries)
	err := p.dbConnect()
	if err != nil {
		return nil, fmt.Errorf("could not connect to the database: %s", err)
	}
	log.Printf("DEBUG: INIT: Backend connected")
	if config.CPUProfilingOutputFile != "" {
		log.Printf("DEBUG: INIT: Enabling CPU profiling")
		var err error
		p.cpuProfile, err = os.Create(config.CPUProfilingOutputFile)
		if err != nil {
			return nil, fmt.Errorf("could not create file %s: %s", config.CPUProfilingOutputFile, err)
		}
		err = pprof.StartCPUProfile(p.cpuProfile)
		if err != nil {
			return nil, fmt.Errorf("could not start CPU profiling: %s", err)
		}
		p.pprofRunning = true
	}
	go p.stats()
	go p.queryAndCache()
	return p, nil
}

func (p *Plugin) Close() {
	if p.pprofRunning {
		log.Printf("DEBUG: CLOSE: Stopping CPU profiling")
		pprof.StopCPUProfile()
	}
	if p.cpuProfile != nil {
		log.Printf("DEBUG: CLOSE: Closing CPU profiling file")
		p.cpuProfile.Close()
	}
}

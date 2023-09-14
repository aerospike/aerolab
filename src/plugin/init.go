package plugin

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"sync"

	"github.com/bestmethod/logger"
	"github.com/creasty/defaults"
	"github.com/rglonek/envconfig"
	"gopkg.in/yaml.v2"
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
	logger.SetLogLevel(config.LogLevel)
	if config.LogLevel >= 5 {
		logger.Debug("==== CONFIG ====")
		yaml.NewEncoder(os.Stdout).Encode(config)
	}
	logger.Debug("INIT: Connect to backend")
	p := &Plugin{
		config: config,
	}
	p.cache.lock = new(sync.RWMutex)
	p.cache.metadata = make(map[string][]*metaEntry)
	err := p.dbConnect()
	if err != nil {
		return nil, fmt.Errorf("could not connect to the database: %s", err)
	}
	logger.Debug("INIT: Backend connected")
	if config.CPUProfilingOutputFile != "" {
		logger.Debug("INIT: Enabling CPU profiling")
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
	go p.queryAndCache()
	return p, nil
}

func (p *Plugin) Close() {
	if p.pprofRunning {
		logger.Debug("CLOSE: Stopping CPU profiling")
		pprof.StopCPUProfile()
	}
	if p.cpuProfile != nil {
		logger.Debug("CLOSE: Closing CPU profiling file")
		p.cpuProfile.Close()
	}
}

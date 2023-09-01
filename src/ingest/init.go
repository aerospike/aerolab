package ingest

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime/pprof"

	"github.com/creasty/defaults"
	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v2"
)

func MakeConfig(setDefaults bool, configFile string, parseEnv bool) (*Config, error) {
	config := new(Config)
	if setDefaults {
		if err := defaults.Set(config); err != nil {
			return nil, fmt.Errorf("could not set defaults: %s", err)
		}
	}
	if configFile != "" {
		cf, err := os.Open(configFile)
		if err != nil {
			return nil, fmt.Errorf("could not open config file: %s", err)
		}
		defer cf.Close()
		err = yaml.NewDecoder(cf).Decode(config)
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

func Init(config *Config) (*Ingest, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}
	p := new(patterns)
	if config.PatternsFile == "" {
		err := yaml.Unmarshal(patternEmbed, p)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal patterns: %s", err)
		}
	} else {
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
	err := p.compile()
	if err != nil {
		return nil, err
	}
	i := &Ingest{
		config:   config,
		patterns: p,
		progress: new(progress),
	}
	err = i.dbConnect()
	if err != nil {
		return nil, fmt.Errorf("could not connect to the database: %s", err)
	}
	if config.CPUProfilingOutputFile != "" {
		var err error
		i.cpuProfile, err = os.Create(config.CPUProfilingOutputFile)
		if err != nil {
			return nil, fmt.Errorf("could not create file %s: %s", config.CPUProfilingOutputFile, err)
		}
		err = pprof.StartCPUProfile(i.cpuProfile)
		if err != nil {
			return nil, fmt.Errorf("could not start CPU profiling: %s", err)
		}
		i.pprofRunning = true
	}
	err = i.loadProgress()
	if err != nil {
		return nil, err
	}
	go i.saveProgressInterval()
	go i.printProgressInterval()
	return i, nil
}

func (i *Ingest) Close() {
	i.saveProgress()
	if i.pprofRunning {
		pprof.StopCPUProfile()
	}
	if i.cpuProfile != nil {
		i.cpuProfile.Close()
	}
}

func (p *patterns) compile() error {
	for i := range p.Timestamps {
		regex, err := regexp.Compile(p.Timestamps[i].Regex)
		if err != nil {
			return fmt.Errorf("failed to compile %s: %s", p.Timestamps[i].Regex, err)
		}
		p.Timestamps[i].regex = regex
	}
	for i := range p.Multiline {
		regex, err := regexp.Compile(p.Multiline[i].ReMatchLines)
		if err != nil {
			return fmt.Errorf("failed to compile %s: %s", p.Multiline[i].ReMatchLines, err)
		}
		p.Multiline[i].reMatchLines = regex
		for j := range p.Multiline[i].ReMatchJoin {
			regex, err := regexp.Compile(p.Multiline[i].ReMatchJoin[j].Re)
			if err != nil {
				return fmt.Errorf("failed to compile %s: %s", p.Multiline[i].ReMatchJoin[j].Re, err)
			}
			p.Multiline[i].ReMatchJoin[j].re = regex
		}
	}
	for i := range p.Patterns {
		for j := range p.Patterns[i].Regex {
			regex, err := regexp.Compile(p.Patterns[i].Regex[j])
			if err != nil {
				return fmt.Errorf("failed to compile %s: %s", p.Patterns[i].Regex[j], err)
			}
			p.Patterns[i].regex = append(p.Patterns[i].regex, regex)
		}
		for j := range p.Patterns[i].Replace {
			regex, err := regexp.Compile(p.Patterns[i].Replace[j].Regex)
			if err != nil {
				return fmt.Errorf("failed to compile %s: %s", p.Patterns[i].Replace[j].Regex, err)
			}
			p.Patterns[i].Replace[j].regex = regex
		}
	}
	return nil
}

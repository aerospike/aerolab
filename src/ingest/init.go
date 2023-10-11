package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/pprof"
	"sync"

	"github.com/aerospike/aerospike-client-go/v6"
	"github.com/bestmethod/logger"
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

func Init(config *Config) (*Ingest, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}
	logger.SetLogLevel(config.LogLevel)
	if config.LogLevel >= 5 {
		logger.Debug("==== CONFIG ====")
		yaml.NewEncoder(os.Stdout).Encode(config)
	}
	p := new(patterns)
	if config.PatternsFile == "" {
		logger.Debug("INIT: Loading embedded patterns")
		err := yaml.Unmarshal(patternEmbed, p)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal patterns: %s", err)
		}
	} else {
		logger.Debug("INIT: Loading %s", config.PatternsFile)
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
	logger.Debug("INIT: Compiling patterns")
	err := p.compile()
	if err != nil {
		return nil, err
	}
	logger.Debug("INIT: Compiling config regexes")
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
	i := &Ingest{
		config:   config,
		patterns: p,
		progress: new(Progress),
	}
	logger.Debug("INIT: Connect to backend")
	err = i.dbConnect()
	if err != nil {
		return nil, fmt.Errorf("could not connect to the database: %s", err)
	}
	logger.Debug("INIT: Backend connected")
	if config.CPUProfilingOutputFile != "" {
		logger.Debug("INIT: Enabling CPU profiling")
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
	logger.Debug("INIT: Update DB labels")
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
	key, _ := aerospike.NewKey(i.config.Aerospike.Namespace, i.patterns.LabelsSetName, "sources")
	metajson, _ := json.Marshal(&metaEntries{
		Entries: []string{sources},
	})
	bin := map[string]interface{}{
		"sources": string(metajson),
	}
	aerr := i.db.Put(i.wp, key, bin)
	if aerr != nil {
		logger.Error("Could not insert sources label: %s", err)
	}
	if i.config.IngestTimeRanges.Enabled {
		key, _ = aerospike.NewKey(i.config.Aerospike.Namespace, i.patterns.LabelsSetName, "timerange")
		metajson, _ := json.Marshal(&metaEntries{
			Entries: []string{i.config.IngestTimeRanges.From.String() + " - " + i.config.IngestTimeRanges.To.String()},
		})
		bin = map[string]interface{}{
			"timerange": string(metajson),
		}
		aerr := i.db.Put(i.wp, key, bin)
		if aerr != nil {
			logger.Error("Could not insert timerange label: %s", err)
		}
	}
	i.endLock = new(sync.Mutex)
	go i.saveProgressInterval()
	go i.printProgressInterval()
	return i, nil
}

func (i *Ingest) Close() {
	logger.Debug("CLOSE: Saving progress")
	i.endLock.Lock()
	i.end = true
	i.endLock.Unlock()
	err := i.saveProgress()
	if err != nil {
		logger.Error("Could not save progress: %s", err)
	}
	err = i.printProgress()
	if err != nil {
		logger.Error("Could not print progress: %s", err)
	}
	if i.pprofRunning {
		logger.Debug("CLOSE: Stopping CPU profiling")
		pprof.StopCPUProfile()
	}
	if i.cpuProfile != nil {
		logger.Debug("CLOSE: Closing CPU profiling file")
		i.cpuProfile.Close()
	}
}

func (p *patterns) compile() error {
	for i := range p.Timestamps {
		logger.Detail("REGEX: compiling timestamps:%s", p.Timestamps[i].Regex)
		regex, err := regexp.Compile(p.Timestamps[i].Regex)
		if err != nil {
			return fmt.Errorf("failed to compile %s: %s", p.Timestamps[i].Regex, err)
		}
		p.Timestamps[i].regex = regex
	}
	for i := range p.Multiline {
		logger.Detail("REGEX: compiling multiline:%s", p.Multiline[i].ReMatchLines)
		regex, err := regexp.Compile(p.Multiline[i].ReMatchLines)
		if err != nil {
			return fmt.Errorf("failed to compile %s: %s", p.Multiline[i].ReMatchLines, err)
		}
		p.Multiline[i].reMatchLines = regex
		for j := range p.Multiline[i].ReMatchJoin {
			logger.Detail("REGEX: compiling multiline-join:%s", p.Multiline[i].ReMatchJoin[j].Re)
			regex, err := regexp.Compile(p.Multiline[i].ReMatchJoin[j].Re)
			if err != nil {
				return fmt.Errorf("failed to compile %s: %s", p.Multiline[i].ReMatchJoin[j].Re, err)
			}
			p.Multiline[i].ReMatchJoin[j].re = regex
		}
	}
	for i := range p.Patterns {
		for j := range p.Patterns[i].Regex {
			logger.Detail("REGEX: compiling pattern:%s", p.Patterns[i].Regex[j])
			regex, err := regexp.Compile(p.Patterns[i].Regex[j])
			if err != nil {
				return fmt.Errorf("failed to compile %s: %s", p.Patterns[i].Regex[j], err)
			}
			p.Patterns[i].regex = append(p.Patterns[i].regex, regex)
		}
		for j := range p.Patterns[i].RegexAdvanced {
			logger.Detail("REGEX: compiling pattern:%s", p.Patterns[i].RegexAdvanced[j].Regex)
			regex, err := regexp.Compile(p.Patterns[i].RegexAdvanced[j].Regex)
			if err != nil {
				return fmt.Errorf("failed to compile %s: %s", p.Patterns[i].RegexAdvanced[j].Regex, err)
			}
			p.Patterns[i].RegexAdvanced[j].regex = regex
		}
		for j := range p.Patterns[i].Replace {
			logger.Detail("REGEX: compiling pattern-replace:%s", p.Patterns[i].Replace[j].Regex)
			regex, err := regexp.Compile(p.Patterns[i].Replace[j].Regex)
			if err != nil {
				return fmt.Errorf("failed to compile %s: %s", p.Patterns[i].Replace[j].Regex, err)
			}
			p.Patterns[i].Replace[j].regex = regex
		}
	}
	return nil
}

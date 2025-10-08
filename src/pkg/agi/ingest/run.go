package ingest

import (
	"errors"
	"fmt"
	"sync"
)

// --- ENV VARS ---
//os.Setenv("LOGINGEST_CPUPROFILE_FILE", "cpu.pprof")
//os.Setenv("LOGINGEST_LOGLEVEL", "6")
//os.Setenv("LOGINGEST_S3SOURCE_ENABLED", "true")
//os.Setenv("LOGINGEST_SFTPSOURCE_ENABLED", "true")
//os.Setenv("LOGINGEST_S3SOURCE_REGION", "ca-central-1")
//os.Setenv("LOGINGEST_S3SOURCE_BUCKET", "") // set outside
//os.Setenv("LOGINGEST_S3SOURCE_KEYID", "") // set outside
//os.Setenv("LOGINGEST_S3SOURCE_SECRET", "") // set outside
//os.Setenv("LOGINGEST_S3SOURCE_PATH", "logs/")
//os.Setenv("LOGINGEST_S3SOURCE_REGEX", "^.*\\.tgz")
//os.Setenv("LOGINGEST_SFTPSOURCE_HOST", "") // set outside
//os.Setenv("LOGINGEST_SFTPSOURCE_PORT", "22")
//os.Setenv("LOGINGEST_SFTPSOURCE_USER", "") // set outside
//os.Setenv("LOGINGEST_SFTPSOURCE_PASSWORD", "") // set outside
//os.Setenv("LOGINGEST_SFTPSOURCE_PATH", "withhist.tgz")
//os.Setenv("LOGINGEST_SFTPSOURCE_REGEX", "^.*\\.tgz")
//LOGINGEST_TIMERANGE_ENABLE: true
//LOGINGEST_TIMERANGE_FROM: ...
//LOGINGEST_TIMERANGE_TO: ...
//LOGINGEST_S3SOURCE_THREADS: 4
//LOGINGEST_SFTPSOURCE_KEYFILE: /loc.f.pem
//LOGINGEST_SFTPSOURCE_THREADS: 4

// Run executes the complete log ingestion pipeline using configuration from a YAML file.
// This is the main entry point for the log ingestion system that handles the entire workflow
// from downloading logs to processing and storing them in the Aerospike database.
//
// The function performs the following steps:
// 1. Load configuration from YAML file and environment variables
// 2. Initialize the ingestion system with database connections
// 3. Download logs from configured sources (S3, SFTP, local)
// 4. Unpack and decompress log files
// 5. Preprocess logs to identify clusters and nodes
// 6. Process logs and collectinfo files concurrently
// 7. Store processed data in Aerospike database
//
// Parameters:
//   - yamlFile: Path to the YAML configuration file. If empty, uses environment variables only.
//
// Returns:
//   - error: nil on success, or an error describing what failed during ingestion
//
// Usage:
//
//	err := ingest.Run("config.yaml")
//	if err != nil {
//	    log.Fatal("Ingestion failed:", err)
//	}
func Run(yamlFile string) error {
	config, err := MakeConfig(true, yamlFile, true)
	if err != nil {
		return fmt.Errorf("MakeConfig: %s", err)
	}
	return RunWithConfig(config)
}

// RunWithConfig executes the complete log ingestion pipeline using a pre-configured Config object.
// This function provides more control than Run() by allowing direct configuration without file parsing.
// It performs the same ingestion workflow but uses the provided configuration directly.
//
// The ingestion process runs log processing and collectinfo processing concurrently for better performance.
// If either process fails, the function will return an error with details about all failures.
//
// Parameters:
//   - config: Pre-configured ingestion configuration with all necessary settings
//
// Returns:
//   - error: nil on success, or a combined error if any step fails
//
// Usage:
//
//	config := &ingest.Config{
//	    Aerospike: aerospikeConfig,
//	    Downloader: downloaderConfig,
//	    // ... other settings
//	}
//	err := ingest.RunWithConfig(config)
//	if err != nil {
//	    log.Fatal("Ingestion failed:", err)
//	}
func RunWithConfig(config *Config) error {
	i, err := Init(config)
	if err != nil {
		return fmt.Errorf("Init: %s", err)
	}
	err = i.Download()
	if err != nil {
		return fmt.Errorf("Download: %s", err)
	}
	err = i.Unpack()
	if err != nil {
		return fmt.Errorf("Unpack: %s", err)
	}
	err = i.PreProcess()
	if err != nil {
		return fmt.Errorf("PreProcess: %s", err)
	}
	nerr := []error{}
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()
		foundLogs, meta, err := i.ProcessLogsPrep()
		if err != nil {
			nerr = append(nerr, fmt.Errorf("ProcessLogsPrep: %s", err))
			return
		}
		err = i.ProcessLogs(foundLogs, meta)
		if err != nil {
			nerr = append(nerr, fmt.Errorf("ProcessLogs: %s", err))
		}
	}()
	go func() {
		defer wg.Done()
		err := i.ProcessCollectInfo()
		if err != nil {
			nerr = append(nerr, fmt.Errorf("ProcessCollectInfo: %s", err))
		}
	}()
	wg.Wait()
	i.Close()
	if len(nerr) > 0 {
		errstr := ""
		for _, e := range nerr {
			if errstr != "" {
				errstr += "; "
			}
			errstr = errstr + e.Error()
		}
		return errors.New(errstr)
	}
	return nil
}

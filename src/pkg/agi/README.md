# AGI Package

The AGI (Aerospike Grafana Integration) package provides comprehensive log ingestion, processing, and visualization capabilities for Aerospike clusters. It consists of three main subpackages that work together to collect, process, and visualize Aerospike logs and metrics.

## Subpackages

### ingest
Handles log ingestion from various sources (S3, SFTP, local files) and processes them for storage in Aerospike databases.

**Key Features:**
- Multi-source log downloading (S3, SFTP, local)
- Automatic log unpacking and decompression
- Log preprocessing and pattern matching
- Aerospike database integration
- Progress tracking and monitoring

**Main Functions:**
- `Run(yamlFile string) error` - Main entry point for log ingestion
- `Init(config *Config) (*Ingest, error)` - Initialize ingest system
- `MakeConfig(setDefaults bool, configFile string, parseEnv bool) (*Config, error)` - Create configuration

### plugin
Provides a Grafana plugin backend for querying processed log data from Aerospike databases.

**Key Features:**
- Grafana datasource plugin backend
- Query processing and caching
- Metrics and timeseries data handling
- Concurrent request management

**Main Functions:**
- `Init(config *Config) (*Plugin, error)` - Initialize plugin system
- `MakeConfig(setDefaults bool, configFile string, parseEnv bool) (*Config, error)` - Create configuration

### grafanafix
Handles Grafana setup, dashboard management, and configuration fixes for optimal Aerospike log visualization.

**Key Features:**
- Automatic dashboard importing
- Timezone configuration
- Annotation management
- Custom labeling and branding

**Main Functions:**
- `Run(g *GrafanaFix)` - Main entry point for Grafana setup
- `MakeConfig(setDefaults bool, configYaml io.Reader, parseEnv bool) (*GrafanaFix, error)` - Create configuration
- `EarlySetup(iniPath string, provisioningDir string, pluginsDir string, pluginUrl string, grafanaPort int) error` - Initial Grafana setup

### notifier
Provides notification capabilities for AGI monitoring and alerting.

**Key Features:**
- Authentication encoding/decoding
- Monitoring integration

**Main Functions:**
- `EncodeAuthJson() (string, error)` - Encode authentication data
- `DecodeAuthJson(val string) (*AgiMonitorAuth, error)` - Decode authentication data

## Usage

The AGI package is typically used as part of the Aerolab ecosystem to provide comprehensive log analysis and visualization for Aerospike clusters. Each subpackage can be used independently or together for a complete monitoring solution.

### Example: Basic Log Ingestion
```go
import "github.com/aerospike/aerolab/pkg/agi/ingest"

// Run log ingestion with configuration file
err := ingest.Run("config.yaml")
if err != nil {
    log.Fatal(err)
}
```

### Example: Grafana Setup
```go
import "github.com/aerospike/aerolab/pkg/agi/grafanafix"

// Setup Grafana with default configuration
grafanafix.Run(nil)
```

## Configuration

Each subpackage supports configuration through YAML files and environment variables. See individual subpackage documentation for specific configuration options.

# Aerolab Package Context Document

This document provides a comprehensive overview of all Go packages in the `pkg/` directory, their purposes, and their exported functions. This is designed to be used as context for AI systems to understand the codebase structure without needing to read individual files.

## Package Overview

The Aerolab `pkg/` directory contains 8 main packages that provide different aspects of functionality:

1. **agi** - Aerospike Grafana Integration for log processing and visualization
2. **backend** - Multi-cloud infrastructure management
3. **conf** - Aerospike configuration management
4. **eks** - Amazon EKS cluster management and expiry
5. **expiry** - Automated resource cleanup and expiration
6. **sshexec** - SSH and SFTP client functionality
7. **utils** - Utility functions and helpers
8. **webui** - Web interface components and utilities

---

## AGI Package (`pkg/agi/`)

**Purpose**: Comprehensive log ingestion, processing, and visualization for Aerospike clusters.

### Subpackages

#### `agi/ingest`
**Purpose**: Log ingestion from multiple sources (S3, SFTP, local) with processing and storage.

**Key Exported Functions**:
- `Run(yamlFile string) error` - Main entry point for log ingestion pipeline
- `RunWithConfig(config *Config) error` - Run ingestion with pre-configured settings
- `Init(config *Config) (*Ingest, error)` - Initialize ingestion system
- `MakeConfig(setDefaults bool, configFile string, parseEnv bool) (*Config, error)` - Create configuration
- `MakeConfigReader(setDefaults bool, configYaml io.Reader, parseEnv bool) (*Config, error)` - Create config from reader

**Key Features**:
- Multi-source downloading (S3, SFTP, local files)
- Automatic decompression and unpacking
- Pattern-based log parsing
- Aerospike database integration
- Progress tracking and monitoring
- Concurrent processing of logs and collectinfo

#### `agi/plugin`
**Purpose**: Grafana datasource plugin backend for querying processed log data.

**Key Exported Functions**:
- `Init(config *Config) (*Plugin, error)` - Initialize plugin system
- `MakeConfig(setDefaults bool, configFile string, parseEnv bool) (*Config, error)` - Create configuration
- `MakeConfigReader(setDefaults bool, configYaml io.Reader, parseEnv bool) (*Config, error)` - Create config from reader

**Key Features**:
- Grafana datasource backend
- Query processing and caching
- Metrics and timeseries handling
- Concurrent request management

#### `agi/grafanafix`
**Purpose**: Grafana setup, dashboard management, and configuration optimization.

**Key Exported Functions**:
- `Run(g *GrafanaFix)` - Main entry point for Grafana setup
- `MakeConfig(setDefaults bool, configYaml io.Reader, parseEnv bool) (*GrafanaFix, error)` - Create configuration
- `EarlySetup(iniPath string, provisioningDir string, pluginsDir string, pluginUrl string, grafanaPort int) error` - Initial setup
- `RandStringRunes(n int) string` - Generate random strings

**Key Features**:
- Automatic dashboard importing
- Timezone configuration
- Annotation management
- Custom labeling and branding

#### `agi/notifier`
**Purpose**: Notification and monitoring integration.

**Key Exported Functions**:
- `EncodeAuthJson() (string, error)` - Encode authentication data
- `DecodeAuthJson(val string) (*AgiMonitorAuth, error)` - Decode authentication data

---

## Backend Package (`pkg/backend/`)

**Purpose**: Unified interface for multi-cloud infrastructure management across AWS, GCP, and Docker.

### Main Package
**Key Exported Functions**:
- `New(project string, c *Config, pollInventoryHourly bool, enabledBackends []backends.BackendType, setInventory *backends.Inventory) (backends.Backend, error)` - Create multi-cloud backend instance

**Key Types**:
- `Config` - Alias for backends.Config with all backend settings

### Subpackages

#### `backend/backends`
**Purpose**: Core backend interface definitions and common functionality.

**Key Exported Functions**:
- `RegisterBackend(name BackendType, c Cloud)` - Register new backend type
- `InternalNew(project string, c *Config, pollInventoryHourly bool, enabledBackends []BackendType, setInventory *Inventory) (Backend, error)` - Internal backend creation
- `ListBackendTypes() []BackendType` - List available backend types

#### `backend/clouds/baws` (AWS Backend)
**Key Exported Functions**:
- `GetEc2Client(credentials *clouds.Credentials, region *string) (*ec2.Client, error)` - Get EC2 client
- `GetEksClient(credentials *clouds.Credentials, region *string) (*eks.Client, error)` - Get EKS client
- `GetCloudformationClient(credentials *clouds.Credentials, region *string) (*cloudformation.Client, error)` - Get CloudFormation client
- `GetIamClient(credentials *clouds.Credentials, region *string) (*iam.Client, error)` - Get IAM client
- `GetRoute53Client(credentials *clouds.Credentials, region *string) (*route53.Client, error)` - Get Route53 client

#### `backend/clouds/bgcp` (GCP Backend)
**Key Exported Functions**:
- `GetCredentials(creds *clouds.GCP, log *logger.Logger) (*google.Credentials, error)` - Get GCP credentials
- `GetClient(creds *clouds.GCP, log *logger.Logger) (*http.Client, error)` - Get authenticated HTTP client

#### `backend/clouds/bdocker` (Docker Backend)
**Key Exported Functions**:
- `ExecWithCLI(...)` - Execute commands in Docker containers

---

## Conf Package (`pkg/conf/`)

**Purpose**: Aerospike configuration file management and editing.

### Subpackages

#### `conf/aerospike/confeditor`
**Purpose**: Configuration editor for standard Aerospike versions.

#### `conf/aerospike/confeditor7`
**Purpose**: Configuration editor for Aerospike 7.x versions with enhanced features.

**Key Features**:
- Multi-version configuration support
- Programmatic configuration editing
- Parameter validation
- Configuration generation and formatting

---

## EKS Package (`pkg/eks/`)

**Purpose**: Amazon EKS cluster management and automated resource cleanup.

### Subpackages

#### `eks/eksexpiry`
**Key Exported Functions**:
- `Expiry()` - Main EKS expiry processing function

**Key Features**:
- Automated EKS cluster expiration
- CloudFormation stack cleanup
- EBS volume cleanup
- IAM resource cleanup
- OIDC provider cleanup

#### `eks/ekctl-templates`
**Purpose**: EKS cluster templates for various configurations.

**Available Templates**:
- `auto-scaler.yaml` - Auto-scaling configuration
- `basic.yaml` - Basic cluster setup
- `full-example.yaml` - Comprehensive example
- `load-balancer.yaml` - Load balancer configuration
- `two-node-groups.yaml` - Multiple node groups

---

## Expiry Package (`pkg/expiry/`)

**Purpose**: Automated resource expiration and cleanup across multiple cloud providers.

### Main Package
**Key Exported Functions**:
- `Expire() error` - Main expiration process (via ExpiryHandler)

**Key Types**:
- `ExpiryHandler` - Main handler for expiration operations

**Key Features**:
- Multi-cloud resource cleanup
- AWS Lambda, GCP Cloud Functions, and standalone deployment
- EKS cluster comprehensive cleanup
- DNS cleanup
- Concurrent processing with proper locking

**Deployment Modes**:
- AWS Lambda (triggered by CloudWatch)
- GCP Cloud Functions (HTTP-triggered)
- GCP Cloud Run (container-based)
- Standalone server (direct execution)

---

## SSHExec Package (`pkg/sshexec/`)

**Purpose**: Comprehensive SSH and SFTP client functionality for remote operations.

**Key Exported Functions**:
- `Exec(i *ExecInput) *ExecOutput` - Execute SSH command with full configuration
- `ExecPrepare(i *ExecInput) (*ssh.Session, *ssh.Client, error)` - Prepare SSH session
- `ExecRun(session *ssh.Session, conn *ssh.Client, i *ExecInput) *ExecOutput` - Run command on session
- `SftpUpload(i *SftpInput) *SftpOutput` - Upload files via SFTP
- `SftpDownload(i *SftpInput) *SftpOutput` - Download files via SFTP
- `SftpList(i *SftpInput) *SftpOutput` - List remote directory contents
- `AddRestoreRequest()` - Request terminal state restoration
- `RestoreTerminal()` - Restore terminal state

**Key Types**:
- `ExecInput` - SSH execution configuration
- `ClientConf` - SSH client connection settings
- `ExecDetail` - Execution-specific configuration
- `ExecOutput` - Command execution results

**Key Features**:
- Interactive and non-interactive SSH sessions
- SFTP file transfer capabilities
- Terminal handling with PTY support
- Session timeout management
- Authentication via password or key
- Window resizing support
- Cross-platform terminal management

---

## Utils Package (`pkg/utils/`)

**Purpose**: Comprehensive collection of utility functions and helpers.

### Subpackages

#### `utils/choice`
**Key Exported Functions**:
- `StringSliceToItems(slice []string) Items` - Convert strings to choice items
- `Choice(title string, items Items) (string, bool, error)` - Interactive choice selection
- `ChoiceWithHeight(title string, items Items, height int) (string, bool, error)` - Choice with custom height

#### `utils/contextio`
**Key Exported Functions**:
- `NewWriter(ctx context.Context, w io.Writer) io.Writer` - Context-aware writer
- `NewReader(ctx context.Context, r io.Reader) io.Reader` - Context-aware reader
- `NewCloser(ctx context.Context, c io.Closer) io.Closer` - Context-aware closer

#### `utils/counters`
**Purpose**: Thread-safe counter implementations.

#### `utils/diff`
**Key Exported Functions**:
- `Diff(oldName string, old []byte, newName string, new []byte) []byte` - Generate unified diff

#### `utils/file`
**Purpose**: File system utilities and helpers.

#### `utils/github`
**Purpose**: GitHub API integration for release management.

#### `utils/installers`
**Key Exported Functions**:
- `GetLatestVersion(stable bool) (*github.Release, error)` - Get latest Aerolab version
- `GetLinuxInstallScript(version *string, prerelease *bool) ([]byte, error)` - Generate install script

**Subpackages**: aerolab, aerospike, compilers, easytc, eksctl, goproxy, grafana, prometheus, vscode

#### `utils/jobqueue`
**Key Exported Functions**:
- `NewSimpleQueue(concurrent int, queued int) *SimpleQueue` - Create simple job queue
- `NewQueueWithIDs(concurrent int, queued int) *QueueWithIDs` - Create job queue with IDs

#### `utils/pager`
**Key Exported Functions**:
- `New(out io.Writer) (*Pager, error)` - Create pager instance

#### `utils/parallelize`
**Purpose**: Parallel processing utilities.

#### `utils/printer`
**Key Exported Functions**:
- `GetTableWriter(renderType string, theme string, sortBy []string, forceColorOff bool, withPager bool) (*TableWriter, error)` - Create table writer
- `String(s string) *string` - String pointer helper

#### `utils/shutdown`
**Key Exported Functions**:
- `IsShuttingDown() bool` - Check shutdown status
- `AddJob()` - Register job with wait group
- `DoneJob()` - Mark job as completed
- `WaitJobs()` - Wait for all jobs to complete
- `AddEarlyCleanupJob(name string, job func(isSignal bool))` - Register early cleanup
- `AddLateCleanupJob(name string, job func(isSignal bool))` - Register late cleanup
- `DeleteEarlyCleanupJob(name string)` - Remove early cleanup job
- `DeleteLateCleanupJob(name string)` - Remove late cleanup job

#### `utils/slack`
**Purpose**: Slack integration for notifications.

#### `utils/structtags`
**Purpose**: Structure tag validation utilities.

#### `utils/versions`
**Key Exported Functions**:
- `Compare(a, b string) int` - Compare version strings
- `Latest(a, b string) string` - Get latest version
- `Oldest(a, b string) string` - Get oldest version

---

## WebUI Package (`pkg/webui/`)

**Purpose**: Web-based user interface components and utilities.

**Key Exported Functions**:
- `InstallWebsite(dst string, website []byte) error` - Extract and install web assets

**Key Variables**:
- `Website []byte` - Embedded website archive

**Key Types**:
- `Page` - Complete page structure definition
- `FormItem` - Form element definitions
- `MenuItem` - Menu structure with navigation
- `InventoryItem` - Data display structures

**Key Features**:
- Website installation from embedded archive
- Comprehensive page rendering structures
- Dynamic form generation and handling
- Hierarchical navigation system
- Inventory display components
- Theme and styling support

**Constants**:
- `ContentTypeForm`, `ContentTypeTable` - Content types
- `BadgeTypeWarning`, `BadgeTypeSuccess`, `BadgeTypeDanger` - Badge types
- `ActiveColorWhite`, `ActiveColorBlue` - Active colors
- `TruncateTimestampCookieName` - Cookie name for preferences

---

## Common Patterns and Design Principles

### Configuration Management
Most packages follow a consistent configuration pattern:
- `MakeConfig()` functions for creating configurations
- Support for YAML files and environment variables
- Default value setting with the `defaults` package
- Validation and error handling

### Error Handling
- Comprehensive error wrapping with context
- Detailed error messages for debugging
- Graceful degradation where possible
- Proper resource cleanup on errors

### Concurrency
- Thread-safe operations where needed
- Proper use of mutexes and channels
- Context-aware operations for cancellation
- Graceful shutdown handling

### Cloud Integration
- Unified interface across multiple cloud providers
- Credential management and authentication
- Region and zone awareness
- Resource tagging and lifecycle management

### Logging and Monitoring
- Structured logging with different levels
- Progress tracking and reporting
- Performance metrics and profiling
- Integration with monitoring systems

This context document provides a comprehensive overview of the Aerolab package ecosystem, enabling AI systems to understand the codebase structure, functionality, and usage patterns without needing to read individual source files.

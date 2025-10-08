# Utils Package

The Utils package provides a comprehensive collection of utility functions and helpers used throughout the Aerolab codebase. It contains various subpackages that handle common operations like file management, parallel processing, user interaction, and system integration.

## Subpackages

### choice
Interactive command-line choice selection utilities.

**Key Features:**
- String slice to choice items conversion
- Interactive selection with customizable height
- Terminal-based user interface

**Main Functions:**
- `StringSliceToItems(slice []string) Items` - Convert string slice to choice items
- `Choice(title string, items Items) (choice string, quitting bool, err error)` - Present interactive choice
- `ChoiceWithHeight(title string, items Items, height int) (choice string, quitting bool, err error)` - Choice with custom height

### contextio
Context-aware I/O operations that can be cancelled or timed out.

**Key Features:**
- Context-aware readers, writers, and closers
- Cancellation support for I/O operations
- Timeout handling

**Main Functions:**
- `NewWriter(ctx context.Context, w io.Writer) io.Writer` - Create context-aware writer
- `NewReader(ctx context.Context, r io.Reader) io.Reader` - Create context-aware reader
- `NewCloser(ctx context.Context, c io.Closer) io.Closer` - Create context-aware closer

### counters
Thread-safe counter implementations for tracking operations.

**Key Features:**
- Integer counters with atomic operations
- Map-based counters for multiple values
- Thread-safe increment/decrement operations

### diff
Text difference utilities for comparing files and content.

**Key Features:**
- Unified diff generation
- File comparison utilities
- Colored diff output support

**Main Functions:**
- `Diff(oldName string, old []byte, newName string, new []byte) []byte` - Generate unified diff

### file
File system utilities and helpers.

**Key Features:**
- File existence checking
- Path manipulation utilities
- Safe file operations

### github
GitHub API integration for release management.

**Key Features:**
- Release information retrieval
- Version checking and updates
- GitHub API client utilities

### installers
Software installation utilities for various tools and dependencies.

**Subpackages:**
- **aerolab** - Aerolab self-installation utilities
- **aerospike** - Aerospike database installation
- **compilers** - Programming language compiler installation (Go, Python, .NET)
- **easytc** - EasyTC installation
- **eksctl** - AWS EKS CLI installation
- **goproxy** - Go proxy server installation
- **grafana** - Grafana installation
- **prometheus** - Prometheus installation
- **vscode** - Visual Studio Code installation

**Key Features:**
- Cross-platform installation scripts
- Version management
- Dependency resolution
- Template-based script generation

**Main Functions:**
- `GetLatestVersion(stable bool) (*github.Release, error)` - Get latest Aerolab version
- `GetLinuxInstallScript(version *string, prerelease *bool) ([]byte, error)` - Generate Linux install script

### jobqueue
Job queue implementations for managing concurrent operations.

**Key Features:**
- Simple job queue with concurrency limits
- Job queue with ID tracking
- Worker pool management

**Main Functions:**
- `NewSimpleQueue(concurrent int, queued int) *SimpleQueue` - Create simple job queue
- `NewQueueWithIDs(concurrent int, queued int) *QueueWithIDs` - Create job queue with ID tracking

### pager
Terminal pager utilities for displaying large amounts of text.

**Key Features:**
- Automatic pager detection (less, more, etc.)
- Cross-platform pager support
- Fallback to stdout when no pager available

**Main Functions:**
- `New(out io.Writer) (*Pager, error)` - Create new pager instance

### parallelize
Parallel processing utilities for concurrent operations.

**Key Features:**
- Parallel function execution
- Error collection and handling
- Configurable concurrency limits

### printer
Table and output formatting utilities.

**Key Features:**
- Table rendering with multiple formats
- Sorting and filtering support
- Color and theme support
- Pager integration

**Main Functions:**
- `GetTableWriter(renderType string, theme string, sortBy []string, forceColorOff bool, withPager bool) (*TableWriter, error)` - Create table writer
- `String(s string) *string` - String pointer helper

### shutdown
Graceful shutdown handling for applications.

**Key Features:**
- Signal handling (SIGINT, SIGTERM)
- Job tracking and waiting
- Early and late cleanup job registration
- Thread-safe shutdown coordination

**Main Functions:**
- `IsShuttingDown() bool` - Check if shutdown is in progress
- `AddJob()` - Add job to wait group
- `DoneJob()` - Mark job as completed
- `WaitJobs()` - Wait for all jobs to complete
- `AddEarlyCleanupJob(name string, job func(isSignal bool))` - Register early cleanup job
- `AddLateCleanupJob(name string, job func(isSignal bool))` - Register late cleanup job
- `DeleteEarlyCleanupJob(name string)` - Remove early cleanup job
- `DeleteLateCleanupJob(name string)` - Remove late cleanup job

### slack
Slack integration utilities for notifications and messaging.

**Key Features:**
- Slack webhook integration
- Message formatting
- Error reporting to Slack channels

### structtags
Structure tag validation utilities.

**Key Features:**
- Required field validation
- Tag parsing and validation
- Struct field analysis

### versions
Version comparison and management utilities.

**Key Features:**
- Semantic version comparison
- Latest version detection
- Version sorting and filtering

**Main Functions:**
- `Compare(a, b string) int` - Compare two version strings
- `Latest(a, b string) string` - Get latest of two versions
- `Oldest(a, b string) string` - Get oldest of two versions

## Usage Examples

### Interactive Choice Selection
```go
import "github.com/aerospike/aerolab/pkg/utils/choice"

items := choice.StringSliceToItems([]string{"option1", "option2", "option3"})
selected, quit, err := choice.Choice("Select an option:", items)
if err != nil || quit {
    return
}
fmt.Printf("Selected: %s\n", selected)
```

### Graceful Shutdown
```go
import "github.com/aerospike/aerolab/pkg/utils/shutdown"

// Register cleanup function
shutdown.AddEarlyCleanupJob("cleanup-temp", func(isSignal bool) {
    // Cleanup temporary files
})

// In worker goroutines
shutdown.AddJob()
defer shutdown.DoneJob()

// In main function
shutdown.WaitJobs() // Wait for all jobs to complete before exit
```

### Version Comparison
```go
import "github.com/aerospike/aerolab/pkg/utils/versions"

result := versions.Compare("1.2.3", "1.2.4") // Returns -1
latest := versions.Latest("1.2.3", "1.2.4")  // Returns "1.2.4"
```

### Table Printing
```go
import "github.com/aerospike/aerolab/pkg/utils/printer"

writer, err := printer.GetTableWriter("table", "default", []string{"name"}, false, true)
if err != nil {
    return err
}
// Use writer to format and display tabular data
```

## Design Principles

The utils package follows these design principles:

1. **Modularity** - Each subpackage focuses on a specific area of functionality
2. **Reusability** - Functions are designed to be reused across different parts of Aerolab
3. **Thread Safety** - Utilities that need to be thread-safe provide appropriate synchronization
4. **Error Handling** - Comprehensive error handling with context
5. **Cross-Platform** - Support for different operating systems where applicable

# logger

A simple, leveled logging package for Go with support for multiple output sinks.

## Features

- **Six log levels**: DETAIL, DEBUG, INFO, WARNING, ERROR, CRITICAL
- **Multiple output sinks**: stderr, files, kernel message buffer (kmsg), in-memory buffers
- **Prefixes**: Add context to log messages with customizable prefixes
- **Raw terminal mode support**: Proper line endings when terminal is in raw mode
- **Thread-safe**: Safe for concurrent use

## Installation

```bash
go get github.com/rglonek/logger
```

## Quick Start

```go
package main

import "github.com/rglonek/logger"

func main() {
    // Use the default logger
    logger.Info("Application started")
    logger.Debug("This won't show - default level is INFO")
    
    // Enable debug logging
    logger.SetLogLevel(logger.DEBUG)
    logger.Debug("Now this shows!")
    
    // Log with formatting
    logger.Info("Processing %d items", 42)
    
    // Warning and error
    logger.Warn("Disk space low: %d%% remaining", 10)
    logger.Error("Failed to connect: %v", err)
}
```

## Log Levels

| Level    | Value | Description                                      |
|----------|-------|--------------------------------------------------|
| CRITICAL | 1     | Critical errors, calls `os.Exit(1)` after logging |
| ERROR    | 2     | Error conditions                                 |
| WARNING  | 3     | Warning messages                                 |
| INFO     | 4     | Informational messages (default)                 |
| DEBUG    | 5     | Debug information                                |
| DETAIL   | 6     | Verbose tracing                                  |

Messages are only logged if their level is at or below the configured level.

## Using Custom Logger Instances

```go
// Create a new logger
log := logger.NewLogger()
log.SetLogLevel(logger.DEBUG)
log.SetPrefix("[myapp] ")

log.Info("Started")  // Output: INFO [myapp] Started

// Create sub-loggers with additional prefixes
workerLog := log.WithPrefix("[worker-1] ")
workerLog.Info("Processing")  // Output: INFO [myapp] [worker-1] Processing

// Create a logger with a different log level
verboseLog := log.WithLogLevel(logger.DETAIL)
```

## Multiple Output Sinks

```go
log := logger.NewLogger()

// Log to a file (in addition to stderr)
err := log.SinkLogToFile("/var/log/myapp.log")
if err != nil {
    log.Error("Failed to open log file: %v", err)
}

// Log to kernel message buffer (Linux only, requires permissions)
err = log.SinkEnableKmesg()
if err != nil {
    log.Warn("Could not enable kmsg: %v", err)
}

// Disable stderr output (useful when only logging to file)
log.SinkDisableStderr()

// Log to an in-memory buffer
buffer := make(chan string, 100)
var truncated bool
log.SinkBuffer(buffer, &truncated)
```

## Raw Terminal Mode

When working with raw terminal mode (e.g., for interactive SSH sessions), normal `\n` line endings don't display correctly because the terminal's automatic CR+LF translation is disabled. The logger handles this automatically:

```go
import (
    "os"
    "github.com/rglonek/logger"
    "golang.org/x/term"
)

func runInteractiveSession() {
    fd := int(os.Stdin.Fd())
    
    // Save terminal state and enter raw mode
    oldState, err := term.MakeRaw(fd)
    if err != nil {
        logger.Error("Failed to enter raw mode: %v", err)
        return
    }
    
    // Tell logger to use \r\n line endings for terminal output
    logger.SetRawTerminalMode(true)
    
    // ... do interactive work ...
    // Logger output will display correctly in raw mode
    
    // Restore terminal and logger
    term.Restore(fd, oldState)
    logger.SetRawTerminalMode(false)
}
```

**Note**: Raw terminal mode only affects output to actual terminals. Output to files, pipes, or redirected stderr is never modified. Terminal detection uses `ioctl` to verify the file descriptor is connected to a real TTY.

## Millisecond Timestamps

```go
log := logger.NewLogger()
log.MillisecondLogging(true)
log.Info("High precision timestamp")
// Output: 2024/01/15 10:30:45.123456 INFO High precision timestamp
```

## Output Format

Log messages are formatted as:

```
YYYY/MM/DD HH:MM:SS LEVEL [prefix]message
```

For example:
```
2024/01/15 10:30:45 INFO Application started
2024/01/15 10:30:45 WARNING [myapp] Disk space low
2024/01/15 10:30:45 ERROR [myapp] [worker-1] Connection failed
```

## Thread Safety

The logger is safe for concurrent use from multiple goroutines. The in-memory buffer sink uses mutex protection to ensure thread-safe writes.

## License

See [LICENSE](LICENSE) file.

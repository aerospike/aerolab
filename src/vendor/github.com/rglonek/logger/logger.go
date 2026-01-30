// Package logger provides a simple, leveled logging library with support for
// multiple output sinks including stderr, files, kernel message buffer (kmsg),
// and in-memory buffers.
//
// The package supports six log levels: DETAIL, DEBUG, INFO, WARNING, ERROR, and CRITICAL.
// Messages are only logged if their level is at or below the configured log level.
//
// # Basic Usage
//
// The package provides both a default logger and the ability to create custom logger instances:
//
//	// Using the default logger
//	logger.SetLogLevel(logger.DEBUG)
//	logger.Info("Application started")
//	logger.Debug("Processing item %d", itemID)
//
//	// Using a custom logger instance
//	log := logger.NewLogger()
//	log.SetLogLevel(logger.INFO)
//	log.Info("Hello, %s!", name)
//
// # Prefixes
//
// Loggers support prefixes that are prepended to all messages:
//
//	log := logger.NewLogger()
//	log.SetPrefix("[myapp] ")
//	log.Info("Started") // Output: INFO [myapp] Started
//
//	// Create a derived logger with an additional prefix
//	subLog := log.WithPrefix("[worker] ")
//	subLog.Info("Processing") // Output: INFO [myapp] [worker] Processing
//
// # Multiple Output Sinks
//
// A logger can write to multiple destinations simultaneously:
//
//	log := logger.NewLogger()
//	log.SinkLogToFile("/var/log/myapp.log")  // Also log to file
//	log.SinkEnableKmesg()                     // Also log to kernel buffer (Linux)
//	log.SinkDisableStderr()                   // Optionally disable stderr
//
// # Raw Terminal Mode
//
// When working with raw terminal mode (e.g., for interactive SSH sessions),
// the logger can automatically convert \n to \r\n for proper display:
//
//	// Before entering raw terminal mode
//	term.MakeRaw(fd)
//	logger.SetRawTerminalMode(true)
//
//	// ... do work with raw terminal ...
//
//	// After restoring terminal
//	term.Restore(fd, oldState)
//	logger.SetRawTerminalMode(false)
//
// This only affects output to actual terminals (detected via ioctl), not files or pipes.
package logger

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

var defaultLevel LogLevel = INFO

var defaultPrefix = ""

// rawTerminalMode is a global atomic flag indicating whether the terminal is in raw mode.
// When true, terminal output will use \r\n line endings instead of \n.
var rawTerminalMode atomic.Bool

// SetRawTerminalMode sets whether the terminal is in raw mode.
// When enabled, log output to terminals (stderr) will use \r\n line endings
// instead of \n. This is necessary when the terminal is in raw mode because
// raw mode disables the kernel's automatic \n to \r\n translation.
//
// Call with true immediately after [term.MakeRaw], and false after [term.Restore].
// This setting is global and affects all logger instances, but only applies to
// output that is actually going to a terminal (detected via ioctl).
//
// Output to files, pipes, or other non-terminal destinations is never affected.
func SetRawTerminalMode(enabled bool) {
	rawTerminalMode.Store(enabled)
}

// GetRawTerminalMode returns whether raw terminal mode is currently enabled.
func GetRawTerminalMode() bool {
	return rawTerminalMode.Load()
}

// terminalWriter wraps an io.Writer and adjusts line endings based on raw terminal mode.
// When raw mode is enabled and the underlying writer is a terminal, \n is converted to \r\n.
type terminalWriter struct {
	w          io.Writer
	isTerminal bool
}

// newTerminalWriter creates a terminalWriter that wraps the given writer.
// It detects whether the writer is a terminal file descriptor using term.IsTerminal,
// which performs an ioctl check to verify it's a real terminal (not a pipe or file).
func newTerminalWriter(w io.Writer) *terminalWriter {
	isTerminal := false
	if f, ok := w.(*os.File); ok {
		isTerminal = term.IsTerminal(int(f.Fd()))
	}
	return &terminalWriter{w: w, isTerminal: isTerminal}
}

// Write implements io.Writer. When raw terminal mode is enabled and the underlying
// writer is a terminal, it converts \n to \r\n for proper display.
func (t *terminalWriter) Write(p []byte) (n int, err error) {
	if t.isTerminal && GetRawTerminalMode() {
		// Convert \n to \r\n for raw terminal mode
		// First normalize any existing \r\n to \n to avoid \r\r\n
		output := bytes.ReplaceAll(p, []byte("\r\n"), []byte("\n"))
		output = bytes.ReplaceAll(output, []byte("\n"), []byte("\r\n"))
		_, err = t.w.Write(output)
		return len(p), err // Return original length to satisfy io.Writer contract
	}
	return t.w.Write(p)
}

// LogLevel represents the severity level of a log message.
// Higher values indicate more verbose logging.
type LogLevel int

const (
	// CRITICAL is the highest severity level. Logging at this level
	// will also call os.Exit(1) after writing the message.
	CRITICAL LogLevel = 1
	// ERROR indicates an error condition that should be investigated.
	ERROR LogLevel = 2
	// WARNING indicates a potential problem or unusual condition.
	WARNING LogLevel = 3
	// INFO is for general informational messages about program execution.
	INFO LogLevel = 4
	// DEBUG provides detailed information useful during development.
	DEBUG LogLevel = 5
	// DETAIL is the most verbose level, for very detailed tracing.
	DETAIL LogLevel = 6
)

// Logger is a leveled logger that can write to multiple output sinks.
// Use [NewLogger] to create a new instance, or use the package-level
// functions to use the default logger.
type Logger struct {
	logLevel            LogLevel
	p                   string
	disableStderr       bool
	logToFile           string
	enableKmesg         bool
	fileLogger          *log.Logger
	stderrLogger        *log.Logger
	kmesg               *os.File
	milliseconds        bool
	sinkBufferLock      *sync.Mutex
	sinkBuffer          chan string
	sinkBufferTruncated *bool
	timeFormat          string
}

// SinkBuffer configures the logger to write messages to an in-memory channel buffer.
// This is useful for capturing log output programmatically.
//
// The buffer parameter is a channel that will receive formatted log messages.
// The truncated parameter is a pointer to a bool that will be set to true if
// the buffer becomes full and messages are dropped.
func (l *Logger) SinkBuffer(buffer chan string, truncated *bool) {
	l.sinkBuffer = buffer
	l.sinkBufferTruncated = truncated
	l.sinkBufferLock = new(sync.Mutex)
	l.timeFormat = "2006-01-02 15:04:05"
}

// SinkDisableStderr disables writing log messages to stderr.
// This is useful when you only want to log to a file or buffer.
func (l *Logger) SinkDisableStderr() {
	l.disableStderr = true
}

// SinkLogToFile configures the logger to also write messages to the specified file.
// The file is opened in append mode, creating it if it doesn't exist.
// Returns an error if the file cannot be opened.
func (l *Logger) SinkLogToFile(name string) (err error) {
	l.logToFile = name
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	l.fileLogger = log.New(f, "", log.Default().Flags())
	return nil
}

// SinkEnableKmesg configures the logger to also write messages to the Linux
// kernel message buffer (/dev/kmsg). This makes log messages visible via dmesg.
// Returns an error if /dev/kmsg cannot be opened (e.g., on non-Linux systems
// or without appropriate permissions).
func (l *Logger) SinkEnableKmesg() error {
	l.enableKmesg = true
	kmsg, err := os.OpenFile("/dev/kmsg", os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	l.kmesg = kmsg
	return nil
}

var defaultLogger = &Logger{
	logLevel:     INFO,
	p:            "",
	stderrLogger: log.New(newTerminalWriter(os.Stderr), "", log.LstdFlags),
}

// Info logs an informational message using the default logger.
// Arguments are handled in the manner of [fmt.Printf].
func Info(format string, v ...interface{}) {
	defaultLogger.Info(format, v...)
}

// Warn logs a warning message using the default logger.
// Arguments are handled in the manner of [fmt.Printf].
func Warn(format string, v ...interface{}) {
	defaultLogger.Warn(format, v...)
}

// Error logs an error message using the default logger.
// Arguments are handled in the manner of [fmt.Printf].
func Error(format string, v ...interface{}) {
	defaultLogger.Error(format, v...)
}

// Critical logs a critical message using the default logger and then calls os.Exit(1).
// Arguments are handled in the manner of [fmt.Printf].
func Critical(format string, v ...interface{}) {
	defaultLogger.Critical(format, v...)
}

// Debug logs a debug message using the default logger.
// Arguments are handled in the manner of [fmt.Printf].
func Debug(format string, v ...interface{}) {
	defaultLogger.Debug(format, v...)
}

// Detail logs a detailed trace message using the default logger.
// Arguments are handled in the manner of [fmt.Printf].
func Detail(format string, v ...interface{}) {
	defaultLogger.Detail(format, v...)
}

// SetPrefix sets the prefix for the default logger.
// The prefix is prepended to all log messages.
// New loggers created with [NewLogger] will inherit this prefix.
func SetPrefix(prefix string) {
	defaultLogger.SetPrefix(prefix)
	defaultPrefix = prefix
}

// SetLogLevel sets the log level for the default logger.
// Messages with a level higher than this will not be logged.
// New loggers created with [NewLogger] will inherit this level.
func SetLogLevel(level LogLevel) {
	defaultLogger.SetLogLevel(level)
	defaultLevel = level
}

// NewLogger creates a new Logger instance with the current default log level and prefix.
// The new logger writes to stderr by default.
func NewLogger() *Logger {
	return &Logger{
		logLevel:     defaultLevel,
		p:            defaultPrefix,
		stderrLogger: log.New(newTerminalWriter(os.Stderr), "", log.LstdFlags),
	}
}

// MillisecondLogging enables or disables millisecond precision in log timestamps.
// When enabled, timestamps include microsecond resolution.
func (l *Logger) MillisecondLogging(enable bool) {
	l.milliseconds = enable
	if enable {
		l.stderrLogger.SetFlags(log.LstdFlags | log.Lmicroseconds)
		if l.fileLogger != nil {
			l.fileLogger.SetFlags(log.LstdFlags | log.Lmicroseconds)
		}
		if l.sinkBuffer != nil {
			l.timeFormat = "2006-01-02 15:04:05.000"
		}
	} else {
		l.stderrLogger.SetFlags(log.LstdFlags)
		if l.fileLogger != nil {
			l.fileLogger.SetFlags(log.LstdFlags)
		}
		if l.sinkBuffer != nil {
			l.timeFormat = "2006-01-02 15:04:05"
		}
	}
}

// WithPrefix returns a new Logger that appends the given prefix to the current prefix.
// The new logger shares output sinks with the original but has an independent prefix.
// This is useful for creating sub-loggers for different components.
func (l *Logger) WithPrefix(prefix string) *Logger {
	newLogger := &Logger{
		logLevel:            l.logLevel,
		p:                   fmt.Sprintf("%s%s", l.p, prefix),
		disableStderr:       l.disableStderr,
		logToFile:           l.logToFile,
		fileLogger:          l.fileLogger,
		kmesg:               l.kmesg,
		enableKmesg:         l.enableKmesg,
		stderrLogger:        l.stderrLogger,
		milliseconds:        l.milliseconds,
		sinkBuffer:          l.sinkBuffer,
		sinkBufferTruncated: l.sinkBufferTruncated,
		sinkBufferLock:      l.sinkBufferLock,
		timeFormat:          l.timeFormat,
	}
	return newLogger
}

// WithLogLevel returns a new Logger with the specified log level.
// The new logger shares output sinks and prefix with the original but has an independent log level.
func (l *Logger) WithLogLevel(level LogLevel) *Logger {
	newLogger := &Logger{
		logLevel:            level,
		p:                   l.p,
		disableStderr:       l.disableStderr,
		logToFile:           l.logToFile,
		fileLogger:          l.fileLogger,
		kmesg:               l.kmesg,
		enableKmesg:         l.enableKmesg,
		stderrLogger:        l.stderrLogger,
		milliseconds:        l.milliseconds,
		sinkBuffer:          l.sinkBuffer,
		sinkBufferTruncated: l.sinkBufferTruncated,
		sinkBufferLock:      l.sinkBufferLock,
		timeFormat:          l.timeFormat,
	}
	return newLogger
}

// SetPrefix sets the prefix for this logger.
// The prefix is prepended to all log messages after the level indicator.
func (l *Logger) SetPrefix(prefix string) {
	l.p = prefix
}

// SetLogLevel sets the log level for this logger.
// Messages with a level higher (more verbose) than this will not be logged.
// A level of 0 disables all logging.
func (l *Logger) SetLogLevel(level LogLevel) {
	if level < 0 {
		level = 0
	}
	l.logLevel = level
}

// Info logs an informational message if the log level is INFO or higher.
// Arguments are handled in the manner of [fmt.Printf].
func (l *Logger) Info(format string, v ...interface{}) {
	if l.logLevel < INFO {
		return
	}
	format = "INFO " + l.p + format
	if !l.disableStderr {
		l.stderrLogger.Printf(format, v...)
	}
	if l.fileLogger != nil {
		l.fileLogger.Printf(format, v...)
	}
	if l.kmesg != nil {
		fmt.Fprintf(l.kmesg, "<5>"+format+"\n", v...)
	}
	if l.sinkBuffer != nil {
		l.sinkBufferLock.Lock()
		if len(l.sinkBuffer) >= cap(l.sinkBuffer) {
			<-l.sinkBuffer
			if l.sinkBufferTruncated != nil {
				*l.sinkBufferTruncated = true
			}
		}
		l.sinkBuffer <- time.Now().Format(l.timeFormat) + fmt.Sprintf(format, v...)
		l.sinkBufferLock.Unlock()
	}
}

// Warn logs a warning message if the log level is WARNING or higher.
// Arguments are handled in the manner of [fmt.Printf].
func (l *Logger) Warn(format string, v ...interface{}) {
	if l.logLevel < WARNING {
		return
	}
	format = "WARNING " + l.p + format
	if !l.disableStderr {
		l.stderrLogger.Printf(format, v...)
	}
	if l.fileLogger != nil {
		l.fileLogger.Printf(format, v...)
	}
	if l.kmesg != nil {
		fmt.Fprintf(l.kmesg, "<4>"+format+"\n", v...)
	}
	if l.sinkBuffer != nil {
		l.sinkBufferLock.Lock()
		if len(l.sinkBuffer) >= cap(l.sinkBuffer) {
			<-l.sinkBuffer
			if l.sinkBufferTruncated != nil {
				*l.sinkBufferTruncated = true
			}
		}
		l.sinkBuffer <- time.Now().Format(l.timeFormat) + fmt.Sprintf(format, v...)
		l.sinkBufferLock.Unlock()
	}
}

// Error logs an error message if the log level is ERROR or higher.
// Arguments are handled in the manner of [fmt.Printf].
func (l *Logger) Error(format string, v ...interface{}) {
	if l.logLevel < ERROR {
		return
	}
	format = "ERROR " + l.p + format
	if !l.disableStderr {
		l.stderrLogger.Printf(format, v...)
	}
	if l.fileLogger != nil {
		l.fileLogger.Printf(format, v...)
	}
	if l.kmesg != nil {
		fmt.Fprintf(l.kmesg, "<3>"+format+"\n", v...)
	}
	if l.sinkBuffer != nil {
		l.sinkBufferLock.Lock()
		if len(l.sinkBuffer) >= cap(l.sinkBuffer) {
			<-l.sinkBuffer
			if l.sinkBufferTruncated != nil {
				*l.sinkBufferTruncated = true
			}
		}
		l.sinkBuffer <- time.Now().Format(l.timeFormat) + fmt.Sprintf(format, v...)
		l.sinkBufferLock.Unlock()
	}
}

// Critical logs a critical message if the log level is CRITICAL or higher,
// then calls [os.Exit](1). This method never returns.
// Arguments are handled in the manner of [fmt.Printf].
func (l *Logger) Critical(format string, v ...interface{}) {
	if l.logLevel >= CRITICAL {
		format = "CRITICAL " + l.p + format
		if !l.disableStderr {
			l.stderrLogger.Printf(format, v...)
		}
		if l.fileLogger != nil {
			l.fileLogger.Printf(format, v...)
		}
		if l.kmesg != nil {
			fmt.Fprintf(l.kmesg, "<2>"+format+"\n", v...)
		}
		if l.sinkBuffer != nil {
			l.sinkBufferLock.Lock()
			if len(l.sinkBuffer) >= cap(l.sinkBuffer) {
				<-l.sinkBuffer
				if l.sinkBufferTruncated != nil {
					*l.sinkBufferTruncated = true
				}
			}
			l.sinkBuffer <- time.Now().Format(l.timeFormat) + fmt.Sprintf(format, v...)
			l.sinkBufferLock.Unlock()
		}
	}
	os.Exit(1)
}

// Debug logs a debug message if the log level is DEBUG or higher.
// Arguments are handled in the manner of [fmt.Printf].
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.logLevel < DEBUG {
		return
	}
	format = "DEBUG " + l.p + format
	if !l.disableStderr {
		l.stderrLogger.Printf(format, v...)
	}
	if l.fileLogger != nil {
		l.fileLogger.Printf(format, v...)
	}
	if l.kmesg != nil {
		fmt.Fprintf(l.kmesg, "<6>"+format+"\n", v...)
	}
	if l.sinkBuffer != nil {
		l.sinkBufferLock.Lock()
		if len(l.sinkBuffer) >= cap(l.sinkBuffer) {
			<-l.sinkBuffer
			if l.sinkBufferTruncated != nil {
				*l.sinkBufferTruncated = true
			}
		}
		l.sinkBuffer <- time.Now().Format(l.timeFormat) + fmt.Sprintf(format, v...)
		l.sinkBufferLock.Unlock()
	}
}

// Detail logs a detailed trace message if the log level is DETAIL.
// This is the most verbose logging level.
// Arguments are handled in the manner of [fmt.Printf].
func (l *Logger) Detail(format string, v ...interface{}) {
	if l.logLevel < DETAIL {
		return
	}
	format = "DETAIL " + l.p + format
	if !l.disableStderr {
		l.stderrLogger.Printf(format, v...)
	}
	if l.fileLogger != nil {
		l.fileLogger.Printf(format, v...)
	}
	if l.kmesg != nil {
		fmt.Fprintf(l.kmesg, "<7>"+format+"\n", v...)
	}
	if l.sinkBuffer != nil {
		l.sinkBufferLock.Lock()
		if len(l.sinkBuffer) >= cap(l.sinkBuffer) {
			<-l.sinkBuffer
			if l.sinkBufferTruncated != nil {
				*l.sinkBufferTruncated = true
			}
		}
		l.sinkBuffer <- time.Now().Format(l.timeFormat) + fmt.Sprintf(format, v...)
		l.sinkBufferLock.Unlock()
	}
}

# go-logger Golang Logger
Logger on steroids.

## Get it
```bash
go get github.com/bestmethod/go-logger
```

## What's new
#### Async option
There is now support for async logging. To avoid slowdown of main code when logging to stdout, one can run the logger in Async mode - which in turn uses goroutines. This allows for the main code execution without delay. To use, set logger.Async to true AFTER running the logger.Init().
###### Warning: Use Async at your own peril. If your logging falls behind the main code, you risk exhausting handles, memory or goroutine space and crashing. This is no designed as a queuing mechanism for logging, but marely to allow for more real-time of main code execution.
```go
logger := new(Logger.Logger)
logger.Init(header, serviceName, stdoutLevels, stderrLevels, devlogLevels)
logger.Async = true
```

## Usage
#### Create object and Init
```go
logger := new(Logger.Logger)
err := logger.Init(header, serviceName, stdoutLevels, stderrLevels, devlogLevels)
```
#### Functions
```go
func (l *Logger) Init(header string,serviceName string,stdoutLevel int,stderrLevel int,devlogLevel int) error {}
func (l *Logger) Debug(format string, args ...interface{}) {}
func (l *Logger) Info(format string, args ...interface{}) {}
func (l *Logger) Warn(format string, args ...interface{}) {}
func (l *Logger) Error(format string, args ...interface{}) {}
func (l *Logger) Critical(format string, args ...interface{}) {}
func (l *Logger) Fatal(format string, args ...interface{}) {}
func (l *Logger) Fatalf(exitCode int,format string, args ...interface{}) {}
func (l *Logger) Destroy() error {}
func (l *Logger) TimeFormat(newFormat string) {}
```

#### Destroy objects before forgetting them
This will destroy the devlog handler and the connection to devlog that it holds in the background (open handle). If you are crazy enough to Init and destroy 100s of loggers in your code, you may want to use this.

Note that this will not stop the logger from working if you use it afterwards. This just disables and closes the devlog type logger as a cleanup procedure. If you do not use devlog logger, don't worry about this at all.
```
err := logger.Destroy()
```

## Example
```go
import "github.com/bestmethod/logger"
import "fmt"
import "os"

logger := new(Logger.Logger)
err := logger.Init("SUBNAME", "SERVICENAME", Logger.LEVEL_DEBUG | Logger.LEVEL_INFO | Logger.LEVEL_WARN, Logger.LEVEL_ERROR | Logger.LEVEL_CRITICAL, Logger.LEVEL_NONE)
if err != nil {
  fmt.Fprintf(os.Stderr, "CRITICAL Could not initialize logger. Quitting. Details: %s\n", err)
  os.Exit(1)
}

// standard logger messages  
logger.Info("This is info message")
logger.Debug("This is debug message")
logger.Error("This is error message")
logger.Warn("This is warning message")
logger.Critical("This is critical message")

// logger messages, like Printf (auto-discovery of printf happens, so same functions are used)
logger.Info("%s %v","This is info message",10)
logger.Debug("%s %v","This is debug message",10)
logger.Error("%s %v","This is error message",10)
logger.Warn("%s %v","This is warning message",10)
logger.Critical("%s %v","This is critical message",10)

// standard fatal exit with custom exit code
code := 1
logger.Fatal("This is a critical message that terminates the program with os.exit(code)", code)

// fatal with printf has a separate fatalf
code = 1
logger.Fatalf(code,"%s","This is a critical message that terminates the program with os.exit(code)")

```

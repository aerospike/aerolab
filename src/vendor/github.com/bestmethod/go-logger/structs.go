// +build !windows

package Logger

import (
	"log/syslog"
	"os"
)

type Logger struct {
	Header      string
	ServiceName string
	pid         int
	Stdout      *os.File
	StdoutLevel int
	Stderr      *os.File
	StderrLevel int
	Devlog      *syslog.Writer
	DevlogLevel int
	osExit      func(code int)
	Async       bool
	Format      string
}

// +build windows

package Logger

import (
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
	DevlogLevel int
	osExit      func(code int)
	Async       bool
	Format      string
}


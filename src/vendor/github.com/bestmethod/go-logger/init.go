package Logger

import (
	"os"
)

func (l *Logger) Init(header string, serviceName string, stdoutLevel int, stderrLevel int, devlogLevel int) error {
	l.pid = os.Getpid()
	l.Header = header
	l.ServiceName = serviceName
	l.StdoutLevel = stdoutLevel
	l.StderrLevel = stderrLevel
	l.DevlogLevel = devlogLevel
        l.Format = "Jan 02 15:04:05-0700"
	var err error
	err = l.DevlogOsCheck()
	if err != nil {
		return err
	}
	l.osExit = os.Exit
	l.Async = false
	err = nil
	if stdoutLevel != 0 {
		//l.Stdout = log.New(os.Stdout, "", 0)
		l.Stdout = os.Stdout
	} else {
		l.Stdout = nil
	}
	if stderrLevel != 0 {
		//l.Stderr = log.New(os.Stderr, "", 0)
		l.Stderr = os.Stderr
	} else {
		l.Stderr = nil
	}
	err = l.DevlogInit()
	return err
}

func (l *Logger) TimeFormat(newFormat string) {
        l.Format = newFormat
}

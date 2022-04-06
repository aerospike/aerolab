package Logger

import (
	"fmt"
	"time"
)

func (l *Logger) Debug(format string, args ...interface{}) {
	var m string
	if len(args) == 0 {
		m = format
	} else {
		m = fmt.Sprintf(format,args...)
	}
	if l.Async == false {
		l.dispatch(LEVEL_DEBUG, m)
	} else {
		go l.dispatch(LEVEL_DEBUG, m)
	}
}

func (l *Logger) Info(format string, args ...interface{}) {
	var m string
	if len(args) == 0 {
		m = format
	} else {
		m = fmt.Sprintf(format,args...)
	}
	if l.Async == false {
		l.dispatch(LEVEL_INFO, m)
	} else {
		go l.dispatch(LEVEL_INFO, m)
	}
}

func (l *Logger) Warn(format string, args ...interface{}) {
	var m string
	if len(args) == 0 {
		m = format
	} else {
		m = fmt.Sprintf(format,args...)
	}
	if l.Async == false {
		l.dispatch(LEVEL_WARN, m)
	} else {
		go l.dispatch(LEVEL_WARN, m)
	}
}

func (l *Logger) Error(format string, args ...interface{}) {
	var m string
	if len(args) == 0 {
		m = format
	} else {
		m = fmt.Sprintf(format,args...)
	}
	if l.Async == false {
		l.dispatch(LEVEL_ERROR, m)
	} else {
		go l.dispatch(LEVEL_ERROR, m)
	}
}

func (l *Logger) Critical(format string, args ...interface{}) {
	var m string
	if len(args) == 0 {
		m = format
	} else {
		m = fmt.Sprintf(format,args...)
	}
	if l.Async == false {
		l.dispatch(LEVEL_CRITICAL, m)
	} else {
		go l.dispatch(LEVEL_CRITICAL, m)
	}
}

func (l *Logger) Fatalf(exitCode int,format string, args ...interface{}) {
	var m string
	if len(args) == 0 {
		m = format
	} else {
		m = fmt.Sprintf(format,args...)
	}
	l.dispatch(LEVEL_CRITICAL, m)
	l.osExit(exitCode)
}

func (l *Logger) Fatal(m string, exitCode int) {
	l.dispatch(LEVEL_CRITICAL, m)
	l.osExit(exitCode)
}

func (l *Logger) dispatch(logLevel int, m string) {
	var mm string
	if l.Header != "" {
		if logLevel == LEVEL_DEBUG {
			mm = fmt.Sprintf("DEBUG    %s %s", l.Header, m)
		} else if logLevel == LEVEL_INFO {
			mm = fmt.Sprintf("INFO     %s %s", l.Header, m)
		} else if logLevel == LEVEL_WARN {
			mm = fmt.Sprintf("WARN     %s %s", l.Header, m)
		} else if logLevel == LEVEL_ERROR {
			mm = fmt.Sprintf("ERROR    %s %s", l.Header, m)
		} else if logLevel == LEVEL_CRITICAL {
			mm = fmt.Sprintf("CRITICAL %s %s", l.Header, m)
		}
	} else {
		if logLevel == LEVEL_DEBUG {
			mm = fmt.Sprintf("DEBUG    %s", m)
		} else if logLevel == LEVEL_INFO {
			mm = fmt.Sprintf("INFO     %s", m)
		} else if logLevel == LEVEL_WARN {
			mm = fmt.Sprintf("WARN     %s", m)
		} else if logLevel == LEVEL_ERROR {
			mm = fmt.Sprintf("ERROR    %s", m)
		} else if logLevel == LEVEL_CRITICAL {
			mm = fmt.Sprintf("CRITICAL %s", m)
		}
	}
	mm = fmt.Sprintf("%s %s[%d]: %s\n", time.Now().UTC().Format(l.Format), l.ServiceName, l.pid, mm)
	if (l.StdoutLevel & logLevel) != 0 {
		l.Stdout.WriteString(mm)
	}
	if (l.StderrLevel & logLevel) != 0 {
		l.Stderr.WriteString(mm)
	}
	l.dispatchDevlog(logLevel, mm)
}

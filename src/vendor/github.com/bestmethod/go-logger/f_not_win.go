// +build !windows

package Logger

import "log/syslog"

func (l *Logger) DevlogOsCheck() error {
	return nil
}

func (l *Logger) Destroy() error {
	if l.Devlog != nil {
		l.DevlogLevel = 0
		return l.Devlog.Close()
	} else {
		return nil
	}
}

func (l *Logger) DevlogInit() error {
	var err error
	if l.DevlogLevel != 0 {
		l.Devlog, err = syslog.Dial("", "", syslog.LOG_DAEMON, l.ServiceName)
	} else {
		l.Devlog = nil
	}
	return err
}

func (l *Logger) dispatchDevlog(logLevel int, mm string) {
	if (l.DevlogLevel & logLevel) != 0 {
		switch logLevel {
		case LEVEL_DEBUG:
			l.Devlog.Debug(mm)
		case LEVEL_INFO:
			l.Devlog.Info(mm)
		case LEVEL_WARN:
			l.Devlog.Warning(mm)
		case LEVEL_ERROR:
			l.Devlog.Err(mm)
		case LEVEL_CRITICAL:
			l.Devlog.Crit(mm)
		}
	}
}
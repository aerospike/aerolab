// +build windows

package Logger

import "errors"

func (l *Logger) DevlogOsCheck() error {
	if l.DevlogLevel != LEVEL_NONE {
		return errors.New("devlog isn't supported on windows")
	}
	return nil
}

func (l *Logger) Destroy() error {
	return nil
}

func (l *Logger) DevlogInit() error {
	return nil
}

func (l *Logger) dispatchDevlog(logLevel int, mm string) {
}

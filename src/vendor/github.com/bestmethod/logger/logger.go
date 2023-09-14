package logger

import (
	"log"
	"os"
)

var logLevel = 4 // 0=NO_LOGGING 1=CRITICAL, 2=ERROR, 3=WARNING, 4=INFO, 5=DEBUG, 6=DETAIL

func SetLogLevel(level int) {
	if level < 0 {
		level = 0
	}
	logLevel = level
}

func Info(format string, v ...interface{}) {
	if logLevel < 4 {
		return
	}
	format = "INFO " + format
	log.Printf(format, v...)
}

func Warn(format string, v ...interface{}) {
	if logLevel < 3 {
		return
	}
	format = "WARNING " + format
	log.Printf(format, v...)
}

func Error(format string, v ...interface{}) {
	if logLevel < 2 {
		return
	}
	format = "ERROR " + format
	log.Printf(format, v...)
}

func Critical(format string, v ...interface{}) {
	if logLevel >= 1 {
		format = "CRITICAL " + format
		log.Printf(format, v...)
	}
	os.Exit(1)
}

func Debug(format string, v ...interface{}) {
	if logLevel < 5 {
		return
	}
	format = "DEBUG " + format
	log.Printf(format, v...)
}

func Detail(format string, v ...interface{}) {
	if logLevel < 6 {
		return
	}
	format = "DETAIL " + format
	log.Printf(format, v...)
}

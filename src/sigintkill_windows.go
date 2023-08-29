package main

import (
	"os"
	"syscall"
)

func exitNow() {
	proc, err := os.FindProcess(syscall.Getpid())
	if err != nil {
		os.Exit(1)
	}
	err = proc.Kill()
	if err != nil {
		os.Exit(1)
	}
}

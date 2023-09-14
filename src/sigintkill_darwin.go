package main

import (
	"syscall"
)

func exitNow() {
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}

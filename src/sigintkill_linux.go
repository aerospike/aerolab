package main

import (
	"syscall"
)

func exitNow() {
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}

func sendSigInt(pid int) error {
	return syscall.Kill(pid, syscall.SIGINT)
}

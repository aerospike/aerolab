package main

import (
	"fmt"
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

func sendSigInt(pid int) error {
	d, e := syscall.LoadDLL("kernel32.dll")
	if e != nil {
		return fmt.Errorf("LoadDLL: %v\n", e)
	}
	p, e := d.FindProc("GenerateConsoleCtrlEvent")
	if e != nil {
		return fmt.Errorf("FindProc: %v\n", e)
	}
	r, _, e := p.Call(syscall.CTRL_BREAK_EVENT, uintptr(pid))
	if r == 0 {
		return fmt.Errorf("GenerateConsoleCtrlEvent: %v\n", e)
	}
	return nil
}

//go:build !windows

package sshexec

import (
	"os"
	"os/signal"
	"syscall"
)

func winResize() {
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	for range sigwinch {
		resize(nil)
	}
}

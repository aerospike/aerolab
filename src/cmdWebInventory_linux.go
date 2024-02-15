package main

import (
	"syscall"
	"unsafe"

	"github.com/gabemarshall/pty"
)

func sysResizeWindow(tty pty.Pty, resizeMessage windowSize) (errno int) {
	_, _, errnox := syscall.Syscall(
		syscall.SYS_IOCTL,
		tty.Fd(),
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&resizeMessage)),
	)
	errno = int(errnox)
	return errno
}

func getWindowsBuild() (string, error) {
	return "", nil
}

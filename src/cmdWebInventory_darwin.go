package main

import (
	"os"
	"syscall"
	"unsafe"
)

func sysResizeWindow(tty *os.File, resizeMessage windowSize) (errno int) {
	_, _, errnox := syscall.Syscall(
		syscall.SYS_IOCTL,
		tty.Fd(),
		syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&resizeMessage)),
	)
	errno = int(errnox)
	return errno
}

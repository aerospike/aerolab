//go:build unix
// +build unix

package termutil

import (
	"syscall"
	"unsafe"

	"golang.org/x/term"
)

// isForeground reports whether the file descriptor refers to a
// terminal that is in the foreground process group.
func isForeground(fd uintptr) (bool, error) {
	// First: does fd refer to a tty?
	if !term.IsTerminal(int(fd)) {
		return false, nil
	}

	// Ask the kernel which process group is in the foreground of
	// the terminal that fd refers to.
	var fgPgid int
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL,
		fd,
		uintptr(syscall.TIOCGPGRP),
		uintptr(unsafe.Pointer(&fgPgid)),
	)
	if err != 0 {
		// errno is returned in the third syscall result
		return false, err
	}

	// Our own process group number.
	ownPgid := syscall.Getpgrp()
	return ownPgid == fgPgid, nil
}

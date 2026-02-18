//go:build windows
// +build windows

package termutil

import (
	"syscall"
)

// isForeground reports whether the file descriptor is attached to a
// console window.  Windows has no notion of a foreground process
// group, so we simply return true when the handle is a console.
func isForeground(fd uintptr) (bool, error) {
	// Convert the Go fd to a Windows HANDLE.
	handle := syscall.Handle(fd)

	// GetConsoleMode will succeed only if the handle is a console
	// input/output device.  It returns an error otherwise.
	var mode uint32
	err := syscall.GetConsoleMode(handle, &mode)
	if err != nil {
		// Not a console – treat it as “not foreground”.
		return false, nil
	}
	return true, nil
}

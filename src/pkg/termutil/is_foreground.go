// is_foreground.go
//
// Exported helper – works on Linux, macOS and Windows.
// It tells you whether the current process is the *foreground* process
// group for the terminal that `fd` refers to.
//
// On Windows there is no job‑control concept, so the function simply
// reports whether `fd` is attached to a console window.

package termutil

func IsForeground(fd uintptr) (bool, error) {
	return isForeground(fd)
}

func IsForegroundNoError(fd uintptr, defaultValOnError bool) bool {
	isForeground, err := IsForeground(fd)
	if err != nil {
		return defaultValOnError
	}
	return isForeground
}

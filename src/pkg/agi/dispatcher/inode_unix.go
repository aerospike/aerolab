//go:build !windows

package dispatcher

import (
	"os"
	"syscall"
)

// inodeFromFileInfo extracts the unix inode number from a FileInfo.
// Returns 0,false on platforms where the underlying stat is not a
// syscall.Stat_t (e.g. an unusual filesystem on a unix that doesn't
// surface the underlying stat). On every supported aerolab dispatcher
// target (linux/darwin/freebsd/openbsd) this is a uint64.
func inodeFromFileInfo(fi os.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Ino), true
}

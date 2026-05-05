//go:build windows

package dispatcher

import "os"

// inodeFromFileInfo always returns (0,false) on Windows because the
// dispatcher isn't expected to run there: the aerolab CLI is built
// for Windows, but `aerolab agi exec dispatch` is a hidden subcommand
// only used on Linux cluster nodes (where it is installed by
// `aerolab cluster add agi-client`). The build still has to succeed
// on Windows, hence this stub. On Windows the file-tail code falls
// back to size-based growth detection without rotation handling.
func inodeFromFileInfo(_ os.FileInfo) (uint64, bool) { return 0, false }

package termutil

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsInteractive reports whether aerolab is running attached to an interactive
// foreground terminal. It returns false when AEROLAB_NONINTERACTIVE is set,
// when stdout is not a terminal, or when the process is not in the foreground
// process group (e.g. web UI, MCP, CI, or piped I/O).
func IsInteractive() bool {
	return os.Getenv("AEROLAB_NONINTERACTIVE") == "" &&
		(isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) &&
		IsForegroundNoError(os.Stdout.Fd(), true)
}

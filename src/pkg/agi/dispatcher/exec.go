package dispatcher

import (
	"context"
	"fmt"
	"os/exec"
)

// runCommand runs argv[0] with argv[1:] and returns combined stdout
// (no stderr) on success, or an error wrapping stderr on failure.
// Used for asinfo, journalctl --cursor and other one-shot probes.
//
// The dispatcher avoids hard-depending on any of these binaries — it
// degrades gracefully when they are missing — so this helper is just
// a thin wrapper around os/exec that captures the right streams.
//
// It is package-private because the dispatcher must never expose a
// shell-out surface to its callers; only the dispatcher itself runs
// commands, and only ones with a fixed argument vector.
func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: exit %d: %s", name, ee.ExitCode(), string(ee.Stderr))
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return string(out), nil
}

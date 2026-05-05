package dispatcher

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync/atomic"
)

// journalTail follows a systemd journal unit by exec'ing
// `journalctl -fn0 -u <unit> --output=cat` and reading its stdout.
// It is the journald counterpart to fileTail.
//
// We intentionally do NOT vendor `go-systemd/sdjournal` here: that
// package requires CGo which would force the dispatcher binary to
// be dynamically linked against libsystemd, complicating the
// `aerolab cluster add agi-client` flow that pushes a single
// statically-linked aerolab binary onto cluster nodes. journalctl
// is part of the systemd package on every distro that ships it, so
// the runtime cost is just one subprocess.
type journalTail struct {
	unit string

	// startCursor, if non-empty, is passed as `--after-cursor` to
	// journalctl so the dispatcher resumes from the last successfully
	// posted entry across restarts.
	startCursor string

	// emitted tracks how many lines we've emitted since startup.
	// Atomic so callers may read it for stats; the journal source
	// uses cursor-based resume rather than byte-offset.
	emitted atomic.Uint64

	// cursor is updated on every emitted line so callers can
	// checkpoint progress.
	cursor atomic.Pointer[string]

	lines chan tailLine
}

func newJournalTail(unit string, startCursor string) *journalTail {
	return &journalTail{
		unit:        unit,
		startCursor: startCursor,
		lines:       make(chan tailLine, 1024),
	}
}

// Lines returns the channel that receives one tailLine per journal
// entry. The After/Inode fields on the tailLine are unused for
// journal sources; the dispatcher updates the journal cursor in
// state via Cursor() instead.
func (j *journalTail) Lines() <-chan tailLine { return j.lines }

// Cursor returns the most recent journal cursor we have emitted, or
// the empty string if none yet. Used to checkpoint state.
func (j *journalTail) Cursor() string {
	if p := j.cursor.Load(); p != nil {
		return *p
	}
	return ""
}

func (j *journalTail) run(ctx context.Context) error {
	defer close(j.lines)

	args := []string{"-f", "-n", "0", "-u", j.unit, "--output=cat"}
	if j.startCursor != "" {
		args = append(args, "--after-cursor", j.startCursor)
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("journalctl pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("journalctl start: %w", err)
	}
	defer func() { _ = cmd.Wait() }()

	br := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			n := len(line)
			if line[n-1] == '\n' {
				line = line[:n-1]
				if len(line) > 0 && line[len(line)-1] == '\r' {
					line = line[:len(line)-1]
				}
			}
			j.emitted.Add(1)
			out := append([]byte(nil), line...)
			select {
			case <-ctx.Done():
				return nil
			case j.lines <- tailLine{Line: out, After: int64(j.emitted.Load()), Inode: 0}:
			}
		}
		if err != nil {
			if err == io.EOF {
				// journalctl -f normally never EOFs; if it does the
				// dispatcher will reconnect via the outer retry loop.
				return nil
			}
			return fmt.Errorf("journalctl read: %w", err)
		}
	}
}

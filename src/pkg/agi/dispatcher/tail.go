package dispatcher

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"
)

// fileTail is an inode-aware log tailer. It opens the configured
// path at a known offset and emits each newline-delimited line on
// Lines(). On rotation (the inode of the path changes while we're
// watching it) it drains the old file descriptor to EOF, closes it,
// reopens the new path at offset 0, and continues.
//
// Lifetime is bound to the context passed to (*fileTail).run; on
// cancel, the goroutine returns and Lines() is closed.
//
// Concurrency contract:
//   - run() must be called exactly once.
//   - Offset() may be called concurrently and returns the byte offset
//     of the next line to be read (i.e. the count of bytes already
//     emitted on Lines()).
//   - Lines() is a receive-only channel; the producer closes it.
type fileTail struct {
	path string

	// startOffset is the offset to seek to on first open. If a
	// rotation occurs we always rewind to 0 on the new file
	// regardless of startOffset.
	startOffset int64

	// startInode, if non-zero, is the expected inode of the file at
	// startOffset. If the inode at open time differs we treat the
	// file as already rotated and reset the offset to 0.
	startInode uint64

	// pollInterval controls how often the tail re-stats the path to
	// look for rotation when read returns io.EOF. 250ms is a good
	// balance between latency and idle CPU.
	pollInterval time.Duration

	// emitted tracks the cumulative byte count emitted on Lines().
	// Atomic so Offset() can read it without locking.
	emitted atomic.Int64

	// inode tracks the inode of the currently-open file. Atomic so
	// callers can fetch a consistent (offset,inode) pair via Offset()
	// + Inode() to checkpoint state.
	inode atomic.Uint64

	lines chan tailLine
}

// tailLine carries one log line plus the offset at which the next
// line will start. Receivers checkpoint (offset,inode) only after
// the line has been successfully POSTed to the AGI server, so the
// resume path is exactly-once at the line level.
type tailLine struct {
	Line  []byte
	After int64
	Inode uint64
}

func newFileTail(path string, startOffset int64, startInode uint64) *fileTail {
	return &fileTail{
		path:         path,
		startOffset:  startOffset,
		startInode:   startInode,
		pollInterval: 250 * time.Millisecond,
		lines:        make(chan tailLine, 1024),
	}
}

// Offset returns the next-byte-to-read offset in the currently open
// file. It is safe to call from any goroutine.
func (t *fileTail) Offset() int64 { return t.emitted.Load() }

// Inode returns the inode of the currently open file (or 0 before
// the first successful open).
func (t *fileTail) Inode() uint64 { return t.inode.Load() }

// Lines returns the channel that receives one tailLine per emitted
// log line. The channel is closed when the tail terminates (context
// cancel or unrecoverable error).
func (t *fileTail) Lines() <-chan tailLine { return t.lines }

// run is the tail loop. It is intended to be called as a goroutine.
// Returns nil on context cancel, or the underlying error otherwise.
func (t *fileTail) run(ctx context.Context) error {
	defer close(t.lines)

	f, err := openAtOffset(t.path, t.startOffset, t.startInode)
	if err != nil {
		return fmt.Errorf("open %s: %w", t.path, err)
	}
	t.setInode(f)
	t.emitted.Store(t.startOffset)

	br := bufio.NewReaderSize(f, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			_ = f.Close()
			return nil
		}
		line, readErr := br.ReadBytes('\n')
		if len(line) > 0 {
			// Strip trailing \n so consumers don't double-count.
			// The AGI listener splits on \n itself.
			n := len(line)
			if line[n-1] == '\n' {
				line = line[:n-1]
				if len(line) > 0 && line[len(line)-1] == '\r' {
					line = line[:len(line)-1]
				}
			}
			t.emitted.Add(int64(n))
			select {
			case <-ctx.Done():
				_ = f.Close()
				return nil
			case t.lines <- tailLine{Line: append([]byte(nil), line...), After: t.emitted.Load(), Inode: t.inode.Load()}:
			}
		}
		if readErr == nil {
			continue
		}
		if !errors.Is(readErr, io.EOF) {
			_ = f.Close()
			return fmt.Errorf("read %s: %w", t.path, readErr)
		}
		// At EOF — wait for either rotation or new data.
		next, rotated, err := t.waitForMoreOrRotate(ctx, f)
		if err != nil {
			_ = f.Close()
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if rotated {
			_ = f.Close()
			f = next
			t.setInode(f)
			t.emitted.Store(0)
			br.Reset(f)
		}
		// Otherwise: same fd, more bytes available — re-loop the read.
	}
}

// waitForMoreOrRotate blocks until either:
//   - the source file has more bytes (returns rotated=false and the
//     same fd), or
//   - the path has been rotated (returns rotated=true and a fresh
//     fd opened at offset 0), or
//   - the context is cancelled (returns ctx.Err()).
func (t *fileTail) waitForMoreOrRotate(ctx context.Context, cur *os.File) (*os.File, bool, error) {
	tk := time.NewTicker(t.pollInterval)
	defer tk.Stop()
	curInode, _ := inodeOf(cur)
	for {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-tk.C:
		}
		// Did the path get replaced? Stat it (NOT the fd).
		fi, err := os.Stat(t.path)
		if err != nil {
			// Path may temporarily not exist during rotation. Keep
			// waiting; if the context expires we'll return.
			continue
		}
		newInode, ok := inodeFromFileInfo(fi)
		if !ok {
			// Non-unix or unusual FS — fall through to "size grew?"
			// detection only.
			if fi.Size() > t.fdSize(cur) {
				return cur, false, nil
			}
			continue
		}
		if newInode != curInode {
			// Rotated. Drain the old fd to EOF first by returning
			// false (caller will read until EOF and re-call us). To
			// keep the API small we just open the new file here.
			nf, err := openAtOffset(t.path, 0, 0)
			if err != nil {
				return nil, false, fmt.Errorf("reopen %s after rotate: %w", t.path, err)
			}
			return nf, true, nil
		}
		if fi.Size() > t.fdSize(cur) {
			return cur, false, nil
		}
		// Idle — keep polling.
	}
}

func (t *fileTail) setInode(f *os.File) {
	if ino, ok := inodeOf(f); ok {
		t.inode.Store(ino)
	}
}

// fdSize returns the current size of the open fd (NOT the path), so
// we can detect "size grew" on the same inode without restating the
// path.
func (t *fileTail) fdSize(f *os.File) int64 {
	fi, err := f.Stat()
	if err != nil {
		return 0
	}
	return fi.Size()
}

// openAtOffset opens path and seeks to the given offset, unless the
// inode of the opened file differs from expectedInode (in which case
// we treat the file as rotated and seek to 0). Returns the open fd
// ready for reading.
func openAtOffset(path string, offset int64, expectedInode uint64) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if expectedInode != 0 {
		if ino, ok := inodeOf(f); ok && ino != expectedInode {
			// Rotated since last run — start at 0.
			offset = 0
		}
	}
	if offset > 0 {
		fi, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if offset > fi.Size() {
			// File was truncated since last run; restart from 0.
			offset = 0
		}
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// inodeOf returns the inode number of the open fd or 0 with ok=false
// on platforms where it cannot be determined (Windows).
func inodeOf(f *os.File) (uint64, bool) {
	fi, err := f.Stat()
	if err != nil {
		return 0, false
	}
	return inodeFromFileInfo(fi)
}

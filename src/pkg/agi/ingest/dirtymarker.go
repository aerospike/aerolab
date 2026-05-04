package ingest

import (
	"errors"
	"fmt"
	"os"
	"path"
)

// dirtyMarkerName is the filename of the in-progress sentinel that
// lives next to the persisted progress JSON files. Its presence on
// disk means "an ingest run reached the point of writing rows to
// the embedded DB and has not yet observed a clean shutdown" — i.e.
// memtable contents may not have been flushed if WAL was off, and
// the byte-offset progress recorded in log-processor.json may have
// advanced past data that never reached an SSTable.
//
// On startup the orchestrating CLI consults this marker BEFORE
// opening the DB. With WAL=off it triggers a soft wipe (DB +
// log-processor.json reset; Downloader / Unpacker / PreProcessor
// progress is preserved so a partial-restart re-uses logs/ that
// already landed on disk). With WAL=on the marker is informational
// only — Pebble's WAL replay restores any un-flushed rows, so the
// progress checkpoint and DB state are still consistent and the
// wipe is skipped.
//
// The dot-prefix keeps the marker out of the regular progress JSON
// list (loadProgress iterates a hard-coded slice of <name>.json
// entries; a .dirty file would not match anyway, but the dot
// guarantees no false positive even if that loop is ever
// generalised).
const dirtyMarkerName = ".dirty"

// DirtyMarkerPath returns the on-disk path of the in-progress
// sentinel for the given progress directory. The path is the same
// regardless of whether the directory exists yet — callers that
// want to write the marker should MkdirAll on the parent first.
func DirtyMarkerPath(progressDir string) string {
	return path.Join(progressDir, dirtyMarkerName)
}

// WriteDirtyMarker creates (or refreshes) the sentinel file at
// DirtyMarkerPath(progressDir). It is safe to call multiple times
// per run; the second call is a no-op when the file is already
// present.
//
// Failure is non-fatal in spirit (the worst case is that a future
// crash-on-WAL-off run misses the wipe and re-ingests one extra
// time), but we surface the error so the operator sees it. Callers
// that prefer best-effort can ignore the return value.
func WriteDirtyMarker(progressDir string) error {
	if progressDir == "" {
		return errors.New("ingest dirty marker: empty progress dir")
	}
	if err := os.MkdirAll(progressDir, 0755); err != nil {
		return fmt.Errorf("ingest dirty marker: mkdir %s: %w", progressDir, err)
	}
	p := DirtyMarkerPath(progressDir)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("ingest dirty marker: create %s: %w", p, err)
	}
	// One human-readable line so an operator who finds the
	// sentinel during forensics knows what it is. The body is
	// not consumed by the recovery path; only the file's
	// existence matters.
	_, _ = f.WriteString("ingest in progress; remove this file only after a clean shutdown\n")
	return f.Close()
}

// ClearDirtyMarker removes the sentinel. It is a no-op when the
// marker is already absent (clean re-runs / fresh hosts) and
// reports any other os error to the caller. Callers that want
// fire-and-forget semantics can ignore the return value — leaving
// a stale marker behind only causes one extra wipe-and-reingest
// on the next startup.
func ClearDirtyMarker(progressDir string) error {
	if progressDir == "" {
		return nil
	}
	p := DirtyMarkerPath(progressDir)
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ingest dirty marker: remove %s: %w", p, err)
	}
	return nil
}

// DirtyMarkerExists reports whether the sentinel is present at
// DirtyMarkerPath(progressDir). Any stat error other than
// os.IsNotExist is treated conservatively as "exists" so a
// transient EFS glitch never silently downgrades a real dirty
// state to "clean".
func DirtyMarkerExists(progressDir string) bool {
	if progressDir == "" {
		return false
	}
	_, err := os.Stat(DirtyMarkerPath(progressDir))
	if err == nil {
		return true
	}
	return !os.IsNotExist(err)
}

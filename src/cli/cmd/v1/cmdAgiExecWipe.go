//go:build !noagi

package cmd

import (
	"errors"
	"fmt"
	"os"
)

// agiWipeOnVersionMismatch removes the on-disk AGI DB directory and
// resets the ingest progress markers so the pipeline re-runs from
// scratch. Used by cmdAgiExecService and cmdAgiExecIngest when
// db.Open returns db.ErrStorageVersionMismatch — the v1→v2 storage
// layout cannot be upgraded in place (the indexed-row payload moved
// from D/ keys to I/ keys), so we wipe and re-ingest.
//
// dbPath is the Pebble data directory (the same value passed to
// db.Options.Path). progressFile is config.ProgressFile.OutputFilePath
// from the ingest config; pass "" to skip clearing it (e.g. when the
// caller does not have an ingest config available).
//
// On success the DB directory exists and is empty, the per-log
// progress markers are gone, and /opt/agi/ingest/steps.json has been
// removed so every ingest pipeline step re-runs.
func agiWipeOnVersionMismatch(dbPath, progressFile string) error {
	if dbPath == "" {
		return errors.New("agi wipe: empty db path")
	}
	if err := os.RemoveAll(dbPath); err != nil {
		return fmt.Errorf("agi wipe: remove db dir %s: %w", dbPath, err)
	}
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return fmt.Errorf("agi wipe: recreate db dir %s: %w", dbPath, err)
	}
	// Reset high-level ingest steps so Download/Unpack/PreProcess/
	// ProcessLogs all re-run. Without this, the pipeline would skip
	// straight past the (already-completed-on-disk-but-now-empty-in-
	// db) ProcessLogs step and the operator would see an empty DB.
	if err := os.Remove("/opt/agi/ingest/steps.json"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("agi wipe: reset steps.json: %w", err)
	}
	// Remove per-log byte-offset progress markers so ProcessLogs
	// re-reads each log file from byte 0. The progress file path is
	// configurable; if the operator left it as the default it lives
	// at the relative path "ingest/progress/" inside the cwd of the
	// ingest process (typically /opt/agi).
	if progressFile != "" {
		if err := os.RemoveAll(progressFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("agi wipe: remove progress %s: %w", progressFile, err)
		}
	}
	return nil
}

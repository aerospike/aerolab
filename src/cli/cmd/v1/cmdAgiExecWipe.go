//go:build !noagi

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/ingest"
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

// agiWipeOnDirty is the softer sibling of agiWipeOnVersionMismatch
// invoked when the orchestrating CLI finds an ingest dirty marker
// on startup AND the DB will be opened with WAL off (i.e. a
// previous run wrote rows that may never have reached an SSTable).
// Unlike the version-mismatch wipe, this preserves Downloader,
// Unpacker and PreProcessor progress so the recovered run reuses
// every artifact that was already written to the input/staging/
// logs directories. Only the LogProcessor's checkpoint is
// invalidated (its byte offsets pointed at memtable contents that
// are gone) and only the ProcessLogs step in steps.json is reset.
//
// The dirty marker itself is removed last; if any earlier step
// fails the marker stays in place so a retry runs the wipe again
// rather than booting against half-cleaned state.
//
// dbPath / progressDir / stepsPath are ALL required. progressDir
// is the directory containing log-processor.json[.gz] (and the
// .dirty sentinel); stepsPath is the absolute path of steps.json
// (typically /opt/agi/ingest/steps.json).
func agiWipeOnDirty(dbPath, progressDir, stepsPath string) error {
	if dbPath == "" {
		return errors.New("agi wipe-on-dirty: empty db path")
	}
	if progressDir == "" {
		return errors.New("agi wipe-on-dirty: empty progress dir")
	}
	if err := os.RemoveAll(dbPath); err != nil {
		return fmt.Errorf("agi wipe-on-dirty: remove db dir %s: %w", dbPath, err)
	}
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return fmt.Errorf("agi wipe-on-dirty: recreate db dir %s: %w", dbPath, err)
	}
	// Remove only the LogProcessor's progress file — Downloader,
	// Unpacker and PreProcessor progress is unchanged because
	// the artifacts those stages produced (input/, staging/,
	// logs/) are still on disk and re-usable. Compressed and
	// uncompressed variants are removed so a config flip
	// between runs (gzipOutput on/off) does not leave the
	// other variant lying around.
	for _, name := range []string{"log-processor.json", "log-processor.json.gz"} {
		p := path.Join(progressDir, name)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("agi wipe-on-dirty: remove %s: %w", p, err)
		}
	}
	// Reset only the ProcessLogs flag in steps.json so the
	// LogProcessor stage re-runs while Download/Unpack/PreProcess
	// stay at their existing complete state. We tolerate a
	// missing steps.json (fresh host or already wiped) and a
	// malformed steps.json (treat as fresh; the next steps
	// write rebuilds it).
	if stepsPath != "" {
		if data, err := os.ReadFile(stepsPath); err == nil {
			steps := new(ingest.IngestSteps)
			if uerr := json.Unmarshal(data, steps); uerr == nil {
				steps.ProcessLogs = false
				steps.ProcessLogsStartTime = time.Time{}
				steps.ProcessLogsEndTime = time.Time{}
				steps.ProcessCollectInfo = false
				steps.ProcessCollectInfoStartTime = time.Time{}
				steps.ProcessCollectInfoEndTime = time.Time{}
				steps.CriticalError = ""
				if buf, merr := json.Marshal(steps); merr == nil {
					if werr := os.WriteFile(stepsPath+".new", buf, 0644); werr == nil {
						//nolint:errcheck
						os.Rename(stepsPath+".new", stepsPath)
					}
				}
			}
		}
	}
	// Last: remove the dirty marker itself. If we got here every
	// invalidated artifact has been cleared, so the next startup
	// can proceed with a clean run.
	if err := ingest.ClearDirtyMarker(progressDir); err != nil {
		return fmt.Errorf("agi wipe-on-dirty: clear marker: %w", err)
	}
	return nil
}

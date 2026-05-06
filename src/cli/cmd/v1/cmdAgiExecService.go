//go:build !noagi

package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
	"github.com/aerospike/aerolab/pkg/agi/ingest"
	"github.com/aerospike/aerolab/pkg/agi/livelisten"
	"github.com/aerospike/aerolab/pkg/agi/plugin"
)

// AgiExecServiceCmd runs the ingest pipeline and the Grafana plugin
// backend together in a single process, sharing one Pebble DB handle.
//
// Why this exists: Pebble acquires an exclusive OS file lock on its
// data directory, so two independent processes (the old agi-plugin and
// agi-ingest systemd units) cannot open the same directory. Running
// them as one process is the simplest and most robust fix — it also
// guarantees the plugin's schema cache always sees what ingest just
// wrote (no cross-process consistency windows, no snapshot
// re-mounting).
//
// Lifecycle:
//  1. Open the shared DB with options built from plugin.yaml. WAL is
//     forced on by default for service mode because the process is
//     long-lived and a SIGTERM mid-ingest must not drop still-in-
//     memtable rows; pass --no-force-wal to override.
//  2. Run the ingest pipeline in a goroutine via the standard
//     AgiExecIngestCmd (with its sharedDB field set). The goroutine
//     returns when ingest finishes or fails; the plugin keeps serving.
//  3. Start the plugin HTTP server in the foreground. On /shutdown or
//     SIGTERM it drains and returns.
//  4. Close plugin, wait for ingest to finish, then close the shared
//     DB.
//
// To force a re-ingest, reset /opt/agi/ingest/steps.json and restart
// the systemd unit; the ingest pipeline picks up from the resulting
// empty steps.
type AgiExecServiceCmd struct {
	AGIName    string  `long:"agi-name" description:"Name of this AGI instance"`
	Async      bool    `long:"async" description:"If set, will asynchronously process logs and collectinfo during ingest"`
	IngestYaml string  `long:"ingest-yaml" description:"Path to ingest YAML config file" default:"/opt/agi/ingest.yaml"`
	PluginYaml string  `long:"plugin-yaml" description:"Path to plugin YAML config file" default:"/opt/agi/plugin.yaml"`
	SkipIngest bool    `long:"skip-ingest" description:"Skip running the ingest pipeline (plugin-only mode, useful for reopening an existing DB)"`
	SkipPlugin bool    `long:"skip-plugin" description:"Skip running the plugin (ingest-only mode, equivalent to 'agi exec ingest')"`
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute opens the shared DB, then orchestrates ingest + plugin as
// described above. Returns the first non-nil error it encounters, but
// always attempts to close both components and the DB before returning.
func (c *AgiExecServiceCmd) Execute(args []string) error {
	cmd := []string{"agi", "exec", "service"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	// Ensure /opt/agi directories exist.
	//nolint:errcheck
	os.MkdirAll("/opt/agi", 0755)
	//nolint:errcheck
	os.MkdirAll("/opt/agi/ingest", 0755)

	// PID file for external process management.
	//nolint:errcheck
	os.WriteFile("/opt/agi/service.pid", []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove("/opt/agi/service.pid")

	// Load both configs. Missing yaml files fall back to defaults.
	ingestYaml := c.IngestYaml
	if _, err := os.Stat(ingestYaml); os.IsNotExist(err) {
		ingestYaml = ""
	}
	ingestCfg, err := ingest.MakeConfig(true, ingestYaml, true)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	pluginYaml := c.PluginYaml
	if _, err := os.Stat(pluginYaml); os.IsNotExist(err) {
		pluginYaml = ""
	}
	pluginCfg, err := plugin.MakeConfig(true, pluginYaml, true)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	// Sanity: the two configs MUST point at the same directory.
	// Silently overriding (the previous behaviour) was the "I changed
	// plugin.yaml and nothing happened" footgun — the operator's
	// override at one path was being ignored and the other path was
	// being used. Refuse to start; the operator either fixes one of
	// the yaml files or accepts the default by clearing both.
	if ingestCfg.DB.Path != pluginCfg.DB.Path {
		return Error(fmt.Errorf("config mismatch: ingest.DB.Path=%q != plugin.DB.Path=%q; both yaml files must agree on the shared db path", ingestCfg.DB.Path, pluginCfg.DB.Path), system, cmd, c, args)
	}

	// Refuse to run with neither component active. Silently
	// opening-and-closing the db with no work to do is rarely what
	// the operator meant; better to fail fast.
	if c.SkipIngest && c.SkipPlugin {
		return Error(errors.New("--skip-ingest and --skip-plugin are both set; nothing to do"), system, cmd, c, args)
	}

	// Build DB options from the plugin's config (larger cache by
	// default; the plugin has to satisfy interactive queries). WAL
	// is governed entirely by plugin.yaml (default off): AGI's
	// source-of-truth is the on-disk log files, not the DB, so a
	// crash-and-re-ingest is the cheaper trade than paying WAL
	// fsync overhead on every batch. The dirty-marker mechanism
	// (below) handles crash recovery by wiping a half-flushed DB
	// and re-running ProcessLogs from the byte-offset checkpoint.
	dbOpts := plugin.DBOptionsFromConfig(pluginCfg)
	if dbOpts.Path == "" {
		dbOpts.Path = db.DefaultPath
	}

	// Crash-safety check (Option A): if a previous ingest run
	// crashed before clearing the dirty marker AND the DB will
	// be opened with WAL off, the on-disk DB may be missing
	// rows that log-processor.json thinks were already ingested.
	// Soft-wipe (DB + log-processor.json + reset steps.ProcessLogs)
	// so ProcessLogs re-runs cleanly. With WAL=on (operator-opt-in
	// via plugin.yaml) we skip the wipe — Pebble's WAL replay on
	// db.Open restores any un-flushed memtable contents and the
	// byte-offset progress is consistent again. Run this BEFORE
	// db.Open so the wipe can rm -rf the directory without a
	// "directory in use" file lock.
	if !dbOpts.EnableWAL && ingest.DirtyMarkerExists(ingestCfg.ProgressFile.OutputFilePath) {
		system.Logger.Warn("ingest dirty marker present at %s with WAL=off; wiping db and resetting log-processor progress", ingest.DirtyMarkerPath(ingestCfg.ProgressFile.OutputFilePath))
		if werr := agiWipeOnDirty(dbOpts.Path, ingestCfg.ProgressFile.OutputFilePath, "/opt/agi/ingest/steps.json"); werr != nil {
			return Error(werr, system, cmd, c, args)
		}
	}

	system.Logger.Info("Opening shared AGI db at %s (WAL=%t)", dbOpts.Path, dbOpts.EnableWAL)
	d, err := db.Open(dbOpts)
	if err != nil {
		// agi-db v2 (clustered-by-time indexed payloads) is not
		// in-place upgradable from v1: the row payload moved from
		// the D/ key to the I/ key. db.Open returns
		// ErrStorageVersionMismatch on an existing v1 directory;
		// we wipe and re-ingest from scratch. This is a one-shot
		// upgrade hop — once the directory is on v2 a normal
		// restart never sees the mismatch path.
		if errors.Is(err, db.ErrStorageVersionMismatch) {
			system.Logger.Warn("agi-db storage version mismatch at %s; wiping and re-ingesting", dbOpts.Path)
			if werr := agiWipeOnVersionMismatch(dbOpts.Path, ingestCfg.ProgressFile.OutputFilePath); werr != nil {
				return Error(werr, system, cmd, c, args)
			}
			d, err = db.Open(dbOpts)
		}
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
	}
	// The service owns the db. Neither the ingest pipeline nor the
	// plugin will close it because they were handed the handle via
	// InitWithDB (ownsDB=false on both).
	defer func() {
		system.Logger.Info("Closing shared AGI db")
		if err := d.Close(); err != nil {
			system.Logger.Warn("shared db close: %s", err)
		}
	}()

	// Pre-build the plugin so the signal handler can reach .Shutdown().
	// Plugin.Listen() now uses a per-plugin ServeMux, so multiple
	// plugin instances in the same process are technically safe, but
	// service mode still constructs exactly one — there is no scenario
	// where two would help.
	var p *plugin.Plugin
	if !c.SkipPlugin {
		p, err = plugin.InitWithDB(pluginCfg, d)
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
	}

	liveCtx, liveCancel := context.WithCancel(context.Background())
	defer liveCancel()
	var liveMu sync.Mutex
	var liveSrv *livelisten.Listener
	var liveIngest *ingest.Ingest
	shutdownLive := func() {
		liveCancel()
		liveMu.Lock()
		srv := liveSrv
		li := liveIngest
		liveMu.Unlock()
		if srv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := srv.Shutdown(ctx); err != nil {
				system.Logger.Warn("live listener shutdown: %s", err)
			}
			cancel()
		}
		if li != nil {
			li.Close()
		}
	}

	// Install a process-level signal handler. On SIGTERM/SIGINT, ask
	// the plugin to drain; the ingest goroutine will flush via its
	// own deferred Close when the process exits. The goroutine below
	// exits when sigCh is closed, which happens via defer at the end
	// of Execute.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigCh)
		close(sigCh)
	}()
	go func() {
		sig, ok := <-sigCh
		if !ok {
			return
		}
		system.Logger.Info("Service: received %s, initiating shutdown", sig)
		shutdownLive()
		if p != nil {
			p.Shutdown()
		}
	}()

	// SIGUSR1 (unix only): flush+rotate the plugin CPU profile without
	// restarting the service. See cmdAgiExecService_cpuprofile_unix.go
	// for the full rationale. On Windows this is a no-op because
	// SIGUSR1 does not exist; operators on Windows must restart the
	// service to obtain a fully-formed pprof file.
	stopCPURotate := installCPUProfileRotateHandler(p, system)
	defer stopCPURotate()

	// Kick off ingest in a goroutine so the plugin can start serving
	// queries immediately (on cached/partial data if the pipeline is
	// still running).
	var ingestWG sync.WaitGroup
	var ingestErr error
	if !c.SkipIngest {
		ingestWG.Add(1)
		go func() {
			defer ingestWG.Done()
			inner := &AgiExecIngestCmd{
				AGIName:        c.AGIName,
				Async:          c.Async,
				YamlFile:       c.IngestYaml,
				sharedDB:       d,
				skipDirtyCheck: true,
			}
			if err := inner.Execute(args); err != nil {
				system.Logger.Warn("Ingest pipeline returned error: %s", err)
				ingestErr = err
			}
		}()
	}

	startLive := func() {
		select {
		case <-liveCtx.Done():
			return
		default:
		}
		if !ingestCfg.Live.Enabled {
			return
		}
		if !dbOpts.EnableWAL {
			system.Logger.Warn("live ingest is enabled but db.enableWAL=false; live listener will not start")
			return
		}
		li, err := ingest.InitWithDB(ingestCfg, d)
		if err != nil {
			system.Logger.Warn("live ingest init failed: %s", err)
			return
		}
		srv := livelisten.New(li, livelisten.Config{
			ListenAddr:  ingestCfg.Live.ListenAddr,
			OffsetsPath: "/opt/agi/live/offsets.json",
			TokensPath:  "/opt/agi/tokens",
			MaxStreams:  ingestCfg.Live.MaxStreams,
			Workers:     ingestCfg.Live.Workers,
		})
		liveMu.Lock()
		liveIngest = li
		liveSrv = srv
		liveMu.Unlock()
		system.Logger.Info("Starting live ingest listener on %s", ingestCfg.Live.ListenAddr)
		if err := srv.Serve(liveCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			system.Logger.Warn("live listener returned error: %s", err)
		}
	}
	if ingestCfg.Live.Enabled {
		if c.SkipIngest {
			go startLive()
		} else {
			go func() {
				ingestWG.Wait()
				if ingestErr != nil {
					return
				}
				startLive()
			}()
		}
	}

	// Plugin CPU profiling is process-global (Go's runtime/pprof CPU
	// profiler can only have one writer at a time) and would otherwise
	// clash with the ingest profile. Defer the plugin profile until
	// ingest is fully closed: ingest's deferred Close runs before its
	// goroutine signals ingestWG.Done, so by the time Wait returns the
	// process-global pprof is free and the resulting cpu.plugin.pprof
	// captures plugin-only work (queries, cache refresh, HTTP). When
	// --skip-ingest is set, the wait group is empty and the coordinator
	// fires immediately, matching the standalone-plugin behaviour.
	if p != nil && pluginCfg.CPUProfilingOutputFile != "" {
		go func() {
			ingestWG.Wait()
			if err := p.StartCPUProfile(); err != nil {
				system.Logger.Warn("plugin CPU profile start failed: %s", err)
			}
		}()
	}

	// Run the plugin HTTP server in the foreground; blocks until
	// /shutdown or SIGTERM triggers Shutdown(). If no plugin is
	// configured (--skip-plugin), fall back to waiting for ingest.
	if p != nil {
		system.Logger.Info("Starting plugin HTTP server on %s:%d", pluginCfg.Service.ListenAddress, pluginCfg.Service.ListenPort)
		if err := p.Listen(); err != nil {
			system.Logger.Warn("Plugin listener returned error: %s", err)
			// Fall through to graceful shutdown of ingest below.
		}
		p.Close()
	}

	shutdownLive()

	// Wait for ingest to finish flushing. ingest installs its own
	// defer i.Close() so its memtable writes are drained before this
	// return; we just need the goroutine to actually exit.
	ingestWG.Wait()

	if ingestErr != nil {
		return Error(ingestErr, system, cmd, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}
